package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// AppSettings stores user-configurable DevApps settings across runs.
type AppSettings struct {
	BaseDir  string `json:"baseDir"`
	Language string `json:"language"` // "en" (default) | "id"
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

// SettingsPath returns %APPDATA%\DevApps\settings.json.
func SettingsPath() (string, error) {
	cfgRoot, err := os.UserConfigDir()
	if err != nil || strings.TrimSpace(cfgRoot) == "" {
		home, hErr := os.UserHomeDir()
		if hErr != nil {
			return "", fmt.Errorf("cannot determine user config folder: %w", hErr)
		}
		cfgRoot = filepath.Join(home, "AppData", "Roaming")
	}
	return filepath.Join(cfgRoot, "DevApps", "settings.json"), nil
}

// LoadSettings loads persisted settings, migrating from legacy DevApp folder if present.
func LoadSettings() (AppSettings, string, error) {
	sPath, err := SettingsPath()
	if err != nil {
		return AppSettings{BaseDir: DefaultBaseDir, Language: "en"}, "", err
	}

	raw, err := os.ReadFile(sPath)
	if err != nil {
		if os.IsNotExist(err) {
			// Check legacy DevApp settings path before falling back to defaults.
			legacyPath := filepath.Join(filepath.Dir(filepath.Dir(sPath)), "DevApp", "settings.json")
			if legacyRaw, lErr := os.ReadFile(legacyPath); lErr == nil {
				raw = legacyRaw
				err = nil
			} else {
				return AppSettings{BaseDir: DefaultBaseDir, Language: "en"}, sPath, nil
			}
		}
		if err != nil {
			return AppSettings{BaseDir: DefaultBaseDir, Language: "en"}, sPath, err
		}
	}

	var s AppSettings
	if err := json.Unmarshal(raw, &s); err != nil {
		return AppSettings{BaseDir: DefaultBaseDir, Language: "en"}, sPath, err
	}
	if strings.TrimSpace(s.BaseDir) == "" {
		s.BaseDir = DefaultBaseDir
	} else {
		s.BaseDir = filepath.Clean(strings.TrimSpace(s.BaseDir))
	}
	s.Language = normalizeLang(s.Language)
	return s, sPath, nil
}

// SaveSettings writes settings atomically to disk.
func SaveSettings(s AppSettings) error {
	sPath, err := SettingsPath()
	if err != nil {
		return err
	}
	if strings.TrimSpace(s.BaseDir) == "" {
		s.BaseDir = DefaultBaseDir
	} else {
		s.BaseDir = filepath.Clean(strings.TrimSpace(s.BaseDir))
	}
	s.Language = normalizeLang(s.Language)

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
