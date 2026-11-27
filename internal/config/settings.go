package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AppSettings stores user-configurable UniDevUI settings across runs.
type AppSettings struct {
	BaseDir                string `json:"baseDir"`
	Language               string `json:"language"`               // "en" (default) | "id"
	CloseAction            string `json:"closeAction"`            // "ask" (default) | "tray" | "quit"
	AutoStart              bool   `json:"autoStart"`              // Run on Windows login (HKCU\...\Run)
	StartInTray            bool   `json:"startInTray"`            // Pass --tray when auto-starting on login
	AutoCheckUpdates       bool   `json:"autoCheckUpdates"`       // Periodically check updates in background
	AutoCheckIntervalHours int    `json:"autoCheckIntervalHours"` // 1, 3, 6 (default), 12, 24
	DelayStartupScan       bool   `json:"delayStartupScan"`       // Defer heavy startup scans until system stabilizes
	LastCheckUnix          int64  `json:"lastCheckUnix,omitempty"` // Persisted Unix timestamp of last completed update check
}

// DefaultSettings returns canonical default preferences for UniDevUI.
func DefaultSettings() AppSettings {
	return AppSettings{
		BaseDir:                DefaultBaseDir,
		Language:               "en",
		CloseAction:            "ask",
		AutoStart:              false,
		StartInTray:            true,
		AutoCheckUpdates:       true,
		AutoCheckIntervalHours: 6,
		DelayStartupScan:       true,
	}
}

// NormalizeSettings validates and fills missing defaults on AppSettings.
func NormalizeSettings(s AppSettings) AppSettings {
	if strings.TrimSpace(s.BaseDir) == "" {
		s.BaseDir = DefaultBaseDir
	} else {
		s.BaseDir = filepath.Clean(strings.TrimSpace(s.BaseDir))
	}
	s.Language = normalizeLang(s.Language)
	switch strings.ToLower(strings.TrimSpace(s.CloseAction)) {
	case "tray", "quit":
		s.CloseAction = strings.ToLower(strings.TrimSpace(s.CloseAction))
	default:
		s.CloseAction = "ask"
	}
	switch s.AutoCheckIntervalHours {
	case 1, 3, 6, 12, 24:
	default:
		if s.AutoCheckIntervalHours <= 0 {
			s.AutoCheckIntervalHours = 6
		} else if s.AutoCheckIntervalHours > 24 {
			s.AutoCheckIntervalHours = 24
		}
	}
	return s
}

// AppDir returns the directory containing the running executable.
func AppDir() string {
	exe, err := os.Executable()
	if err != nil {
		wd, wErr := os.Getwd()
		if wErr != nil {
			return DefaultBaseDir
		}
		return wd
	}
	if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil {
		exe = resolved
	}
	return filepath.Dir(exe)
}

// SettingsPath returns %APPDATA%\UniDevUI\settings.json.
func SettingsPath() (string, error) {
	cfgRoot, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(cfgRoot) == "" {
		home, hErr := os.UserHomeDir()
		if hErr != nil {
			return "", fmt.Errorf("cannot determine user config folder: %w", hErr)
		}
		cfgRoot = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(cfgRoot, "UniDevUI", "settings.json"), nil
}

// LoadSettings loads persisted settings, migrating from legacy DevApps/DevApp folders if present.
func LoadSettings() (AppSettings, string, error) {
	def := DefaultSettings()
	sPath, err := SettingsPath()
	if err != nil {
		return def, "", err
	}

	raw, err := os.ReadFile(sPath)
	if err != nil {
		if os.IsNotExist(err) {
			appDataRoot := filepath.Dir(filepath.Dir(sPath))
			for _, legacyName := range []string{"DevApps", "DevApp"} {
				legacyPath := filepath.Join(appDataRoot, legacyName, "settings.json")
				if legacyRaw, lErr := os.ReadFile(legacyPath); lErr == nil {
					raw = legacyRaw
					err = nil
					break
				}
			}
			if err != nil {
				return def, sPath, nil
			}
		}
		if err != nil {
			return def, sPath, err
		}
	}

	s := DefaultSettings()
	if err := json.Unmarshal(raw, &s); err != nil {
		return def, sPath, err
	}
	return NormalizeSettings(s), sPath, nil
}

// SaveSettings writes settings atomically to disk.
func SaveSettings(s AppSettings) error {
	sPath, err := SettingsPath()
	if err != nil {
		return err
	}
	s = NormalizeSettings(s)

	if err := os.MkdirAll(filepath.Dir(sPath), 0o755); err != nil {
		return fmt.Errorf("failed creating settings folder: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(sPath, data, 0o644)
}

func normalizeLang(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "id":
		return "id"
	default:
		return "en"
	}
}
