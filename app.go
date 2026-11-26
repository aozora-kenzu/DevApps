package main

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"github.com/aozora-kenzu/UniDevUI/internal/archive"
	"github.com/aozora-kenzu/UniDevUI/internal/config"
	"github.com/aozora-kenzu/UniDevUI/internal/essentials"
	"github.com/aozora-kenzu/UniDevUI/internal/lts"
	"github.com/aozora-kenzu/UniDevUI/internal/pathmgr"
	"github.com/aozora-kenzu/UniDevUI/internal/selfupdate"
	"github.com/aozora-kenzu/UniDevUI/internal/tools"
)

// AppVersion is the current UniDevUI desktop application version (overridable via -ldflags for testing).
var AppVersion = "1.0.2"

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
	ID            string   `json:"id"`
	Name          string   `json:"name"`
	Category      string   `json:"category"`
	WingetID      string   `json:"wingetId"`
	Source        string   `json:"source"`
	RequiresUAC   bool     `json:"requiresUac"`
	Versions      []string `json:"versions,omitempty"`
	Description   string   `json:"description"`
	Installed     bool     `json:"installed"`
	InstalledVer  string   `json:"installedVer"`
	AvailableVer  string   `json:"availableVer"`
	HasUpdate     bool     `json:"hasUpdate"`
	UpdateIgnored bool     `json:"updateIgnored"`
	VersionStatus string   `json:"versionStatus"` // "uptodate" | "outdated" | "ahead" | "detected" | "ignored" | ""
	MatchedBy     string   `json:"matchedBy,omitempty"`
	DownloadSize  int64    `json:"downloadSize"`
}

// InstalledAppView describes one installed Windows application discovered via Winget.
type InstalledAppView struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	InstalledVer    string `json:"installedVer"`
	AvailableVer    string `json:"availableVer"`
	Source          string `json:"source"`
	HasUpdate       bool   `json:"hasUpdate"`
	Ignored         bool   `json:"ignored"`
	WingetPinned    bool   `json:"wingetPinned"`
	IgnoreReason    string `json:"ignoreReason,omitempty"`
	ManagedByWinget bool   `json:"managedByWinget"`
	PinnedCurated   bool   `json:"pinnedCurated"`
	CuratedAppID    string `json:"curatedAppId,omitempty"`
	DownloadSize    int64  `json:"downloadSize"`
}

// WingetUpdateTarget identifies one Winget package to update via Update All.
type WingetUpdateTarget struct {
	Name     string `json:"name"`
	WingetID string `json:"wingetId"`
	Source   string `json:"source"`
}

// BatchEssentialItem identifies one essential app and optional version to install in a batch queue.
type BatchEssentialItem struct {
	ID      string `json:"id"`
	Version string `json:"version"`
}

// DashboardState is the complete snapshot rendered by the frontend.
type DashboardState struct {
	AppDir             string               `json:"appDir"`
	BaseDir            string               `json:"baseDir"`
	ConfPath           string               `json:"confPath"`
	EssentialsConfPath string               `json:"essentialsConfPath"`
	IgnoredConfPath    string               `json:"ignoredConfPath"`
	Packages           []PackageView        `json:"packages"`
	Tools              []tools.ToolStatus   `json:"tools"`
	Essentials         []EssentialView      `json:"essentials"`
	InstalledApps      []InstalledAppView   `json:"installedApps"`
	WingetEnv          essentials.EnvStatus    `json:"wingetEnv"`
	UserPathList       []string                `json:"userPathList"`
	UserPathEntries    []pathmgr.UserPathEntry `json:"userPathEntries"`
	Logs               []TerminalLogEvent      `json:"logs"`
	Busy               bool                    `json:"busy"`
	LastLTSCheck       string                  `json:"lastLtsCheck"`
	Language           string                  `json:"language"`
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

// App is the main Wails application service.
type App struct {
	ctx      context.Context
	appDir   string
	dirMu    sync.RWMutex
	baseDir  string
	language string

	taskMu     sync.Mutex
	busy       bool
	cancelTask context.CancelFunc

	logMu        sync.Mutex
	logHistory   []TerminalLogEvent
	lastLTSCheck string

	essMu            sync.RWMutex
	wingetEnv        essentials.EnvStatus
	wingetByID       map[string]essentials.WingetTableRow
	wingetByName     map[string]essentials.WingetTableRow
	wingetRows       []essentials.WingetTableRow
	regApps          []essentials.RegistryAppEntry
	fallbackRepoVers map[string]string
	wingetCacheDone  bool

	watchMu           sync.Mutex
	watchCancel       context.CancelFunc
	watchIgnorePkgMod time.Time
	watchIgnoreEssMod time.Time
	watchIgnoreIgnMod time.Time

	sizeMu        sync.RWMutex
	urlSizes      map[string]int64
	sizeCachePath string

	appUpdateMu   sync.RWMutex
	appUpdateInfo selfupdate.UpdateInfo
	appUpdatePath string
}

// NewApp creates a new App instance. Config files live in AppDir (beside UniDevUI.exe);
// extraction output defaults to C:\AppPath or persisted settings.
func NewApp() *App {
	settings, _, _ := config.LoadSettings()
	base := settings.BaseDir
	if strings.TrimSpace(base) == "" {
		base = config.DefaultBaseDir
	}
	lang := settings.Language
	if lang != "id" {
		lang = "en"
	}
	sizeCachePath := ""
	appUpdatePath := ""
	if sp, spErr := config.SettingsPath(); spErr == nil && sp != "" {
		cfgDir := filepath.Dir(sp)
		sizeCachePath = filepath.Join(cfgDir, "size_cache.json")
		appUpdatePath = filepath.Join(cfgDir, "app_update_cache.json")
	}
	cachedUpdate, _ := selfupdate.LoadCachedInfo(appUpdatePath, AppVersion, selfupdate.DefaultCheckCooldown)
	return &App{
		appDir:           config.AppDir(),
		baseDir:          base,
		language:         lang,
		wingetEnv:        essentials.CheckEnvironment(),
		wingetByID:       make(map[string]essentials.WingetTableRow),
		wingetByName:     make(map[string]essentials.WingetTableRow),
		regApps:          essentials.QueryRegistryUninstallApps(),
		fallbackRepoVers: make(map[string]string),
		urlSizes:         lts.LoadSizeCache(sizeCachePath),
		sizeCachePath:    sizeCachePath,
		appUpdateInfo:    cachedUpdate,
		appUpdatePath:    appUpdatePath,
	}
}

func (a *App) getBaseDir() string {
	a.dirMu.RLock()
	defer a.dirMu.RUnlock()
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

	// Refresh CLI/SDK latest versions & Winget installed map in background without delaying startup.
	go func() {
		bgCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		if res := tools.RefreshLatestToolVersions(bgCtx, nil); len(res) > 0 {
			a.logMu.Lock()
			if a.lastLTSCheck == "" {
				a.lastLTSCheck = time.Now().Format("15:04:05")
			}
			a.logMu.Unlock()
			runtime.EventsEmit(a.ctx, "state:refresh")
		}
	}()

	go func() {
		bgCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		a.refreshWingetCache(bgCtx)
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()

	go func() {
		bgCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
		defer cancel()
		if a.refreshPackageSizes(bgCtx) {
			runtime.EventsEmit(a.ctx, "state:refresh")
		}
		if a.refreshEssentialSizes(bgCtx) {
			runtime.EventsEmit(a.ctx, "state:refresh")
		}
	}()

	go func() {
		info := a.refreshAppUpdate(ctx, false)
		if info.HasUpdate {
			a.emitBiLog("INFO",
				fmt.Sprintf("UniDevUI update available: v%s -> v%s (open About to update)", info.CurrentVersion, info.LatestVersion),
				fmt.Sprintf("Update UniDevUI tersedia: v%s -> v%s (buka Tentang untuk memperbarui)", info.CurrentVersion, info.LatestVersion),
			)
		}
		runtime.EventsEmit(a.ctx, "state:refresh")
	}()
}

func (a *App) shutdown(ctx context.Context) {
	a.stopConfigWatcher()
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
		_, isIgnored := ignoredMap[wKey]
		ev := EssentialView{
			ID:           item.ID,
			Name:         item.Name,
			Category:     item.Category,
			WingetID:     item.WingetID,
			Source:       item.Source,
			RequiresUAC:  item.RequiresUAC,
			Versions:     item.Versions,
			Description:  item.Description,
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

			if isIgnored {
				ev.UpdateIgnored = true
				ev.HasUpdate = false
				ev.VersionStatus = "ignored"
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
		} else if isIgnored {
			ev.UpdateIgnored = true
			ev.VersionStatus = "ignored"
		}
		essViews = append(essViews, ev)
	}
	installedApps := a.buildInstalledAppViews(wingetRows, ignoredList, ignoredMap, curatedByWingetID)
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

	a.appUpdateMu.RLock()
	appUpdateCopy := a.appUpdateInfo
	a.appUpdateMu.RUnlock()
	if appUpdateCopy.CurrentVersion == "" {
		appUpdateCopy.CurrentVersion = selfupdate.NormalizeVersion(AppVersion)
	}

	return DashboardState{
		AppDir:             a.appDir,
		BaseDir:            baseDir,
		ConfPath:           pkgConfPath,
		EssentialsConfPath: essConfPath,
		IgnoredConfPath:    ignConfPath,
		Packages:           views,
		Tools:              toolsList,
		Essentials:         essViews,
		InstalledApps:      installedApps,
		WingetEnv:          wEnv,
		UserPathList:       userPaths,
		UserPathEntries:    userPathEntries,
		Logs:               logsCopy,
		Busy:               busy,
		LastLTSCheck:       lastCheck,
		Language:           a.getLanguage(),
		AppVersion:         AppVersion,
		AppUpdate:          appUpdateCopy,
	}, nil
}

func (a *App) buildInstalledAppViews(
	rows []essentials.WingetTableRow,
	ignoredList []essentials.IgnoredApp,
	ignoredMap map[string]essentials.IgnoredApp,
	curatedByWingetID map[string]string,
) []InstalledAppView {
	out := make([]InstalledAppView, 0, len(rows)+len(ignoredList))
	seen := make(map[string]bool, len(rows))

	for _, r := range rows {
		id := strings.TrimSpace(r.ID)
		if id == "" {
			continue
		}
		key := strings.ToLower(id)
		if seen[key] {
			continue
		}
		seen[key] = true

		ignEntry, isIgnored := ignoredMap[key]
		avail := strings.TrimSpace(r.Available)
		curatedID := curatedByWingetID[key]
		out = append(out, InstalledAppView{
			ID:              id,
			Name:            r.Name,
			InstalledVer:    strings.TrimSpace(r.Version),
			AvailableVer:    avail,
			Source:          strings.TrimSpace(r.Source),
			HasUpdate:       avail != "" && !isIgnored,
			Ignored:         isIgnored,
			WingetPinned:    isIgnored && ignEntry.WingetPinned,
			IgnoreReason:    ignEntry.Reason,
			ManagedByWinget: essentials.IsManageableWingetID(id),
			PinnedCurated:   curatedID != "",
			CuratedAppID:    curatedID,
			DownloadSize:    a.getCachedURLSize("__ess:" + key),
		})
	}

	// Ensure any entry saved in ignored_updates.conf appears under Ignored even if filtered out from wingetRows.
	for _, ig := range ignoredList {
		key := strings.ToLower(strings.TrimSpace(ig.ID))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		curatedID := curatedByWingetID[key]
		out = append(out, InstalledAppView{
			ID:              ig.ID,
			Name:            ig.Name,
			Source:          ig.Source,
			Ignored:         true,
			WingetPinned:    ig.WingetPinned,
			IgnoreReason:    ig.Reason,
			ManagedByWinget: essentials.IsManageableWingetID(ig.ID),
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
		if aItem.ManagedByWinget != bItem.ManagedByWinget {
			return aItem.ManagedByWinget
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
	env := essentials.CheckEnvironment()
	regApps := essentials.QueryRegistryUninstallApps()
	var byID, byName map[string]essentials.WingetTableRow
	var rows []essentials.WingetTableRow
	if env.WingetAvailable {
		idMap, nameMap, rawRows, err := essentials.QueryInstalledEntries(ctx)
		if err == nil {
			byID = idMap
			byName = nameMap
			rows = rawRows
		}
	}

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
					sem := make(chan struct{}, 6)
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
	for k, v := range repoVers {
		a.fallbackRepoVers[k] = v
	}
	a.wingetCacheDone = true
	a.essMu.Unlock()
}

// RefreshWingetStatus manually refreshes Winget availability, UAC state, and installed app map.
func (a *App) RefreshWingetStatus() (DashboardState, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	a.refreshWingetCache(ctx)
	st, err := a.GetDashboardState()
	if err != nil {
		return st, err
	}
	a.emitBiLog("OK",
		fmt.Sprintf("Winget status refreshed (%d Windows Apps checked).", len(st.Essentials)),
		fmt.Sprintf("Status Winget diperbarui (%d Windows Apps diperiksa).", len(st.Essentials)),
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

// SearchWingetApps searches the Winget catalog for apps not in the curated list.
func (a *App) SearchWingetApps(query string) ([]essentials.WingetTableRow, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	rows, err := essentials.SearchWinget(ctx, query)
	if err != nil {
		a.emitBiLog("ERROR", err.Error(), err.Error())
		return nil, err
	}

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

	a.emitBiLog("INFO",
		fmt.Sprintf("Winget search %q returned %d results.", query, len(annotated)),
		fmt.Sprintf("Pencarian Winget %q menemukan %d hasil.", query, len(annotated)),
	)
	return annotated, nil
}

// GetWingetPackageDetails returns live `winget show` metadata and a resolved icon data URI.
func (a *App) GetWingetPackageDetails(wingetID, source string) (essentials.PackageDetails, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	det, err := essentials.ShowPackageDetails(ctx, wingetID, source, true)
	if err != nil {
		return essentials.PackageDetails{}, err
	}
	if det.InstallerSize > 0 {
		a.cacheResolvedSize("__ess:"+strings.ToLower(strings.TrimSpace(wingetID)), det.InstallerSize, det.InstallerURL)
	}
	return det, nil
}

// EnrichWingetSearch fetches publisher, description, homepage, license, and icons in parallel
// for the top Winget search results so users can inspect exact app details before installing.
func (a *App) EnrichWingetSearch(rows []essentials.WingetTableRow) []essentials.PackageDetails {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	details := essentials.EnrichSearchRows(ctx, rows, 12)
	for _, d := range details {
		if d.ID != "" && d.InstallerSize > 0 {
			a.cacheResolvedSize("__ess:"+strings.ToLower(strings.TrimSpace(d.ID)), d.InstallerSize, d.InstallerURL)
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

// UpdateAllInstalled updates a list of installed Winget apps sequentially while respecting ignored_updates.conf.
func (a *App) UpdateAllInstalled(targets []WingetUpdateTarget) error {
	if len(targets) == 0 {
		return a.biErr("no updates queued", "tidak ada aplikasi untuk diperbarui")
	}
	ignConfPath, _ := essentials.EnsureIgnoredConfig(a.appDir)
	_, ignoredMap, _ := essentials.ParseIgnoredFile(ignConfPath)

	queue := make([]WingetUpdateTarget, 0, len(targets))
	for _, t := range targets {
		id := strings.TrimSpace(t.WingetID)
		if id == "" || !essentials.IsManageableWingetID(id) {
			continue
		}
		if _, isIgnored := ignoredMap[strings.ToLower(id)]; isIgnored {
			continue
		}
		queue = append(queue, t)
	}
	if len(queue) == 0 {
		return a.biErr("no updatable non-ignored apps found", "tidak ada aplikasi non-ignored yang bisa diperbarui")
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
			fmt.Sprintf("Starting sequential Winget update for %d applications...", total),
			fmt.Sprintf("Memulai pembaruan berurutan Winget untuk %d aplikasi...", total),
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
				fmt.Sprintf("[%d/%d] Updating %s (%s)...", idx+1, total, display, item.WingetID),
				fmt.Sprintf("[%d/%d] Memperbarui %s (%s)...", idx+1, total, display, item.WingetID),
			)
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
			if err := essentials.RunEssentialAction(ctx, app, "update", "latest", a.emitBiLog, batchProgress); err != nil {
				failed++
				a.emitBiLog("ERROR",
					fmt.Sprintf("[%d/%d] Failed updating %s: %v", idx+1, total, display, err),
					fmt.Sprintf("[%d/%d] Gagal memperbarui %s: %v", idx+1, total, display, err),
				)
			} else {
				succeeded++
				a.emitBiLog("OK",
					fmt.Sprintf("[%d/%d] Updated %s.", idx+1, total, display),
					fmt.Sprintf("[%d/%d] Berhasil memperbarui %s.", idx+1, total, display),
				)
			}
		}

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

// IgnoreInstalledApp excludes an app from updates & active Installed lists, and pins it in Windows Winget if supported.
func (a *App) IgnoreInstalledApp(id, name, source, reason string) (DashboardState, error) {
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
	if essentials.IsManageableWingetID(trimmedID) {
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

	if err := essentials.AddIgnoredApp(ignConfPath, essentials.IgnoredApp{
		ID:           trimmedID,
		Name:         display,
		Source:       strings.TrimSpace(source),
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
			fmt.Sprintf("Ignored %s (%s) in ignored_updates.conf.", display, trimmedID),
			fmt.Sprintf("Mengabaikan %s (%s) di ignored_updates.conf.", display, trimmedID),
		)
	}
	return a.GetDashboardState()
}

// UnignoreInstalledApp restores an ignored app to active Installed & Updates tracking and removes any Windows Winget pin.
func (a *App) UnignoreInstalledApp(id string) (DashboardState, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return DashboardState{}, a.biErr("empty app ID", "ID aplikasi kosong")
	}
	ignConfPath, err := essentials.EnsureIgnoredConfig(a.appDir)
	if err != nil {
		return DashboardState{}, err
	}
	removed, found, err := essentials.RemoveIgnoredApp(ignConfPath, trimmedID)
	if err != nil {
		return DashboardState{}, err
	}
	a.markIgnConfigSelfWritten(ignConfPath)

	display := removed.Name
	if display == "" {
		display = trimmedID
	}

	if essentials.IsManageableWingetID(trimmedID) {
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
	baseDir := a.baseDir
	a.dirMu.Unlock()

	if err := config.SaveSettings(config.AppSettings{BaseDir: baseDir, Language: norm}); err != nil {
		a.emitBiLog("WARN",
			fmt.Sprintf("Failed saving language preference: %v", err),
			fmt.Sprintf("Gagal menyimpan preferensi bahasa: %v", err),
		)
	}
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

	if err := config.SaveSettings(config.AppSettings{BaseDir: cleanNew, Language: a.getLanguage()}); err != nil {
		a.emitBiLog("WARN",
			fmt.Sprintf("Directory changed to %s, but failed saving settings.json: %v", cleanNew, err),
			fmt.Sprintf("Folder diubah ke %s, namun gagal menyimpan settings.json: %v", cleanNew, err),
		)
	}

	a.dirMu.Lock()
	a.baseDir = cleanNew
	a.dirMu.Unlock()

	a.emitBiLog("OK",
		fmt.Sprintf("Output path changed to: %s (configs remain in %s)", cleanNew, a.appDir),
		fmt.Sprintf("Output path diubah ke: %s (config tetap di %s)", cleanNew, a.appDir),
	)
	return a.GetDashboardState()
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

		updates := make(map[string][]config.VersionEntry)
		var mu sync.Mutex
		var wg sync.WaitGroup

		for _, pkg := range pkgs {
			if pkg.LTSProvider == "" || pkg.LTSProvider == "manual" {
				continue
			}
			p := pkg
			wg.Add(1)
			go func() {
				defer wg.Done()
				vers, fErr := lts.FetchLatestVersions(ctx, p.LTSProvider)
				if fErr != nil {
					a.emitBiLog("WARN",
						fmt.Sprintf("[%s] update check failed (%s): %v", p.ID, p.LTSProvider, fErr),
						fmt.Sprintf("[%s] gagal cek update (%s): %v", p.ID, p.LTSProvider, fErr),
					)
					return
				}
				if len(vers) == 0 {
					return
				}
				mu.Lock()
				updates[p.ID] = vers
				mu.Unlock()

				var list []string
				for _, v := range vers {
					list = append(list, v.Version)
				}
				verList := strings.Join(list, ", ")
				a.emitBiLog("OK",
					fmt.Sprintf("[%s] latest versions: %s", p.ID, verList),
					fmt.Sprintf("[%s] versi terbaru: %s", p.ID, verList),
				)
			}()
		}

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
	sem := make(chan struct{}, 6)

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
	defer a.taskMu.Unlock()
	if a.busy {
		return nil, a.biErr(
			"another task is currently running, wait for it to finish or click Cancel",
			"sedang ada proses lain yang berjalan, tunggu selesai atau klik Cancel",
		)
	}
	ctx, cancel := context.WithCancel(a.ctx)
	a.busy = true
	a.cancelTask = cancel
	runtime.EventsEmit(a.ctx, "task:busy", true)
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
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
