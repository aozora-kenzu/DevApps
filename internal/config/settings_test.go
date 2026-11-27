package config

import (
	"encoding/json"
	"testing"
)

func TestSettingsDefaultsAndNormalization(t *testing.T) {
	// Simulate upgrading from legacy settings.json that only stored baseDir and language.
	legacyJSON := `{"baseDir": "D:\\DevTools", "language": "ID"}`
	s := DefaultSettings()
	if err := json.Unmarshal([]byte(legacyJSON), &s); err != nil {
		t.Fatalf("unmarshal legacy json: %v", err)
	}
	s = NormalizeSettings(s)
	if s.BaseDir != `D:\DevTools` || s.Language != "id" {
		t.Fatalf("unexpected normalized baseDir/language: %+v", s)
	}
	if s.CloseAction != "ask" || !s.StartInTray || !s.AutoCheckUpdates || !s.DelayStartupScan || s.AutoCheckIntervalHours != 6 {
		t.Fatalf("expected missing fields to retain defaults, got %+v", s)
	}

	explicitFalse := `{"baseDir":"","closeAction":"TRAY","autoStart":true,"startInTray":false,"autoCheckUpdates":false,"autoCheckIntervalHours":99,"delayStartupScan":false}`
	s2 := DefaultSettings()
	if err := json.Unmarshal([]byte(explicitFalse), &s2); err != nil {
		t.Fatalf("unmarshal explicit false: %v", err)
	}
	s2 = NormalizeSettings(s2)
	if s2.BaseDir != DefaultBaseDir || s2.CloseAction != "tray" || !s2.AutoStart || s2.StartInTray || s2.AutoCheckUpdates || s2.DelayStartupScan || s2.AutoCheckIntervalHours != 24 {
		t.Fatalf("unexpected explicit normalization result: %+v", s2)
	}
}
