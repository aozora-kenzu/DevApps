package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/aozora-kenzu/UniDevUI/internal/archive"
	"github.com/aozora-kenzu/UniDevUI/internal/autostart"
	"github.com/aozora-kenzu/UniDevUI/internal/config"
	"github.com/aozora-kenzu/UniDevUI/internal/essentials"
	"github.com/aozora-kenzu/UniDevUI/internal/lts"
	"github.com/aozora-kenzu/UniDevUI/internal/pathmgr"
	"github.com/aozora-kenzu/UniDevUI/internal/pkgmgr"
	"github.com/aozora-kenzu/UniDevUI/internal/selfupdate"
	"github.com/aozora-kenzu/UniDevUI/internal/syspower"
	"github.com/aozora-kenzu/UniDevUI/internal/tools"
	"github.com/aozora-kenzu/UniDevUI/internal/tray"
)

// AppVersion is the current UniDevUI desktop application version (overridable via -ldflags for testing).
var AppVersion = "1.1.2"

// VersionView describes one package version plus local install and PATH status.
type VersionView struct {
	Version      string `json:"version"`
	URL          string `json:"url"`
	DownloadSize int64  `json:"downloadSize"`
	Installed    bool   `json:"installed"`
	Active       bool   `json:"active"`
	BinDir       string `json:"binDir,omitempty"`
}

// PackageView describes an application entry for the UI.
type PackageView struct {
	ID              string                `json:"id"`
	Name            string                `json:"name"`
	Category        string                `json:"category"`
	BinSubdir       string                `json:"binSubdir"`
	Binary          string                `json:"binary"`
	LTSProvider     string                `json:"ltsProvider"`
	Description     string                `json:"description"`
	ActiveVer       string                `json:"activeVer"`
	ActiveDir       string                `json:"activeDir"`
	LatestVer       string                `json:"latestVer"`
	BranchPrefix    string                `json:"branchPrefix"`
	BranchLatestVer string                `json:"branchLatestVer"`
	BranchStatus    string                `json:"branchStatus"` // "uptodate" | "outdated" | "ahead" | ""
	WinActiveVer    string                `json:"winActiveVer"`
	WinActivePath   string                `json:"winActivePath"`
	WinActiveScope  string                `json:"winActiveScope"`
	VersionStatus   string                `json:"versionStatus"` // "uptodate" | "outdated" | "ahead" | "not_installed" | "unknown"
	Conflict        *pathmgr.ConflictInfo `json:"conflict,omitempty"`
	Versions        []VersionView         `json:"versions"`
}

// EssentialView describes one Windows Apps entry plus detected Winget state.
type EssentialView struct {
	ID             string   `json:"id"`
	Name           string   `json:"name"`
	Category       string   `json:"category"`
	WingetID       string   `json:"wingetId"`
	Source         string   `json:"source"`
	RequiresUAC    bool     `json:"requiresUac"`
	Versions       []string `json:"versions,omitempty"`
	Description    string   `json:"description"`
	IconURL        string   `json:"iconUrl,omitempty"`
	Installed      bool     `json:"installed"`
	InstalledVer   string   `json:"installedVer"`
	AvailableVer   string   `json:"availableVer"`
	HasUpdate      bool     `json:"hasUpdate"`
	UpdateIgnored  bool     `json:"updateIgnored"`
	SkippedVersion string   `json:"skippedVersion,omitempty"`
	PauseUntil     string   `json:"pauseUntil,omitempty"`
	VersionStatus  string   `json:"versionStatus"` // "uptodate" | "outdated" | "ahead" | "detected" | "ignored" | ""
	MatchedBy      string   `json:"matchedBy,omitempty"`
	DownloadSize   int64    `json:"downloadSize"`
}

// InstalledAppView describes one installed Windows application or CLI package across Winget and Package Managers.
type InstalledAppView struct {
	Manager         string `json:"manager"` // "winget" | "scoop" | "choco" | "npm" | "pnpm" | "bun" | "uv"
	ID              string `json:"id"`
	Name            string `json:"name"`
	InstalledVer    string `json:"installedVer"`
	AvailableVer    string `json:"availableVer"`
	Source          string `json:"source"`
	Description     string `json:"description,omitempty"`
	Homepage        string `json:"homepage,omitempty"`
	IconURL         string `json:"iconUrl,omitempty"`
	HasUpdate       bool   `json:"hasUpdate"`
	Ignored         bool   `json:"ignored"`
	SkippedVersion  string `json:"skippedVersion,omitempty"`
	PauseUntil      string `json:"pauseUntil,omitempty"`
	WingetPinned    bool   `json:"wingetPinned"`
	IgnoreReason    string `json:"ignoreReason,omitempty"`
	ManagedByWinget bool   `json:"managedByWinget"`
	CanUpdate       bool   `json:"canUpdate"`
	CanUninstall    bool   `json:"canUninstall"`
	PinnedCurated   bool   `json:"pinnedCurated"`
	CuratedAppID    string `json:"curatedAppId,omitempty"`
	DownloadSize    int64  `json:"downloadSize"`
}

// WingetUpdateTarget identifies one installed package (Winget or Package Manager) to update via Update All.
type WingetUpdateTarget struct {
	Manager      string `json:"manager"`
	Name         string `json:"name"`
	WingetID     string `json:"wingetId"`
	Source       string `json:"source"`
	AvailableVer string `json:"availableVer,omitempty"`
}

// BatchEssentialItem identifies one essential app and optional version to install in a batch queue.
type BatchEssentialItem struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// DashboardState is the complete snapshot rendered by the frontend.
type DashboardState struct {
	AppDir             string                  `json:"appDir"`
	BaseDir            string                  `json:"baseDir"`
	ConfPath           string                  `json:"confPath"`
	EssentialsConfPath string                  `json:"essentialsConfPath"`
	IgnoredConfPath    string                  `json:"ignoredConfPath"`
	Packages           []PackageView           `json:"packages"`
	Tools              []tools.ToolStatus      `json:"tools"`
	PkgManagers        []pkgmgr.ManagerEnv     `json:"pkgManagers"`
	Essentials         []EssentialView         `json:"essentials"`
	InstalledApps      []InstalledAppView      `json:"installedApps"`
	WingetEnv          essentials.EnvStatus    `json:"wingetEnv"`
	UserPathList       []string                `json:"userPathList"`
	UserPathEntries    []pathmgr.UserPathEntry `json:"userPathEntries"`
	Logs               []TerminalLogEvent      `json:"logs"`
	Busy               bool                    `json:"busy"`
	LastLTSCheck       string                  `json:"lastLtsCheck"`
	Language           string                  `json:"language"`
	Settings           config.AppSettings      `json:"settings"`
	AppVersion         string                  `json:"appVersion"`
	AppUpdate          selfupdate.UpdateInfo   `json:"appUpdate"`
}

// TerminalLogEvent represents a single line emitted to the UI terminal drawer.
type TerminalLogEvent struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	MessageID string `json:"messageId,omitempty"`
}

// UnifiedSearchStart summarizes which active package sources were launched for a single query.
type UnifiedSearchStart struct {
	Token   int      `json:"token"`
	Query   string   `json:"query"`
	Total   int      `json:"total"`
	Sources []string `json:"sources"`
	JSMgrs  []string `json:"jsMgrs,omitempty"`
	PyMgrs  []string `json:"pyMgrs,omitempty"`
}

// UnifiedSearchChunk streams one completed source's search results back to the frontend immediately.
type UnifiedSearchChunk struct {
	Token       int                         `json:"token"`
	Query       string                      `json:"query"`
	SourceGroup string                      `json:"sourceGroup"` // "winget" | "scoop" | "choco" | "js" | "py" | "cargo" | "dotnet" | "psgallery"
	JSMgrs      []string                    `json:"jsMgrs,omitempty"`
	PyMgrs      []string                    `json:"pyMgrs,omitempty"`
	WingetRows  []essentials.WingetTableRow `json:"wingetRows,omitempty"`
	PkgRows     []pkgmgr.PackageEntry       `json:"pkgRows,omitempty"`
	Error       string                      `json:"error,omitempty"`
	Completed   int                         `json:"completed"`
	Total       int                         `json:"total"`
	Pending     []string                    `json:"pending"`
	Done        bool                        `json:"done"`
}

// App is the main Wails application service.
type App struct {
	ctx           context.Context
	appDir        string
	dirMu         sync.RWMutex
	baseDir       string
	language      string
	settings      config.AppSettings
	startedHidden bool
	trayIconICO   []byte
	trayIconPNG   []byte
	trayMgr       *tray.Manager
	allowQuit     atomic.Bool
	windowVisible atomic.Bool
	hasTaskError  atomic.Bool
	scanBusy      atomic.Bool
	updateCount   atomic.Int32

	taskMu     sync.Mutex
	busy       bool
	cancelTask context.CancelFunc

	searchMu     sync.Mutex
	searchCancel context.CancelFunc
	searchSeq    int

	logMu        sync.Mutex
	logHistory   []TerminalLogEvent
	lastLTSCheck string

	essMu            sync.RWMutex
	wingetEnv        essentials.EnvStatus
	wingetByID       map[string]essentials.WingetTableRow
	wingetByName     map[string]essentials.WingetTableRow
	wingetRows       []essentials.WingetTableRow
	pkgMgrEnvs       []pkgmgr.ManagerEnv
	pkgMgrRows       []pkgmgr.PackageEntry
	regApps          []essentials.RegistryAppEntry
	fallbackRepoVers map[string]string
	wingetCacheDone  bool

	watchMu           sync.Mutex
	watchCancel       context.CancelFunc
	watchIgnorePkgMod time.Time
	watchIgnoreEssMod time.Time
	watchIgnoreIgnMod time.Time

	refreshMu    sync.Mutex
	refreshTimer *time.Timer

	monitorMu       sync.Mutex
	monitorCancel   context.CancelFunc
	lastMonitorRun  time.Time
	notifiedUpdates map[string]string

	sizeMu        sync.RWMutex
	urlSizes      map[string]int64
	sizeCachePath string
	iconCachePath string

	appUpdateMu   sync.RWMutex
	appUpdateInfo selfupdate.UpdateInfo
	appUpdatePath string
}

// NewApp creates a new App instance. Config files live in AppDir (beside UniDevUI.exe);
// extraction output defaults to C:\AppPath or persisted settings.
func NewApp(startedHidden bool, trayIconICO, trayIconPNG []byte) *App {
	settings, _, _ := config.LoadSettings()
	if regEn, regTray, rErr := autostart.IsEnabled(); rErr == nil {
		settings.AutoStart = regEn
		if regEn {
			settings.StartInTray = regTray
		}
	}
	settings = config.NormalizeSettings(settings)

	sizeCachePath := ""
	iconCachePath := ""
	appUpdatePath := ""
	if sp, spErr := config.SettingsPath(); spErr == nil && sp != "" {
		cfgDir := filepath.Dir(sp)
		sizeCachePath = filepath.Join(cfgDir, "size_cache.json")
		iconCachePath = filepath.Join(cfgDir, "icon_database_v2.json")
		appUpdatePath = filepath.Join(cfgDir, "app_update_cache.json")
	}
	if iconCachePath != "" {
		_ = pkgmgr.DefaultIconStore().LoadFromFile(iconCachePath)
	}
	cachedUpdate, _ := selfupdate.LoadCachedInfo(appUpdatePath, AppVersion, selfupdate.DefaultCheckCooldown)
	lastLTS := ""
	if settings.LastCheckUnix > 0 {
		lastLTS = time.Unix(settings.LastCheckUnix, 0).Format("15:04:05")
	}
	a := &App{
		appDir:           config.AppDir(),
		baseDir:          settings.BaseDir,
		language:         settings.Language,
		settings:         settings,
		startedHidden:    startedHidden,
		trayIconICO:      trayIconICO,
		trayIconPNG:      trayIconPNG,
		lastLTSCheck:     lastLTS,
		wingetEnv:        essentials.CheckEnvironment(),
		wingetByID:       make(map[string]essentials.WingetTableRow),
		wingetByName:     make(map[string]essentials.WingetTableRow),
		regApps:          essentials.QueryRegistryUninstallApps(),
		fallbackRepoVers: make(map[string]string),
		notifiedUpdates:  make(map[string]string),
		urlSizes:         lts.LoadSizeCache(sizeCachePath),
		sizeCachePath:    sizeCachePath,
		iconCachePath:    iconCachePath,
		appUpdateInfo:    cachedUpdate,
		appUpdatePath:    appUpdatePath,
	}
	a.windowVisible.Store(!startedHidden)
	return a
}

func (a *App) getBaseDir() string {
	a.dirMu.RLock()
	defer a.dirMu.RUnlock()
	if strings.TrimSpace(a.baseDir) == "" {
		return config.DefaultBaseDir
	}
	return a.baseDir
}

func (a *App) getLanguage() string {
	a.dirMu.RLock()
	defer a.dirMu.RUnlock()
	if a.language == "id" {
		return "id"
	}
	return "en"
}

func (a *App) getSettings() config.AppSettings {
	a.dirMu.RLock()
	s := a.settings
	if strings.TrimSpace(a.baseDir) != "" {
		s.BaseDir = a.baseDir
	}
	if strings.TrimSpace(a.language) != "" {
		s.Language = a.language
	}
	a.dirMu.RUnlock()
	if s.BaseDir == "" && s.CloseAction == "" && s.AutoCheckIntervalHours == 0 {
		return config.DefaultSettings()
	}
	return config.NormalizeSettings(s)
}

func (a *App) requestStateRefresh() {
	if a.ctx == nil {
		return
	}
	a.refreshMu.Lock()
	if a.refreshTimer != nil {
		a.refreshTimer.Stop()
	}
	a.refreshTimer = time.AfterFunc(250*time.Millisecond, func() {
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "state:refresh")
		}
	})
	a.refreshMu.Unlock()
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if exePath, eErr := a.resolvedExePath(); eErr == nil && exePath != "" {
		go selfupdate.CleanupStaleArtifacts(exePath)
	}
	baseDir := a.getBaseDir()
	pkgConf, pkgErr := config.EnsureConfig(a.appDir)
	essConf, essErr := essentials.EnsureConfig(a.appDir)
	ignConf, ignErr := essentials.EnsureIgnoredConfig(a.appDir)
	if pkgErr != nil {
		a.emitBiLog("ERROR", pkgErr.Error(), pkgErr.Error())
	} else if essErr != nil {
		a.emitBiLog("ERROR", essErr.Error(), essErr.Error())
	} else if ignErr != nil {
		a.emitBiLog("ERROR", ignErr.Error(), ignErr.Error())
	} else {
		a.emitBiLog("INFO",
			fmt.Sprintf("UniDevUI ready. App dir: %s | Output path: %s", a.appDir, baseDir),
			fmt.Sprintf("UniDevUI siap. Folder aplikasi: %s | Output path: %s", a.appDir, baseDir),
		)
	}
	a.startConfigWatcher(pkgConf, essConf, ignConf)

	curSettings := a.getSettings()
	a.trayMgr = tray.New(a.trayIconICO, a.trayIconPNG, curSettings.Language, curSettings.AutoStart, "UniDevUI", tray.Callbacks{
		OnShow: func() { a.ShowWindow() },
		OnNavRuntimes: func() {
			a.ShowWindow()
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "nav:runtimes")
			}
		},
		OnNavUpdates: func() {
			a.ShowWindow()
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "nav:updates")
			}
		},
		OnNavSearch: func() {
			a.ShowWindow()
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "nav:search")
			}
		},
		OnCheckUpdates:    func() { go a.CheckUpdatesFromTray() },
		OnToggleAutoStart: func() { a.toggleAutoStartFromTray() },
		OnNotifyClick: func() {
			a.ShowWindow()
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "nav:updates")
			}
		},
		OnOSShutdown: func() { a.QuitApp() },
		OnExit:       func() { a.QuitApp() },
	})
	a.trayMgr.Start()
	a.syncTrayAndPowerState()

	if a.startedHidden {
		a.emitBiLog("INFO",
			"UniDevUI started minimized in System Tray; deferred update scan scheduled.",
			"UniDevUI dijalankan tersembunyi di System Tray; scan pembaruan dijadwalkan setelah sistem stabil.",
		)
	}

	monCtx, monCancel := context.WithCancel(ctx)
	a.monitorMu.Lock()
	a.monitorCancel = monCancel
	a.monitorMu.Unlock()

	go a.runStartupSequence(monCtx)
	go a.runUpdateMonitorLoop(monCtx)
}

func (a *App) shutdown(ctx context.Context) {
	a.stopConfigWatcher()
	a.monitorMu.Lock()
	if a.monitorCancel != nil {
		a.monitorCancel()
		a.monitorCancel = nil
	}
	a.monitorMu.Unlock()
	a.refreshMu.Lock()
	if a.refreshTimer != nil {
		a.refreshTimer.Stop()
		a.refreshTimer = nil
	}
	a.refreshMu.Unlock()
	if a.trayMgr != nil {
		a.trayMgr.Stop()
	}
	a.CancelActiveTask()
}

func (a *App) markConfigMod(dst *time.Time, confPath string) {
	if info, err := os.Stat(confPath); err == nil {
		a.watchMu.Lock()
		*dst = info.ModTime()
		a.watchMu.Unlock()
	}
}

func (a *App) markPkgConfigSelfWritten(confPath string) {
	a.markConfigMod(&a.watchIgnorePkgMod, confPath)
}

func (a *App) markEssConfigSelfWritten(confPath string) {
	a.markConfigMod(&a.watchIgnoreEssMod, confPath)
}

func (a *App) markIgnConfigSelfWritten(confPath string) {
	a.markConfigMod(&a.watchIgnoreIgnMod, confPath)
}

func (a *App) stopConfigWatcher() {
	a.watchMu.Lock()
	if a.watchCancel != nil {
		a.watchCancel()
		a.watchCancel = nil
	}
	a.watchMu.Unlock()
}

func (a *App) startConfigWatcher(pkgConfPath, essConfPath, ignConfPath string) {
	a.stopConfigWatcher()
	if pkgConfPath == "" && essConfPath == "" && ignConfPath == "" {
		return
	}

	watchCtx, cancel := context.WithCancel(context.Background())
	a.watchMu.Lock()
	a.watchCancel = cancel
	a.watchMu.Unlock()

	go func() {
		var lastPkgMod, lastEssMod, lastIgnMod time.Time
		if info, err := os.Stat(pkgConfPath); err == nil {
			lastPkgMod = info.ModTime()
		}
		if info, err := os.Stat(essConfPath); err == nil {
			lastEssMod = info.ModTime()
		}
		if info, err := os.Stat(ignConfPath); err == nil {
			lastIgnMod = info.ModTime()
		}

		ticker := time.NewTicker(1500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-watchCtx.Done():
				return
			case <-ticker.C:
				changed := false
				if info, err := os.Stat(pkgConfPath); err == nil && info.ModTime().After(lastPkgMod) {
					lastPkgMod = info.ModTime()
					a.watchMu.Lock()
					ignored := !a.watchIgnorePkgMod.IsZero() && !info.ModTime().After(a.watchIgnorePkgMod)
					a.watchMu.Unlock()
					if !ignored {
						changed = true
						a.emitBiLog("INFO",
							"Changes detected in packages.conf, reloading list...",
							"Perubahan pada packages.conf terdeteksi, memuat ulang daftar...",
						)
						if pkgs, _, pErr := config.ParseFile(pkgConfPath); pErr != nil {
							a.emitBiLog("ERROR",
								fmt.Sprintf("Failed to reload packages.conf: %v", pErr),
								fmt.Sprintf("Gagal memuat ulang packages.conf: %v", pErr),
							)
						} else {
							a.emitBiLog("OK",
								fmt.Sprintf("Package list reloaded (%d packages).", len(pkgs)),
								fmt.Sprintf("Daftar paket berhasil dimuat (%d paket).", len(pkgs)),
							)
						}
					}
				}
				if info, err := os.Stat(essConfPath); err == nil && info.ModTime().After(lastEssMod) {
					lastEssMod = info.ModTime()
					a.watchMu.Lock()
					ignored := !a.watchIgnoreEssMod.IsZero() && !info.ModTime().After(a.watchIgnoreEssMod)
					a.watchMu.Unlock()
					if !ignored {
						changed = true
						a.emitBiLog("INFO",
							"Changes detected in essentials.conf, reloading list...",
							"Perubahan pada essentials.conf terdeteksi, memuat ulang daftar...",
						)
						if apps, _, eErr := essentials.ParseFile(essConfPath); eErr != nil {
							a.emitBiLog("ERROR",
								fmt.Sprintf("Failed to reload essentials.conf: %v", eErr),
								fmt.Sprintf("Gagal memuat ulang essentials.conf: %v", eErr),
							)
						} else {
							a.emitBiLog("OK",
								fmt.Sprintf("Windows Apps list reloaded (%d apps).", len(apps)),
								fmt.Sprintf("Daftar Windows Apps berhasil dimuat (%d aplikasi).", len(apps)),
							)
						}
					}
				}
				if info, err := os.Stat(ignConfPath); err == nil && info.ModTime().After(lastIgnMod) {
					lastIgnMod = info.ModTime()
					a.watchMu.Lock()
					ignored := !a.watchIgnoreIgnMod.IsZero() && !info.ModTime().After(a.watchIgnoreIgnMod)
					a.watchMu.Unlock()
					if !ignored {
						changed = true
						a.emitBiLog("INFO",
							"Changes detected in ignored_updates.conf, reloading...",
							"Perubahan pada ignored_updates.conf terdeteksi, memuat ulang...",
						)
						if list, _, iErr := essentials.ParseIgnoredFile(ignConfPath); iErr != nil {
							a.emitBiLog("ERROR",
								fmt.Sprintf("Failed to reload ignored_updates.conf: %v", iErr),
								fmt.Sprintf("Gagal memuat ulang ignored_updates.conf: %v", iErr),
							)
						} else {
							a.emitBiLog("OK",
								fmt.Sprintf("Ignored list reloaded (%d entries).", len(list)),
								fmt.Sprintf("Daftar abaikan berhasil dimuat (%d entri).", len(list)),
							)
						}
					}
				}
				if changed {
					go func() {
						sCtx, cancel := context.WithTimeout(watchCtx, 20*time.Second)
						defer cancel()
						updated := a.refreshPackageSizes(sCtx)
						if a.refreshEssentialSizes(sCtx) {
							updated = true
						}
						if updated {
							runtime.EventsEmit(a.ctx, "state:refresh")
						}
					}()
					runtime.EventsEmit(a.ctx, "state:refresh")
				}
			}
		}
	}()
}

// ReloadConfig reloads packages.conf, essentials.conf, and refreshes Winget status, logging a completion summary.
func (a *App) ReloadConfig() (DashboardState, error) {
	tools.InvalidateStatusCache()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	a.refreshWingetCache(ctx)
	go func() {
		sCtx, sCancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer sCancel()
		updated := a.refreshPackageSizes(sCtx)
		if a.refreshEssentialSizes(sCtx) {
			updated = true
		}
		if updated && a.ctx != nil {
			runtime.EventsEmit(a.ctx, "state:refresh")
		}
	}()

	st, err := a.GetDashboardState()
	if err != nil {
		return st, err
	}
	a.emitBiLog("OK",
		fmt.Sprintf("Configuration reloaded (%d runtime packages, %d CLI/SDK tools, %d Windows Apps).", len(st.Packages), len(st.Tools), len(st.Essentials)),
		fmt.Sprintf("Konfigurasi berhasil dimuat (%d paket runtime, %d CLI/SDK tool, %d Windows Apps).", len(st.Packages), len(st.Tools), len(st.Essentials)),
	)
	return a.GetDashboardState()
}

// GetDashboardState loads packages.conf, essentials.conf, disk install status, User PATH, and cached Winget statuses.
func (a *App) GetDashboardState() (DashboardState, error) {
	baseDir := a.getBaseDir()
	pkgConfPath, err := config.EnsureConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}
	essConfPath, err := essentials.EnsureConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}
	ignConfPath, err := essentials.EnsureIgnoredConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}

	pkgs, warnings, err := config.ParseFile(pkgConfPath)
	if err != nil {
		return DashboardState{}, err
	}
	for _, w := range warnings {
		a.emitBiLog("WARN", w, w)
	}

	essList, essWarnings, err := essentials.ParseFile(essConfPath)
	if err != nil {
		return DashboardState{}, err
	}
	for _, w := range essWarnings {
		a.emitBiLog("WARN", w, w)
	}

	ignoredList, ignoredMap, err := essentials.ParseIgnoredFile(ignConfPath)
	if err != nil {
		a.emitBiLog("WARN", err.Error(), err.Error())
		ignoredMap = make(map[string]essentials.IgnoredApp)
	}

	activeMap, err := pathmgr.ActiveVersions(baseDir)
	if err != nil {
		a.emitBiLog("WARN",
			fmt.Sprintf("Failed reading User PATH: %v", err),
			fmt.Sprintf("Gagal membaca User PATH: %v", err),
		)
		activeMap = map[string]string{}
	}

	userPaths, _, _ := pathmgr.GetUserPathEntries()
	userPathEntries, _ := pathmgr.InspectUserPathEntries(baseDir)

	views := make([]PackageView, 0, len(pkgs))
	for _, p := range pkgs {
		activeVer := activeMap[strings.ToLower(p.ID)]
		pv := PackageView{
			ID:          p.ID,
			Name:        p.Name,
			Category:    p.Category,
			BinSubdir:   p.BinSubdir,
			Binary:      p.Binary,
			LTSProvider: p.LTSProvider,
			Description: p.Description,
			ActiveVer:   activeVer,
			Versions:    make([]VersionView, 0, len(p.Versions)),
		}

		seenVers := make(map[string]bool)
		for _, v := range p.Versions {
			seenVers[v.Version] = true
			installDir := filepath.Join(baseDir, p.ID, v.Version)
			installed := dirExists(installDir)
			isActive := activeVer != "" && strings.EqualFold(activeVer, v.Version)
			binDir := ""
			if installed {
				binDir = archive.ResolveBinDir(installDir, p.BinSubdir, p.Binary)
			}
			if isActive {
				pv.ActiveDir = binDir
				if p.Binary != "" && binDir != "" {
					pv.Conflict = pathmgr.DetectBinaryConflict(p.Binary, binDir)
				}
			}
			pv.Versions = append(pv.Versions, VersionView{
				Version:      v.Version,
				URL:          v.URL,
				DownloadSize: a.getCachedURLSize(v.URL),
				Installed:    installed,
				Active:       isActive,
				BinDir:       binDir,
			})
		}

		if entries, err := os.ReadDir(filepath.Join(baseDir, p.ID)); err == nil {
			for _, e := range entries {
				if !e.IsDir() || seenVers[e.Name()] {
					continue
				}
				ver := e.Name()
				installDir := filepath.Join(baseDir, p.ID, ver)
				binDir := archive.ResolveBinDir(installDir, p.BinSubdir, p.Binary)
				isActive := activeVer != "" && strings.EqualFold(activeVer, ver)
				if isActive {
					pv.ActiveDir = binDir
					if p.Binary != "" && binDir != "" {
						pv.Conflict = pathmgr.DetectBinaryConflict(p.Binary, binDir)
					}
				}
				pv.Versions = append(pv.Versions, VersionView{
					Version:   ver,
					URL:       "",
					Installed: true,
					Active:    isActive,
					BinDir:    binDir,
				})
			}
		}

		views = append(views, pv)
	}

	populateVersionComparison(a.ctx, baseDir, views)

	a.essMu.RLock()
	wEnv := a.wingetEnv
	byID := a.wingetByID
	wingetRows := a.wingetRows
	pkgMgrEnvs := append([]pkgmgr.ManagerEnv(nil), a.pkgMgrEnvs...)
	pkgMgrRows := append([]pkgmgr.PackageEntry(nil), a.pkgMgrRows...)
	regApps := a.regApps
	repoVers := a.fallbackRepoVers
	curatedByWingetID := make(map[string]string, len(essList))
	for _, item := range essList {
		if k := strings.ToLower(strings.TrimSpace(item.WingetID)); k != "" {
			curatedByWingetID[k] = item.ID
		}
	}

	essViews := make([]EssentialView, 0, len(essList))
	for _, item := range essList {
		wKey := strings.ToLower(strings.TrimSpace(item.WingetID))
		ignEntry, hasIgn := ignoredMap[wKey]
		ev := EssentialView{
			ID:           item.ID,
			Name:         item.Name,
			Category:     item.Category,
			WingetID:     item.WingetID,
			Source:       item.Source,
			RequiresUAC:  item.RequiresUAC,
			Versions:     item.Versions,
			Description:  item.Description,
			IconURL:      pkgmgr.LookupIconMedia(pkgmgr.ManagerWinget, item.WingetID, item.Name).Icon,
			DownloadSize: a.getCachedURLSize("__ess:" + wKey),
		}
		match := essentials.MatchInstalledEssential(item, byID, regApps, wingetRows)
		if match.Installed {
			ev.Installed = true
			ev.InstalledVer = match.InstalledVer
			ev.AvailableVer = match.AvailableVer
			ev.HasUpdate = strings.TrimSpace(match.AvailableVer) != ""
			ev.MatchedBy = essentials.DescribeDetectionSource(match.MatchedVia)

			if !match.ExactWinget && ev.AvailableVer == "" {
				if latestRepo := repoVers[wKey]; latestRepo != "" {
					ev.AvailableVer = latestRepo
					if ev.InstalledVer != "" {
						cmp := pathmgr.CompareSemver(ev.InstalledVer, latestRepo)
						if cmp < 0 {
							ev.HasUpdate = true
						}
					}
				}
			}

			pauseActive := essentials.IsPausedActive(ignEntry, time.Now())
			if pauseActive {
				ev.PauseUntil = ignEntry.PauseUntil
			}
			fullIgnored, verSkipped := essentials.EvaluateIgnoredPolicy(ignEntry, hasIgn, ev.AvailableVer)
			if fullIgnored {
				ev.UpdateIgnored = true
				ev.HasUpdate = false
				ev.VersionStatus = "ignored"
			} else if verSkipped {
				if strings.TrimSpace(ignEntry.SkippedVersion) != "" {
					ev.SkippedVersion = ignEntry.SkippedVersion
				}
				ev.HasUpdate = false
				ev.VersionStatus = "uptodate"
			} else {
				switch {
				case ev.HasUpdate:
					ev.VersionStatus = "outdated"
				case ev.InstalledVer == "":
					ev.VersionStatus = "detected"
				case !match.ExactWinget && ev.AvailableVer != "" && pathmgr.CompareSemver(ev.InstalledVer, ev.AvailableVer) > 0:
					ev.VersionStatus = "ahead"
				default:
					ev.VersionStatus = "uptodate"
				}
			}
		} else if fullIgn, _ := essentials.EvaluateIgnoredPolicy(ignEntry, hasIgn, ""); fullIgn {
			ev.UpdateIgnored = true
			ev.VersionStatus = "ignored"
		}
		essViews = append(essViews, ev)
	}
	installedApps := a.buildInstalledAppViews(wingetRows, pkgMgrRows, ignoredList, ignoredMap, curatedByWingetID)
	a.essMu.RUnlock()

	a.taskMu.Lock()
	busy := a.busy
	a.taskMu.Unlock()

	a.logMu.Lock()
	logsCopy := append([]TerminalLogEvent(nil), a.logHistory...)
	lastCheck := a.lastLTSCheck
	a.logMu.Unlock()

	toolsList := tools.ListToolStatuses(baseDir)
	for i := range toolsList {
		switch toolsList[i].ID {
		case "vsbuildtools":
			toolsList[i].DownloadSize = a.getCachedURLSize(tools.VSBootstrapURL)
		case "webview2":
			toolsList[i].DownloadSize = a.getCachedURLSize("__tool:webview2")
		}
	}
	if len(pkgMgrEnvs) == 0 {
		for _, tl := range toolsList {
			if tl.Category == "pkgmgr" {
				pkgMgrEnvs = append(pkgMgrEnvs, pkgmgr.ManagerEnv{
					ID:        tl.ID,
					Name:      tl.Name,
					Available: tl.Installed,
					Version:   strings.TrimPrefix(tl.Version, "v"),
					ExePath:   tl.Location,
				})
			}
		}
	}

	a.appUpdateMu.RLock()
	appUpdateCopy := a.appUpdateInfo
	a.appUpdateMu.RUnlock()
	if appUpdateCopy.CurrentVersion == "" {
		appUpdateCopy.CurrentVersion = selfupdate.NormalizeVersion(AppVersion)
	}

	state := DashboardState{
		AppDir:             a.appDir,
		BaseDir:            baseDir,
		ConfPath:           pkgConfPath,
		EssentialsConfPath: essConfPath,
		IgnoredConfPath:    ignConfPath,
		Packages:           views,
		Tools:              toolsList,
		PkgManagers:        pkgMgrEnvs,
		Essentials:         essViews,
		InstalledApps:      installedApps,
		WingetEnv:          wEnv,
		UserPathList:       userPaths,
		UserPathEntries:    userPathEntries,
		Logs:               logsCopy,
		Busy:               busy,
		LastLTSCheck:       lastCheck,
		Language:           a.getLanguage(),
		Settings:           a.getSettings(),
		AppVersion:         AppVersion,
		AppUpdate:          appUpdateCopy,
	}
	updItems := extractUpdateNoticeItems(state)
	a.updateCount.Store(int32(len(updItems)))
	if a.trayMgr != nil {
		a.syncTrayAndPowerState()
	}
	return state, nil
}

func (a *App) buildInstalledAppViews(
	rows []essentials.WingetTableRow,
	pkgRows []pkgmgr.PackageEntry,
	ignoredList []essentials.IgnoredApp,
	ignoredMap map[string]essentials.IgnoredApp,
	curatedByWingetID map[string]string,
) []InstalledAppView {
	out := make([]InstalledAppView, 0, len(rows)+len(pkgRows)+len(ignoredList))
	seen := make(map[string]bool, len(rows)+len(pkgRows)+len(ignoredList))

	lookupIgnored := func(mgr, id string) (essentials.IgnoredApp, bool) {
		k1 := strings.ToLower(pkgmgr.IgnoreKey(mgr, id))
		if ig, ok := ignoredMap[k1]; ok {
			return ig, true
		}
		if pkgmgr.NormalizeManager(mgr) == pkgmgr.ManagerWinget {
			if ig, ok := ignoredMap[strings.ToLower(strings.TrimSpace(id))]; ok {
				return ig, true
			}
		}
		return essentials.IgnoredApp{}, false
	}

	for _, r := range rows {
		id := strings.TrimSpace(r.ID)
		if id == "" {
			continue
		}
		dedupKey := "winget:" + strings.ToLower(id)
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		avail := strings.TrimSpace(r.Available)
		ignEntry, hasIgn := lookupIgnored(pkgmgr.ManagerWinget, id)
		pauseActive := essentials.IsPausedActive(ignEntry, time.Now())
		fullIgn, verSkipped := essentials.EvaluateIgnoredPolicy(ignEntry, hasIgn, avail)
		skipVer := ""
		pauseUntil := ""
		if pauseActive {
			pauseUntil = ignEntry.PauseUntil
		}
		if verSkipped && strings.TrimSpace(ignEntry.SkippedVersion) != "" {
			skipVer = ignEntry.SkippedVersion
		}
		curatedID := curatedByWingetID[strings.ToLower(id)]
		manageable := essentials.IsManageableWingetID(id)
		out = append(out, InstalledAppView{
			Manager:         pkgmgr.ManagerWinget,
			ID:              id,
			Name:            r.Name,
			InstalledVer:    strings.TrimSpace(r.Version),
			AvailableVer:    avail,
			Source:          strings.TrimSpace(r.Source),
			IconURL:         pkgmgr.LookupIconMedia(pkgmgr.ManagerWinget, id, r.Name).Icon,
			HasUpdate:       avail != "" && !fullIgn && !verSkipped,
			Ignored:         fullIgn,
			SkippedVersion:  skipVer,
			PauseUntil:      pauseUntil,
			WingetPinned:    fullIgn && ignEntry.WingetPinned,
			IgnoreReason:    ignEntry.Reason,
			ManagedByWinget: manageable,
			CanUpdate:       manageable,
			CanUninstall:    manageable,
			PinnedCurated:   curatedID != "",
			CuratedAppID:    curatedID,
			DownloadSize:    a.getCachedURLSize("__ess:" + strings.ToLower(id)),
		})
	}

	for _, p := range pkgRows {
		mgr := pkgmgr.NormalizeManager(p.Manager)
		id := strings.TrimSpace(p.ID)
		if mgr == "" || id == "" {
			continue
		}
		dedupKey := mgr + ":" + strings.ToLower(id)
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		avail := strings.TrimSpace(p.AvailableVer)
		ignEntry, hasIgn := lookupIgnored(mgr, id)
		pauseActive := essentials.IsPausedActive(ignEntry, time.Now())
		fullIgn, verSkipped := essentials.EvaluateIgnoredPolicy(ignEntry, hasIgn, avail)
		skipVer := ""
		pauseUntil := ""
		if pauseActive {
			pauseUntil = ignEntry.PauseUntil
		}
		if verSkipped && strings.TrimSpace(ignEntry.SkippedVersion) != "" {
			skipVer = ignEntry.SkippedVersion
		}
		iconURL := strings.TrimSpace(p.IconURL)
		if iconURL == "" {
			iconURL = pkgmgr.LookupIconMedia(mgr, id, p.Name).Icon
		}
		out = append(out, InstalledAppView{
			Manager:        mgr,
			ID:             id,
			Name:           pkgmgr.FormatDisplayName(id, p.Name),
			InstalledVer:   strings.TrimSpace(p.Version),
			AvailableVer:   avail,
			Source:         strings.TrimSpace(p.Source),
			Description:    strings.TrimSpace(p.Description),
			Homepage:       strings.TrimSpace(p.Homepage),
			IconURL:        iconURL,
			HasUpdate:      p.HasUpdate && avail != "" && !fullIgn && !verSkipped,
			Ignored:        fullIgn,
			SkippedVersion: skipVer,
			PauseUntil:     pauseUntil,
			IgnoreReason:   ignEntry.Reason,
			CanUpdate:      true,
			CanUninstall:   !pkgmgr.IsCoreProtectedPackage(mgr, id),
		})
	}

	// Ensure any full-ignored entry saved in ignored_updates.conf appears under Ignored even if absent from active scans.
	for _, ig := range ignoredList {
		if strings.TrimSpace(ig.SkippedVersion) != "" || strings.TrimSpace(ig.PauseUntil) != "" {
			continue
		}
		mgr, rawID := pkgmgr.SplitIgnoreKey(ig.ID, ig.Source)
		key := strings.ToLower(strings.TrimSpace(rawID))
		if key == "" {
			continue
		}
		dedupKey := mgr + ":" + key
		if seen[dedupKey] {
			continue
		}
		seen[dedupKey] = true

		manageable := mgr == pkgmgr.ManagerWinget && essentials.IsManageableWingetID(rawID)
		curatedID := ""
		if mgr == pkgmgr.ManagerWinget {
			curatedID = curatedByWingetID[key]
		}
		out = append(out, InstalledAppView{
			Manager:         mgr,
			ID:              rawID,
			Name:            ig.Name,
			Source:          ig.Source,
			IconURL:         pkgmgr.LookupIconMedia(mgr, rawID, ig.Name).Icon,
			Ignored:         true,
			WingetPinned:    ig.WingetPinned,
			IgnoreReason:    ig.Reason,
			ManagedByWinget: manageable,
			CanUpdate:       manageable || mgr != pkgmgr.ManagerWinget,
			CanUninstall:    manageable || (mgr != pkgmgr.ManagerWinget && !pkgmgr.IsCoreProtectedPackage(mgr, rawID)),
			PinnedCurated:   curatedID != "",
			CuratedAppID:    curatedID,
			DownloadSize:    a.getCachedURLSize("__ess:" + key),
		})
	}

	sort.SliceStable(out, func(i, j int) bool {
		aItem, bItem := out[i], out[j]
		if aItem.Ignored != bItem.Ignored {
			return !aItem.Ignored
		}
		if aItem.HasUpdate != bItem.HasUpdate {
			return aItem.HasUpdate
		}
		aManage := aItem.ManagedByWinget || aItem.Manager != pkgmgr.ManagerWinget
		bManage := bItem.ManagedByWinget || bItem.Manager != pkgmgr.ManagerWinget
		if aManage != bManage {
			return aManage
		}
		ni := strings.ToLower(aItem.Name)
		nj := strings.ToLower(bItem.Name)
		if ni != nj {
			return ni < nj
		}
		return strings.ToLower(aItem.ID) < strings.ToLower(bItem.ID)
	})
	return out
}

func (a *App) refreshWingetCache(ctx context.Context) {
	baseDir := a.getBaseDir()
	env := essentials.CheckEnvironment()
	regApps := essentials.QueryRegistryUninstallApps()
	var byID, byName map[string]essentials.WingetTableRow
	var rows []essentials.WingetTableRow
	var pkgMgrEnvs []pkgmgr.ManagerEnv
	var pkgMgrRows []pkgmgr.PackageEntry

	var scanWG sync.WaitGroup
	if env.WingetAvailable {
		scanWG.Add(1)
		go func() {
			defer scanWG.Done()
			idMap, nameMap, rawRows, err := essentials.QueryInstalledEntries(ctx)
			if err == nil {
				byID = idMap
				byName = nameMap
				rows = rawRows
			}
		}()
	}
	scanWG.Add(1)
	go func() {
		defer scanWG.Done()
		pkgMgrEnvs, pkgMgrRows = pkgmgr.QueryAllInstalled(ctx, baseDir)
	}()
	scanWG.Wait()

	// For apps detected via Windows Registry / display name (where Winget ARP ID differs or was ahead of repo),
	// fetch live latest repo versions via `winget show` so UniDevUI shows accurate Up-to-date / Ahead / Update comparison.
	repoVers := make(map[string]string)
	if env.WingetAvailable {
		if essConfPath, err := essentials.EnsureConfig(a.appDir); err == nil {
			if list, _, pErr := essentials.ParseFile(essConfPath); pErr == nil {
				var fallbackItems []essentials.EssentialApp
				for _, item := range list {
					m := essentials.MatchInstalledEssential(item, byID, regApps, rows)
					if m.Installed && !m.ExactWinget && m.AvailableVer == "" {
						fallbackItems = append(fallbackItems, item)
					}
				}
				if len(fallbackItems) > 0 {
					var mu sync.Mutex
					var wg sync.WaitGroup
					sem := make(chan struct{}, 2)
					for _, item := range fallbackItems {
						it := item
						wg.Add(1)
						go func() {
							defer wg.Done()
							select {
							case sem <- struct{}{}:
								defer func() { <-sem }()
							case <-ctx.Done():
								return
							}
							if det, err := essentials.ShowPackageDetails(ctx, it.WingetID, it.Source, false); err == nil {
								ver := essentials.CleanInstalledVersion(det.Version)
								if ver != "" {
									mu.Lock()
									repoVers[strings.ToLower(strings.TrimSpace(it.WingetID))] = ver
									mu.Unlock()
								}
							}
						}()
					}
					wg.Wait()
				}
			}
		}
	}

	a.essMu.Lock()
	a.wingetEnv = env
	a.regApps = regApps
	if byID != nil {
		a.wingetByID = byID
		a.wingetByName = byName
		a.wingetRows = rows
	}
	a.pkgMgrEnvs = pkgMgrEnvs
	a.pkgMgrRows = pkgMgrRows
	for k, v := range repoVers {
		a.fallbackRepoVers[k] = v
	}
	a.wingetCacheDone = true
	a.essMu.Unlock()
}

// RefreshWingetStatus manually refreshes Winget + Package Manager availability, UAC state, and installed app map.
func (a *App) RefreshWingetStatus() (DashboardState, error) {
	tools.InvalidateStatusCache()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	a.refreshWingetCache(ctx)
	st, err := a.GetDashboardState()
	if err != nil {
		return st, err
	}
	a.emitBiLog("OK",
		fmt.Sprintf("Catalog refreshed (%d curated apps, %d installed entries across Winget & package managers).", len(st.Essentials), len(st.InstalledApps)),
		fmt.Sprintf("Katalog diperbarui (%d aplikasi esensial, %d entri terpasang di Winget & package managers).", len(st.Essentials), len(st.InstalledApps)),
	)
	return st, nil
}

// RestartAsAdmin prompts UAC elevation and launches UniDevUI.exe as Administrator, closing the standard instance.
func (a *App) RestartAsAdmin() error {
	if essentials.IsProcessElevated() {
		a.emitBiLog("INFO", "UniDevUI is already running as Administrator.", "UniDevUI sudah berjalan sebagai Administrator.")
		return nil
	}
	if err := essentials.RestartAsAdmin(); err != nil {
		a.emitBiLog("WARN", err.Error(), fmt.Sprintf("Permintaan UAC Admin dibatalkan atau gagal: %v", err))
		return err
	}
	runtime.Quit(a.ctx)
	return nil
}

// InstallWingetCLI installs Microsoft App Installer (winget) from GitHub releases.
func (a *App) InstallWingetCLI() error {
	ctx, err := a.beginTask()
	if err != nil {
		return err
	}
	cacheDir := filepath.Join(a.getBaseDir(), "_cache")
	go func() {
		defer a.endTask()
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "install", Percent: -1, Detail: "Installing Winget (App Installer)..."})
		if err := essentials.InstallWinget(ctx, cacheDir, a.emitBiLog); err != nil {
			a.emitBiLog("ERROR",
				fmt.Sprintf("Failed installing Winget: %v", err),
				fmt.Sprintf("Gagal memasang Winget: %v", err),
			)
		} else {
			a.refreshWingetCache(ctx)
			a.emitBiLog("OK", "Winget installed and ready.", "Winget berhasil dipasang dan siap digunakan.")
		}
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()
	return nil
}

// RunEssentialTool installs, updates, or fixes a single app from essentials.conf via silent winget.
func (a *App) RunEssentialTool(appID, mode, version string) error {
	essConfPath, err := essentials.EnsureConfig(a.appDir)
	if err != nil {
		return err
	}
	list, _, err := essentials.ParseFile(essConfPath)
	if err != nil {
		return err
	}

	var target *essentials.EssentialApp
	for i := range list {
		if strings.EqualFold(list[i].ID, appID) {
			target = &list[i]
			break
		}
	}
	if target == nil {
		return a.biErr(
			fmt.Sprintf("essential app %s not found in essentials.conf", appID),
			fmt.Sprintf("aplikasi %s tidak ditemukan di essentials.conf", appID),
		)
	}

	ctx, err := a.beginTask()
	if err != nil {
		return err
	}

	go func() {
		defer a.endTask()
		verLabel := version
		if verLabel == "" {
			verLabel = "latest"
		}
		effectiveMode := mode
		if strings.EqualFold(mode, "update") {
			a.essMu.RLock()
			match := essentials.MatchInstalledEssential(*target, a.wingetByID, a.regApps, a.wingetRows)
			a.essMu.RUnlock()
			if match.Installed && !match.ExactWinget {
				effectiveMode = "install"
				a.emitBiLog("INFO",
					fmt.Sprintf("%s was detected via %s; running official Winget installer to upgrade in-place...", target.Name, essentials.DescribeDetectionSource(match.MatchedVia)),
					fmt.Sprintf("%s terdeteksi lewat %s; menjalankan installer resmi Winget untuk menimpa/update di tempat...", target.Name, essentials.DescribeDetectionSource(match.MatchedVia)),
				)
			}
		}
		a.emitBiLog("INFO",
			fmt.Sprintf("Starting %s for %s (%s, version: %s)...", mode, target.Name, target.WingetID, verLabel),
			fmt.Sprintf("Memulai %s untuk %s (%s, versi: %s)...", mode, target.Name, target.WingetID, verLabel),
		)
		appTarget := *target
		appTarget.ExpectedSize = a.getCachedURLSize("__ess:" + strings.ToLower(strings.TrimSpace(target.WingetID)))
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "install", Percent: -1, Detail: fmt.Sprintf("%s %s...", mode, target.Name)})
		if err := essentials.RunEssentialAction(ctx, appTarget, effectiveMode, version, a.emitBiLog, a.emitProgress); err != nil {
			a.emitBiLog("ERROR",
				fmt.Sprintf("%s failed for %s: %v", mode, target.Name, err),
				fmt.Sprintf("%s gagal untuk %s: %v", mode, target.Name, err),
			)
		} else {
			a.refreshWingetCache(ctx)
			a.emitBiLog("OK",
				fmt.Sprintf("%s completed for %s.", mode, target.Name),
				fmt.Sprintf("%s selesai untuk %s.", mode, target.Name),
			)
		}
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()
	return nil
}

// InstallSelectedEssentials installs a batch queue of selected essential apps silently.
func (a *App) InstallSelectedEssentials(items []BatchEssentialItem) error {
	if len(items) == 0 {
		return a.biErr("no applications selected", "belum ada aplikasi yang dipilih")
	}
	essConfPath, err := essentials.EnsureConfig(a.appDir)
	if err != nil {
		return err
	}
	list, _, err := essentials.ParseFile(essConfPath)
	if err != nil {
		return err
	}

	byID := make(map[string]essentials.EssentialApp, len(list))
	for _, item := range list {
		byID[strings.ToLower(item.ID)] = item
	}

	type queueEntry struct {
		app     essentials.EssentialApp
		version string
	}
	var queue []queueEntry
	for _, req := range items {
		if app, ok := byID[strings.ToLower(req.ID)]; ok {
			queue = append(queue, queueEntry{app: app, version: req.Version})
		}
	}
	if len(queue) == 0 {
		return a.biErr("selected apps not found in essentials.conf", "aplikasi yang dipilih tidak ditemukan di essentials.conf")
	}

	ctx, err := a.beginTask()
	if err != nil {
		return err
	}

	go func() {
		defer a.endTask()
		total := len(queue)
		succeeded, failed := 0, 0
		a.emitBiLog("INFO",
			fmt.Sprintf("Starting batch silent install for %d applications...", total),
			fmt.Sprintf("Memulai instalasi silent massal untuk %d aplikasi...", total),
		)

		for idx, q := range queue {
			if ctx.Err() != nil {
				a.emitBiLog("WARN", "Batch installation cancelled.", "Instalasi massal dibatalkan.")
				break
			}
			pct := (float64(idx) / float64(total)) * 100.0
			batchDetail := fmt.Sprintf("[%d/%d] %s", idx+1, total, q.app.Name)
			runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{
				Stage:   "install",
				Percent: pct,
				Detail:  fmt.Sprintf("%s...", batchDetail),
			})
			a.emitBiLog("INFO",
				fmt.Sprintf("[%d/%d] Silent installing %s (%s)...", idx+1, total, q.app.Name, q.app.WingetID),
				fmt.Sprintf("[%d/%d] Menginstal silent %s (%s)...", idx+1, total, q.app.Name, q.app.WingetID),
			)
			q.app.ExpectedSize = a.getCachedURLSize("__ess:" + strings.ToLower(strings.TrimSpace(q.app.WingetID)))
			batchProgress := func(ev archive.ProgressEvent) {
				ev.Detail = batchDetail
				a.emitProgress(ev)
			}
			if err := essentials.RunEssentialAction(ctx, q.app, "install", q.version, a.emitBiLog, batchProgress); err != nil {
				failed++
				a.emitBiLog("ERROR",
					fmt.Sprintf("[%d/%d] Failed installing %s: %v", idx+1, total, q.app.Name, err),
					fmt.Sprintf("[%d/%d] Gagal menginstal %s: %v", idx+1, total, q.app.Name, err),
				)
			} else {
				succeeded++
				a.emitBiLog("OK",
					fmt.Sprintf("[%d/%d] Installed %s.", idx+1, total, q.app.Name),
					fmt.Sprintf("[%d/%d] Berhasil menginstal %s.", idx+1, total, q.app.Name),
				)
			}
		}

		a.refreshWingetCache(context.Background())
		a.emitBiLog("OK",
			fmt.Sprintf("Batch install finished: %d succeeded, %d failed.", succeeded, failed),
			fmt.Sprintf("Instalasi massal selesai: %d berhasil, %d gagal.", succeeded, failed),
		)
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()
	return nil
}

func (a *App) annotateWingetRows(ctx context.Context, rows []essentials.WingetTableRow) []essentials.WingetTableRow {
	a.essMu.RLock()
	cacheDone := a.wingetCacheDone
	a.essMu.RUnlock()
	if !cacheDone {
		a.refreshWingetCache(ctx)
	}

	var ignoredMap map[string]essentials.IgnoredApp
	if ignConfPath, err := essentials.EnsureIgnoredConfig(a.appDir); err == nil {
		_, ignoredMap, _ = essentials.ParseIgnoredFile(ignConfPath)
	}

	a.essMu.RLock()
	annotated := essentials.AnnotateSearchRowsInstalled(rows, a.wingetByID, a.regApps, a.wingetRows, ignoredMap)
	a.essMu.RUnlock()
	for i := range annotated {
		if annotated[i].IconURL == "" {
			annotated[i].IconURL = pkgmgr.LookupIconMedia(pkgmgr.ManagerWinget, annotated[i].ID, annotated[i].Name).Icon
		}
	}
	return annotated
}

// SearchWingetApps searches the Winget catalog for apps not in the curated list.
func (a *App) SearchWingetApps(query string) ([]essentials.WingetTableRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	rows, err := essentials.SearchWinget(ctx, query)
	if err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return nil, err
	}

	annotated := a.annotateWingetRows(ctx, rows)

	a.emitBiLog("INFO",
		fmt.Sprintf("Winget search %q returned %d results.", query, len(annotated)),
		fmt.Sprintf("Pencarian Winget %q menemukan %d hasil.", query, len(annotated)),
	)
	return annotated, nil
}

// SearchPackageManager queries a Tab 1/Tab 2 extra package manager (scoop, choco, npm, pnpm, bun, uv)
// and marks entries already installed on this machine.
func (a *App) SearchPackageManager(manager, query string) ([]pkgmgr.PackageEntry, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	a.essMu.RLock()
	installedSnapshot := append([]pkgmgr.PackageEntry(nil), a.pkgMgrRows...)
	a.essMu.RUnlock()

	rows, err := pkgmgr.SearchPackages(ctx, manager, query, a.getBaseDir(), installedSnapshot)
	if err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return nil, err
	}
	a.emitBiLog("INFO",
		fmt.Sprintf("[%s] search %q returned %d results.", pkgmgr.NormalizeManager(manager), query, len(rows)),
		fmt.Sprintf("[%s] pencarian %q menemukan %d hasil.", pkgmgr.NormalizeManager(manager), query, len(rows)),
	)
	return rows, nil
}

type unifiedSearchTask struct {
	sourceGroup string
	jsMgrs      []string
	pyMgrs      []string
	run         func(ctx context.Context) ([]essentials.WingetTableRow, []pkgmgr.PackageEntry, error)
}

// CancelUnifiedSearch aborts any in-flight multi-manager search immediately.
func (a *App) CancelUnifiedSearch() {
	a.searchMu.Lock()
	if a.searchCancel != nil {
		a.searchCancel()
		a.searchCancel = nil
	}
	a.searchSeq++
	a.searchMu.Unlock()
}

// StartUnifiedSearch runs parallel package search across Winget (including MS Store) and all installed
// extra package managers, streaming each source's results immediately via "pkgsearch:chunk".
func (a *App) StartUnifiedSearch(token int, query string) (UnifiedSearchStart, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		a.CancelUnifiedSearch()
		return UnifiedSearchStart{Token: token, Query: ""}, nil
	}

	baseDir := a.getBaseDir()
	a.essMu.RLock()
	wingetAvail := a.wingetEnv.WingetAvailable
	installedSnapshot := append([]pkgmgr.PackageEntry(nil), a.pkgMgrRows...)
	a.essMu.RUnlock()

	envs := pkgmgr.DetectManagerEnvs(baseDir)
	envAvail := make(map[string]bool, len(envs))
	for _, e := range envs {
		envAvail[e.ID] = e.Available
	}

	collectAvail := func(candidates ...string) []string {
		var res []string
		for _, c := range candidates {
			if envAvail[c] {
				res = append(res, c)
			}
		}
		return res
	}
	availJS := collectAvail(pkgmgr.ManagerNpm, pkgmgr.ManagerPnpm, pkgmgr.ManagerBun)
	availPY := collectAvail(pkgmgr.ManagerUv, pkgmgr.ManagerPip)

	var tasks []unifiedSearchTask
	if wingetAvail {
		tasks = append(tasks, unifiedSearchTask{
			sourceGroup: "winget",
			run: func(ctx context.Context) ([]essentials.WingetTableRow, []pkgmgr.PackageEntry, error) {
				rows, err := essentials.SearchWinget(ctx, q)
				if err != nil {
					return nil, nil, err
				}
				return a.annotateWingetRows(ctx, rows), nil, nil
			},
		})
	}
	for _, directMgr := range []string{pkgmgr.ManagerScoop, pkgmgr.ManagerChoco} {
		if !envAvail[directMgr] {
			continue
		}
		mgrID := directMgr
		tasks = append(tasks, unifiedSearchTask{
			sourceGroup: mgrID,
			run: func(ctx context.Context) ([]essentials.WingetTableRow, []pkgmgr.PackageEntry, error) {
				rows, err := pkgmgr.SearchPackages(ctx, mgrID, q, baseDir, installedSnapshot)
				return nil, rows, err
			},
		})
	}
	appendSharedRegistryTask := func(group string, mgrs []string) {
		if len(mgrs) == 0 {
			return
		}
		mgrsCopy := append([]string(nil), mgrs...)
		t := unifiedSearchTask{
			sourceGroup: group,
			run: func(ctx context.Context) ([]essentials.WingetTableRow, []pkgmgr.PackageEntry, error) {
				rows, err := pkgmgr.SearchPackages(ctx, mgrsCopy[0], q, baseDir, installedSnapshot)
				if err != nil {
					return nil, nil, err
				}
				rows = pkgmgr.AnnotateInstalledAcrossManagers(rows, mgrsCopy, installedSnapshot)
				return nil, rows, nil
			},
		}
		if group == "js" {
			t.jsMgrs = mgrsCopy
		} else if group == "py" {
			t.pyMgrs = mgrsCopy
		}
		tasks = append(tasks, t)
	}
	appendSharedRegistryTask("js", availJS)
	appendSharedRegistryTask("py", availPY)
	for _, devMgr := range []string{pkgmgr.ManagerCargo, pkgmgr.ManagerDotnet, pkgmgr.ManagerPSGallery} {
		if !envAvail[devMgr] {
			continue
		}
		mgrID := devMgr
		tasks = append(tasks, unifiedSearchTask{
			sourceGroup: mgrID,
			run: func(ctx context.Context) ([]essentials.WingetTableRow, []pkgmgr.PackageEntry, error) {
				rows, err := pkgmgr.SearchPackages(ctx, mgrID, q, baseDir, installedSnapshot)
				return nil, rows, err
			},
		})
	}

	sources := make([]string, 0, len(tasks))
	for _, t := range tasks {
		sources = append(sources, t.sourceGroup)
	}

	a.searchMu.Lock()
	if a.searchCancel != nil {
		a.searchCancel()
		a.searchCancel = nil
	}
	a.searchSeq = token
	if len(tasks) == 0 {
		a.searchMu.Unlock()
		return UnifiedSearchStart{Token: token, Query: q, Total: 0}, nil
	}
	searchCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	a.searchCancel = cancel
	a.searchMu.Unlock()

	a.emitBiLog("INFO",
		fmt.Sprintf("Unified search %q across %d active sources (%s)...", q, len(sources), strings.Join(sources, ", ")),
		fmt.Sprintf("Pencarian terpadu %q di %d sumber aktif (%s)...", q, len(sources), strings.Join(sources, ", ")),
	)

	go a.executeUnifiedSearchTasks(searchCtx, cancel, token, q, tasks, func(chunk UnifiedSearchChunk) {
		a.searchMu.Lock()
		isLatest := a.searchSeq == token
		if chunk.Done && isLatest {
			a.searchCancel = nil
		}
		a.searchMu.Unlock()

		if !isLatest || searchCtx.Err() != nil {
			return
		}
		if chunk.Error != "" {
			a.emitBiLog("WARN",
				fmt.Sprintf("[%s] search failed: %s", chunk.SourceGroup, chunk.Error),
				fmt.Sprintf("[%s] pencarian gagal: %s", chunk.SourceGroup, chunk.Error),
			)
		}
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "pkgsearch:chunk", chunk)
		}
	})

	return UnifiedSearchStart{
		Token:   token,
		Query:   q,
		Total:   len(sources),
		Sources: sources,
		JSMgrs:  availJS,
		PyMgrs:  availPY,
	}, nil
}

func (a *App) executeUnifiedSearchTasks(
	searchCtx context.Context,
	cancel context.CancelFunc,
	token int,
	query string,
	tasks []unifiedSearchTask,
	emit func(UnifiedSearchChunk),
) {
	defer cancel()
	total := len(tasks)
	if total == 0 {
		return
	}

	orderedSources := make([]string, total)
	pendingMap := make(map[string]bool, total)
	for i, t := range tasks {
		orderedSources[i] = t.sourceGroup
		pendingMap[t.sourceGroup] = true
	}

	var mu sync.Mutex
	completed := 0
	var wg sync.WaitGroup

	for _, t := range tasks {
		task := t
		wg.Add(1)
		go func() {
			defer wg.Done()
			srcCtx, srcCancel := context.WithTimeout(searchCtx, 22*time.Second)
			defer srcCancel()

			wRows, pRows, err := task.run(srcCtx)
			if searchCtx.Err() != nil {
				return
			}

			mu.Lock()
			completed++
			delete(pendingMap, task.sourceGroup)
			pending := make([]string, 0, len(pendingMap))
			for _, s := range orderedSources {
				if pendingMap[s] {
					pending = append(pending, s)
				}
			}
			done := completed >= total
			curCompleted := completed
			mu.Unlock()

			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			emit(UnifiedSearchChunk{
				Token:       token,
				Query:       query,
				SourceGroup: task.sourceGroup,
				JSMgrs:      append([]string(nil), task.jsMgrs...),
				PyMgrs:      append([]string(nil), task.pyMgrs...),
				WingetRows:  wRows,
				PkgRows:     pRows,
				Error:       errStr,
				Completed:   curCompleted,
				Total:       total,
				Pending:     pending,
				Done:        done,
			})
		}()
	}
	wg.Wait()
}

// GetWingetPackageDetails returns live `winget show` metadata plus UniGetUI icons/screenshots.
func (a *App) GetWingetPackageDetails(wingetID, source string) (essentials.PackageDetails, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	det, err := essentials.ShowPackageDetails(ctx, wingetID, source, true)
	if err != nil {
		return essentials.PackageDetails{}, err
	}
	det.Manager = pkgmgr.ManagerWinget
	media := pkgmgr.LookupIconMedia(pkgmgr.ManagerWinget, det.ID, det.Name)
	if det.IconURL == "" {
		det.IconURL = media.Icon
	}
	if len(det.Screenshots) == 0 && len(media.Screenshots) > 0 {
		det.Screenshots = media.Screenshots
	}
	if det.InstallerSize > 0 {
		a.cacheResolvedSize("__ess:"+strings.ToLower(strings.TrimSpace(wingetID)), det.InstallerSize, det.InstallerURL)
	}
	return det, nil
}

// GetUniversalPackageDetails resolves rich metadata, icons, and preview screenshots for any Tab 2 package manager
// (Winget, MS Store, Scoop, Chocolatey, npm, pnpm, Bun, uv).
func (a *App) GetUniversalPackageDetails(manager, pkgID, source string) (essentials.PackageDetails, error) {
	mgr := pkgmgr.NormalizeManager(manager)
	if mgr == pkgmgr.ManagerWinget {
		return a.GetWingetPackageDetails(pkgID, source)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return pkgmgr.GetPackageDetails(ctx, mgr, pkgID, source, a.getBaseDir())
}

// EnrichWingetSearch fetches publisher, description, homepage, license, icons, and screenshots in parallel
// for the top Winget search results so users can inspect exact app details before installing.
func (a *App) EnrichWingetSearch(rows []essentials.WingetTableRow) []essentials.PackageDetails {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	details := essentials.EnrichSearchRows(ctx, rows, 12)
	for i := range details {
		details[i].Manager = pkgmgr.ManagerWinget
		if details[i].ID != "" {
			media := pkgmgr.LookupIconMedia(pkgmgr.ManagerWinget, details[i].ID, details[i].Name)
			if details[i].IconURL == "" {
				details[i].IconURL = media.Icon
			}
			if len(details[i].Screenshots) == 0 && len(media.Screenshots) > 0 {
				details[i].Screenshots = media.Screenshots
			}
			if details[i].InstallerSize > 0 {
				a.cacheResolvedSize("__ess:"+strings.ToLower(strings.TrimSpace(details[i].ID)), details[i].InstallerSize, details[i].InstallerURL)
			}
		}
	}
	return details
}

// OpenExternalURL safely opens an http(s) homepage or publisher URL in the default Windows browser.
func (a *App) OpenExternalURL(rawURL string) error {
	trimmed := strings.TrimSpace(rawURL)
	u, err := url.Parse(trimmed)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return a.biErr("invalid external URL", "URL eksternal tidak valid")
	}
	runtime.BrowserOpenURL(a.ctx, trimmed)
	return nil
}

func (a *App) resolvedExePath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, rErr := filepath.EvalSymlinks(exe); rErr == nil && strings.TrimSpace(resolved) != "" {
		return resolved, nil
	}
	return exe, nil
}

func (a *App) refreshAppUpdate(ctx context.Context, force bool) selfupdate.UpdateInfo {
	if ctx == nil {
		ctx = context.Background()
	}
	a.appUpdateMu.RLock()
	cachePath := a.appUpdatePath
	prev := a.appUpdateInfo
	a.appUpdateMu.RUnlock()

	if !force {
		if cached, fresh := selfupdate.LoadCachedInfo(cachePath, AppVersion, selfupdate.DefaultCheckCooldown); fresh {
			a.appUpdateMu.Lock()
			a.appUpdateInfo = cached
			a.appUpdateMu.Unlock()
			return cached
		}
	}

	info, err := selfupdate.CheckLatestRelease(ctx, AppVersion)
	if err != nil {
		if prev.LatestVersion != "" && !force {
			prev.CurrentVersion = selfupdate.NormalizeVersion(AppVersion)
			prev.HasUpdate = selfupdate.IsNewerVersion(prev.LatestVersion, AppVersion)
			info = prev
		} else {
			info.CurrentVersion = selfupdate.NormalizeVersion(AppVersion)
			info.CheckError = err.Error()
		}
	} else {
		_ = selfupdate.SaveCachedInfo(cachePath, info)
	}

	a.appUpdateMu.Lock()
	a.appUpdateInfo = info
	a.appUpdateMu.Unlock()
	return info
}

// CheckAppUpdate queries GitHub Releases for a newer UniDevUI release without using the startup cache cooldown.
func (a *App) CheckAppUpdate() (DashboardState, error) {
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	info := a.refreshAppUpdate(ctx, true)
	if info.CheckError != "" {
		a.emitBiLog("WARN",
			fmt.Sprintf("Failed checking UniDevUI release update: %s", info.CheckError),
			fmt.Sprintf("Gagal memeriksa pembaruan UniDevUI: %s", info.CheckError),
		)
	} else if info.HasUpdate {
		a.emitBiLog("OK",
			fmt.Sprintf("UniDevUI update found: v%s -> v%s (%s)", info.CurrentVersion, info.LatestVersion, info.AssetName),
			fmt.Sprintf("Update UniDevUI ditemukan: v%s -> v%s (%s)", info.CurrentVersion, info.LatestVersion, info.AssetName),
		)
	} else {
		a.emitBiLog("OK",
			fmt.Sprintf("UniDevUI v%s is up to date.", info.CurrentVersion),
			fmt.Sprintf("UniDevUI v%s sudah versi terbaru.", info.CurrentVersion),
		)
	}
	return a.GetDashboardState()
}

// ApplyAppUpdate downloads the latest UniDevUI.exe release, verifies SHA-256, swaps binaries, and restarts the app.
func (a *App) ApplyAppUpdate() error {
	exePath, err := a.resolvedExePath()
	if err != nil || strings.TrimSpace(exePath) == "" {
		return a.biErr("cannot locate running executable path", "tidak dapat menemukan path executable yang sedang berjalan")
	}

	ctx, err := a.beginTask()
	if err != nil {
		return err
	}

	go func() {
		defer a.endTask()
		a.emitBiLog("INFO",
			fmt.Sprintf("Checking UniDevUI release metadata for update (current v%s)...", selfupdate.NormalizeVersion(AppVersion)),
			fmt.Sprintf("Memeriksa metadata rilis UniDevUI untuk update (saat ini v%s)...", selfupdate.NormalizeVersion(AppVersion)),
		)

		info := a.refreshAppUpdate(ctx, true)
		if info.CheckError != "" {
			a.emitBiLog("ERROR",
				fmt.Sprintf("App update check failed: %s", info.CheckError),
				fmt.Sprintf("Pemeriksaan update aplikasi gagal: %s", info.CheckError),
			)
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "state:refresh")
			}
			return
		}
		if !info.HasUpdate {
			a.emitBiLog("OK",
				fmt.Sprintf("UniDevUI v%s is already up to date.", info.CurrentVersion),
				fmt.Sprintf("UniDevUI v%s sudah menggunakan versi terbaru.", info.CurrentVersion),
			)
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "state:refresh")
			}
			return
		}
		if strings.TrimSpace(info.DownloadURL) == "" {
			a.emitBiLog("WARN",
				fmt.Sprintf("Release v%s does not expose a UniDevUI.exe asset; please download from %s", info.LatestVersion, info.ReleaseURL),
				fmt.Sprintf("Rilis v%s tidak memiliki aset UniDevUI.exe; silakan unduh dari %s", info.LatestVersion, info.ReleaseURL),
			)
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "state:refresh")
			}
			return
		}

		a.emitBiLog("INFO",
			fmt.Sprintf("Downloading UniDevUI v%s (%s)...", info.LatestVersion, info.DownloadURL),
			fmt.Sprintf("Mengunduh UniDevUI v%s (%s)...", info.LatestVersion, info.DownloadURL),
		)

		backupPath, swapErr := selfupdate.DownloadVerifyAndSwap(ctx, info, exePath, a.emitProgress, a.emitBiLog)
		if swapErr != nil {
			a.emitBiLog("ERROR",
				fmt.Sprintf("UniDevUI self-update failed: %v", swapErr),
				fmt.Sprintf("Gagal memperbarui UniDevUI: %v", swapErr),
			)
			a.emitProgress(archive.ProgressEvent{Stage: "idle", Percent: 0})
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "state:refresh")
			}
			return
		}

		a.emitBiLog("OK",
			fmt.Sprintf("Verified SHA-256 and updated binary to v%s (backup: %s). Restarting...", info.LatestVersion, filepath.Base(backupPath)),
			fmt.Sprintf("SHA-256 terverifikasi dan biner diperbarui ke v%s (cadangan: %s). Memulai ulang...", info.LatestVersion, filepath.Base(backupPath)),
		)

		if startErr := selfupdate.StartDetachedProcess(exePath); startErr != nil {
			_ = os.Rename(backupPath, exePath)
			a.emitBiLog("ERROR",
				fmt.Sprintf("Failed restarting updated executable (rolled back): %v", startErr),
				fmt.Sprintf("Gagal menjalankan ulang executable baru (dikembalikan ke semula): %v", startErr),
			)
			a.emitProgress(archive.ProgressEvent{Stage: "idle", Percent: 0})
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "state:refresh")
			}
			return
		}

		time.Sleep(250 * time.Millisecond)
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "task:busy", false)
			runtime.Quit(a.ctx)
		}
	}()

	return nil
}

func (a *App) runWingetByIDAction(name, wingetID, source, mode string) error {
	trimmedID := strings.TrimSpace(wingetID)
	if trimmedID == "" {
		return a.biErr("empty winget ID", "winget ID kosong")
	}
	ctx, err := a.beginTask()
	if err != nil {
		return err
	}
	display := strings.TrimSpace(name)
	if display == "" {
		display = trimmedID
	}
	app := essentials.EssentialApp{
		ID:           trimmedID,
		Name:         display,
		WingetID:     trimmedID,
		Source:       source,
		ExpectedSize: a.getCachedURLSize("__ess:" + strings.ToLower(trimmedID)),
	}
	go func() {
		defer a.endTask()
		verbEN, verbID := "Silent installing", "Menginstal silent"
		pastEN, pastID := "Installed", "Berhasil menginstal"
		detailVerb := "Installing"
		if mode == "update" {
			verbEN, verbID = "Updating", "Memperbarui"
			pastEN, pastID = "Updated", "Berhasil memperbarui"
			detailVerb = "Updating"
		} else if mode == "uninstall" {
			verbEN, verbID = "Uninstalling", "Menghapus"
			pastEN, pastID = "Uninstalled", "Berhasil menghapus"
			detailVerb = "Uninstalling"
		}
		a.emitBiLog("INFO",
			fmt.Sprintf("%s %s (%s)...", verbEN, display, trimmedID),
			fmt.Sprintf("%s %s (%s)...", verbID, display, trimmedID),
		)
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "install", Percent: -1, Detail: fmt.Sprintf("%s %s...", detailVerb, display)})
		if err := essentials.RunEssentialAction(ctx, app, mode, "latest", a.emitBiLog, a.emitProgress); err != nil {
			a.emitBiLog("ERROR",
				fmt.Sprintf("Failed %s %s: %v", strings.ToLower(verbEN), display, err),
				fmt.Sprintf("Gagal memproses %s %s: %v", mode, display, err),
			)
		} else {
			a.refreshWingetCache(ctx)
			a.emitBiLog("OK",
				fmt.Sprintf("%s %s (%s).", pastEN, display, trimmedID),
				fmt.Sprintf("%s %s (%s).", pastID, display, trimmedID),
			)
		}
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()
	return nil
}

// InstallWingetByID silent-installs any Winget search result directly without needing it in essentials.conf.
func (a *App) InstallWingetByID(name, wingetID, source string) error {
	return a.runWingetByIDAction(name, wingetID, source, "install")
}

// UpdateWingetByID updates an installed Winget application with live ConPTY download progress.
func (a *App) UpdateWingetByID(name, wingetID, source string) error {
	return a.runWingetByIDAction(name, wingetID, source, "update")
}

// RunPackageAction executes install, update, or uninstall for any Tab 2 package manager (winget, scoop, choco, npm, pnpm, bun, uv).
func (a *App) RunPackageAction(manager, pkgID, name, source, mode string) error {
	mgr := pkgmgr.NormalizeManager(manager)
	if mgr == pkgmgr.ManagerWinget {
		return a.runWingetByIDAction(name, pkgID, source, mode)
	}
	if !pkgmgr.IsSupportedExtraManager(mgr) {
		return a.biErr(
			fmt.Sprintf("unsupported package manager: %s", manager),
			fmt.Sprintf("package manager tidak didukung: %s", manager),
		)
	}
	trimmedID := strings.TrimSpace(pkgID)
	if trimmedID == "" {
		return a.biErr("empty package ID", "ID paket kosong")
	}
	ctx, err := a.beginTask()
	if err != nil {
		return err
	}
	display := strings.TrimSpace(name)
	if display == "" {
		display = trimmedID
	}
	baseDir := a.getBaseDir()

	go func() {
		defer a.endTask()
		verb := strings.ToLower(strings.TrimSpace(mode))
		if verb == "" {
			verb = "install"
		}
		a.emitBiLog("INFO",
			fmt.Sprintf("[%s] %s %s...", mgr, verb, display),
			fmt.Sprintf("[%s] menjalankan %s %s...", mgr, verb, display),
		)
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{
			Stage:   "install",
			Percent: -1,
			Detail:  fmt.Sprintf("[%s] %s %s...", mgr, verb, display),
		})
		if err := pkgmgr.RunAction(ctx, mgr, trimmedID, source, verb, baseDir, a.emitBiLog); err != nil {
			a.emitBiLog("ERROR",
				fmt.Sprintf("[%s] %s %s failed: %v", mgr, verb, display, err),
				fmt.Sprintf("[%s] %s %s gagal: %v", mgr, verb, display, err),
			)
		} else {
			tools.InvalidateStatusCache()
			a.refreshWingetCache(context.Background())
			a.emitBiLog("OK",
				fmt.Sprintf("[%s] %s %s completed.", mgr, verb, display),
				fmt.Sprintf("[%s] %s %s selesai.", mgr, verb, display),
			)
		}
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()
	return nil
}

// UpdateAllInstalled updates a list of installed apps/packages across Winget and extra managers sequentially while respecting ignored_updates.conf.
func (a *App) UpdateAllInstalled(targets []WingetUpdateTarget) error {
	if len(targets) == 0 {
		return a.biErr("no updates queued", "tidak ada aplikasi untuk diperbarui")
	}
	ignConfPath, _ := essentials.EnsureIgnoredConfig(a.appDir)
	_, ignoredMap, _ := essentials.ParseIgnoredFile(ignConfPath)

	queue := make([]WingetUpdateTarget, 0, len(targets))
	for _, t := range targets {
		id := strings.TrimSpace(t.WingetID)
		if id == "" {
			continue
		}
		mgr := pkgmgr.NormalizeManager(t.Manager)
		if mgr == pkgmgr.ManagerWinget && !essentials.IsManageableWingetID(id) {
			continue
		}
		if mgr != pkgmgr.ManagerWinget && !pkgmgr.IsSupportedExtraManager(mgr) {
			continue
		}
		ignKey := strings.ToLower(pkgmgr.IgnoreKey(mgr, id))
		ignEntry, hasIgn := ignoredMap[ignKey]
		if !hasIgn && mgr == pkgmgr.ManagerWinget {
			ignEntry, hasIgn = ignoredMap[strings.ToLower(id)]
		}
		if fullIgn, verSkipped := essentials.EvaluateIgnoredPolicy(ignEntry, hasIgn, t.AvailableVer); fullIgn || verSkipped {
			continue
		}
		t.Manager = mgr
		queue = append(queue, t)
	}
	if len(queue) == 0 {
		return a.biErr("no updatable non-ignored apps found", "tidak ada aplikasi non-ignored yang bisa diperbarui")
	}

	ctx, err := a.beginTask()
	if err != nil {
		return err
	}
	baseDir := a.getBaseDir()

	go func() {
		defer a.endTask()
		total := len(queue)
		succeeded, failed := 0, 0
		a.emitBiLog("INFO",
			fmt.Sprintf("Starting sequential update for %d packages...", total),
			fmt.Sprintf("Memulai pembaruan berurutan untuk %d paket...", total),
		)

		for idx, item := range queue {
			if ctx.Err() != nil {
				a.emitBiLog("WARN", "Batch update cancelled.", "Pembaruan massal dibatalkan.")
				break
			}
			display := strings.TrimSpace(item.Name)
			if display == "" {
				display = item.WingetID
			}
			batchDetail := fmt.Sprintf("[%d/%d] %s", idx+1, total, display)
			pct := (float64(idx) / float64(total)) * 100.0
			runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{
				Stage:   "install",
				Percent: pct,
				Detail:  fmt.Sprintf("%s...", batchDetail),
			})
			a.emitBiLog("INFO",
				fmt.Sprintf("[%d/%d] [%s] Updating %s (%s)...", idx+1, total, item.Manager, display, item.WingetID),
				fmt.Sprintf("[%d/%d] [%s] Memperbarui %s (%s)...", idx+1, total, item.Manager, display, item.WingetID),
			)

			var updateErr error
			if item.Manager == pkgmgr.ManagerWinget {
				app := essentials.EssentialApp{
					ID:           item.WingetID,
					Name:         display,
					WingetID:     item.WingetID,
					Source:       item.Source,
					ExpectedSize: a.getCachedURLSize("__ess:" + strings.ToLower(strings.TrimSpace(item.WingetID))),
				}
				batchProgress := func(ev archive.ProgressEvent) {
					ev.Detail = batchDetail
					a.emitProgress(ev)
				}
				updateErr = essentials.RunEssentialAction(ctx, app, "update", "latest", a.emitBiLog, batchProgress)
			} else {
				updateErr = pkgmgr.RunAction(ctx, item.Manager, item.WingetID, item.Source, "update", baseDir, a.emitBiLog)
			}

			if updateErr != nil {
				failed++
				a.emitBiLog("ERROR",
					fmt.Sprintf("[%d/%d] Failed updating %s: %v", idx+1, total, display, updateErr),
					fmt.Sprintf("[%d/%d] Gagal memperbarui %s: %v", idx+1, total, display, updateErr),
				)
			} else {
				succeeded++
				a.emitBiLog("OK",
					fmt.Sprintf("[%d/%d] Updated %s.", idx+1, total, display),
					fmt.Sprintf("[%d/%d] Berhasil memperbarui %s.", idx+1, total, display),
				)
			}
		}

		tools.InvalidateStatusCache()
		a.refreshWingetCache(context.Background())
		a.emitBiLog("OK",
			fmt.Sprintf("Batch update finished: %d succeeded, %d failed.", succeeded, failed),
			fmt.Sprintf("Pembaruan massal selesai: %d berhasil, %d gagal.", succeeded, failed),
		)
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()
	return nil
}

// SkipInstalledAppVersion skips only one specific available version (e.g. "2.5.0") while allowing future updates to notify again.
func (a *App) SkipInstalledAppVersion(manager, id, name, source, skipVersion string) (DashboardState, error) {
	mgr := pkgmgr.NormalizeManager(manager)
	trimmedID := strings.TrimSpace(id)
	cleanVer := strings.TrimPrefix(strings.TrimSpace(skipVersion), "v")
	if trimmedID == "" || cleanVer == "" {
		return DashboardState{}, a.biErr("empty app ID or version", "ID aplikasi atau versi kosong")
	}
	ignConfPath, err := essentials.EnsureIgnoredConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}

	display := strings.TrimSpace(name)
	if display == "" {
		display = trimmedID
	}
	storeID := pkgmgr.IgnoreKey(mgr, trimmedID)
	storeSource := strings.TrimSpace(source)
	if storeSource == "" || (mgr != pkgmgr.ManagerWinget && !strings.EqualFold(storeSource, mgr)) {
		storeSource = mgr
	}
	if err := essentials.AddIgnoredApp(ignConfPath, essentials.IgnoredApp{
		ID:             storeID,
		Name:           display,
		Source:         storeSource,
		WingetPinned:   false,
		SkippedVersion: cleanVer,
		Reason:         fmt.Sprintf("Skipped v%s via UniDevUI", cleanVer),
	}); err != nil {
		return DashboardState{}, err
	}
	a.markIgnConfigSelfWritten(ignConfPath)
	a.emitBiLog("OK",
		fmt.Sprintf("Skipped v%s for %s (%s); newer versions will notify normally.", cleanVer, display, storeID),
		fmt.Sprintf("Melewati v%s untuk %s (%s); versi lebih baru tetap akan diberitahukan.", cleanVer, display, storeID),
	)
	return a.GetDashboardState()
}

// IgnoreInstalledApp excludes an app or CLI package from updates & active Installed lists, and pins it in Windows Winget if applicable.
func (a *App) IgnoreInstalledApp(manager, id, name, source, reason string) (DashboardState, error) {
	mgr := pkgmgr.NormalizeManager(manager)
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return DashboardState{}, a.biErr("empty app ID", "ID aplikasi kosong")
	}
	ignConfPath, err := essentials.EnsureIgnoredConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}

	display := strings.TrimSpace(name)
	if display == "" {
		display = trimmedID
	}
	if strings.TrimSpace(reason) == "" {
		reason = "Ignored via UniDevUI"
	}

	pinned := false
	if mgr == pkgmgr.ManagerWinget && essentials.IsManageableWingetID(trimmedID) {
		pinCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		pinErr := essentials.WingetPinAdd(pinCtx, trimmedID, source, a.emitBiLog)
		cancel()
		if pinErr != nil {
			a.emitBiLog("WARN",
				fmt.Sprintf("Saved %s to ignored_updates.conf, but Windows Winget pin returned: %v", display, pinErr),
				fmt.Sprintf("%s disimpan ke ignored_updates.conf, tetapi winget pin mengembalikan: %v", display, pinErr),
			)
		} else {
			pinned = true
		}
	}

	storeID := pkgmgr.IgnoreKey(mgr, trimmedID)
	storeSource := strings.TrimSpace(source)
	if storeSource == "" || (mgr != pkgmgr.ManagerWinget && !strings.EqualFold(storeSource, mgr)) {
		storeSource = mgr
	}
	if err := essentials.AddIgnoredApp(ignConfPath, essentials.IgnoredApp{
		ID:           storeID,
		Name:         display,
		Source:       storeSource,
		WingetPinned: pinned,
		Reason:       strings.TrimSpace(reason),
	}); err != nil {
		return DashboardState{}, err
	}
	a.markIgnConfigSelfWritten(ignConfPath)

	if pinned {
		a.emitBiLog("OK",
			fmt.Sprintf("Ignored %s (%s) in UniDevUI & pinned blocking in Windows Winget.", display, trimmedID),
			fmt.Sprintf("Mengabaikan %s (%s) di UniDevUI & mengunci pin Winget di Windows.", display, trimmedID),
		)
	} else {
		a.emitBiLog("OK",
			fmt.Sprintf("Ignored %s (%s) in ignored_updates.conf.", display, storeID),
			fmt.Sprintf("Mengabaikan %s (%s) di ignored_updates.conf.", display, storeID),
		)
	}
	return a.GetDashboardState()
}

// PauseInstalledAppUpdates suppresses update notifications for an app/package until N days from today (default 7).
func (a *App) PauseInstalledAppUpdates(manager, id, name, source string, days int) (DashboardState, error) {
	mgr := pkgmgr.NormalizeManager(manager)
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return DashboardState{}, a.biErr("empty app ID", "ID aplikasi kosong")
	}
	if days <= 0 {
		days = 7
	} else if days > 365 {
		days = 365
	}
	ignConfPath, err := essentials.EnsureIgnoredConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}

	display := strings.TrimSpace(name)
	if display == "" {
		display = trimmedID
	}
	storeID := pkgmgr.IgnoreKey(mgr, trimmedID)
	storeSource := strings.TrimSpace(source)
	if storeSource == "" || (mgr != pkgmgr.ManagerWinget && !strings.EqualFold(storeSource, mgr)) {
		storeSource = mgr
	}
	untilDate := time.Now().AddDate(0, 0, days).Format("2006-01-02")
	if err := essentials.AddIgnoredApp(ignConfPath, essentials.IgnoredApp{
		ID:           storeID,
		Name:         display,
		Source:       storeSource,
		WingetPinned: false,
		PauseUntil:   untilDate,
		Reason:       fmt.Sprintf("Paused updates until %s via UniDevUI", untilDate),
	}); err != nil {
		return DashboardState{}, err
	}
	a.markIgnConfigSelfWritten(ignConfPath)
	a.emitBiLog("OK",
		fmt.Sprintf("Paused update alerts for %s (%s) until %s (%d days).", display, storeID, untilDate, days),
		fmt.Sprintf("Menunda notifikasi update untuk %s (%s) hingga %s (%d hari).", display, storeID, untilDate, days),
	)
	return a.GetDashboardState()
}

// UnignoreInstalledApp restores an ignored app/package to active Installed & Updates tracking and removes any Windows Winget pin.
func (a *App) UnignoreInstalledApp(manager, id string) (DashboardState, error) {
	mgr := pkgmgr.NormalizeManager(manager)
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return DashboardState{}, a.biErr("empty app ID", "ID aplikasi kosong")
	}
	ignConfPath, err := essentials.EnsureIgnoredConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}

	storeID := pkgmgr.IgnoreKey(mgr, trimmedID)
	removed, found, err := essentials.RemoveIgnoredApp(ignConfPath, storeID)
	if err != nil {
		return DashboardState{}, err
	}
	if !found && storeID != trimmedID {
		if r2, f2, err2 := essentials.RemoveIgnoredApp(ignConfPath, trimmedID); err2 == nil && f2 {
			removed, found = r2, true
		}
	}
	a.markIgnConfigSelfWritten(ignConfPath)

	display := removed.Name
	if display == "" {
		display = trimmedID
	}

	if mgr == pkgmgr.ManagerWinget && essentials.IsManageableWingetID(trimmedID) {
		pinCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		pinErr := essentials.WingetPinRemove(pinCtx, trimmedID, removed.Source, a.emitBiLog)
		cancel()
		if pinErr != nil {
			a.emitBiLog("WARN",
				fmt.Sprintf("Removed %s from ignored_updates.conf, but Windows Winget unpin returned: %v", display, pinErr),
				fmt.Sprintf("%s dihapus dari ignored_updates.conf, tetapi winget pin remove mengembalikan: %v", display, pinErr),
			)
		}
	}

	if found {
		a.emitBiLog("OK",
			fmt.Sprintf("Restored %s (%s) to active Installed & Updates lists.", display, trimmedID),
			fmt.Sprintf("Mengembalikan %s (%s) ke daftar Installed & Updates aktif.", display, trimmedID),
		)
	}
	return a.GetDashboardState()
}

// PinWingetApp appends a Winget search result to essentials.conf so it persists in the Windows Apps list.
func (a *App) PinWingetApp(name, wingetID, source, category string) (DashboardState, error) {
	essConfPath, err := essentials.EnsureConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}
	if category == "" {
		category = "utilities"
	}
	if source == "" {
		source = "winget"
	}
	desc := fmt.Sprintf("Added via Winget Search (%s)", wingetID)
	dCtx, dCancel := context.WithTimeout(context.Background(), 5*time.Second)
	if det, dErr := essentials.ShowPackageDetails(dCtx, wingetID, source, false); dErr == nil {
		if singleLine := singleLineSummary(det.Description, 140); singleLine != "" {
			if det.Publisher != "" {
				desc = fmt.Sprintf("%s — %s", det.Publisher, singleLine)
			} else {
				desc = singleLine
			}
		} else if det.Publisher != "" {
			desc = fmt.Sprintf("Published by %s (%s)", det.Publisher, wingetID)
		}
	}
	dCancel()

	err = essentials.AppendCustomApp(essConfPath, essentials.EssentialApp{
		Name:        name,
		Category:    category,
		WingetID:    wingetID,
		Source:      source,
		Description: desc,
	})
	if err != nil {
		return DashboardState{}, err
	}
	a.markEssConfigSelfWritten(essConfPath)
	a.emitBiLog("OK",
		fmt.Sprintf("Pinned %s (%s) to essentials.conf.", name, wingetID),
		fmt.Sprintf("Menambahkan %s (%s) ke essentials.conf.", name, wingetID),
	)
	return a.GetDashboardState()
}

func singleLineSummary(raw string, maxRunes int) string {
	joined := strings.Join(strings.Fields(raw), " ")
	if joined == "" {
		return ""
	}
	r := []rune(joined)
	if len(r) <= maxRunes {
		return joined
	}
	return strings.TrimSpace(string(r[:maxRunes-1])) + "…"
}

func (a *App) openInNotepad(confPath string) error {
	cmdStr := fmt.Sprintf("notepad.exe %s", confPath)
	a.emitBiLog("CMD", cmdStr, cmdStr)
	cmd := exec.Command("notepad.exe", confPath)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to open Notepad: %w", err)
	}
	return nil
}

// OpenEssentialsConf launches Notepad to edit essentials.conf next to UniDevUI.exe.
func (a *App) OpenEssentialsConf() error {
	confPath, err := essentials.EnsureConfig(a.appDir)
	if err != nil {
		return err
	}
	return a.openInNotepad(confPath)
}

// OpenIgnoredConf launches Notepad to edit ignored_updates.conf next to UniDevUI.exe.
func (a *App) OpenIgnoredConf() error {
	confPath, err := essentials.EnsureIgnoredConfig(a.appDir)
	if err != nil {
		return err
	}
	return a.openInNotepad(confPath)
}

// SetLanguage updates the persisted UI language ("en" or "id").
func (a *App) SetLanguage(lang string) (DashboardState, error) {
	norm := "en"
	if strings.EqualFold(strings.TrimSpace(lang), "id") {
		norm = "id"
	}
	a.dirMu.Lock()
	a.language = norm
	s := a.settings
	if strings.TrimSpace(s.BaseDir) == "" {
		s.BaseDir = a.baseDir
	}
	s.Language = norm
	s = config.NormalizeSettings(s)
	a.settings = s
	a.dirMu.Unlock()

	if err := config.SaveSettings(s); err != nil {
		a.emitBiLog("WARN",
			fmt.Sprintf("Failed saving language preference: %v", err),
			fmt.Sprintf("Gagal menyimpan preferensi bahasa: %v", err),
		)
	}
	a.syncTrayAndPowerState()
	return a.GetDashboardState()
}

func populateVersionComparison(ctx context.Context, baseDir string, views []PackageView) {
	if ctx == nil {
		ctx = context.Background()
	}

	var wg sync.WaitGroup
	for i := range views {
		idx := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			pv := &views[idx]

			for _, v := range pv.Versions {
				if pv.LatestVer == "" || pathmgr.CompareSemver(v.Version, pv.LatestVer) > 0 {
					pv.LatestVer = v.Version
				}
			}

			if pv.Binary != "" {
				scope, fullPath := pathmgr.ResolveEffectiveBinary(pv.Binary)
				if fullPath != "" {
					pv.WinActivePath = fullPath
					appPrefix := strings.ToLower(filepath.Clean(filepath.Join(baseDir, pv.ID))) + `\`
					if strings.HasPrefix(strings.ToLower(filepath.Clean(fullPath)), appPrefix) {
						pv.WinActiveScope = "UniDevUI (User PATH)"
						if pv.ActiveVer != "" {
							pv.WinActiveVer = pv.ActiveVer
						} else {
							pv.WinActiveVer = pathmgr.ProbeBinaryVersion(ctx, pv.ID, fullPath)
						}
					} else {
						pv.WinActiveScope = scope
						pv.WinActiveVer = pathmgr.ProbeBinaryVersion(ctx, pv.ID, fullPath)
					}
				}
			}

			if pv.WinActiveVer != "" {
				branch := versionBranchPrefix(pv.ID, pv.WinActiveVer)
				if branch != "" {
					pv.BranchPrefix = branch
					for _, v := range pv.Versions {
						if versionBranchPrefix(pv.ID, v.Version) == branch {
							if pv.BranchLatestVer == "" || pathmgr.CompareSemver(v.Version, pv.BranchLatestVer) > 0 {
								pv.BranchLatestVer = v.Version
							}
						}
					}
					if pv.BranchLatestVer != "" {
						cmpBranch := pathmgr.CompareSemver(pv.WinActiveVer, pv.BranchLatestVer)
						switch {
						case cmpBranch == 0:
							pv.BranchStatus = "uptodate"
						case cmpBranch < 0:
							pv.BranchStatus = "outdated"
						default:
							pv.BranchStatus = "ahead"
						}
					}
				}
			}

			switch {
			case pv.WinActivePath == "":
				pv.VersionStatus = "not_installed"
			case pv.WinActiveVer == "" || pv.LatestVer == "":
				pv.VersionStatus = "unknown"
			default:
				cmp := pathmgr.CompareSemver(pv.WinActiveVer, pv.LatestVer)
				switch {
				case cmp == 0:
					pv.VersionStatus = "uptodate"
				case cmp < 0:
					pv.VersionStatus = "outdated"
				default:
					pv.VersionStatus = "ahead"
				}
			}
		}()
	}
	wg.Wait()
}

func versionBranchPrefix(appID, ver string) string {
	clean := strings.TrimSpace(strings.TrimPrefix(strings.ToLower(ver), "v"))
	if clean == "" {
		return ""
	}
	if dash := strings.IndexAny(clean, "-+"); dash >= 0 {
		clean = clean[:dash]
	}
	parts := strings.Split(clean, ".")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	switch strings.ToLower(appID) {
	case "php", "go", "composer", "python":
		if len(parts) >= 2 && parts[1] != "" {
			return parts[0] + "." + parts[1]
		}
	}
	return parts[0]
}

// PickOutputDirectory opens a native Windows folder selection dialog.
func (a *App) PickOutputDirectory() (string, error) {
	title := "Select UniDevUI Extraction Output Directory"
	if a.getLanguage() == "id" {
		title = "Pilih Folder Output Ekstraksi UniDevUI"
	}
	selected, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: a.getBaseDir(),
	})
	if err != nil {
		return "", err
	}
	return selected, nil
}

// SetOutputDir changes the archive extraction directory, persists it, and refreshes state.
// Note: packages.conf and essentials.conf always remain beside UniDevUI.exe.
func (a *App) SetOutputDir(newDir string) (DashboardState, error) {
	a.taskMu.Lock()
	isBusy := a.busy
	a.taskMu.Unlock()
	if isBusy {
		return DashboardState{}, a.biErr(
			"cannot change output directory while another task is running",
			"tidak dapat mengubah folder output saat proses lain sedang berjalan",
		)
	}

	trimmed := strings.TrimSpace(strings.Trim(newDir, `"`))
	if trimmed == "" {
		return DashboardState{}, a.biErr(
			"output folder path cannot be empty",
			"path folder output tidak boleh kosong",
		)
	}
	cleanNew := filepath.Clean(trimmed)
	if !filepath.IsAbs(cleanNew) {
		return DashboardState{}, a.biErr(
			"output folder path must be an absolute path (e.g. D:\\AppPath)",
			"path folder output harus berupa path absolut (misal D:\\AppPath)",
		)
	}

	if err := os.MkdirAll(cleanNew, 0o755); err != nil {
		return DashboardState{}, fmt.Errorf("failed to create %s: %w", cleanNew, err)
	}

	a.dirMu.Lock()
	a.baseDir = cleanNew
	s := a.settings
	s.BaseDir = cleanNew
	s.Language = a.language
	s = config.NormalizeSettings(s)
	a.settings = s
	a.dirMu.Unlock()

	if err := config.SaveSettings(s); err != nil {
		a.emitBiLog("WARN",
			fmt.Sprintf("Directory changed to %s, but failed saving settings.json: %v", cleanNew, err),
			fmt.Sprintf("Folder diubah ke %s, namun gagal menyimpan settings.json: %v", cleanNew, err),
		)
	}

	a.emitBiLog("OK",
		fmt.Sprintf("Output path changed to: %s (configs remain in %s)", cleanNew, a.appDir),
		fmt.Sprintf("Output path diubah ke: %s (config tetap di %s)", cleanNew, a.appDir),
	)
	return a.GetDashboardState()
}

// SaveAppSettings updates all user preferences (output folder, language, close/tray behavior,
// Windows autostart, and background update monitor) and synchronizes Windows Registry Run key.
func (a *App) SaveAppSettings(in config.AppSettings) (DashboardState, error) {
	a.taskMu.Lock()
	isBusy := a.busy
	a.taskMu.Unlock()

	norm := config.NormalizeSettings(in)
	cur := a.getSettings()
	if norm.LastCheckUnix == 0 && cur.LastCheckUnix > 0 {
		norm.LastCheckUnix = cur.LastCheckUnix
	}
	baseChanged := !strings.EqualFold(filepath.Clean(norm.BaseDir), filepath.Clean(cur.BaseDir))
	if baseChanged {
		if isBusy {
			return DashboardState{}, a.biErr(
				"cannot change output directory while another task is running",
				"tidak dapat mengubah folder output saat proses lain sedang berjalan",
			)
		}
		if !filepath.IsAbs(norm.BaseDir) {
			return DashboardState{}, a.biErr(
				"output folder path must be an absolute path (e.g. D:\\AppPath)",
				"path folder output harus berupa path absolut (misal D:\\AppPath)",
			)
		}
		if err := os.MkdirAll(norm.BaseDir, 0o755); err != nil {
			return DashboardState{}, fmt.Errorf("failed to create %s: %w", norm.BaseDir, err)
		}
	}

	if err := autostart.Sync(norm.AutoStart, norm.StartInTray); err != nil {
		return DashboardState{}, a.biErr(
			fmt.Sprintf("failed updating Windows startup entry: %v", err),
			fmt.Sprintf("gagal mengatur startup Windows: %v", err),
		)
	}

	a.dirMu.Lock()
	a.settings = norm
	a.baseDir = norm.BaseDir
	a.language = norm.Language
	a.dirMu.Unlock()

	if err := config.SaveSettings(norm); err != nil {
		return DashboardState{}, err
	}
	a.syncTrayAndPowerState()

	a.emitBiLog("OK",
		"Settings saved successfully.",
		"Pengaturan berhasil disimpan.",
	)
	return a.GetDashboardState()
}

// MinimizeWindow minimizes the UniDevUI window and engages Efficiency Mode when idle.
func (a *App) MinimizeWindow() {
	if a.ctx != nil {
		runtime.WindowMinimise(a.ctx)
	}
	time.AfterFunc(150*time.Millisecond, a.syncTrayAndPowerState)
}

// SyncPowerState re-evaluates EcoQoS and tray state after window focus or restore transitions.
func (a *App) SyncPowerState() {
	a.syncTrayAndPowerState()
}

// HideToTray hides the main UniDevUI window into the Windows System Tray.
func (a *App) HideToTray() {
	a.windowVisible.Store(false)
	if a.ctx != nil {
		runtime.WindowHide(a.ctx)
	}
	debug.FreeOSMemory()
	a.syncTrayAndPowerState()
}

// ShowWindow restores and focuses the UniDevUI window from the System Tray.
func (a *App) ShowWindow() {
	a.windowVisible.Store(true)
	a.hasTaskError.Store(false)
	if a.ctx != nil {
		runtime.WindowUnminimise(a.ctx)
		runtime.WindowShow(a.ctx)
	}
	a.syncTrayAndPowerState()
	if !a.hasWingetCache() {
		go a.ensureInitialCacheLoaded()
	}
}

// QuitApp stops the tray icon cleanly and exits the UniDevUI application.
func (a *App) QuitApp() {
	a.allowQuit.Store(true)
	if a.trayMgr != nil {
		a.trayMgr.Stop()
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
}

// ConfirmCloseAction handles the user's choice from the close confirmation modal ("tray" or "quit")
// and optionally remembers that choice in settings.json.
func (a *App) ConfirmCloseAction(action string, remember bool) error {
	act := strings.ToLower(strings.TrimSpace(action))
	if act != "tray" && act != "quit" {
		return fmt.Errorf("unsupported close action: %s", action)
	}
	if remember {
		a.dirMu.Lock()
		s := a.settings
		s.BaseDir = a.baseDir
		s.Language = a.language
		s.CloseAction = act
		s = config.NormalizeSettings(s)
		a.settings = s
		a.dirMu.Unlock()
		_ = config.SaveSettings(s)
		a.requestStateRefresh()
	}
	if act == "tray" {
		a.HideToTray()
		return nil
	}
	a.QuitApp()
	return nil
}

func (a *App) handleBeforeClose(_ context.Context) bool {
	if a.allowQuit.Load() {
		if a.trayMgr != nil {
			a.trayMgr.Stop()
		}
		return false
	}
	s := a.getSettings()
	switch s.CloseAction {
	case "quit":
		a.allowQuit.Store(true)
		if a.trayMgr != nil {
			a.trayMgr.Stop()
		}
		return false
	case "tray":
		a.HideToTray()
		return true
	default:
		a.ShowWindow()
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "app:confirm-close")
		}
		return true
	}
}

func (a *App) toggleAutoStartFromTray() {
	s := a.getSettings()
	s.AutoStart = !s.AutoStart
	if s.AutoStart {
		// Windows startup launches silently into the System Tray unless opened explicitly.
		s.StartInTray = true
	}
	if _, err := a.SaveAppSettings(s); err == nil {
		a.requestStateRefresh()
	}
}

func (a *App) isWindowVisibleOnScreen() bool {
	if !a.windowVisible.Load() {
		return false
	}
	if a.ctx != nil && runtime.WindowIsMinimised(a.ctx) {
		return false
	}
	return true
}

func (a *App) syncTrayAndPowerState() {
	a.taskMu.Lock()
	busy := a.busy || a.scanBusy.Load()
	a.taskMu.Unlock()

	visible := a.isWindowVisibleOnScreen()
	// Engage Windows Efficiency Mode (EcoQoS) whenever hidden in Tray or minimized & idle.
	syspower.SetEfficiencyMode(!visible && !busy)

	if a.trayMgr == nil {
		return
	}
	s := a.getSettings()
	updCount := int(a.updateCount.Load())
	status := tray.StatusIdle
	tip := "UniDevUI"
	switch {
	case busy:
		status = tray.StatusBlue
		if s.Language == "id" {
			tip = "UniDevUI — Sedang memproses..."
		} else {
			tip = "UniDevUI — Task in progress..."
		}
	case a.hasTaskError.Load():
		status = tray.StatusOrange
		if s.Language == "id" {
			tip = "UniDevUI — Selesai dengan peringatan/error"
		} else {
			tip = "UniDevUI — Finished with warnings/errors"
		}
	case updCount > 0:
		status = tray.StatusGreen
		if s.Language == "id" {
			tip = fmt.Sprintf("UniDevUI — %d update tersedia", updCount)
		} else {
			tip = fmt.Sprintf("UniDevUI — %d update(s) available", updCount)
		}
	}
	a.trayMgr.Update(status, updCount, s.Language, s.AutoStart, tip)
}

func (a *App) persistLastCheckUnix(ts int64) {
	a.dirMu.Lock()
	s := a.settings
	if strings.TrimSpace(s.BaseDir) == "" {
		s.BaseDir = a.baseDir
	}
	if strings.TrimSpace(s.Language) == "" {
		s.Language = a.language
	}
	s.LastCheckUnix = ts
	s = config.NormalizeSettings(s)
	a.settings = s
	a.dirMu.Unlock()
	_ = config.SaveSettings(s)
}

// CheckUpdatesFromTray triggers a background scan immediately from the System Tray menu.
func (a *App) CheckUpdatesFromTray() {
	a.runBackgroundUpdateCycle(a.ctx, true)
}

// SyncDefaultPackages appends missing built-in entries to both packages.conf and essentials.conf without overwriting edits.
func (a *App) SyncDefaultPackages() (DashboardState, error) {
	pkgConf, err := config.EnsureConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}
	essConf, err := essentials.EnsureConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}

	addedPkg, err := config.SyncMissingDefaults(pkgConf)
	if err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return DashboardState{}, err
	}
	addedEss, err := essentials.SyncMissingDefaults(essConf)
	if err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return DashboardState{}, err
	}

	if len(addedPkg) == 0 && len(addedEss) == 0 {
		a.emitBiLog("INFO",
			"All built-in apps are already present in packages.conf and essentials.conf.",
			"Semua aplikasi bawaan sudah tersedia di packages.conf dan essentials.conf.",
		)
	} else {
		if len(addedPkg) > 0 {
			a.markPkgConfigSelfWritten(pkgConf)
			a.emitBiLog("OK",
				fmt.Sprintf("Added built-in apps to packages.conf: %s", strings.Join(addedPkg, ", ")),
				fmt.Sprintf("Menambahkan aplikasi bawaan ke packages.conf: %s", strings.Join(addedPkg, ", ")),
			)
		}
		if len(addedEss) > 0 {
			a.markEssConfigSelfWritten(essConf)
			a.emitBiLog("OK",
				fmt.Sprintf("Added built-in apps to essentials.conf: %s", strings.Join(addedEss, ", ")),
				fmt.Sprintf("Menambahkan aplikasi bawaan ke essentials.conf: %s", strings.Join(addedEss, ", ")),
			)
		}
	}
	return a.GetDashboardState()
}

// AddCustomDirToPath appends a tool bin folder (such as %USERPROFILE%\go\bin) to User PATH.
func (a *App) AddCustomDirToPath(dir string) (DashboardState, error) {
	added, err := pathmgr.AddDirToUserPath(dir)
	if err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return DashboardState{}, err
	}
	if added {
		a.emitBiLog("OK",
			fmt.Sprintf("Added directory to User PATH (HKCU\\Environment): %s", dir),
			fmt.Sprintf("Direktori ditambahkan ke User PATH (HKCU\\Environment): %s", dir),
		)
	} else {
		a.emitBiLog("INFO",
			fmt.Sprintf("Directory is already in User PATH: %s", dir),
			fmt.Sprintf("Direktori sudah ada di User PATH: %s", dir),
		)
	}
	return a.GetDashboardState()
}

// PickAndAddUserPathDir lets the user browse for a folder and append it to User PATH (HKCU).
func (a *App) PickAndAddUserPathDir() (DashboardState, error) {
	title := "Select Folder to Add to User PATH"
	if a.getLanguage() == "id" {
		title = "Pilih Folder untuk Ditambahkan ke User PATH"
	}
	selected, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title:            title,
		DefaultDirectory: a.getBaseDir(),
	})
	if err != nil || strings.TrimSpace(selected) == "" {
		return a.GetDashboardState()
	}
	return a.AddCustomDirToPath(selected)
}

// MoveUserPathEntry reorders an entry inside User PATH (delta < 0 moves up, delta > 0 moves down).
func (a *App) MoveUserPathEntry(index, delta int) (DashboardState, error) {
	if _, err := pathmgr.MoveUserPathEntry(index, delta); err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return DashboardState{}, err
	}
	dirWordEN, dirWordID := "up", "ke atas"
	if delta > 0 {
		dirWordEN, dirWordID = "down", "ke bawah"
	}
	a.emitBiLog("OK",
		fmt.Sprintf("Moved User PATH entry #%d %s (priority updated).", index+1, dirWordEN),
		fmt.Sprintf("Urutan User PATH #%d digeser %s (prioritas diperbarui).", index+1, dirWordID),
	)
	return a.GetDashboardState()
}

// RemoveUserPathEntry removes one entry from HKCU\Environment\Path.
func (a *App) RemoveUserPathEntry(index int, entryPath string) (DashboardState, error) {
	if _, err := pathmgr.RemoveUserPathEntry(index, entryPath); err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return DashboardState{}, err
	}
	a.emitBiLog("OK",
		fmt.Sprintf("Removed from User PATH (HKCU\\Environment): %s", entryPath),
		fmt.Sprintf("Dihapus dari User PATH (HKCU\\Environment): %s", entryPath),
	)
	return a.GetDashboardState()
}

// CleanInvalidUserPath removes missing directories and duplicate entries from HKCU\Environment\Path.
func (a *App) CleanInvalidUserPath() (DashboardState, error) {
	removed, _, err := pathmgr.CleanUserPathEntries()
	if err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return DashboardState{}, err
	}
	if len(removed) == 0 {
		a.emitBiLog("INFO",
			"No missing folders or duplicates found in User PATH.",
			"Tidak ada folder rusak atau duplikat di User PATH.",
		)
	} else {
		a.emitBiLog("OK",
			fmt.Sprintf("Cleaned %d missing/duplicate User PATH entries.", len(removed)),
			fmt.Sprintf("Membersihkan %d entri User PATH yang rusak/duplikat.", len(removed)),
		)
	}
	return a.GetDashboardState()
}

// OpenUserPathDir opens a User PATH entry directory in Windows Explorer.
func (a *App) OpenUserPathDir(rawDir string) error {
	target := pathmgr.ExpandPathEntry(rawDir)
	if !dirExists(target) {
		return a.biErr(
			fmt.Sprintf("folder not found on disk: %s", target),
			fmt.Sprintf("folder tidak ditemukan di disk: %s", target),
		)
	}
	cmdStr := fmt.Sprintf("explorer.exe %s", target)
	a.emitBiLog("CMD", cmdStr, cmdStr)
	return exec.Command("explorer.exe", target).Start()
}

// RefreshUserPath reloads User PATH entries without waiting for remote version feeds.
func (a *App) RefreshUserPath() (DashboardState, error) {
	st, err := a.GetDashboardState()
	if err != nil {
		return st, err
	}
	a.emitBiLog("OK",
		fmt.Sprintf("User PATH reloaded (%d entries).", len(st.UserPathList)),
		fmt.Sprintf("User PATH dimuat ulang (%d entri).", len(st.UserPathList)),
	)
	return st, nil
}

// OpenPackagesConf launches Notepad to edit packages.conf next to UniDevUI.exe.
func (a *App) OpenPackagesConf() error {
	confPath, err := config.EnsureConfig(a.appDir)
	if err != nil {
		return err
	}
	return a.openInNotepad(confPath)
}

// OpenAppPathFolder opens the configured output folder in Windows Explorer.
func (a *App) OpenAppPathFolder() error {
	baseDir := a.getBaseDir()
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return err
	}
	cmdStr := fmt.Sprintf("explorer.exe %s", baseDir)
	a.emitBiLog("CMD", cmdStr, cmdStr)
	return exec.Command("explorer.exe", baseDir).Start()
}

// DownloadAndInstall downloads and extracts the selected version into <baseDir>\<appID>\<version>.
// It deliberately does NOT modify Windows PATH automatically.
func (a *App) DownloadAndInstall(appID, version string) error {
	baseDir := a.getBaseDir()
	confPath, err := config.EnsureConfig(a.appDir)
	if err != nil {
		return err
	}
	pkgs, _, err := config.ParseFile(confPath)
	if err != nil {
		return err
	}

	var targetPkg *config.AppPackage
	var targetVer *config.VersionEntry
	for i := range pkgs {
		if strings.EqualFold(pkgs[i].ID, appID) {
			targetPkg = &pkgs[i]
			for j := range pkgs[i].Versions {
				if pkgs[i].Versions[j].Version == version {
					targetVer = &pkgs[i].Versions[j]
					break
				}
			}
			break
		}
	}
	if targetPkg == nil || targetVer == nil {
		return a.biErr(
			fmt.Sprintf("package %s version %s not found in packages.conf", appID, version),
			fmt.Sprintf("paket %s versi %s tidak ditemukan di packages.conf", appID, version),
		)
	}

	ctx, err := a.beginTask()
	if err != nil {
		return err
	}

	go func() {
		defer a.endTask()
		a.emitBiLog("INFO",
			fmt.Sprintf("Downloading %s v%s -> %s", targetPkg.Name, targetVer.Version, targetVer.URL),
			fmt.Sprintf("Memulai download %s v%s -> %s", targetPkg.Name, targetVer.Version, targetVer.URL),
		)

		cacheDir := filepath.Join(baseDir, "_cache")
		archivePath, dlErr := archive.DownloadFile(ctx, targetVer.URL, cacheDir, func(ev archive.ProgressEvent) {
			runtime.EventsEmit(a.ctx, "task:progress", ev)
		}, a.emitBiLog)
		if dlErr != nil {
			a.emitBiLog("ERROR",
				fmt.Sprintf("%s v%s download failed: %v", targetPkg.Name, targetVer.Version, dlErr),
				fmt.Sprintf("%s v%s gagal diunduh: %v", targetPkg.Name, targetVer.Version, dlErr),
			)
			runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
			runtime.EventsEmit(a.ctx, "state:refresh")
			return
		}

		targetExtractDir := filepath.Join(baseDir, targetPkg.ID, targetVer.Version)
		a.emitBiLog("INFO",
			fmt.Sprintf("Download complete (%s). Extracting to %s...", filepath.Base(archivePath), targetExtractDir),
			fmt.Sprintf("Download selesai (%s). Mengekstrak ke %s...", filepath.Base(archivePath), targetExtractDir),
		)
		installDir, extErr := archive.InstallPackage(ctx, archivePath, baseDir, targetPkg.ID, targetVer.Version, func(ev archive.ProgressEvent) {
			runtime.EventsEmit(a.ctx, "task:progress", ev)
		}, a.emitBiLog)
		if extErr != nil {
			a.emitBiLog("ERROR",
				fmt.Sprintf("Extraction failed: %v", extErr),
				fmt.Sprintf("Ekstrak gagal: %v", extErr),
			)
			runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
			runtime.EventsEmit(a.ctx, "state:refresh")
			return
		}

		_ = os.Remove(archivePath)
		binDir := archive.ResolveBinDir(installDir, targetPkg.BinSubdir, targetPkg.Binary)
		a.emitBiLog("OK",
			fmt.Sprintf("%s v%s installed at %s (binary dir: %s)", targetPkg.Name, targetVer.Version, installDir, binDir),
			fmt.Sprintf("%s v%s terpasang di %s (binary dir: %s)", targetPkg.Name, targetVer.Version, installDir, binDir),
		)
		a.emitBiLog("INFO",
			`Windows PATH was not modified automatically. Click "Add to PATH" to activate this version.`,
			`PATH Windows tidak diubah otomatis. Klik tombol "Add to PATH" jika ingin mengaktifkan versi ini.`,
		)
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()

	return nil
}

// AddToPath activates an installed package version in HKCU\Environment\Path, replacing any previous entry for that app.
func (a *App) AddToPath(appID, version string) (DashboardState, error) {
	baseDir := a.getBaseDir()
	confPath, err := config.EnsureConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}
	pkgs, _, err := config.ParseFile(confPath)
	if err != nil {
		return DashboardState{}, err
	}

	var pkg config.AppPackage
	found := false
	for _, p := range pkgs {
		if strings.EqualFold(p.ID, appID) {
			pkg = p
			found = true
			break
		}
	}
	if !found {
		return DashboardState{}, a.biErr(
			fmt.Sprintf("application %s not found in packages.conf", appID),
			fmt.Sprintf("aplikasi %s tidak ditemukan di packages.conf", appID),
		)
	}

	installDir := filepath.Join(baseDir, pkg.ID, version)
	if !dirExists(installDir) {
		return DashboardState{}, a.biErr(
			fmt.Sprintf("%s v%s is not extracted at %s yet. Please download it first", pkg.Name, version, installDir),
			fmt.Sprintf("%s v%s belum diekstrak ke %s. Silakan download terlebih dahulu", pkg.Name, version, installDir),
		)
	}

	binDir := archive.ResolveBinDir(installDir, pkg.BinSubdir, pkg.Binary)
	if _, err := pathmgr.SetAppPath(baseDir, pkg.ID, binDir); err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return DashboardState{}, err
	}

	a.emitBiLog("OK",
		fmt.Sprintf("User PATH (HKCU\\Environment) updated: %s -> %s", pkg.Name, binDir),
		fmt.Sprintf("User PATH (HKCU\\Environment) diperbarui: %s -> %s", pkg.Name, binDir),
	)
	a.emitBiLog("INFO",
		"WM_SETTINGCHANGE broadcast sent. Open a new terminal (CMD/PowerShell) to use this version.",
		"Sinyal WM_SETTINGCHANGE terkirim. Buka terminal baru (CMD/PowerShell) untuk menggunakan versi ini.",
	)

	if conflict := pathmgr.DetectBinaryConflict(pkg.Binary, binDir); conflict != nil {
		a.emitBiLog("WARN",
			fmt.Sprintf("PATH conflict detected in %s: %s takes higher priority in Windows!", conflict.Scope, conflict.FullPath),
			fmt.Sprintf("Konflik PATH terdeteksi di %s: %s lebih diprioritaskan oleh Windows!", conflict.Scope, conflict.FullPath),
		)
	}

	return a.GetDashboardState()
}

// RemoveFromPath removes an application's entry from HKCU\Environment\Path.
func (a *App) RemoveFromPath(appID string) (DashboardState, error) {
	removed, err := pathmgr.RemoveAppPath(a.getBaseDir(), appID)
	if err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return DashboardState{}, err
	}
	if removed {
		a.emitBiLog("OK",
			fmt.Sprintf("PATH entry for %s removed from User PATH (WM_SETTINGCHANGE sent).", appID),
			fmt.Sprintf("Entry PATH untuk %s telah dihapus dari User PATH (WM_SETTINGCHANGE terkirim).", appID),
		)
	} else {
		a.emitBiLog("INFO",
			fmt.Sprintf("No active PATH entry found for %s.", appID),
			fmt.Sprintf("Tidak ada entry PATH aktif untuk %s.", appID),
		)
	}
	return a.GetDashboardState()
}

// DeleteInstalledVersion deletes an extracted version directory and removes it from PATH if active.
func (a *App) DeleteInstalledVersion(appID, version string) (DashboardState, error) {
	baseDir := a.getBaseDir()
	activeMap, _ := pathmgr.ActiveVersions(baseDir)
	if activeVer := activeMap[strings.ToLower(appID)]; strings.EqualFold(activeVer, version) {
		_, _ = pathmgr.RemoveAppPath(baseDir, appID)
		a.emitBiLog("INFO",
			fmt.Sprintf("Removing %s v%s from User PATH before deleting folder...", appID, version),
			fmt.Sprintf("Menonaktifkan %s v%s dari User PATH sebelum menghapus folder...", appID, version),
		)
	}

	targetDir := filepath.Join(baseDir, appID, version)
	if err := os.RemoveAll(targetDir); err != nil {
		a.emitBiLog("ERROR",
			fmt.Sprintf("Failed to delete %s: %v", targetDir, err),
			fmt.Sprintf("Gagal menghapus %s: %v", targetDir, err),
		)
		return DashboardState{}, err
	}
	a.emitBiLog("OK",
		fmt.Sprintf("Folder %s deleted.", targetDir),
		fmt.Sprintf("Folder %s berhasil dihapus.", targetDir),
	)
	return a.GetDashboardState()
}

// UpdateLTSLinks queries official upstream feeds for all configured apps and updates packages.conf.
func (a *App) UpdateLTSLinks() error {
	confPath, err := config.EnsureConfig(a.appDir)
	if err != nil {
		return err
	}
	pkgs, _, err := config.ParseFile(confPath)
	if err != nil {
		return err
	}

	ctx, err := a.beginTask()
	if err != nil {
		return err
	}

	go func() {
		defer a.endTask()
		a.emitBiLog("INFO",
			"Checking latest LTS & stable versions for packages, CLI/SDK tools, and Winget...",
			"Memeriksa versi LTS & stabil terbaru untuk paket, CLI/SDK tools, dan Winget...",
		)

		updates := a.fetchLTSUpdates(ctx, pkgs, 4, true)
		var mu sync.Mutex
		var wg sync.WaitGroup

		toolCount := 0
		wg.Add(1)
		go func() {
			defer wg.Done()
			res := tools.RefreshLatestToolVersions(ctx, a.emitBiLog)
			mu.Lock()
			toolCount = len(res)
			mu.Unlock()
			for toolID, ver := range res {
				a.emitBiLog("OK",
					fmt.Sprintf("[tool:%s] latest version: %s", toolID, ver),
					fmt.Sprintf("[tool:%s] versi terbaru: %s", toolID, ver),
				)
			}
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			a.refreshWingetCache(ctx)
		}()

		wg.Add(1)
		go func() {
			defer wg.Done()
			info := a.refreshAppUpdate(ctx, true)
			if info.HasUpdate {
				a.emitBiLog("OK",
					fmt.Sprintf("[UniDevUI] update available: v%s -> v%s (open About to update)", info.CurrentVersion, info.LatestVersion),
					fmt.Sprintf("[UniDevUI] update tersedia: v%s -> v%s (buka Tentang untuk memperbarui)", info.CurrentVersion, info.LatestVersion),
				)
			}
		}()

		wg.Wait()

		if len(updates) == 0 {
			if toolCount > 0 {
				a.logMu.Lock()
				a.lastLTSCheck = time.Now().Format("15:04:05")
				a.logMu.Unlock()
				a.emitBiLog("OK",
					fmt.Sprintf("Update check finished: %d CLI/SDK tools checked. Package list reloaded.", toolCount),
					fmt.Sprintf("Pengecekan update selesai: %d CLI/SDK tool diperiksa. Daftar paket berhasil dimuat.", toolCount),
				)
			} else {
				a.emitBiLog("WARN",
					"Update check finished: no updates could be fetched from upstream sources.",
					"Pengecekan update selesai: tidak ada pembaruan yang berhasil diambil dari sumber resmi.",
				)
			}
			runtime.EventsEmit(a.ctx, "state:refresh")
			return
		}

		if err := config.UpdateVersionsInFile(confPath, updates); err != nil {
			a.emitBiLog("ERROR",
				fmt.Sprintf("Failed saving updates to packages.conf: %v", err),
				fmt.Sprintf("Gagal menyimpan pembaruan ke packages.conf: %v", err),
			)
		} else {
			a.markPkgConfigSelfWritten(confPath)
			a.logMu.Lock()
			a.lastLTSCheck = time.Now().Format("15:04:05")
			a.logMu.Unlock()
			a.refreshPackageSizes(ctx)
			a.emitBiLog("OK",
				fmt.Sprintf("Update check finished: packages.conf updated (%d apps), %d CLI/SDK tools checked. Package list reloaded.", len(updates), toolCount),
				fmt.Sprintf("Pengecekan update selesai: packages.conf diperbarui (%d aplikasi), %d CLI/SDK tool diperiksa. Daftar paket berhasil dimuat.", len(updates), toolCount),
			)
		}
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()

	return nil
}

func (a *App) getCachedURLSize(rawURL string) int64 {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return 0
	}
	a.sizeMu.RLock()
	sz := a.urlSizes[trimmed]
	a.sizeMu.RUnlock()
	return sz
}

func (a *App) refreshPackageSizes(ctx context.Context) bool {
	confPath, err := config.EnsureConfig(a.appDir)
	if err != nil {
		return false
	}
	pkgs, _, err := config.ParseFile(confPath)
	if err != nil {
		return false
	}
	var urls []string
	for _, p := range pkgs {
		for _, v := range p.Versions {
			if strings.TrimSpace(v.URL) != "" {
				urls = append(urls, v.URL)
			}
		}
	}
	urls = append(urls, tools.VSBootstrapURL)

	// Discover standalone x64 WebView2 installer URL via Winget in parallel.
	wv2Key := "__tool:webview2"
	var wv2InstallerURL string
	if det, dErr := essentials.ShowPackageDetails(ctx, tools.WebView2WingetID, "winget", false); dErr == nil {
		if strings.TrimSpace(det.InstallerURL) != "" {
			wv2InstallerURL = strings.TrimSpace(det.InstallerURL)
			urls = append(urls, wv2InstallerURL)
		}
	}

	a.sizeMu.RLock()
	snapshot := make(map[string]int64, len(a.urlSizes))
	for k, v := range a.urlSizes {
		snapshot[k] = v
	}
	cachePath := a.sizeCachePath
	a.sizeMu.RUnlock()

	found := lts.ProbeMissingSizes(ctx, urls, snapshot)
	var wv2Size int64
	if wv2InstallerURL != "" {
		if sz := found[wv2InstallerURL]; sz > 0 {
			wv2Size = sz
		} else if sz := snapshot[wv2InstallerURL]; sz > 0 && snapshot[wv2Key] != sz {
			wv2Size = sz
		}
	}
	if len(found) == 0 && wv2Size == 0 {
		return false
	}

	a.sizeMu.Lock()
	for k, v := range found {
		a.urlSizes[k] = v
	}
	if wv2Size > 0 {
		a.urlSizes[wv2Key] = wv2Size
	}
	merged := make(map[string]int64, len(a.urlSizes))
	for k, v := range a.urlSizes {
		merged[k] = v
	}
	a.sizeMu.Unlock()

	_ = lts.SaveSizeCache(cachePath, merged)
	return true
}

func (a *App) emitProgress(ev archive.ProgressEvent) {
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "task:progress", ev)
	}
}

func (a *App) cacheResolvedSize(key string, size int64, rawURL string) {
	if size <= 0 || key == "" {
		return
	}
	a.sizeMu.Lock()
	changed := a.urlSizes[key] != size
	a.urlSizes[key] = size
	if u := strings.TrimSpace(rawURL); u != "" {
		if a.urlSizes[u] != size {
			a.urlSizes[u] = size
			changed = true
		}
	}
	if !changed {
		a.sizeMu.Unlock()
		return
	}
	cachePath := a.sizeCachePath
	merged := make(map[string]int64, len(a.urlSizes))
	for k, v := range a.urlSizes {
		merged[k] = v
	}
	a.sizeMu.Unlock()
	_ = lts.SaveSizeCache(cachePath, merged)
}

func (a *App) refreshEssentialSizes(ctx context.Context) bool {
	if !essentials.CheckEnvironment().WingetAvailable {
		return false
	}
	essConfPath, err := essentials.EnsureConfig(a.appDir)
	if err != nil {
		return false
	}
	apps, _, err := essentials.ParseFile(essConfPath)
	if err != nil || len(apps) == 0 {
		return false
	}

	a.sizeMu.RLock()
	snapshot := make(map[string]int64, len(a.urlSizes))
	for k, v := range a.urlSizes {
		snapshot[k] = v
	}
	cachePath := a.sizeCachePath
	a.sizeMu.RUnlock()

	var todo []essentials.EssentialApp
	for _, app := range apps {
		if strings.EqualFold(app.Source, "msstore") || strings.TrimSpace(app.WingetID) == "" {
			continue
		}
		key := "__ess:" + strings.ToLower(strings.TrimSpace(app.WingetID))
		if snapshot[key] > 0 {
			continue
		}
		todo = append(todo, app)
	}
	if len(todo) == 0 {
		return false
	}

	found := make(map[string]int64)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 2)

	for _, item := range todo {
		app := item
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			det, err := essentials.ShowPackageDetails(ctx, app.WingetID, app.Source, false)
			if err != nil || strings.TrimSpace(det.InstallerURL) == "" {
				return
			}
			sz := snapshot[det.InstallerURL]
			if sz <= 0 {
				sz = lts.ProbeURLSize(ctx, det.InstallerURL)
			}
			if sz > 0 {
				key := "__ess:" + strings.ToLower(strings.TrimSpace(app.WingetID))
				mu.Lock()
				found[key] = sz
				found[det.InstallerURL] = sz
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if len(found) == 0 {
		return false
	}

	a.sizeMu.Lock()
	for k, v := range found {
		a.urlSizes[k] = v
	}
	merged := make(map[string]int64, len(a.urlSizes))
	for k, v := range a.urlSizes {
		merged[k] = v
	}
	a.sizeMu.Unlock()

	_ = lts.SaveSizeCache(cachePath, merged)
	return true
}

// InstallCLITool runs a CLI/SDK tool action ("install" | "update" | "fix") for wails2, wails3, tauri, vsbuildtools, webview2.
func (a *App) InstallCLITool(toolID, mode string) error {
	ctx, err := a.beginTask()
	if err != nil {
		return err
	}

	baseDir := a.getBaseDir()
	go func() {
		defer a.endTask()
		a.emitBiLog("INFO",
			fmt.Sprintf("Starting tool %s (%s)...", toolID, mode),
			fmt.Sprintf("Menjalankan %s tool %s...", mode, toolID),
		)
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "install", Percent: -1, Detail: fmt.Sprintf("%s %s...", mode, toolID)})
		if err := tools.InstallToolWithProgress(ctx, toolID, mode, baseDir, a.emitBiLog, a.emitProgress); err != nil {
			a.emitBiLog("ERROR",
				fmt.Sprintf("Tool %s (%s) failed: %v", toolID, mode, err),
				fmt.Sprintf("Proses %s pada tool %s gagal: %v", mode, toolID, err),
			)
		} else {
			a.emitBiLog("OK",
				fmt.Sprintf("Tool %s (%s) finished.", toolID, mode),
				fmt.Sprintf("Proses %s pada tool %s selesai.", mode, toolID),
			)
		}
		runtime.EventsEmit(a.ctx, "task:progress", archive.ProgressEvent{Stage: "idle", Percent: 0})
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()

	return nil
}

// CancelActiveTask aborts the currently running background operation, if any.
func (a *App) CancelActiveTask() bool {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	if a.busy && a.cancelTask != nil {
		a.emitBiLog("WARN",
			"Cancelling active background task...",
			"Membatalkan tugas yang sedang berjalan...",
		)
		a.cancelTask()
		return true
	}
	return false
}

func (a *App) beginTask() (context.Context, error) {
	a.taskMu.Lock()
	if a.busy {
		a.taskMu.Unlock()
		return nil, a.biErr(
			"another task is currently running, wait for it to finish or click Cancel",
			"sedang ada proses lain yang berjalan, tunggu selesai atau klik Cancel",
		)
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.busy = true
	a.cancelTask = cancel
	a.hasTaskError.Store(false)
	a.taskMu.Unlock()
	runtime.EventsEmit(a.ctx, "task:busy", true)
	a.syncTrayAndPowerState()
	return ctx, nil
}

func (a *App) endTask() {
	a.taskMu.Lock()
	if a.cancelTask != nil {
		a.cancelTask()
	}
	a.busy = false
	a.cancelTask = nil
	a.taskMu.Unlock()
	runtime.EventsEmit(a.ctx, "task:busy", false)
	debug.FreeOSMemory()
	a.syncTrayAndPowerState()
}

func (a *App) biErr(enMsg, idMsg string) error {
	if a.getLanguage() == "id" {
		return fmt.Errorf("%s", idMsg)
	}
	return fmt.Errorf("%s", enMsg)
}

func (a *App) emitBiLog(level, enMsg, idMsg string) {
	if idMsg == "" {
		idMsg = enMsg
	}
	ev := TerminalLogEvent{
		Timestamp: time.Now().Format("15:04:05"),
		Level:     level,
		Message:   enMsg,
		MessageID: idMsg,
	}
	a.logMu.Lock()
	a.logHistory = append(a.logHistory, ev)
	if len(a.logHistory) > 400 {
		a.logHistory = a.logHistory[len(a.logHistory)-400:]
	}
	a.logMu.Unlock()

	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "terminal:log", ev)
	}
	if strings.EqualFold(level, "ERROR") {
		a.hasTaskError.Store(true)
		if a.trayMgr != nil {
			go a.syncTrayAndPowerState()
		}
	}
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
