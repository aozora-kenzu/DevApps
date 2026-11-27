//go:build windows

package autostart

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runRegSubkey = `Software\Microsoft\Windows\CurrentVersion\Run`
	appValueName = "UniDevUI"
	TrayFlag     = "--tray"
)

// FormatCommand builds the Windows Run registry command line for exePath.
func FormatCommand(exePath string, startInTray bool) string {
	clean := filepath.Clean(strings.TrimSpace(exePath))
	if startInTray {
		return fmt.Sprintf(`"%s" %s`, clean, TrayFlag)
	}
	return fmt.Sprintf(`"%s"`, clean)
}

// ParseCommand extracts the executable path and --tray flag from a Run registry value.
func ParseCommand(raw string) (exePath string, startInTray bool) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", false
	}
	if strings.HasPrefix(trimmed, `"`) {
		if end := strings.Index(trimmed[1:], `"`); end >= 0 {
			exePath = trimmed[1 : 1+end]
			rest := strings.ToLower(trimmed[2+end:])
			startInTray = strings.Contains(rest, TrayFlag) || strings.Contains(rest, "--minimized")
			return exePath, startInTray
		}
	}
	fields := strings.Fields(trimmed)
	if len(fields) > 0 {
		exePath = fields[0]
		for _, f := range fields[1:] {
			lf := strings.ToLower(f)
			if lf == TrayFlag || lf == "--minimized" {
				startInTray = true
			}
		}
	}
	return exePath, startInTray
}

// HasTrayArg checks whether os.Args requests hidden tray startup.
func HasTrayArg(args []string) bool {
	for _, a := range args {
		la := strings.ToLower(strings.TrimSpace(a))
		if la == TrayFlag || la == "--minimized" {
			return true
		}
	}
	return false
}

// IsEnabled inspects HKCU\Software\Microsoft\Windows\CurrentVersion\Run for UniDevUI.
func IsEnabled() (enabled bool, startInTray bool, err error) {
	return isEnabledAt(registry.CURRENT_USER, runRegSubkey, appValueName)
}

// Sync updates HKCU\Software\Microsoft\Windows\CurrentVersion\Run for the current executable.
func Sync(enabled, startInTray bool) error {
	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("cannot resolve executable path: %w", err)
	}
	if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil && strings.TrimSpace(resolved) != "" {
		exe = resolved
	}
	return syncAt(registry.CURRENT_USER, runRegSubkey, appValueName, exe, enabled, startInTray)
}

func isEnabledAt(root registry.Key, subkey, valueName string) (bool, bool, error) {
	k, err := registry.OpenKey(root, subkey, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	defer k.Close()

	val, _, err := k.GetStringValue(valueName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, false, nil
		}
		return false, false, err
	}
	exe, tray := ParseCommand(val)
	return strings.TrimSpace(exe) != "", tray, nil
}

func syncAt(root registry.Key, subkey, valueName, exePath string, enabled, startInTray bool) error {
	if !enabled {
		k, err := registry.OpenKey(root, subkey, registry.SET_VALUE)
		if err != nil {
			if errors.Is(err, registry.ErrNotExist) {
				return nil
			}
			return err
		}
		defer k.Close()
		err = k.DeleteValue(valueName)
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}

	k, _, err := registry.CreateKey(root, subkey, registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("failed opening startup registry key: %w", err)
	}
	defer k.Close()
	return k.SetStringValue(valueName, FormatCommand(exePath, startInTray))
}
