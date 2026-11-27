//go:build windows

package syspower

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	processPowerThrottling              = 4
	processPowerThrottlingVersion       = 1
	processPowerThrottlingExecSpeedFlag = 0x1
)

type processPowerThrottlingState struct {
	Version     uint32
	ControlMask uint32
	StateMask   uint32
}

type systemPowerStatus struct {
	ACLineStatus        byte
	BatteryFlag         byte
	BatteryLifePercent  byte
	SystemStatusFlag    byte // bit 0 (1) = Battery Saver is on (Windows 10/11)
	BatteryLifeTime     uint32
	BatteryFullLifeTime uint32
}

var (
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")
	wininet  = windows.NewLazySystemDLL("wininet.dll")

	procSetProcessInformation      = kernel32.NewProc("SetProcessInformation")
	procGetSystemPowerStatus       = kernel32.NewProc("GetSystemPowerStatus")
	procInternetGetConnectedState  = wininet.NewProc("InternetGetConnectedState")

	ecoMu      sync.Mutex
	ecoEnabled bool
)

// SetEfficiencyMode enables or disables Windows EcoQoS (ProcessPowerThrottling ExecutionSpeed)
// for the current UniDevUI process without altering process priority class.
func SetEfficiencyMode(enable bool) bool {
	ecoMu.Lock()
	defer ecoMu.Unlock()
	if ecoEnabled == enable {
		return true
	}

	var ctrlMask, stateMask uint32
	if enable {
		ctrlMask = processPowerThrottlingExecSpeedFlag
		stateMask = processPowerThrottlingExecSpeedFlag
	}
	st := processPowerThrottlingState{
		Version:     processPowerThrottlingVersion,
		ControlMask: ctrlMask,
		StateMask:   stateMask,
	}

	proc := windows.CurrentProcess()
	r1, _, _ := procSetProcessInformation.Call(
		uintptr(proc),
		processPowerThrottling,
		uintptr(unsafe.Pointer(&st)),
		uintptr(unsafe.Sizeof(st)),
	)
	if r1 != 0 {
		ecoEnabled = enable
		return true
	}
	return false
}

// IsEfficiencyModeActive reports the last successfully applied EcoQoS state.
func IsEfficiencyModeActive() bool {
	ecoMu.Lock()
	defer ecoMu.Unlock()
	return ecoEnabled
}

// IsBatterySaverOn returns true when Windows Battery Saver is currently active.
func IsBatterySaverOn() bool {
	var st systemPowerStatus
	r1, _, _ := procGetSystemPowerStatus.Call(uintptr(unsafe.Pointer(&st)))
	if r1 == 0 {
		return false
	}
	return (st.SystemStatusFlag & 1) != 0
}

// IsInternetConnected returns true if Windows reports an active network connection.
func IsInternetConnected() bool {
	var flags uint32
	r1, _, _ := procInternetGetConnectedState.Call(uintptr(unsafe.Pointer(&flags)), 0)
	return r1 != 0
}

// ShouldDeferBackgroundScan advises skipping automatic background scans when offline or in Battery Saver mode.
func ShouldDeferBackgroundScan() (deferScan bool, reason string) {
	if !IsInternetConnected() {
		return true, "offline"
	}
	if IsBatterySaverOn() {
		return true, "battery_saver"
	}
	return false, ""
}
