package essentials

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// IgnoredApp represents an application excluded from updates & active Installed lists (or skipping one specific version / paused temporarily).
type IgnoredApp struct {
	ID             string `json:"id"` // Winget ID (or local ARP/MSIX ID / pkgmgr prefixed key)
	Name           string `json:"name"`
	Source         string `json:"source"`
	WingetPinned   bool   `json:"wingetPinned"`
	SkippedVersion string `json:"skippedVersion,omitempty"` // Non-empty means only this exact available version is skipped.
	PauseUntil     string `json:"pauseUntil,omitempty"`     // Non-empty YYYY-MM-DD pauses update alerts until this date.
	Reason         string `json:"reason"`
}

// IgnoredConfigPath returns ignored_updates.conf inside appDir (alongside UniDevUI.exe).
func IgnoredConfigPath(appDir string) string {
	return filepath.Join(appDir, "ignored_updates.conf")
}

// EnsureIgnoredConfig creates ignored_updates.conf with a helpful header if missing.
func EnsureIgnoredConfig(appDir string) (string, error) {
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", fmt.Errorf("failed creating app directory %s: %w", appDir, err)
	}
	confPath := IgnoredConfigPath(appDir)
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		if err := os.WriteFile(confPath, []byte(DefaultIgnoredConf()), 0o644); err != nil {
			return "", fmt.Errorf("failed creating %s: %w", confPath, err)
		}
	}
	return confPath, nil
}

// DefaultIgnoredConf returns the initial template for ignored_updates.conf.
func DefaultIgnoredConf() string {
	return `# ==============================================================================
# UniDevUI - Ignored Apps & Pinned Versions (Windows Apps / Updates)
# ==============================================================================
# File ini tersimpan di folder tempat UniDevUI.exe berada.
# Aplikasi di daftar ini disembunyikan dari daftar Update & Installed utama,
# serta ikut di-pin di Winget Windows jika berasal dari source winget/msstore.
# Edit dengan Notepad lalu klik Reload, atau kelola langsung lewat UI UniDevUI.
# ==============================================================================
`
}

// ParseIgnoredFile reads ignored_updates.conf into a slice and lowercase-ID lookup map.
func ParseIgnoredFile(confPath string) ([]IgnoredApp, map[string]IgnoredApp, error) {
	byID := make(map[string]IgnoredApp)
	f, err := os.Open(confPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, byID, nil
		}
		return nil, byID, fmt.Errorf("failed reading %s: %w", confPath, err)
	}
	defer f.Close()

	var list []IgnoredApp
	var current *IgnoredApp

	flush := func() {
		if current != nil && current.ID != "" {
			if current.Name == "" {
				current.Name = current.ID
			}
			key := strings.ToLower(current.ID)
			if _, exists := byID[key]; !exists {
				byID[key] = *current
				list = append(list, *current)
			}
		}
		current = nil
	}

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flush()
			id := strings.TrimSpace(line[1 : len(line)-1])
			if id != "" {
				current = &IgnoredApp{ID: id}
			}
			continue
		}
		if current == nil {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(line[:eq]))
		v := strings.TrimSpace(line[eq+1:])
		switch k {
		case "name":
			current.Name = v
		case "source":
			current.Source = strings.ToLower(v)
		case "winget_pinned":
			current.WingetPinned = strings.EqualFold(v, "true") || v == "1" || strings.EqualFold(v, "yes")
		case "skipped_version":
			current.SkippedVersion = strings.TrimSpace(v)
		case "pause_until":
			current.PauseUntil = strings.TrimSpace(v)
		case "reason":
			current.Reason = v
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, byID, err
	}
	flush()
	return list, byID, nil
}

// AddIgnoredApp persists entry to ignored_updates.conf without duplicates.
func AddIgnoredApp(confPath string, entry IgnoredApp) error {
	list, byID, err := ParseIgnoredFile(confPath)
	if err != nil {
		return err
	}
	key := strings.ToLower(strings.TrimSpace(entry.ID))
	if key == "" {
		return fmt.Errorf("empty package ID")
	}
	entry.ID = strings.TrimSpace(entry.ID)
	if entry.Name == "" {
		entry.Name = entry.ID
	}

	if existing, ok := byID[key]; ok {
		if existing.WingetPinned == entry.WingetPinned &&
			existing.Name == entry.Name &&
			existing.SkippedVersion == entry.SkippedVersion &&
			existing.PauseUntil == entry.PauseUntil &&
			existing.Reason == entry.Reason {
			return nil
		}
		for i := range list {
			if strings.EqualFold(list[i].ID, entry.ID) {
				list[i] = entry
				break
			}
		}
		return writeIgnoredFile(confPath, list)
	}

	list = append(list, entry)
	return writeIgnoredFile(confPath, list)
}

// RemoveIgnoredApp deletes an entry matching id from ignored_updates.conf.
func RemoveIgnoredApp(confPath, id string) (IgnoredApp, bool, error) {
	list, _, err := ParseIgnoredFile(confPath)
	if err != nil {
		return IgnoredApp{}, false, err
	}
	var removed IgnoredApp
	found := false
	filtered := make([]IgnoredApp, 0, len(list))
	for _, item := range list {
		if strings.EqualFold(item.ID, strings.TrimSpace(id)) {
			removed = item
			found = true
			continue
		}
		filtered = append(filtered, item)
	}
	if !found {
		return IgnoredApp{}, false, nil
	}
	if err := writeIgnoredFile(confPath, filtered); err != nil {
		return IgnoredApp{}, false, err
	}
	return removed, true, nil
}

func writeIgnoredFile(confPath string, list []IgnoredApp) error {
	var b strings.Builder
	b.WriteString(strings.TrimRight(DefaultIgnoredConf(), "\r\n"))
	for _, item := range list {
		b.WriteString("\r\n\r\n")
		b.WriteString(fmt.Sprintf("[%s]\r\n", item.ID))
		b.WriteString(fmt.Sprintf("name = %s\r\n", item.Name))
		if item.Source != "" {
			b.WriteString(fmt.Sprintf("source = %s\r\n", item.Source))
		}
		if item.WingetPinned {
			b.WriteString("winget_pinned = true\r\n")
		} else {
			b.WriteString("winget_pinned = false\r\n")
		}
		if item.SkippedVersion != "" {
			b.WriteString(fmt.Sprintf("skipped_version = %s\r\n", item.SkippedVersion))
		}
		if item.PauseUntil != "" {
			b.WriteString(fmt.Sprintf("pause_until = %s\r\n", item.PauseUntil))
		}
		if item.Reason != "" {
			b.WriteString(fmt.Sprintf("reason = %s\r\n", item.Reason))
		}
	}
	b.WriteString("\r\n")
	return os.WriteFile(confPath, []byte(b.String()), 0o644)
}

// IsPausedActive returns true when PauseUntil (YYYY-MM-DD) has not yet expired relative to now.
func IsPausedActive(entry IgnoredApp, now time.Time) bool {
	raw := strings.TrimSpace(entry.PauseUntil)
	if raw == "" {
		return false
	}
	until, err := time.ParseInLocation("2006-01-02", raw, now.Location())
	if err != nil {
		return false
	}
	// Inclusive through end of pause_until local date.
	endOfDay := time.Date(until.Year(), until.Month(), until.Day(), 23, 59, 59, 0, now.Location())
	return !now.After(endOfDay)
}

// EvaluateIgnoredPolicy determines whether an app is fully ignored or only temporarily suppressing/skipping updates.
func EvaluateIgnoredPolicy(entry IgnoredApp, hasEntry bool, availVer string) (fullIgnored, versionSkipped bool) {
	return evaluateIgnoredPolicyAt(entry, hasEntry, availVer, time.Now())
}

func evaluateIgnoredPolicyAt(entry IgnoredApp, hasEntry bool, availVer string, now time.Time) (fullIgnored, versionSkipped bool) {
	if !hasEntry {
		return false, false
	}
	skipVer := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(entry.SkippedVersion)), "v")
	pauseRaw := strings.TrimSpace(entry.PauseUntil)
	if skipVer == "" && pauseRaw == "" {
		return true, false
	}
	if IsPausedActive(entry, now) {
		return false, true
	}
	curAvail := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(availVer)), "v")
	if skipVer != "" && curAvail != "" && curAvail == skipVer {
		return false, true
	}
	return false, false
}

// IsManageableWingetID returns true when an ID represents a real repository package rather than a synthetic ARP/MSIX key.
func IsManageableWingetID(id string) bool {
	trimmed := strings.TrimSpace(id)
	if trimmed == "" {
		return false
	}
	upper := strings.ToUpper(trimmed)
	return !strings.HasPrefix(upper, `ARP\`) && !strings.HasPrefix(upper, `MSIX\`)
}

// WingetPinAdd pins a package with `--blocking` in Windows Winget so `winget upgrade --all` also skips it.
func WingetPinAdd(ctx context.Context, wingetID, source string, log Logger) error {
	if !IsManageableWingetID(wingetID) {
		return nil
	}
	exe := findWingetExe()
	if exe == "" {
		return fmt.Errorf("winget.exe not found")
	}
	args := []string{"pin", "add", "--id", wingetID, "-e", "--blocking", "--accept-source-agreements"}
	src := strings.ToLower(strings.TrimSpace(source))
	if src == "winget" || src == "msstore" {
		args = append(args, "--source", src)
	}
	cmdStr := "winget " + strings.Join(args, " ")
	if log != nil {
		log("CMD", cmdStr, cmdStr)
	}
	runCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		txt := cleanTerminalFrames(string(out))
		// Treat already-pinned as success.
		if strings.Contains(strings.ToLower(txt), "already") && strings.Contains(strings.ToLower(txt), "pin") {
			return nil
		}
		return fmt.Errorf("%v (%s)", err, strings.TrimSpace(txt))
	}
	return nil
}

// WingetPinRemove removes a package pin from Windows Winget.
func WingetPinRemove(ctx context.Context, wingetID, source string, log Logger) error {
	if !IsManageableWingetID(wingetID) {
		return nil
	}
	exe := findWingetExe()
	if exe == "" {
		return fmt.Errorf("winget.exe not found")
	}
	args := []string{"pin", "remove", "--id", wingetID, "-e", "--accept-source-agreements"}
	src := strings.ToLower(strings.TrimSpace(source))
	if src == "winget" || src == "msstore" {
		args = append(args, "--source", src)
	}
	cmdStr := "winget " + strings.Join(args, " ")
	if log != nil {
		log("CMD", cmdStr, cmdStr)
	}
	runCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	cmd := exec.CommandContext(runCtx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		txt := cleanTerminalFrames(string(out))
		if strings.Contains(strings.ToLower(txt), "not pinned") || strings.Contains(strings.ToLower(txt), "no pin") {
			return nil
		}
		return fmt.Errorf("%v (%s)", err, strings.TrimSpace(txt))
	}
	return nil
}
