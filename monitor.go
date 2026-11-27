package main

import (
	"context"
	"fmt"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/aozora-kenzu/UniDevUI/internal/config"
	"github.com/aozora-kenzu/UniDevUI/internal/lts"
	"github.com/aozora-kenzu/UniDevUI/internal/pathmgr"
	"github.com/aozora-kenzu/UniDevUI/internal/pkgmgr"
	"github.com/aozora-kenzu/UniDevUI/internal/syspower"
	"github.com/aozora-kenzu/UniDevUI/internal/tools"
)

type updateNoticeItem struct {
	Key     string
	Label   string
	Version string
}

func computeStartupScanDelay(startedHidden bool, s config.AppSettings) time.Duration {
	if startedHidden {
		return 45 * time.Second
	}
	if s.DelayStartupScan {
		return 6 * time.Second
	}
	return 1 * time.Second
}

func shouldSkipStartupScanByCooldown(s config.AppSettings, now time.Time) bool {
	if s.LastCheckUnix <= 0 {
		return false
	}
	last := time.Unix(s.LastCheckUnix, 0)
	if last.After(now) {
		return false
	}
	hours := s.AutoCheckIntervalHours
	if hours <= 0 {
		hours = 6
	}
	return now.Sub(last) < time.Duration(hours)*time.Hour
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}

func (a *App) isTaskBusy() bool {
	a.taskMu.Lock()
	defer a.taskMu.Unlock()
	return a.busy
}

func (a *App) hasWingetCache() bool {
	a.essMu.RLock()
	done := a.wingetCacheDone
	a.essMu.RUnlock()
	return done
}

// ensureInitialCacheLoaded populates the Winget / Package Manager inventory when an interactive
// window opens after a silent --tray boot skipped the heavy startup scan.
func (a *App) ensureInitialCacheLoaded() {
	if a.hasWingetCache() || a.isTaskBusy() {
		return
	}
	parentCtx := a.ctx
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	stepCtx, cancel := context.WithTimeout(parentCtx, 35*time.Second)
	a.refreshWingetCache(stepCtx)
	cancel()
	debug.FreeOSMemory()
	a.requestStateRefresh()
}

// runStartupSequence runs lightweight metadata checks early and defers heavier subprocess scans
// until the system and UI have settled, preventing startup CPU spikes.
func (a *App) runStartupSequence(ctx context.Context) {
	defer a.syncTrayAndPowerState()

	if a.iconCachePath != "" {
		iconCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
		downloaded, err := pkgmgr.DefaultIconStore().EnsureSynced(iconCtx, a.iconCachePath, pkgmgr.IconDatabaseURL, pkgmgr.IconDatabaseTTL)
		cancel()
		if err == nil && downloaded {
			a.emitBiLog("INFO",
				fmt.Sprintf("Synced UniGetUI icon database (%d package icons).", pkgmgr.DefaultIconStore().EntryCount()),
				fmt.Sprintf("Database ikon UniGetUI tersinkron (%d ikon paket).", pkgmgr.DefaultIconStore().EntryCount()),
			)
			a.requestStateRefresh()
		}
	}

	if ctx.Err() != nil {
		return
	}

	s := a.getSettings()
	// When booted silently to tray (--tray) and checked recently (or auto-check disabled),
	// keep CPU & disk completely quiet and defer until the next scheduled interval or manual open.
	if a.startedHidden && (!s.AutoCheckUpdates || shouldSkipStartupScanByCooldown(s, time.Now())) {
		if s.LastCheckUnix > 0 {
			a.monitorMu.Lock()
			a.lastMonitorRun = time.Unix(s.LastCheckUnix, 0)
			a.monitorMu.Unlock()
		}
		if s.AutoCheckUpdates {
			a.emitBiLog("INFO",
				"Startup background scan skipped (already checked within configured cooldown interval).",
				"Scan pembaruan startup dilewati (sudah diperiksa dalam jeda interval terjadwal).",
			)
		}
		return
	}

	if deferScan, _ := syspower.ShouldDeferBackgroundScan(); !deferScan {
		if info := a.refreshAppUpdate(ctx, false); info.HasUpdate {
			a.emitBiLog("INFO",
				fmt.Sprintf("UniDevUI update available: v%s -> v%s (open About to update)", info.CurrentVersion, info.LatestVersion),
				fmt.Sprintf("Update UniDevUI tersedia: v%s -> v%s (buka Tentang untuk memperbarui)", info.CurrentVersion, info.LatestVersion),
			)
			a.requestStateRefresh()
		}
	}

	delay := computeStartupScanDelay(a.startedHidden, a.getSettings())
	if !sleepWithContext(ctx, delay) {
		return
	}

	s = a.getSettings()
	if a.startedHidden {
		if deferScan, reason := syspower.ShouldDeferBackgroundScan(); deferScan {
			a.emitBiLog("INFO",
				fmt.Sprintf("Deferred startup update scan (%s); will retry later.", reason),
				fmt.Sprintf("Menunda scan update startup (%s); akan dicoba ulang nanti.", reason),
			)
			interval := time.Duration(maxInt(s.AutoCheckIntervalHours, 1)) * time.Hour
			a.monitorMu.Lock()
			a.lastMonitorRun = time.Now().Add(-interval + 15*time.Minute)
			a.monitorMu.Unlock()
			return
		}
	}

	a.scanBusy.Store(true)
	a.syncTrayAndPowerState()
	defer func() {
		a.scanBusy.Store(false)
		debug.FreeOSMemory()
		a.syncTrayAndPowerState()
	}()

	withinCooldown := shouldSkipStartupScanByCooldown(s, time.Now())

	// Phase 1: Lightweight remote version checks for CLI tools / pkg managers (skip if within cooldown or offline).
	if !withinCooldown {
		if deferScan, _ := syspower.ShouldDeferBackgroundScan(); !deferScan {
			stepCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
			res := tools.RefreshLatestToolVersions(stepCtx, nil)
			cancel()
			if len(res) > 0 {
				a.logMu.Lock()
				if a.lastLTSCheck == "" {
					a.lastLTSCheck = time.Now().Format("15:04:05")
				}
				a.logMu.Unlock()
				a.requestStateRefresh()
			}
		}
	}

	if !sleepWithContext(ctx, 1200*time.Millisecond) {
		return
	}

	// Phase 2: Throttled installed apps + multi-manager scan (only if user isn't running an install task).
	if !a.isTaskBusy() {
		stepCtx, cancel := context.WithTimeout(ctx, 35*time.Second)
		a.refreshWingetCache(stepCtx)
		cancel()
		a.requestStateRefresh()
	}

	if !sleepWithContext(ctx, 1200*time.Millisecond) {
		return
	}

	// Phase 3: Probe only uncached installer sizes gently when online/unthrottled.
	if !withinCooldown {
		if deferScan, _ := syspower.ShouldDeferBackgroundScan(); !deferScan {
			stepCtx, cancel := context.WithTimeout(ctx, 25*time.Second)
			updated := a.refreshPackageSizes(stepCtx)
			if a.refreshEssentialSizes(stepCtx) {
				updated = true
			}
			cancel()
			if updated {
				a.requestStateRefresh()
			}
		}
	}

	now := time.Now()
	a.monitorMu.Lock()
	a.lastMonitorRun = now
	a.monitorMu.Unlock()
	if !withinCooldown {
		a.persistLastCheckUnix(now.Unix())
	}

	if a.getSettings().AutoCheckUpdates {
		a.notifyAvailableUpdatesIfNew(false)
	}
}

func (a *App) runUpdateMonitorLoop(ctx context.Context) {
	if s := a.getSettings(); s.LastCheckUnix > 0 {
		a.monitorMu.Lock()
		if a.lastMonitorRun.IsZero() {
			a.lastMonitorRun = time.Unix(s.LastCheckUnix, 0)
		}
		a.monitorMu.Unlock()
	}

	ticker := time.NewTicker(1 * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s := a.getSettings()
			if !s.AutoCheckUpdates || a.isTaskBusy() {
				continue
			}
			interval := time.Duration(s.AutoCheckIntervalHours) * time.Hour
			if interval < time.Hour {
				interval = time.Hour
			}
			a.monitorMu.Lock()
			last := a.lastMonitorRun
			a.monitorMu.Unlock()
			if !last.IsZero() && time.Since(last) >= interval {
				a.runBackgroundUpdateCycle(ctx, false)
			}
		}
	}
}

func (a *App) runBackgroundUpdateCycle(parentCtx context.Context, userTriggered bool) {
	if a.isTaskBusy() {
		if userTriggered && a.trayMgr != nil {
			title, body := formatMonitorBusyNotification(a.getLanguage())
			a.trayMgr.ShowNotification(title, body)
		}
		return
	}
	s := a.getSettings()
	if !userTriggered {
		if deferScan, reason := syspower.ShouldDeferBackgroundScan(); deferScan {
			a.emitBiLog("INFO",
				fmt.Sprintf("Deferred scheduled update scan (%s).", reason),
				fmt.Sprintf("Menunda pemindaian update terjadwal (%s).", reason),
			)
			interval := time.Duration(maxInt(s.AutoCheckIntervalHours, 1)) * time.Hour
			a.monitorMu.Lock()
			a.lastMonitorRun = time.Now().Add(-interval + 15*time.Minute)
			a.monitorMu.Unlock()
			return
		}
	}
	if parentCtx == nil {
		parentCtx = context.Background()
	}
	if userTriggered {
		a.hasTaskError.Store(false)
	}
	a.scanBusy.Store(true)
	a.syncTrayAndPowerState()
	defer func() {
		a.scanBusy.Store(false)
		debug.FreeOSMemory()
		a.syncTrayAndPowerState()
	}()

	ctx, cancel := context.WithTimeout(parentCtx, 45*time.Second)
	defer cancel()

	if confPath, err := config.EnsureConfig(a.appDir); err == nil {
		if pkgs, _, pErr := config.ParseFile(confPath); pErr == nil {
			updates := a.fetchLTSUpdates(ctx, pkgs, 3, false)
			if len(updates) > 0 {
				if wErr := config.UpdateVersionsInFile(confPath, updates); wErr == nil {
					a.markPkgConfigSelfWritten(confPath)
				}
			}
		}
	}

	_ = tools.RefreshLatestToolVersions(ctx, nil)
	a.refreshWingetCache(ctx)
	_ = a.refreshAppUpdate(ctx, userTriggered)

	now := time.Now()
	a.logMu.Lock()
	a.lastLTSCheck = now.Format("15:04:05")
	a.logMu.Unlock()

	a.monitorMu.Lock()
	a.lastMonitorRun = now
	a.monitorMu.Unlock()
	a.persistLastCheckUnix(now.Unix())

	a.requestStateRefresh()
	a.notifyAvailableUpdatesIfNew(userTriggered)
}

func maxInt(v, fallback int) int {
	if v <= 0 {
		return fallback
	}
	return v
}

func (a *App) fetchLTSUpdates(ctx context.Context, pkgs []config.AppPackage, workers int, logUpdates bool) map[string][]config.VersionEntry {
	if workers <= 0 {
		workers = 3
	}
	updates := make(map[string][]config.VersionEntry)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, workers)

	for _, pkg := range pkgs {
		if pkg.LTSProvider == "" || pkg.LTSProvider == "manual" {
			continue
		}
		p := pkg
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			vers, fErr := lts.FetchLatestVersions(ctx, p.LTSProvider)
			if fErr != nil {
				if logUpdates {
					a.emitBiLog("WARN",
						fmt.Sprintf("[%s] update check failed (%s): %v", p.ID, p.LTSProvider, fErr),
						fmt.Sprintf("[%s] gagal cek update (%s): %v", p.ID, p.LTSProvider, fErr),
					)
				}
				return
			}
			if len(vers) == 0 {
				return
			}
			mu.Lock()
			updates[p.ID] = vers
			mu.Unlock()

			if logUpdates {
				var list []string
				for _, v := range vers {
					list = append(list, v.Version)
				}
				verList := strings.Join(list, ", ")
				a.emitBiLog("OK",
					fmt.Sprintf("[%s] latest versions: %s", p.ID, verList),
					fmt.Sprintf("[%s] versi terbaru: %s", p.ID, verList),
				)
			}
		}()
	}
	wg.Wait()
	return updates
}

func extractUpdateNoticeItems(st DashboardState) []updateNoticeItem {
	var items []updateNoticeItem
	seen := make(map[string]bool)

	for _, pkg := range st.Packages {
		if pkg.ActiveVer == "" || pkg.LatestVer == "" {
			continue
		}
		if pathmgr.CompareSemver(pkg.LatestVer, pkg.ActiveVer) > 0 {
			key := "pkg:" + strings.ToLower(pkg.ID)
			if !seen[key] {
				seen[key] = true
				items = append(items, updateNoticeItem{
					Key:     key,
					Label:   pkg.Name,
					Version: strings.TrimPrefix(pkg.LatestVer, "v"),
				})
			}
		}
	}

	for _, tl := range st.Tools {
		if tl.Installed && tl.VersionStatus == "outdated" && tl.LatestVersion != "" {
			key := "tool:" + strings.ToLower(tl.ID)
			if !seen[key] {
				seen[key] = true
				items = append(items, updateNoticeItem{
					Key:     key,
					Label:   tl.Name,
					Version: strings.TrimPrefix(tl.LatestVersion, "v"),
				})
			}
		}
	}

	for _, app := range st.InstalledApps {
		if app.Ignored || !app.HasUpdate || !app.CanUpdate || app.AvailableVer == "" {
			continue
		}
		key := fmt.Sprintf("app:%s:%s", strings.ToLower(app.Manager), strings.ToLower(app.ID))
		if !seen[key] {
			seen[key] = true
			items = append(items, updateNoticeItem{
				Key:     key,
				Label:   app.Name,
				Version: strings.TrimPrefix(app.AvailableVer, "v"),
			})
		}
	}

	if st.AppUpdate.HasUpdate && st.AppUpdate.LatestVersion != "" {
		key := "self:unidevui"
		seen[key] = true
		items = append(items, updateNoticeItem{
			Key:     key,
			Label:   "UniDevUI",
			Version: strings.TrimPrefix(st.AppUpdate.LatestVersion, "v"),
		})
	}

	sort.SliceStable(items, func(i, j int) bool {
		return strings.ToLower(items[i].Label) < strings.ToLower(items[j].Label)
	})
	return items
}

func (a *App) collectAvailableUpdateItems() []updateNoticeItem {
	st, err := a.GetDashboardState()
	if err != nil {
		return nil
	}
	return extractUpdateNoticeItems(st)
}

func diffNewNotifications(prev map[string]string, current []updateNoticeItem) (fresh []updateNoticeItem, updatedMap map[string]string) {
	updatedMap = make(map[string]string, len(current))
	for _, it := range current {
		updatedMap[it.Key] = it.Version
		if prevVer, ok := prev[it.Key]; !ok || prevVer != it.Version {
			fresh = append(fresh, it)
		}
	}
	return fresh, updatedMap
}

func formatUpdateNotificationMessage(lang string, items []updateNoticeItem) (title, body string) {
	n := len(items)
	const maxPreview = 3
	previews := make([]string, 0, maxPreview)
	for i := 0; i < n && i < maxPreview; i++ {
		if items[i].Version != "" {
			previews = append(previews, fmt.Sprintf("%s (v%s)", items[i].Label, items[i].Version))
		} else {
			previews = append(previews, items[i].Label)
		}
	}
	extra := n - len(previews)

	if lang == "id" {
		title = fmt.Sprintf("UniDevUI: %d Pembaruan Tersedia", n)
		if extra > 0 {
			body = fmt.Sprintf("%s, dan %d lainnya. Klik untuk melihat.", strings.Join(previews, ", "), extra)
		} else {
			body = fmt.Sprintf("%s. Klik untuk melihat & update.", strings.Join(previews, ", "))
		}
		return title, body
	}
	title = fmt.Sprintf("UniDevUI: %d Update(s) Available", n)
	if extra > 0 {
		body = fmt.Sprintf("%s, and %d more. Click to review.", strings.Join(previews, ", "), extra)
	} else {
		body = fmt.Sprintf("%s. Click to review & update.", strings.Join(previews, ", "))
	}
	return title, body
}

func formatMonitorBusyNotification(lang string) (string, string) {
	if lang == "id" {
		return "UniDevUI Sedang Sibuk", "Ada proses instalasi/pembaruan lain yang sedang berjalan."
	}
	return "UniDevUI Is Busy", "Another install or update task is already running."
}

func (a *App) notifyAvailableUpdatesIfNew(userTriggered bool) {
	items := a.collectAvailableUpdateItems()
	s := a.getSettings()
	a.updateCount.Store(int32(len(items)))
	a.syncTrayAndPowerState()

	a.monitorMu.Lock()
	if a.notifiedUpdates == nil {
		a.notifiedUpdates = make(map[string]string)
	}
	fresh, nextMap := diffNewNotifications(a.notifiedUpdates, items)
	a.notifiedUpdates = nextMap
	a.monitorMu.Unlock()

	visible := a.isWindowVisibleOnScreen()

	if len(fresh) == 0 {
		if userTriggered && a.trayMgr != nil {
			if len(items) > 0 {
				title, body := formatUpdateNotificationMessage(s.Language, items)
				a.trayMgr.ShowNotification(title, body)
			} else if s.Language == "id" {
				a.trayMgr.ShowNotification("UniDevUI: Semua Sudah Terbaru", "Tidak ada pembaruan baru untuk runtime atau aplikasi Anda.")
			} else {
				a.trayMgr.ShowNotification("UniDevUI: All Up to Date", "No new updates found for your runtimes or installed apps.")
			}
		}
		return
	}

	title, body := formatUpdateNotificationMessage(s.Language, items)
	a.emitBiLog("INFO",
		fmt.Sprintf("Background monitor found %d available update(s): %s", len(items), body),
		fmt.Sprintf("Pemantau latar belakang menemukan %d pembaruan: %s", len(items), body),
	)
	// Smart toast suppression: omit OS balloon if user is actively viewing the main window,
	// unless triggered manually via the System Tray "Check Updates Now" menu.
	if a.trayMgr != nil && (userTriggered || !visible) {
		a.trayMgr.ShowNotification(title, body)
	}
}
