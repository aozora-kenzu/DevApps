package essentials

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf8"

	"golang.org/x/sys/windows"

	"github.com/aozora-kenzu/UniDevUI/internal/archive"
)

const wingetMsixBundleURL = "https://github.com/microsoft/winget-cli/releases/latest/download/Microsoft.DesktopAppInstaller_8wekyb3d8bbwe.msixbundle"

// Logger receives bilingual status updates.
type Logger func(level, enMsg, idMsg string)

// EnvStatus describes Winget availability and UAC elevation state.
type EnvStatus struct {
	WingetAvailable bool   `json:"wingetAvailable"`
	WingetVersion   string `json:"wingetVersion"`
	WingetPath      string `json:"wingetPath"`
	IsAdmin         bool   `json:"isAdmin"`
}

// WingetTableRow represents one row parsed from winget list/upgrade/search output.
type WingetTableRow struct {
	Name         string `json:"name"`
	ID           string `json:"id"`
	Version      string `json:"version"`
	Available    string `json:"available,omitempty"`
	Match        string `json:"match,omitempty"`
	Source       string `json:"source,omitempty"`
	Installed    bool   `json:"installed,omitempty"`
	InstalledVer string `json:"installedVer,omitempty"`
	HasUpdate    bool   `json:"hasUpdate,omitempty"`
	MatchedBy    string `json:"matchedBy,omitempty"`
}

// CheckEnvironment inspects whether winget.exe is available and whether UniDevUI is elevated (UAC Admin).
func CheckEnvironment() EnvStatus {
	st := EnvStatus{
		IsAdmin: IsProcessElevated(),
	}
	if exe := findWingetExe(); exe != "" {
		st.WingetAvailable = true
		st.WingetPath = exe
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, exe, "--version")
		cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if out, err := cmd.Output(); err == nil {
			st.WingetVersion = strings.TrimSpace(string(out))
		}
	}
	return st
}

// IsProcessElevated returns true if the current process runs with Administrator UAC privileges.
func IsProcessElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// RestartAsAdmin launches a new elevated UniDevUI instance via UAC "runas".
func RestartAsAdmin() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	workDir := filepath.Dir(exe)

	verbPtr, err := windows.UTF16PtrFromString("runas")
	if err != nil {
		return err
	}
	filePtr, err := windows.UTF16PtrFromString(exe)
	if err != nil {
		return err
	}
	dirPtr, err := windows.UTF16PtrFromString(workDir)
	if err != nil {
		return err
	}

	if err := windows.ShellExecute(0, verbPtr, filePtr, nil, dirPtr, windows.SW_SHOWNORMAL); err != nil {
		return fmt.Errorf("UAC elevation cancelled or failed: %w", err)
	}
	return nil
}

// InstallWinget downloads the official Microsoft App Installer bundle and registers it via PowerShell.
func InstallWinget(ctx context.Context, cacheDir string, log Logger) error {
	if log != nil {
		log("INFO",
			"Downloading official Microsoft App Installer (winget) bundle...",
			"Mengunduh paket resmi Microsoft App Installer (winget)...",
		)
	}
	bundlePath, err := archive.DownloadFile(ctx, wingetMsixBundleURL, cacheDir, nil, func(lvl, en, id string) {
		if log != nil {
			log(lvl, en, id)
		}
	})
	if err != nil {
		return err
	}

	psCmd := fmt.Sprintf("Add-AppxPackage -Path '%s'", strings.ReplaceAll(bundlePath, "'", "''"))
	if log != nil {
		log("CMD", "powershell Add-AppxPackage Microsoft.DesktopAppInstaller.msixbundle", "powershell Add-AppxPackage Microsoft.DesktopAppInstaller.msixbundle")
	}
	return runWingetStreaming(ctx, log, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", psCmd)
}

// QueryInstalledMap runs `winget list` once and returns installed & upgradeable info keyed by lowercase Winget ID (and name fallback).
func QueryInstalledMap(ctx context.Context) (byID map[string]WingetTableRow, byName map[string]WingetTableRow, err error) {
	byID, byName, _, err = QueryInstalledEntries(ctx)
	return byID, byName, err
}

// QueryInstalledEntries runs `winget list` once and returns maps plus full ordered slice of installed rows.
func QueryInstalledEntries(ctx context.Context) (byID map[string]WingetTableRow, byName map[string]WingetTableRow, rows []WingetTableRow, err error) {
	byID = make(map[string]WingetTableRow)
	byName = make(map[string]WingetTableRow)

	exe := findWingetExe()
	if exe == "" {
		return byID, byName, nil, fmt.Errorf("winget.exe not found")
	}

	runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, exe, "list", "--accept-source-agreements")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, runErr := cmd.CombinedOutput()
	rows = ParseWingetTable(string(out))
	if len(rows) == 0 && runErr != nil {
		return byID, byName, nil, runErr
	}

	for _, r := range rows {
		idKey := strings.ToLower(strings.TrimSpace(r.ID))
		if idKey != "" {
			if existing, exists := byID[idKey]; !exists || (existing.Available == "" && r.Available != "") {
				byID[idKey] = r
			}
		}
		nameKey := strings.ToLower(strings.TrimSpace(r.Name))
		if nameKey != "" {
			if _, exists := byName[nameKey]; !exists {
				byName[nameKey] = r
			}
		}
	}
	return byID, byName, rows, nil
}

// SearchWinget queries the Winget catalog for arbitrary user-entered search terms.
func SearchWinget(ctx context.Context, query string) ([]WingetTableRow, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	exe := findWingetExe()
	if exe == "" {
		return nil, fmt.Errorf("winget.exe not found")
	}

	runCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, exe, "search", q, "--accept-source-agreements")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	rows := ParseWingetTable(string(out))
	if len(rows) == 0 && err != nil {
		txt := strings.TrimSpace(cleanTerminalFrames(string(out)))
		if strings.Contains(strings.ToLower(txt), "no package found") {
			return nil, nil
		}
		return nil, fmt.Errorf("winget search error: %w", err)
	}
	if len(rows) > 30 {
		rows = rows[:30]
	}
	return rows, nil
}

// RunEssentialAction runs silent install, update, or fix for a single EssentialApp,
// streaming live download progress via ConPTY when progressFn is supplied.
func RunEssentialAction(ctx context.Context, app EssentialApp, mode, version string, log Logger, progressFn archive.ProgressCallback) error {
	exe := findWingetExe()
	if exe == "" {
		return fmt.Errorf("winget.exe is not installed on this Windows system")
	}

	var args []string
	switch strings.ToLower(mode) {
	case "update":
		args = []string{"upgrade", "--id", app.WingetID, "-e", "--silent", "--accept-package-agreements", "--accept-source-agreements"}
	case "fix":
		args = []string{"install", "--id", app.WingetID, "-e", "--force", "--silent", "--accept-package-agreements", "--accept-source-agreements"}
	default:
		args = []string{"install", "--id", app.WingetID, "-e", "--silent", "--accept-package-agreements", "--accept-source-agreements"}
	}

	if app.Source != "" {
		args = append(args, "--source", app.Source)
	}
	if mode != "update" && version != "" && !strings.EqualFold(version, "latest") {
		args = append(args, "-v", version)
	}

	cmdLine := "winget " + strings.Join(args, " ")
	if log != nil {
		log("CMD", cmdLine, cmdLine)
	}

	err := execWingetAction(ctx, app.Name, app.ExpectedSize, log, progressFn, exe, args...)
	if err == nil {
		return nil
	}
	// Registry-matched packages (e.g. Firefox/Chrome/Sublime) may not link to Winget's upgrade index (0x8a150014);
	// fall back cleanly to `winget install` so the package can still be updated in place.
	if strings.EqualFold(mode, "update") && isWingetNoInstalledMatchErr(err) {
		fallbackArgs := append([]string{"install"}, args[1:]...)
		if log != nil {
			log("WARN",
				fmt.Sprintf("%s not linked in Winget upgrade index (0x8a150014), switching to winget install...", app.Name),
				fmt.Sprintf("%s tidak tertaut di indeks upgrade Winget (0x8a150014), beralih ke winget install...", app.Name),
			)
			log("CMD", "winget "+strings.Join(fallbackArgs, " "), "winget "+strings.Join(fallbackArgs, " "))
		}
		return execWingetAction(ctx, app.Name, app.ExpectedSize, log, progressFn, exe, fallbackArgs...)
	}
	return err
}

func execWingetAction(ctx context.Context, name string, expectedSize int64, log Logger, progressFn archive.ProgressCallback, exe string, args ...string) error {
	if progressFn != nil {
		if err := runCommandWithConPTYTotal(ctx, name, expectedSize, log, progressFn, exe, args...); err == nil {
			return nil
		} else if ctx.Err() != nil {
			return err
		} else if !strings.Contains(err.Error(), "winget exited") {
			// Fall back cleanly to standard pipe only if ConPTY creation/launch itself failed.
			return runWingetStreaming(ctx, log, exe, args...)
		} else {
			return err
		}
	}
	return runWingetStreaming(ctx, log, exe, args...)
}

func isWingetNoInstalledMatchErr(err error) bool {
	if err == nil {
		return false
	}
	upper := strings.ToUpper(err.Error())
	return strings.Contains(upper, "8A150014") || strings.Contains(upper, "-1978335212") || strings.Contains(upper, "2316632084")
}

// ParseWingetTable parses fixed-width column output from `winget list` or `winget search`.
func ParseWingetTable(raw string) []WingetTableRow {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	rawLines := strings.Split(normalized, "\n")

	lines := make([]string, 0, len(rawLines))
	for _, rl := range rawLines {
		// Winget overwrites spinner characters using '\r'; keep only the final segment after '\r'.
		if idx := strings.LastIndex(rl, "\r"); idx >= 0 {
			rl = rl[idx+1:]
		}
		lines = append(lines, rl)
	}

	headerIdx := -1
	var colOffsets []colSpec
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "Name") && strings.Contains(trimmed, "Id") && strings.Contains(trimmed, "Version") {
			if i+1 < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i+1]), "---") {
				headerIdx = i
				colOffsets = detectColumns(line)
				break
			}
		}
	}
	if headerIdx < 0 || len(colOffsets) < 3 {
		return nil
	}

	var rows []WingetTableRow
	for i := headerIdx + 2; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasSuffix(trimmed, "upgrades available.") || strings.HasPrefix(trimmed, "<") && strings.Contains(trimmed, "truncated") {
			continue
		}
		row := sliceRowByColumns(line, colOffsets)
		if row.ID == "" || row.Name == "" {
			continue
		}
		rows = append(rows, row)
	}
	return rows
}

type colSpec struct {
	name      string
	runeStart int
}

func detectColumns(headerLine string) []colSpec {
	candidates := []string{"Name", "Id", "Version", "Available", "Match", "Source"}
	var cols []colSpec
	for _, colName := range candidates {
		byteIdx := findColumnHeaderIndex(headerLine, colName)
		if byteIdx >= 0 {
			runeIdx := utf8.RuneCountInString(headerLine[:byteIdx])
			cols = append(cols, colSpec{name: colName, runeStart: runeIdx})
		}
	}
	return cols
}

func findColumnHeaderIndex(line, header string) int {
	offset := 0
	for {
		idx := strings.Index(line[offset:], header)
		if idx < 0 {
			return -1
		}
		pos := offset + idx
		leftOK := pos == 0 || line[pos-1] == ' '
		end := pos + len(header)
		rightOK := end == len(line) || line[end] == ' '
		if leftOK && rightOK {
			return pos
		}
		offset = pos + len(header)
	}
}

func sliceRowByColumns(line string, cols []colSpec) WingetTableRow {
	runes := []rune(line)
	var row WingetTableRow
	for i, c := range cols {
		if c.runeStart >= len(runes) {
			continue
		}
		end := len(runes)
		if i+1 < len(cols) && cols[i+1].runeStart < end {
			end = cols[i+1].runeStart
		}
		val := strings.TrimSpace(string(runes[c.runeStart:end]))
		switch c.name {
		case "Name":
			row.Name = val
		case "Id":
			row.ID = val
		case "Version":
			row.Version = val
		case "Available":
			row.Available = val
		case "Match":
			row.Match = val
		case "Source":
			row.Source = val
		}
	}
	return row
}

func runWingetStreaming(ctx context.Context, log Logger, exe string, args ...string) error {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed starting %s: %w", filepath.Base(exe), err)
	}

	var wg sync.WaitGroup
	stream := func(r io.Reader, level string) {
		defer wg.Done()
		buf := make([]byte, 4096)
		var pending bytes.Buffer
		for {
			n, readErr := r.Read(buf)
			if n > 0 {
				pending.Write(buf[:n])
				for {
					data := pending.Bytes()
					idx := bytes.IndexAny(data, "\r\n")
					if idx < 0 {
						break
					}
					line := strings.TrimSpace(string(data[:idx]))
					pending.Next(idx + 1)
					if isMeaningfulWingetLine(line) && log != nil {
						log(level, line, line)
					}
				}
			}
			if readErr != nil {
				rem := strings.TrimSpace(pending.String())
				if isMeaningfulWingetLine(rem) && log != nil {
					log(level, rem, rem)
				}
				break
			}
		}
	}

	wg.Add(2)
	go stream(stdout, "STDOUT")
	go stream(stderr, "STDERR")
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("winget exited with error: %w", err)
	}
	return nil
}

func isMeaningfulWingetLine(line string) bool {
	if line == "" || line == "-" || line == `\` || line == "|" || line == "/" {
		return false
	}
	// Skip block-character progress bar redraws (`███░░ 12 MB / 50 MB`), standalone byte fragments (`40.1 MB`, `1`, `.4 MB`, `MB`), or `%` ticks.
	if strings.ContainsRune(line, '█') || strings.ContainsRune(line, '▒') || strings.ContainsRune(line, '░') {
		return false
	}
	if reWingetRatio.MatchString(line) || reWingetSingleBytes.MatchString(line) || reWingetPctOnly.MatchString(line) || reProgressFragment.MatchString(line) {
		return false
	}
	return true
}

func cleanTerminalFrames(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	var out []string
	for _, l := range lines {
		if idx := strings.LastIndex(l, "\r"); idx >= 0 {
			l = l[idx+1:]
		}
		if t := strings.TrimSpace(l); t != "" {
			out = append(out, t)
		}
	}
	return strings.Join(out, "\n")
}

func findWingetExe() string {
	if p, err := exec.LookPath("winget.exe"); err == nil {
		return p
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		cand := filepath.Join(localAppData, "Microsoft", "WindowsApps", "winget.exe")
		if info, err := os.Stat(cand); err == nil && !info.IsDir() {
			return cand
		}
	}
	return ""
}
