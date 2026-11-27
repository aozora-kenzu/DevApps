//go:build windows

package syspower

import (
	"testing"
	"unsafe"
)

func TestPowerStructsAndEfficiencyModeToggle(t *testing.T) {
	if got := unsafe.Sizeof(processPowerThrottlingState{}); got != 12 {
		t.Fatalf("unexpected processPowerThrottlingState size: got %d, want 12", got)
	}
	if got := unsafe.Sizeof(systemPowerStatus{}); got != 12 {
		t.Fatalf("unexpected systemPowerStatus size: got %d, want 12", got)
	}

	if !SetEfficiencyMode(true) {
		t.Log("SetEfficiencyMode(true) returned false on this Windows build")
	}
	SetEfficiencyMode(false)
	if IsEfficiencyModeActive() {
		t.Fatal("expected EfficiencyMode to be false after disabling")
	}
	_ = IsBatterySaverOn()
	_ = IsInternetConnected()
	_, _ = ShouldDeferBackgroundScan()
}
