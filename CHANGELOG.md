# Changelog

All notable changes to **UniDevUI** are documented in this file.

## [v1.1.2] - 2026-11-27

### Added
- **Quiet Windows Startup (`--tray`) & Tray-First Auto-Start:** Added `internal/autostart` (`HKCU\...\Run` registry integration with `--tray`) and Wails `StartHidden` + `SingleInstanceLock` guards so UniDevUI launches silently into the Windows System Tray at startup or reboot without popping up the main window unless opened manually.
- **Full Win32 System Tray Service (`internal/tray`):**
  - Dynamic 32×32 PNG-in-ICO status badge rendering: **Idle**, **Green** (updates available + count, auto-cleared when 0 updates remain), **Blue** (task active), and **Orange** (warnings/errors).
  - Rich context menu with direct tab/workspace navigation (`Open UniDevUI`, `Runtimes & Dev Stack`, `Updates (N)`, `Search Packages`, `Check Updates Now`, `Start with Windows (--tray)`, and `Exit`).
  - Native Windows `WM_QUERYENDSESSION` / `WM_ENDSESSION` shutdown hook so OS restarts/shutdowns exit cleanly without blocking on confirmation dialogs.
- **Close Action Confirmation Modal & In-App Settings Modal:** Added Settings UI (`Ctrl`/Gear button) for configuring output path, UI language, close behavior (`Always ask`, `Minimize to Tray`, `Exit`), Windows startup toggle, background update interval (`1h`–`24h`), and startup CPU throttle (`delayStartupScan`).
- **Windows Power & Resource Optimizations (`internal/syspower`):**
  - **Efficiency Mode (EcoQoS):** Engages `ProcessPowerThrottling` (`ExecutionSpeed`) while hidden in the Tray or minimized and idle, restoring normal QoS when opened or running tasks.
  - **Network & Battery Saver Guard:** Defers automatic background scans cleanly when offline (`InternetGetConnectedState`) or when Windows Battery Saver is active (`GetSystemPowerStatus`).
  - **Idle Memory Trimming:** Reclaims unused Go heap pages via `debug.FreeOSMemory()` after background scans, completed tasks, and hiding to Tray.
- **Persistent Cooldown & Smart Toast Suppression (`monitor.go`):**
  - Persists `lastCheckUnix` in `settings.json` to skip redundant heavy scans across frequent restarts within the configured check interval, loading Tab 2 caches on demand if opened later.
  - Suppresses redundant Windows toast notifications when UniDevUI's main window is already visible on screen unless manually triggered from the Tray menu.
- **Multi-Package Manager Ecosystem (`internal/pkgmgr`, `internal/tools/pkgmgr.go`):**
  - Added unified search, installed scanning, updates, details, and UniGetUI icon database (`icon_database_v2.json`) enrichment across **Winget**, **MS Store**, **Scoop**, **Chocolatey**, **npm**, **pnpm**, **Bun**, **uv**, **pip** (including `.bat`/`.cmd` wrappers and skipping `WindowsApps` 0-byte stubs), **Cargo**, **.NET Tools**, and **PowerShell Gallery**.
- **Granular Update Policies:** Added **Skip Specific Version (`Skip v...`)** and **Temporary 7-Day Update Pause (`Pause 7d` / `pause_until` in `ignored_updates.conf`)** with automatic expiry and manual **Resume Updates**.

### Changed & Improved
- **Frameless Dark Desktop Window & Clean Layout:** Switched to a frameless custom titlebar window with full-height sidebar, responsive environment summaries (no overlap on unmaximized windows), header-anchored `Install Selected (N)` batch button, icon-only Terminal Output header actions, and Terminal drawer constrained within `.main-pane`.
- **Background Update Monitor:** Deduped update notifications, added 45s silent-tray / 6s UI-settled startup scan staggering to eliminate startup CPU spikes, and synced tray update counts across UI actions.

### Verified Tests & Tooling
- Expanded unit tests across `app_test.go`, `search_test.go`, `internal/autostart`, `internal/config/settings_test.go`, `internal/essentials/essentials_test.go`, `internal/pkgmgr`, `internal/syspower`, `internal/tools/tools_test.go`, and `internal/tray`.

---

## [v1.0.2] - 2026-11-26
- Added GitHub Release update checker, notification badge in header/About modal, and safe in-app binary self-updater.

## [v1.0.1] - 2026-11-26
- Updated UniDevUI screenshots, bilingual documentation, and UI refinements.

## [v1.0.0] - 2026-11-25
- Initial release with Portable Runtimes, `HKCU` User PATH management, Curated Windows Apps, Winget search/updates, and ConPTY streaming progress.
