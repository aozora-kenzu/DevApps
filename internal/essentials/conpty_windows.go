package essentials

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"
	"unsafe"

	"golang.org/x/sys/windows"

	"DevApp/internal/archive"
)

var (
	reVTFrameBreak      = regexp.MustCompile(`\x1b\[[0-9;]*[HfGKJ]`)
	reANSISeq           = regexp.MustCompile(`\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)|\x1b\[[0-9;?]*[ -/]*[@-~]|\x1b[@-Z\\-_]`)
	reWingetRatio       = regexp.MustCompile(`(?i)(\d+(?:[.,]\d+)?)\s*(B|KB|KiB|MB|MiB|GB|GiB)\s*/\s*(\d+(?:[.,]\d+)?)\s*(B|KB|KiB|MB|MiB|GB|GiB)`)
	reWingetSingleBytes = regexp.MustCompile(`(?i)^[█▒░\s]*(\d+(?:[.,]\d+)?)\s*(B|KB|KiB|MB|MiB|GB|GiB)[█▒░\s]*$`)
	reWingetPctOnly     = regexp.MustCompile(`^[█▒░\s]*(\d{1,3}(?:\.\d+)?)\s*%[█▒░\s]*$`)
	reWingetPct         = regexp.MustCompile(`(?m)\b(\d{1,3}(?:\.\d+)?)\s*%`)
	reProgressFragment  = regexp.MustCompile(`(?i)^[█▒░\s]*(?:[.,]?\d+(?:[.,]\d*)?(?:\s*(?:B|KB|KiB|MB|MiB|GB|GiB|%|/))?|(?:B|KB|KiB|MB|MiB|GB|GiB|%|/))[█▒░\s]*$`)
)

// SplitWingetConsoleFrames splits VT/ConPTY console output into readable trimmed lines.
func SplitWingetConsoleFrames(chunk string) []string {
	// ConPTY translates cursor jumps/line clears into CSI sequences; turn them into line separators first.
	broken := reVTFrameBreak.ReplaceAllString(chunk, "\n")
	cleaned := reANSISeq.ReplaceAllString(broken, "")
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")
	cleaned = strings.ReplaceAll(cleaned, "\r", "\n")

	rawLines := strings.Split(cleaned, "\n")
	out := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		// Remove leftover control runes < 0x20 except tab.
		trimmed := strings.TrimSpace(strings.Map(func(r rune) rune {
			if r < 0x20 && r != '\t' {
				return -1
			}
			return r
		}, l))
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// ParseWingetProgressLine extracts download bytes/total or percentage from a winget console frame.
func ParseWingetProgressLine(line string) (downloaded, total int64, pct float64, ok bool) {
	if m := reWingetRatio.FindStringSubmatch(line); len(m) == 5 {
		dlVal, err1 := parseLocaleFloat(m[1])
		totVal, err2 := parseLocaleFloat(m[3])
		if err1 == nil && err2 == nil && totVal > 0 {
			dlBytes := int64(dlVal * unitMultiplier(m[2]))
			totBytes := int64(totVal * unitMultiplier(m[4]))
			if totBytes > 0 {
				if dlBytes > totBytes {
					dlBytes = totBytes
				}
				p := (float64(dlBytes) / float64(totBytes)) * 100.0
				return dlBytes, totBytes, p, true
			}
		}
	}

	if m := reWingetSingleBytes.FindStringSubmatch(line); len(m) == 3 {
		if dlVal, err := parseLocaleFloat(m[1]); err == nil && dlVal >= 0 {
			dlBytes := int64(dlVal * unitMultiplier(m[2]))
			return dlBytes, -1, -1, true
		}
	}

	if ((strings.ContainsRune(line, '█') || strings.ContainsRune(line, '▒') || strings.ContainsRune(line, '░')) && reWingetPct.MatchString(line)) ||
		reWingetPctOnly.MatchString(line) {
		if m := reWingetPct.FindStringSubmatch(line); len(m) == 2 {
			if p, err := strconv.ParseFloat(m[1], 64); err == nil && p >= 0 && p <= 100 {
				return 0, 0, p, true
			}
		}
	}

	return 0, 0, 0, false
}

func parseLocaleFloat(s string) (float64, error) {
	return strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
}

func unitMultiplier(unit string) float64 {
	switch strings.ToUpper(strings.TrimSpace(unit)) {
	case "B":
		return 1
	case "KB", "KIB":
		return 1024
	case "MB", "MIB":
		return 1024 * 1024
	case "GB", "GIB":
		return 1024 * 1024 * 1024
	default:
		return 1
	}
}

type wingetStreamState struct {
	detailPrefix string
	log          Logger
	progress     archive.ProgressCallback
	knownTotal   int64

	lastLogged string
	startTime  time.Time
	firstBytes int64
	lastEmit   time.Time
	lastBytes  int64
	lastSpeed  float64
}

func (s *wingetStreamState) handleLine(line string) {
	if dl, tot, pct, ok := ParseWingetProgressLine(line); ok {
		if tot > 0 {
			s.knownTotal = tot
		} else if s.knownTotal > 0 {
			tot = s.knownTotal
			if dl > 0 {
				if dl > tot {
					dl = tot
				}
				pct = (float64(dl) / float64(tot)) * 100.0
			} else if pct >= 0 {
				dl = int64(float64(tot) * (pct / 100.0))
			}
		}

		if s.progress != nil {
			now := time.Now()
			if dl > 0 {
				if s.startTime.IsZero() {
					s.startTime = now
					s.firstBytes = dl
					s.lastEmit = now
					s.lastBytes = dl
				}
				dt := now.Sub(s.lastEmit).Seconds()
				if dl > s.lastBytes && dt >= 0.65 {
					instSpeed := float64(dl-s.lastBytes) / dt
					if s.lastSpeed == 0 {
						s.lastSpeed = instSpeed
					} else {
						s.lastSpeed = 0.6*s.lastSpeed + 0.4*instSpeed
					}
					s.lastEmit = now
					s.lastBytes = dl
				} else if s.lastSpeed == 0 && dl > s.firstBytes {
					elapsed := now.Sub(s.startTime).Seconds()
					if elapsed >= 0.3 {
						s.lastSpeed = float64(dl-s.firstBytes) / elapsed
					}
				}
			}
			outTot := tot
			if outTot <= 0 {
				outTot = -1
			}
			s.progress(archive.ProgressEvent{
				Stage:      "download",
				Percent:    pct,
				Downloaded: dl,
				Total:      outTot,
				SpeedBps:   s.lastSpeed,
				Detail:     s.detailPrefix,
			})
		}
		return
	}

	if !isMeaningfulWingetLine(line) {
		return
	}

	if s.progress != nil && (strings.Contains(strings.ToLower(line), "starting package install") ||
		strings.Contains(strings.ToLower(line), "successfully verified installer hash")) {
		s.progress(archive.ProgressEvent{
			Stage:   "install",
			Percent: -1,
			Detail:  s.detailPrefix,
		})
	}

	if line == s.lastLogged {
		return
	}
	s.lastLogged = line
	if s.log != nil {
		s.log("STDOUT", line, line)
	}
}

// runCommandWithConPTY runs winget inside a headless Win32 PseudoConsole so it emits real-time byte download progress.
func runCommandWithConPTY(ctx context.Context, detailPrefix string, log Logger, progress archive.ProgressCallback, exe string, args ...string) error {
	return runCommandWithConPTYTotal(ctx, detailPrefix, 0, log, progress, exe, args...)
}

func runCommandWithConPTYTotal(ctx context.Context, detailPrefix string, knownTotal int64, log Logger, progress archive.ProgressCallback, exe string, args ...string) error {
	var inRead, inWrite, outRead, outWrite windows.Handle
	if err := windows.CreatePipe(&inRead, &inWrite, nil, 0); err != nil {
		return err
	}
	if err := windows.CreatePipe(&outRead, &outWrite, nil, 0); err != nil {
		_ = windows.CloseHandle(inRead)
		_ = windows.CloseHandle(inWrite)
		return err
	}

	var hPC windows.Handle
	if err := windows.CreatePseudoConsole(windows.Coord{X: 160, Y: 40}, inRead, outWrite, 0, &hPC); err != nil {
		_ = windows.CloseHandle(inRead)
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		_ = windows.CloseHandle(outWrite)
		return fmt.Errorf("CreatePseudoConsole: %w", err)
	}
	// Close child ends once ConPTY holds its own duplicates.
	_ = windows.CloseHandle(inRead)
	_ = windows.CloseHandle(outWrite)

	attrList, err := windows.NewProcThreadAttributeList(1)
	if err != nil {
		windows.ClosePseudoConsole(hPC)
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		return err
	}
	if err := attrList.Update(windows.PROC_THREAD_ATTRIBUTE_PSEUDOCONSOLE, unsafe.Pointer(hPC), unsafe.Sizeof(hPC)); err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		return err
	}

	cmdLine := buildWindowsCmdLine(exe, args)
	cmdLinePtr, err := syscall.UTF16PtrFromString(cmdLine)
	if err != nil {
		attrList.Delete()
		windows.ClosePseudoConsole(hPC)
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		return err
	}

	var si windows.StartupInfoEx
	si.Cb = uint32(unsafe.Sizeof(si))
	// Clear inherited parent stdout/stderr pipes via STARTF_USESTDHANDLES (with zeroed handles)
	// so the child attaches strictly to the ConPTY console handles rather than parent pipes.
	si.Flags = windows.STARTF_USESTDHANDLES | windows.STARTF_USESHOWWINDOW
	si.ShowWindow = windows.SW_HIDE
	si.ProcThreadAttributeList = attrList.List()

	var pi windows.ProcessInformation
	flags := uint32(windows.EXTENDED_STARTUPINFO_PRESENT | windows.CREATE_UNICODE_ENVIRONMENT)
	createErr := windows.CreateProcess(
		(*uint16)(unsafe.Pointer(nil)),
		(*uint16)(unsafe.Pointer(cmdLinePtr)),
		nil,
		nil,
		false,
		flags,
		nil,
		nil,
		&si.StartupInfo,
		&pi,
	)
	attrList.Delete()
	if createErr != nil {
		windows.ClosePseudoConsole(hPC)
		_ = windows.CloseHandle(inWrite)
		_ = windows.CloseHandle(outRead)
		return fmt.Errorf("CreateProcess(%s): %w", filepath.Base(exe), createErr)
	}
	_ = windows.CloseHandle(pi.Thread)

	// Cancel process if caller context is cancelled.
	doneWait := make(chan uint32, 1)
	go func() {
		_, _ = windows.WaitForSingleObject(pi.Process, windows.INFINITE)
		var code uint32
		_ = windows.GetExitCodeProcess(pi.Process, &code)
		// Closing ConPTY signals EOF to the outRead reader goroutine.
		windows.ClosePseudoConsole(hPC)
		_ = windows.CloseHandle(inWrite)
		doneWait <- code
	}()

	go func() {
		select {
		case <-ctx.Done():
			_ = windows.TerminateProcess(pi.Process, 1)
		case <-doneWait:
		}
	}()

	// Re-create a non-consuming watch for exit code since doneWait can only be read cleanly once via procWait.
	return readConPTYUntilExit(ctx, pi.Process, outRead, detailPrefix, knownTotal, log, progress)
}

func readConPTYUntilExit(
	ctx context.Context,
	proc windows.Handle,
	outRead windows.Handle,
	detailPrefix string,
	knownTotal int64,
	log Logger,
	progress archive.ProgressCallback,
) error {
	_ = ctx
	outFile := createReadFile(outRead)
	defer func() {
		_ = outFile.Close()
		_ = windows.CloseHandle(proc)
	}()

	st := &wingetStreamState{
		detailPrefix: detailPrefix,
		knownTotal:   knownTotal,
		log:          log,
		progress:     progress,
	}

	vt := newVTTerminal(st)
	buf := make([]byte, 4096)
	for {
		n, err := outFile.Read(buf)
		if n > 0 {
			vt.Feed(string(buf[:n]))
		}
		if err != nil {
			if err != io.EOF {
				// pipe closed after ClosePseudoConsole
			}
			break
		}
	}
	vt.Flush()

	_, _ = windows.WaitForSingleObject(proc, windows.INFINITE)
	var exitCode uint32
	if err := windows.GetExitCodeProcess(proc, &exitCode); err != nil {
		return fmt.Errorf("winget exit status error: %w", err)
	}
	if exitCode != 0 {
		return fmt.Errorf("winget exited with code %d (0x%X)", exitCode, exitCode)
	}
	return nil
}

// vtTerminal reconstructs in-place cursor updates emitted by Windows ConPTY
// so differential progress updates (e.g. overwriting "1.2 MB" -> "1.4 MB" at column X)
// stay whole lines instead of fragmenting into "1", ".4 MB", or "MB".
type vtTerminal struct {
	st         *wingetStreamState
	rows       map[int][]rune
	lastEmitted map[int]string
	curRow     int
	curCol     int
	carry      string
}

func newVTTerminal(st *wingetStreamState) *vtTerminal {
	return &vtTerminal{
		st:          st,
		rows:        make(map[int][]rune),
		lastEmitted: make(map[int]string),
	}
}

func (vt *vtTerminal) rowString(r int) string {
	line := vt.rows[r]
	if len(line) == 0 {
		return ""
	}
	return strings.TrimSpace(strings.Map(func(ch rune) rune {
		if ch == 0 || (ch < 0x20 && ch != '\t') {
			return ' '
		}
		return ch
	}, string(line)))
}

func (vt *vtTerminal) emitRow(r int, allowLog bool) {
	txt := vt.rowString(r)
	if txt == "" {
		return
	}
	if _, _, _, isProg := ParseWingetProgressLine(txt); isProg {
		vt.st.handleLine(txt)
		return
	}
	if !allowLog || vt.lastEmitted[r] == txt {
		return
	}
	vt.lastEmitted[r] = txt
	vt.st.handleLine(txt)
}

func (vt *vtTerminal) clearAllRows() {
	keys := vt.sortedRowKeys()
	for _, r := range keys {
		vt.emitRow(r, true)
	}
	vt.rows = make(map[int][]rune)
	vt.lastEmitted = make(map[int]string)
	vt.curRow = 0
	vt.curCol = 0
}

func (vt *vtTerminal) sortedRowKeys() []int {
	keys := make([]int, 0, len(vt.rows))
	for k := range vt.rows {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	return keys
}

func (vt *vtTerminal) putRune(ch rune) {
	if vt.curCol < 0 {
		vt.curCol = 0
	}
	if vt.curCol > 512 {
		return
	}
	buf := vt.rows[vt.curRow]
	for len(buf) <= vt.curCol {
		buf = append(buf, ' ')
	}
	buf[vt.curCol] = ch
	vt.rows[vt.curRow] = buf
	vt.curCol++
}

func (vt *vtTerminal) Feed(chunk string) {
	s := vt.carry + chunk
	vt.carry = ""

	for i := 0; i < len(s); {
		b := s[i]
		if b == '\x1b' {
			if i+1 >= len(s) {
				vt.carry = s[i:]
				break
			}
			next := s[i+1]
			if next == '[' {
				end := -1
				for j := i + 2; j < len(s); j++ {
					if s[j] >= 0x40 && s[j] <= 0x7E {
						end = j
						break
					}
				}
				if end < 0 {
					vt.carry = s[i:]
					break
				}
				vt.applyCSI(s[i+2:end], s[end])
				i = end + 1
				continue
			}
			if next == ']' {
				end := -1
				step := 0
				for j := i + 2; j < len(s); j++ {
					if s[j] == '\x07' {
						end = j
						step = 1
						break
					}
					if s[j] == '\x1b' && j+1 < len(s) && s[j+1] == '\\' {
						end = j
						step = 2
						break
					}
				}
				if end < 0 {
					vt.carry = s[i:]
					break
				}
				i = end + step
				continue
			}
			i += 2
			continue
		}

		switch b {
		case '\r':
			vt.emitRow(vt.curRow, false)
			vt.curCol = 0
			i++
		case '\n':
			vt.emitRow(vt.curRow, true)
			vt.curRow++
			vt.curCol = 0
			i++
		case '\t':
			vt.putRune(' ')
			i++
		default:
			if !utf8.FullRuneInString(s[i:]) && len(s)-i < utf8.UTFMax {
				vt.carry = s[i:]
				i = len(s)
				continue
			}
			r, sz := utf8.DecodeRuneInString(s[i:])
			if r >= 0x20 {
				vt.putRune(r)
			}
			i += sz
		}
	}

	// Emit live progress even before trailing \r/\n arrives.
	vt.emitRow(vt.curRow, false)
}

func (vt *vtTerminal) Flush() {
	if vt.carry != "" {
		vt.carry = ""
	}
	for _, r := range vt.sortedRowKeys() {
		vt.emitRow(r, true)
	}
}

func (vt *vtTerminal) applyCSI(params string, cmd byte) {
	parts := strings.Split(strings.TrimPrefix(params, "?"), ";")
	arg := func(idx, def int) int {
		if idx >= len(parts) || parts[idx] == "" {
			return def
		}
		if n, err := strconv.Atoi(parts[idx]); err == nil && n > 0 {
			return n
		}
		return def
	}

	switch cmd {
	case 'H', 'f':
		newRow := arg(0, 1) - 1
		newCol := arg(1, 1) - 1
		if newRow != vt.curRow {
			vt.emitRow(vt.curRow, true)
			vt.curRow = newRow
		}
		vt.curCol = newCol
	case 'G':
		vt.curCol = arg(0, 1) - 1
	case 'C':
		vt.curCol += arg(0, 1)
	case 'D':
		vt.curCol -= arg(0, 1)
		if vt.curCol < 0 {
			vt.curCol = 0
		}
	case 'A':
		vt.emitRow(vt.curRow, true)
		vt.curRow -= arg(0, 1)
		if vt.curRow < 0 {
			vt.curRow = 0
		}
	case 'B':
		vt.emitRow(vt.curRow, true)
		vt.curRow += arg(0, 1)
	case 'K':
		mode := arg(0, 0)
		if params == "" || params == "0" {
			mode = 0
		}
		buf := vt.rows[vt.curRow]
		switch mode {
		case 0:
			if vt.curCol < len(buf) {
				vt.rows[vt.curRow] = buf[:vt.curCol]
			}
		case 1:
			for k := 0; k <= vt.curCol && k < len(buf); k++ {
				buf[k] = ' '
			}
			vt.rows[vt.curRow] = buf
		case 2:
			delete(vt.rows, vt.curRow)
		}
	case 'J':
		if params == "2" || params == "3" {
			vt.clearAllRows()
		}
	}
}

func createReadFile(h windows.Handle) io.ReadCloser {
	return &handleReader{h: h}
}

type handleReader struct {
	h windows.Handle
}

func (r *handleReader) Read(p []byte) (int, error) {
	var n uint32
	err := windows.ReadFile(r.h, p, &n, nil)
	if err != nil {
		if err == windows.ERROR_BROKEN_PIPE || err == windows.ERROR_HANDLE_EOF {
			return int(n), io.EOF
		}
		return int(n), err
	}
	if n == 0 {
		return 0, io.EOF
	}
	return int(n), nil
}

func (r *handleReader) Close() error {
	if r.h != 0 && r.h != windows.InvalidHandle {
		err := windows.CloseHandle(r.h)
		r.h = windows.InvalidHandle
		return err
	}
	return nil
}

func buildWindowsCmdLine(exe string, args []string) string {
	var parts []string
	parts = append(parts, syscall.EscapeArg(exe))
	for _, a := range args {
		parts = append(parts, syscall.EscapeArg(a))
	}
	return strings.Join(parts, " ")
}
