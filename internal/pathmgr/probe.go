package pathmgr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"
)

var (
	verCacheMu sync.RWMutex
	verCache   = make(map[string]string)

	reSemver  = regexp.MustCompile(`\b(?:go|v)?(\d+\.\d+(?:\.\d+)?)\b`)
	re7ZipVer = regexp.MustCompile(`(?i)7-Zip(?:\s*\([^)]+\))?\s+(\d+\.\d+(?:\.\d+)?)`)
)

// ProbeBinaryVersion executes the resolved Windows executable with its version flag
// and returns the detected version string (cached by file path, size, and mtime).
func ProbeBinaryVersion(ctx context.Context, appID, exePath string) string {
	if strings.TrimSpace(exePath) == "" {
		return ""
	}
	// Skip WindowsApps Store aliases that can trigger store stubs instead of real binaries.
	if strings.Contains(strings.ToLower(exePath), `\microsoft\windowsapps\`) {
		return ""
	}

	info, err := os.Stat(exePath)
	if err != nil || info.IsDir() {
		return ""
	}

	cacheKey := fmt.Sprintf("%s|%s|%d|%d", strings.ToLower(appID), strings.ToLower(exePath), info.Size(), info.ModTime().UnixNano())
	if strings.HasSuffix(strings.ToLower(exePath), ".bat") || strings.HasSuffix(strings.ToLower(exePath), ".cmd") {
		_, phpPath := ResolveEffectiveBinary("php.exe")
		cacheKey += "|" + strings.ToLower(phpPath)
	}

	verCacheMu.RLock()
	if cached, ok := verCache[cacheKey]; ok {
		verCacheMu.RUnlock()
		return cached
	}
	verCacheMu.RUnlock()

	args := versionArgsForApp(appID, filepath.Base(exePath))
	runCtx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
	defer cancel()

	cmd := exec.CommandContext(runCtx, exePath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	// Provide up-to-date HKLM+HKCU PATH so batch wrappers (like composer.bat -> php) resolve accurately.
	cmd.Env = replacePathEnv(os.Environ(), EffectiveProcessPath())

	out, err := cmd.CombinedOutput()
	ver := extractVersionString(appID, string(out))
	if err != nil && ver == "" {
		return ""
	}

	verCacheMu.Lock()
	verCache[cacheKey] = ver
	verCacheMu.Unlock()
	return ver
}

func versionArgsForApp(appID, baseName string) []string {
	id := strings.ToLower(appID)
	bn := strings.ToLower(baseName)
	switch {
	case id == "go" || bn == "go.exe":
		return []string{"version"}
	case id == "php" || bn == "php.exe":
		return []string{"-v"}
	case id == "gcc" || bn == "gcc.exe":
		return []string{"-dumpfullversion", "-dumpversion"}
	case id == "7zip" || bn == "7zr.exe" || bn == "7z.exe" || bn == "7za.exe":
		return nil
	default:
		return []string{"--version"}
	}
}

func extractVersionString(appID, output string) string {
	trimmed := strings.TrimSpace(output)
	if trimmed == "" {
		return ""
	}
	firstLine := strings.Split(strings.ReplaceAll(trimmed, "\r\n", "\n"), "\n")[0]

	if strings.EqualFold(appID, "7zip") {
		if m := re7ZipVer.FindStringSubmatch(firstLine); len(m) == 2 {
			return m[1]
		}
	}

	if m := reSemver.FindStringSubmatch(firstLine); len(m) == 2 {
		return m[1]
	}
	return ""
}

func replacePathEnv(env []string, newPath string) []string {
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if len(kv) >= 5 && strings.EqualFold(kv[:5], "PATH=") {
			out = append(out, "PATH="+newPath)
			replaced = true
		} else {
			out = append(out, kv)
		}
	}
	if !replaced {
		out = append(out, "PATH="+newPath)
	}
	return out
}
