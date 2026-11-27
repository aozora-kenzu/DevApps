//go:build windows

package tray

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unicode/utf16"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Icon status states matching UniGetUI tray semantics.
const (
	StatusIdle   = "idle"
	StatusGreen  = "green"
	StatusBlue   = "blue"
	StatusOrange = "orange"
)

const (
	wsOverlappedWindow = 0x00CF0000
	wmDestroy          = 0x0002
	wmClose            = 0x0010
	wmQueryEndSession  = 0x0011
	wmEndSession       = 0x0016
	wmNull             = 0x0000
	wmCommand          = 0x0111
	wmLButtonUp        = 0x0202
	wmLButtonDblClk    = 0x0203
	wmRButtonUp        = 0x0205
	wmContextMenu      = 0x007B
	wmApp              = 0x8000

	wmTrayCallback = wmApp + 1
	wmTrayUpdate   = wmApp + 2
	wmTrayBalloon  = wmApp + 3

	ninBalloonUserClick = 0x0405

	nimAdd        = 0x00000000
	nimModify     = 0x00000001
	nimDelete     = 0x00000002
	nimSetVersion = 0x00000004
	notifyIconV3  = 3

	nifMessage = 0x00000001
	nifIcon    = 0x00000002
	nifTip     = 0x00000004
	nifInfo    = 0x00000010

	niifUser      = 0x00000004
	niifLargeIcon = 0x00000020

	imageIcon     = 1
	lrLoadFromFil = 0x00000010
	lrDefaultSize = 0x00000040

	mfString      = 0x00000000
	mfChecked     = 0x00000008
	mfUnchecked   = 0x00000000
	mfSeparator   = 0x00000800
	tpmLeftAlign  = 0x0000
	tpmBottomAln  = 0x0020
	tpmRightBtn   = 0x0002

	cmdOpen        = 1001
	cmdRuntimes    = 1002
	cmdUpdates     = 1003
	cmdSearch      = 1004
	cmdCheckUpdate = 1005
	cmdAutoStart   = 1006
	cmdExit        = 1007
)

var (
	user32   = windows.NewLazySystemDLL("user32.dll")
	shell32  = windows.NewLazySystemDLL("shell32.dll")
	kernel32 = windows.NewLazySystemDLL("kernel32.dll")

	procRegisterClassExW    = user32.NewProc("RegisterClassExW")
	procCreateWindowExW     = user32.NewProc("CreateWindowExW")
	procDestroyWindow       = user32.NewProc("DestroyWindow")
	procDefWindowProcW      = user32.NewProc("DefWindowProcW")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procTranslateMessage    = user32.NewProc("TranslateMessage")
	procDispatchMessageW    = user32.NewProc("DispatchMessageW")
	procPostMessageW        = user32.NewProc("PostMessageW")
	procPostQuitMessage     = user32.NewProc("PostQuitMessage")
	procLoadImageW          = user32.NewProc("LoadImageW")
	procDestroyIcon         = user32.NewProc("DestroyIcon")
	procCreatePopupMenu     = user32.NewProc("CreatePopupMenu")
	procDestroyMenu         = user32.NewProc("DestroyMenu")
	procAppendMenuW         = user32.NewProc("AppendMenuW")
	procSetMenuDefaultItem  = user32.NewProc("SetMenuDefaultItem")
	procGetCursorPos        = user32.NewProc("GetCursorPos")
	procSetForegroundWindow = user32.NewProc("SetForegroundWindow")
	procTrackPopupMenuEx    = user32.NewProc("TrackPopupMenuEx")
	procRegisterWindowMsgW  = user32.NewProc("RegisterWindowMessageW")
	procShellNotifyIconW    = shell32.NewProc("Shell_NotifyIconW")
	procGetModuleHandleW    = kernel32.NewProc("GetModuleHandleW")
)

type wndClassExW struct {
	cbSize        uint32
	style         uint32
	lpfnWndProc   uintptr
	cbClsExtra    int32
	cbWndExtra    int32
	hInstance     windows.Handle
	hIcon         windows.Handle
	hCursor       windows.Handle
	hbrBackground windows.Handle
	lpszMenuName  *uint16
	lpszClassName *uint16
	hIconSm       windows.Handle
}

type point struct {
	X int32
	Y int32
}

type msgStruct struct {
	Hwnd    windows.Handle
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type notifyIconDataW struct {
	CbSize           uint32
	HWnd             windows.Handle
	UID              uint32
	UFlags           uint32
	UCallbackMessage uint32
	HIcon            windows.Handle
	SzTip            [128]uint16
	DwState          uint32
	DwStateMask      uint32
	SzInfo           [256]uint16
	UVersion         uint32
	SzInfoTitle      [64]uint16
	DwInfoFlags      uint32
	GuidItem         windows.GUID
	HBalloonIcon     windows.Handle
}

// Callbacks connects Win32 tray events to application actions.
type Callbacks struct {
	OnShow            func()
	OnNavRuntimes     func()
	OnNavUpdates      func()
	OnNavSearch       func()
	OnCheckUpdates    func()
	OnToggleAutoStart func()
	OnNotifyClick     func()
	OnOSShutdown      func()
	OnExit            func()
}

// Manager owns the pure-Go Win32 system tray icon, status badges, context menu, and balloon notifications.
type Manager struct {
	callbacks Callbacks
	iconICO   []byte
	iconPNG   []byte

	mu            sync.RWMutex
	lang          string
	autoStart     bool
	tooltip       string
	status        string
	updateCount   int
	pendingTitle  string
	pendingBody   string
	hwnd          windows.Handle
	icons         map[string]windows.Handle
	tempFiles     []string
	taskbarMsg    uint32
	stopped       bool
	startedOnce   bool
}

var (
	globalMgrMu sync.RWMutex
	globalMgr   *Manager
)

// New creates a tray Manager initialized with embedded .ico and .png assets for status overlay badges.
func New(iconICO, iconPNG []byte, lang string, autoStart bool, tooltip string, cb Callbacks) *Manager {
	if strings.TrimSpace(tooltip) == "" {
		tooltip = "UniDevUI"
	}
	return &Manager{
		callbacks: cb,
		iconICO:   iconICO,
		iconPNG:   iconPNG,
		lang:      normLang(lang),
		autoStart: autoStart,
		tooltip:   tooltip,
		status:    StatusIdle,
		icons:     make(map[string]windows.Handle),
	}
}

// Start runs the Win32 message loop for the tray icon in a dedicated locked OS thread.
func (m *Manager) Start() {
	m.mu.Lock()
	if m.startedOnce {
		m.mu.Unlock()
		return
	}
	m.startedOnce = true
	m.mu.Unlock()

	globalMgrMu.Lock()
	globalMgr = m
	globalMgrMu.Unlock()

	ready := make(chan struct{})
	go m.runLoop(ready)
	<-ready
}

// Update refreshes tooltip text, status badge icon, update count, menu language, and auto-start checkmark.
func (m *Manager) Update(status string, updateCount int, lang string, autoStart bool, tooltip string) {
	m.mu.Lock()
	if status != "" {
		m.status = status
	}
	m.updateCount = updateCount
	m.lang = normLang(lang)
	m.autoStart = autoStart
	if strings.TrimSpace(tooltip) != "" {
		m.tooltip = tooltip
	}
	hwnd := m.hwnd
	m.mu.Unlock()

	if hwnd != 0 {
		_, _, _ = procPostMessageW.Call(uintptr(hwnd), wmTrayUpdate, 0, 0)
	}
}

// ShowNotification displays a Windows notification balloon from the UniDevUI tray icon.
func (m *Manager) ShowNotification(title, body string) {
	if strings.TrimSpace(title) == "" && strings.TrimSpace(body) == "" {
		return
	}
	m.mu.Lock()
	m.pendingTitle = strings.TrimSpace(title)
	m.pendingBody = strings.TrimSpace(body)
	hwnd := m.hwnd
	m.mu.Unlock()

	if hwnd != 0 {
		_, _, _ = procPostMessageW.Call(uintptr(hwnd), wmTrayBalloon, 0, 0)
	}
}

// Stop removes the tray icon cleanly and exits its Win32 message loop.
func (m *Manager) Stop() {
	m.mu.Lock()
	if m.stopped {
		m.mu.Unlock()
		return
	}
	m.stopped = true
	hwnd := m.hwnd
	m.mu.Unlock()

	if hwnd != 0 {
		_, _, _ = procPostMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
	}
}

func (m *Manager) runLoop(ready chan<- struct{}) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	hInst, _, _ := procGetModuleHandleW.Call(0)
	className := windows.StringToUTF16Ptr("UniDevUITrayHiddenWindow")
	wndProc := syscall.NewCallback(trayWndProc)

	wc := wndClassExW{
		cbSize:        uint32(unsafe.Sizeof(wndClassExW{})),
		lpfnWndProc:   wndProc,
		hInstance:     windows.Handle(hInst),
		lpszClassName: className,
	}
	_, _, _ = procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))

	tbMsgPtr := windows.StringToUTF16Ptr("TaskbarCreated")
	tbMsg, _, _ := procRegisterWindowMsgW.Call(uintptr(unsafe.Pointer(tbMsgPtr)))

	hwndRaw, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(className)),
		wsOverlappedWindow,
		0, 0, 0, 0,
		0, 0, hInst, 0,
	)
	hwnd := windows.Handle(hwndRaw)
	icons, temps := m.loadTrayIcons()

	m.mu.Lock()
	m.hwnd = hwnd
	m.icons = icons
	m.tempFiles = temps
	m.taskbarMsg = uint32(tbMsg)
	m.mu.Unlock()

	if hwnd != 0 {
		m.addIcon()
	}
	close(ready)

	if hwnd == 0 {
		return
	}

	var msg msgStruct
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
		_, _, _ = procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		_, _, _ = procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}

	m.cleanup()
}

func (m *Manager) loadTrayIcons() (map[string]windows.Handle, []string) {
	handles := make(map[string]windows.Handle)
	var tempPaths []string

	dots := map[string]*color.NRGBA{
		StatusIdle:   nil,
		StatusGreen:  {R: 34, G: 197, B: 94, A: 255},
		StatusBlue:   {R: 59, G: 130, B: 246, A: 255},
		StatusOrange: {R: 245, G: 158, B: 11, A: 255},
	}

	var baseImg image.Image
	if len(m.iconPNG) > 0 {
		if decoded, err := png.Decode(bytes.NewReader(m.iconPNG)); err == nil {
			baseImg = decoded
		}
	}

	tmpDir := os.TempDir()
	pid := os.Getpid()
	loadFile := func(key string, raw []byte) {
		p := filepath.Join(tmpDir, fmt.Sprintf("unidevui-tray-%d-%s.ico", pid, key))
		if err := os.WriteFile(p, raw, 0o644); err != nil {
			return
		}
		tempPaths = append(tempPaths, p)
		ptr := windows.StringToUTF16Ptr(p)
		if h, _, _ := procLoadImageW.Call(0, uintptr(unsafe.Pointer(ptr)), imageIcon, 0, 0, lrLoadFromFil|lrDefaultSize); h != 0 {
			handles[key] = windows.Handle(h)
		}
	}

	if baseImg != nil {
		for key, dot := range dots {
			if raw, err := buildBadgeICOBytes(baseImg, dot); err == nil {
				loadFile(key, raw)
			}
		}
	}
	if handles[StatusIdle] == 0 && len(m.iconICO) > 0 {
		loadFile(StatusIdle, m.iconICO)
	}
	return handles, tempPaths
}

func buildBadgeICOBytes(src image.Image, dot *color.NRGBA) ([]byte, error) {
	const size = 32
	canvas := image.NewNRGBA(image.Rect(0, 0, size, size))
	bounds := src.Bounds()
	srcW := bounds.Dx()
	srcH := bounds.Dy()
	if srcW > 0 && srcH > 0 {
		for y := 0; y < size; y++ {
			sy := bounds.Min.Y + (y*srcH)/size
			for x := 0; x < size; x++ {
				sx := bounds.Min.X + (x*srcW)/size
				canvas.Set(x, y, src.At(sx, sy))
			}
		}
	}

	if dot != nil {
		const cx, cy = 24.0, 24.0
		const outerR = 7.4
		const innerR = 5.4
		ring := color.NRGBA{R: 20, G: 22, B: 27, A: 255}
		for y := 15; y < size; y++ {
			for x := 15; x < size; x++ {
				dx := float64(x) + 0.5 - cx
				dy := float64(y) + 0.5 - cy
				d := math.Hypot(dx, dy)
				if d <= innerR {
					canvas.SetNRGBA(x, y, *dot)
				} else if d <= outerR {
					canvas.SetNRGBA(x, y, ring)
				}
			}
		}
	}

	var pngBuf bytes.Buffer
	if err := png.Encode(&pngBuf, canvas); err != nil {
		return nil, err
	}
	pngBytes := pngBuf.Bytes()

	// Standard Windows Vista+ PNG-compressed 1-entry .ico container (6-byte header + 16-byte dir entry + PNG payload).
	var out bytes.Buffer
	out.Grow(22 + len(pngBytes))
	var hdr [22]byte
	binary.LittleEndian.PutUint16(hdr[0:2], 0)
	binary.LittleEndian.PutUint16(hdr[2:4], 1)
	binary.LittleEndian.PutUint16(hdr[4:6], 1)
	hdr[6] = size
	hdr[7] = size
	hdr[8] = 0
	hdr[9] = 0
	binary.LittleEndian.PutUint16(hdr[10:12], 1)
	binary.LittleEndian.PutUint16(hdr[12:14], 32)
	binary.LittleEndian.PutUint32(hdr[14:18], uint32(len(pngBytes)))
	binary.LittleEndian.PutUint32(hdr[18:22], 22)
	_, _ = out.Write(hdr[:])
	_, _ = out.Write(pngBytes)
	return out.Bytes(), nil
}

func (m *Manager) activeIconLocked() windows.Handle {
	if h := m.icons[m.status]; h != 0 {
		return h
	}
	return m.icons[StatusIdle]
}

func (m *Manager) baseNID() notifyIconDataW {
	m.mu.RLock()
	hwnd := m.hwnd
	hIcon := m.activeIconLocked()
	tip := m.tooltip
	m.mu.RUnlock()

	var nid notifyIconDataW
	nid.CbSize = uint32(unsafe.Sizeof(nid))
	nid.HWnd = hwnd
	nid.UID = 1
	nid.UCallbackMessage = wmTrayCallback
	nid.HIcon = hIcon
	copyUTF16(nid.SzTip[:], tip)
	return nid
}

func (m *Manager) addIcon() {
	nid := m.baseNID()
	nid.UFlags = nifMessage | nifIcon | nifTip
	_, _, _ = procShellNotifyIconW.Call(nimAdd, uintptr(unsafe.Pointer(&nid)))
	nid.UVersion = notifyIconV3
	_, _, _ = procShellNotifyIconW.Call(nimSetVersion, uintptr(unsafe.Pointer(&nid)))
}

func (m *Manager) modifyIconAndTip() {
	nid := m.baseNID()
	nid.UFlags = nifTip | nifIcon
	_, _, _ = procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

func (m *Manager) emitBalloon() {
	m.mu.Lock()
	title := m.pendingTitle
	body := m.pendingBody
	m.mu.Unlock()

	if title == "" && body == "" {
		return
	}
	nid := m.baseNID()
	nid.UFlags = nifInfo | nifIcon | nifTip
	nid.DwInfoFlags = niifUser | niifLargeIcon
	nid.HBalloonIcon = nid.HIcon
	copyUTF16(nid.SzInfoTitle[:], title)
	copyUTF16(nid.SzInfo[:], body)
	_, _, _ = procShellNotifyIconW.Call(nimModify, uintptr(unsafe.Pointer(&nid)))
}

func (m *Manager) cleanup() {
	m.mu.Lock()
	hwnd := m.hwnd
	icons := m.icons
	temps := m.tempFiles
	m.hwnd = 0
	m.icons = nil
	m.tempFiles = nil
	m.mu.Unlock()

	if hwnd != 0 {
		var nid notifyIconDataW
		nid.CbSize = uint32(unsafe.Sizeof(nid))
		nid.HWnd = hwnd
		nid.UID = 1
		_, _, _ = procShellNotifyIconW.Call(nimDelete, uintptr(unsafe.Pointer(&nid)))
	}
	for _, h := range icons {
		if h != 0 {
			_, _, _ = procDestroyIcon.Call(uintptr(h))
		}
	}
	for _, p := range temps {
		if p != "" {
			_ = os.Remove(p)
		}
	}
}

func (m *Manager) showContextMenu() {
	m.mu.RLock()
	hwnd := m.hwnd
	lang := m.lang
	autoStart := m.autoStart
	updCount := m.updateCount
	m.mu.RUnlock()
	if hwnd == 0 {
		return
	}

	hMenuRaw, _, _ := procCreatePopupMenu.Call()
	if hMenuRaw == 0 {
		return
	}
	hMenu := windows.Handle(hMenuRaw)
	defer func() {
		_, _, _ = procDestroyMenu.Call(uintptr(hMenu))
	}()

	labels := menuLabelsForLang(lang, updCount)
	appendStr := func(flags uintptr, id uintptr, text string) {
		ptr := windows.StringToUTF16Ptr(text)
		_, _, _ = procAppendMenuW.Call(uintptr(hMenu), flags, id, uintptr(unsafe.Pointer(ptr)))
	}

	appendStr(mfString, cmdOpen, labels.Open)
	appendStr(mfString, cmdRuntimes, labels.Runtimes)
	appendStr(mfString, cmdUpdates, labels.Updates)
	appendStr(mfString, cmdSearch, labels.Search)
	appendStr(mfString, cmdCheckUpdate, labels.CheckUpdates)
	_, _, _ = procAppendMenuW.Call(uintptr(hMenu), mfSeparator, 0, 0)
	autoFlags := uintptr(mfString | mfUnchecked)
	if autoStart {
		autoFlags = uintptr(mfString | mfChecked)
	}
	appendStr(autoFlags, cmdAutoStart, labels.AutoStart)
	_, _, _ = procAppendMenuW.Call(uintptr(hMenu), mfSeparator, 0, 0)
	appendStr(mfString, cmdExit, labels.Exit)
	_, _, _ = procSetMenuDefaultItem.Call(uintptr(hMenu), cmdOpen, 0)

	var pt point
	_, _, _ = procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	_, _, _ = procSetForegroundWindow.Call(uintptr(hwnd))
	_, _, _ = procTrackPopupMenuEx.Call(
		uintptr(hMenu),
		tpmLeftAlign|tpmBottomAln|tpmRightBtn,
		uintptr(pt.X),
		uintptr(pt.Y),
		uintptr(hwnd),
		0,
	)
	_, _, _ = procPostMessageW.Call(uintptr(hwnd), wmNull, 0, 0)
}

func trayWndProc(hwnd, msg, wParam, lParam uintptr) uintptr {
	globalMgrMu.RLock()
	m := globalMgr
	globalMgrMu.RUnlock()

	if m != nil {
		m.mu.RLock()
		tbMsg := m.taskbarMsg
		m.mu.RUnlock()
		if tbMsg != 0 && uint32(msg) == tbMsg {
			m.addIcon()
			return 0
		}

		switch uint32(msg) {
		case wmQueryEndSession:
			if m.callbacks.OnOSShutdown != nil {
				m.callbacks.OnOSShutdown()
			}
			return 1
		case wmEndSession:
			if wParam != 0 && m.callbacks.OnOSShutdown != nil {
				m.callbacks.OnOSShutdown()
				_, _, _ = procPostMessageW.Call(hwnd, wmClose, 0, 0)
			}
			return 0
		case wmTrayUpdate:
			m.modifyIconAndTip()
			return 0
		case wmTrayBalloon:
			m.emitBalloon()
			return 0
		case wmTrayCallback:
			switch uint32(lParam & 0xFFFF) {
			case wmLButtonUp, wmLButtonDblClk:
				if m.callbacks.OnShow != nil {
					go m.callbacks.OnShow()
				}
			case wmRButtonUp, wmContextMenu:
				m.showContextMenu()
			case ninBalloonUserClick:
				if m.callbacks.OnNotifyClick != nil {
					go m.callbacks.OnNotifyClick()
				} else if m.callbacks.OnShow != nil {
					go m.callbacks.OnShow()
				}
			}
			return 0
		case wmCommand:
			switch uint32(wParam & 0xFFFF) {
			case cmdOpen:
				if m.callbacks.OnShow != nil {
					go m.callbacks.OnShow()
				}
			case cmdRuntimes:
				if m.callbacks.OnNavRuntimes != nil {
					go m.callbacks.OnNavRuntimes()
				}
			case cmdUpdates:
				if m.callbacks.OnNavUpdates != nil {
					go m.callbacks.OnNavUpdates()
				}
			case cmdSearch:
				if m.callbacks.OnNavSearch != nil {
					go m.callbacks.OnNavSearch()
				}
			case cmdCheckUpdate:
				if m.callbacks.OnCheckUpdates != nil {
					go m.callbacks.OnCheckUpdates()
				}
			case cmdAutoStart:
				if m.callbacks.OnToggleAutoStart != nil {
					go m.callbacks.OnToggleAutoStart()
				}
			case cmdExit:
				if m.callbacks.OnExit != nil {
					go m.callbacks.OnExit()
				}
			}
			return 0
		case wmClose:
			_, _, _ = procDestroyWindow.Call(hwnd)
			return 0
		case wmDestroy:
			_, _, _ = procPostQuitMessage.Call(0)
			return 0
		}
	}

	ret, _, _ := procDefWindowProcW.Call(hwnd, msg, wParam, lParam)
	return ret
}

type menuLabels struct {
	Open         string
	Runtimes     string
	Updates      string
	Search       string
	CheckUpdates string
	AutoStart    string
	Exit         string
}

func menuLabelsForLang(lang string, updateCount int) menuLabels {
	if normLang(lang) == "id" {
		upd := "Pembaruan"
		if updateCount > 0 {
			upd = fmt.Sprintf("Pembaruan (%d)", updateCount)
		}
		return menuLabels{
			Open:         "Buka UniDevUI",
			Runtimes:     "Runtimes & Dev Stack",
			Updates:      upd,
			Search:       "Cari Paket",
			CheckUpdates: "Cek Pembaruan Sekarang",
			AutoStart:    "Jalankan Saat Startup (--tray)",
			Exit:         "Keluar UniDevUI",
		}
	}
	upd := "Updates"
	if updateCount > 0 {
		upd = fmt.Sprintf("Updates (%d)", updateCount)
	}
	return menuLabels{
		Open:         "Open UniDevUI",
		Runtimes:     "Runtimes & Dev Stack",
		Updates:      upd,
		Search:       "Search Packages",
		CheckUpdates: "Check Updates Now",
		AutoStart:    "Start with Windows (--tray)",
		Exit:         "Exit UniDevUI",
	}
}

func normLang(lang string) string {
	if strings.EqualFold(strings.TrimSpace(lang), "id") {
		return "id"
	}
	return "en"
}

func copyUTF16(dst []uint16, s string) {
	if len(dst) == 0 {
		return
	}
	encoded := utf16.Encode([]rune(s))
	limit := len(dst) - 1
	if len(encoded) > limit {
		encoded = encoded[:limit]
	}
	copy(dst, encoded)
	dst[len(encoded)] = 0
}
