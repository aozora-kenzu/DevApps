package pkgmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/aozora-kenzu/UniDevUI/internal/pathmgr"
	"github.com/aozora-kenzu/UniDevUI/internal/tools"
)

const (
	ManagerWinget    = "winget"
	ManagerScoop     = "scoop"
	ManagerChoco     = "choco"
	ManagerNpm       = "npm"
	ManagerPnpm      = "pnpm"
	ManagerBun       = "bun"
	ManagerUv        = "uv"
	ManagerPip       = "pip"
	ManagerCargo     = "cargo"
	ManagerDotnet    = "dotnet"
	ManagerPSGallery = "psgallery"
)

// SupportedManagers defines the canonical display order for Tab 2 multi-manager support.
var SupportedManagers = []struct {
	ID   string
	Name string
}{
	{ID: ManagerScoop, Name: "Scoop"},
	{ID: ManagerChoco, Name: "Chocolatey"},
	{ID: ManagerNpm, Name: "npm"},
	{ID: ManagerPnpm, Name: "pnpm"},
	{ID: ManagerBun, Name: "Bun"},
	{ID: ManagerUv, Name: "uv"},
	{ID: ManagerPip, Name: "pip"},
	{ID: ManagerCargo, Name: "Cargo"},
	{ID: ManagerDotnet, Name: ".NET Tool"},
	{ID: ManagerPSGallery, Name: "PSGallery"},
}

var httpClient = &http.Client{Timeout: 12 * time.Second}

// ManagerEnv describes whether a non-Winget CLI package manager is available on the host.
type ManagerEnv struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	Available bool   `json:"available"`
	Version   string `json:"version"`
	ExePath   string `json:"exePath"`
}

// PackageEntry represents an installed or searchable package managed by Scoop, Chocolatey, npm, pnpm, Bun, or uv.
type PackageEntry struct {
	Manager      string `json:"manager"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	Version      string `json:"version"`
	AvailableVer string `json:"availableVer,omitempty"`
	Source       string `json:"source,omitempty"`
	Description  string `json:"description,omitempty"`
	Homepage     string `json:"homepage,omitempty"`
	IconURL      string `json:"iconUrl,omitempty"`
	Installed    bool   `json:"installed,omitempty"`
	InstalledVer string `json:"installedVer,omitempty"`
	HasUpdate    bool   `json:"hasUpdate,omitempty"`
}

// Logger receives bilingual command output lines.
type Logger func(level, enMsg, idMsg string)

// IgnoreKey returns the canonical ignored_updates.conf key for a manager + package ID.
// Winget & local ARP/MSIX IDs keep bare IDs for backwards compatibility; other managers are prefixed.
func IgnoreKey(manager, id string) string {
	cleanID := strings.TrimSpace(id)
	mgr := NormalizeManager(manager)
	if mgr == "" || mgr == ManagerWinget {
		return cleanID
	}
	prefix := mgr + ":"
	if strings.HasPrefix(strings.ToLower(cleanID), prefix) {
		return cleanID
	}
	return prefix + cleanID
}

// SplitIgnoreKey splits a stored ignored_updates.conf key into manager + raw package ID.
func SplitIgnoreKey(storedID, sourceHint string) (manager, rawID string) {
	trimmed := strings.TrimSpace(storedID)
	lower := strings.ToLower(trimmed)
	for _, spec := range SupportedManagers {
		prefix := spec.ID + ":"
		if strings.HasPrefix(lower, prefix) {
			return spec.ID, strings.TrimSpace(trimmed[len(prefix):])
		}
	}
	src := NormalizeManager(sourceHint)
	if src != "" && src != ManagerWinget {
		return src, trimmed
	}
	return ManagerWinget, trimmed
}

// NormalizeManager normalizes manager strings and treats empty/msstore/arp/msix sources as winget where appropriate.
func NormalizeManager(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "", ManagerWinget, "msstore":
		return ManagerWinget
	case ManagerScoop, ManagerChoco, ManagerNpm, ManagerPnpm, ManagerBun, ManagerUv, ManagerPip, ManagerCargo, ManagerDotnet, ManagerPSGallery:
		return strings.ToLower(strings.TrimSpace(raw))
	default:
		return strings.ToLower(strings.TrimSpace(raw))
	}
}

// IsSupportedExtraManager returns true when id names one of the secondary package managers.
func IsSupportedExtraManager(id string) bool {
	norm := NormalizeManager(id)
	for _, spec := range SupportedManagers {
		if spec.ID == norm {
			return true
		}
	}
	return false
}

// IsCoreProtectedPackage returns true for self-bootstrapped manager packages that shouldn't be uninstalled via UI.
func IsCoreProtectedPackage(manager, pkgID string) bool {
	mgr := NormalizeManager(manager)
	id := strings.ToLower(strings.TrimSpace(pkgID))
	return (mgr == ManagerChoco && id == "chocolatey") ||
		(mgr == ManagerNpm && id == "npm") ||
		(mgr == ManagerPip && (id == "pip" || id == "setuptools" || id == "wheel"))
}

// QueryAllInstalled concurrently probes every available secondary package manager on the system.
func QueryAllInstalled(ctx context.Context, baseDir string) ([]ManagerEnv, []PackageEntry) {
	if ctx == nil {
		ctx = context.Background()
	}

	toolStatusByID := make(map[string]tools.ToolStatus)
	for _, tl := range tools.ListToolStatuses(baseDir) {
		if tl.Category == "pkgmgr" {
			toolStatusByID[tl.ID] = tl
		}
	}

	envs := make([]ManagerEnv, len(SupportedManagers))
	perMgr := make([][]PackageEntry, len(SupportedManagers))
	sem := make(chan struct{}, 3)
	var wg sync.WaitGroup

	for i, spec := range SupportedManagers {
		idx := i
		mgrID := spec.ID
		mgrName := spec.Name
		tl := toolStatusByID[mgrID]
		exe := tl.Location
		if exe == "" {
			exe = tools.FindPkgMgrExe(mgrID, baseDir)
		}
		envs[idx] = ManagerEnv{
			ID:        mgrID,
			Name:      mgrName,
			Available: exe != "",
			Version:   strings.TrimPrefix(tl.Version, "v"),
			ExePath:   exe,
		}
		if exe == "" {
			continue
		}

		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			listCtx, cancel := context.WithTimeout(ctx, 18*time.Second)
			defer cancel()
			rows, err := listInstalledByManager(listCtx, mgrID, exe, baseDir)
			if err == nil {
				perMgr[idx] = rows
			}
		}()
	}
	wg.Wait()

	var all []PackageEntry
	for _, rows := range perMgr {
		for i := range rows {
			if rows[i].IconURL == "" {
				rows[i].IconURL = LookupIconMedia(rows[i].Manager, rows[i].ID, rows[i].Name).Icon
			}
		}
		all = append(all, rows...)
	}
	return envs, all
}

// DetectManagerEnvs quickly detects which non-Winget CLI package managers have binaries available on the host.
func DetectManagerEnvs(baseDir string) []ManagerEnv {
	envs := make([]ManagerEnv, len(SupportedManagers))
	for i, spec := range SupportedManagers {
		exe := tools.FindPkgMgrExe(spec.ID, baseDir)
		envs[i] = ManagerEnv{
			ID:        spec.ID,
			Name:      spec.Name,
			Available: exe != "",
			ExePath:   exe,
		}
	}
	return envs
}

// AnnotateInstalledAcrossManagers marks search results that match installed packages across any of the given managers.
func AnnotateInstalledAcrossManagers(rows []PackageEntry, managers []string, installed []PackageEntry) []PackageEntry {
	if len(rows) == 0 || len(managers) == 0 || len(installed) == 0 {
		return rows
	}
	priority := make(map[string]int, len(managers))
	for idx, m := range managers {
		norm := NormalizeManager(m)
		if _, exists := priority[norm]; !exists {
			priority[norm] = idx
		}
	}

	instByKey := make(map[string]PackageEntry)
	instPrioByKey := make(map[string]int)
	for _, item := range installed {
		mgr := NormalizeManager(item.Manager)
		prio, ok := priority[mgr]
		if !ok {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(item.ID))
		if prevPrio, exists := instPrioByKey[key]; !exists || prio < prevPrio {
			instByKey[key] = item
			instPrioByKey[key] = prio
		}
	}

	for i := range rows {
		key := strings.ToLower(strings.TrimSpace(rows[i].ID))
		if match, ok := instByKey[key]; ok {
			rows[i].Manager = NormalizeManager(match.Manager)
			rows[i].Installed = true
			rows[i].InstalledVer = match.Version
			avail := strings.TrimSpace(rows[i].Version)
			if avail != "" && match.Version != "" && pathmgr.CompareSemver(avail, match.Version) > 0 {
				rows[i].AvailableVer = avail
				rows[i].HasUpdate = true
			} else if match.HasUpdate && match.AvailableVer != "" {
				rows[i].AvailableVer = match.AvailableVer
				rows[i].HasUpdate = true
			}
		}
	}
	return rows
}

// SearchPackages queries the specified package manager catalog and annotates any matches already installed.
func SearchPackages(ctx context.Context, manager, query, baseDir string, installed []PackageEntry) ([]PackageEntry, error) {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	mgr := NormalizeManager(manager)
	exe := tools.FindPkgMgrExe(mgr, baseDir)

	var rows []PackageEntry
	var err error
	switch mgr {
	case ManagerScoop:
		if exe == "" {
			return nil, fmt.Errorf("scoop.cmd not found; install Scoop from Tab 1 -> Package Managers")
		}
		rows, err = searchScoop(ctx, exe, baseDir, q)
	case ManagerChoco:
		if exe == "" {
			return nil, fmt.Errorf("choco.exe not found; install Chocolatey from Tab 1 -> Package Managers")
		}
		rows, err = searchChoco(ctx, exe, baseDir, q)
	case ManagerNpm, ManagerPnpm, ManagerBun:
		rows, err = searchNpmRegistry(ctx, mgr, q)
	case ManagerUv, ManagerPip:
		rows, err = searchPyPI(ctx, mgr, q)
	case ManagerCargo:
		rows, err = searchCargoWithBaseURL(ctx, defaultCratesIOURL, q)
	case ManagerDotnet:
		rows, err = searchDotnetNuGetWithBaseURL(ctx, defaultNuGetSearchURL, q)
	case ManagerPSGallery:
		rows, err = searchNuGetV2OData(ctx, DefaultPSGalleryFeedURL, ManagerPSGallery, q)
	default:
		return nil, fmt.Errorf("unsupported package manager: %s", manager)
	}
	if err != nil {
		return nil, err
	}

	for i := range rows {
		if rows[i].IconURL == "" {
			rows[i].IconURL = LookupIconMedia(mgr, rows[i].ID, rows[i].Name).Icon
		}
	}
	return AnnotateInstalledAcrossManagers(rows, []string{mgr}, installed), nil
}

// RunAction executes install, update, or uninstall for a package via the specified manager CLI.
func RunAction(ctx context.Context, manager, pkgID, source, mode, baseDir string, log Logger) error {
	mgr := NormalizeManager(manager)
	id := strings.TrimSpace(pkgID)
	if id == "" {
		return fmt.Errorf("empty package ID")
	}
	exe := tools.FindPkgMgrExe(mgr, baseDir)
	if exe == "" {
		return fmt.Errorf("%s CLI not found; install it from Tab 1 -> Package Managers first", mgr)
	}

	normMode := strings.ToLower(strings.TrimSpace(mode))
	if normMode == "uninstall" && IsCoreProtectedPackage(mgr, id) {
		return fmt.Errorf("refusing to uninstall core package manager component %s", id)
	}

	env := tools.BuildMergedEnv(baseDir)
	toToolsLog := func() tools.Logger {
		if log == nil {
			return nil
		}
		return func(lvl, en, idMsg string) { log(lvl, en, idMsg) }
	}()

	var args []string
	elevateFallback := false
	switch mgr {
	case ManagerScoop:
		switch normMode {
		case "update":
			args = []string{"update", id}
		case "uninstall":
			args = []string{"uninstall", id}
		default:
			target := id
			if src := strings.TrimSpace(source); src != "" && !strings.Contains(id, "/") {
				target = src + "/" + id
			}
			args = []string{"install", target}
		}
	case ManagerChoco:
		elevateFallback = true
		switch normMode {
		case "update":
			args = []string{"upgrade", id, "-y", "--no-progress"}
		case "uninstall":
			args = []string{"uninstall", id, "-y", "--no-progress"}
		default:
			args = []string{"install", id, "-y", "--no-progress"}
		}
	case ManagerNpm:
		switch normMode {
		case "uninstall":
			args = []string{"uninstall", "-g", id}
		default:
			args = []string{"install", "-g", id + "@latest"}
		}
	case ManagerPnpm:
		switch normMode {
		case "uninstall":
			args = []string{"remove", "-g", id}
		default:
			args = []string{"add", "-g", id + "@latest"}
		}
	case ManagerBun:
		switch normMode {
		case "uninstall":
			args = []string{"remove", "-g", id}
		default:
			args = []string{"add", "-g", id + "@latest"}
		}
	case ManagerUv:
		switch normMode {
		case "update":
			args = []string{"tool", "upgrade", id}
		case "uninstall":
			args = []string{"tool", "uninstall", id}
		default:
			args = []string{"tool", "install", id}
		}
	case ManagerPip:
		switch normMode {
		case "uninstall":
			args = []string{"uninstall", "-y", id}
		default:
			args = []string{"install", "--upgrade", id, "--disable-pip-version-check"}
		}
	case ManagerCargo:
		switch normMode {
		case "uninstall":
			args = []string{"uninstall", id}
		default:
			args = []string{"install", id}
		}
	case ManagerDotnet:
		switch normMode {
		case "update":
			args = []string{"tool", "update", "-g", id}
		case "uninstall":
			args = []string{"tool", "uninstall", "-g", id}
		default:
			args = []string{"tool", "install", "-g", id}
		}
	case ManagerPSGallery:
		if !isSafeSimplePkgID(id) {
			return fmt.Errorf("invalid PowerShell Gallery package identifier: %q", id)
		}
		var psCmd string
		switch normMode {
		case "update":
			psCmd = fmt.Sprintf("try { Update-PSResource -Name '%s' -Scope CurrentUser -TrustRepository -Quiet -ErrorAction Stop } catch { Update-Module -Name '%s' -Force }", id, id)
		case "uninstall":
			psCmd = fmt.Sprintf("try { Uninstall-PSResource -Name '%s' -Quiet -ErrorAction Stop } catch { Uninstall-Module -Name '%s' -AllVersions -Force }", id, id)
		default:
			psCmd = fmt.Sprintf("try { Install-PSResource -Name '%s' -Scope CurrentUser -TrustRepository -Quiet -ErrorAction Stop } catch { Install-Module -Name '%s' -Scope CurrentUser -Force -AllowClobber }", id, id)
		}
		args = []string{"-NoProfile", "-NonInteractive", "-Command", psCmd}
	default:
		return fmt.Errorf("unsupported package manager: %s", manager)
	}

	if log != nil {
		cmdLine := fmt.Sprintf("%s %s", exe, strings.Join(args, " "))
		log("CMD", cmdLine, cmdLine)
	}
	if elevateFallback {
		return tools.RunWithElevationFallback(ctx, env, toToolsLog, exe, args...)
	}
	return tools.RunStreamingCmd(ctx, env, toToolsLog, exe, args...)
}

func listInstalledByManager(ctx context.Context, mgrID, exe, baseDir string) ([]PackageEntry, error) {
	env := tools.BuildMergedEnv(baseDir)
	switch mgrID {
	case ManagerScoop:
		out, err := runCmdCapture(ctx, env, exe, "export")
		if err != nil {
			return nil, err
		}
		rows := ParseScoopExport(out)
		if len(rows) > 0 {
			if statusOut, _ := runCmdCapture(ctx, env, exe, "status"); statusOut != "" {
				updates := ParseScoopStatus(statusOut)
				for i := range rows {
					if avail, ok := updates[strings.ToLower(rows[i].ID)]; ok && avail != "" {
						rows[i].AvailableVer = avail
						rows[i].HasUpdate = true
					}
				}
			}
		}
		return rows, nil

	case ManagerChoco:
		out, err := runCmdCapture(ctx, env, exe, "list", "-r")
		if err != nil {
			return nil, err
		}
		rows := ParseChocoPipeList(out)
		if len(rows) > 0 {
			outdatedRaw, _ := runCmdCapture(ctx, env, exe, "outdated", "-r")
			updates := ParseChocoOutdated(outdatedRaw)
			for i := range rows {
				if avail, ok := updates[strings.ToLower(rows[i].ID)]; ok && avail != "" {
					rows[i].AvailableVer = avail
					rows[i].HasUpdate = true
				}
			}
		}
		return rows, nil

	case ManagerNpm:
		out, err := runCmdCapture(ctx, env, exe, "ls", "-g", "--depth=0", "--json")
		if err != nil && strings.TrimSpace(out) == "" {
			return nil, err
		}
		rows := ParseNpmGlobalJSON(out)
		if len(rows) > 0 {
			// `npm outdated` exits 1 when updates exist; read stdout regardless of exit code.
			outdatedRaw, _ := runCmdCapture(ctx, env, exe, "outdated", "-g", "--json")
			updates := ParseNpmOutdatedJSON(outdatedRaw)
			for i := range rows {
				if avail, ok := updates[strings.ToLower(rows[i].ID)]; ok && avail != "" && pathmgr.CompareSemver(avail, rows[i].Version) > 0 {
					rows[i].AvailableVer = avail
					rows[i].HasUpdate = true
				}
			}
		}
		return rows, nil

	case ManagerPnpm:
		out, err := runCmdCapture(ctx, env, exe, "ls", "-g", "--depth=0", "--json")
		if err != nil && strings.TrimSpace(out) == "" {
			return nil, err
		}
		rows := ParsePnpmGlobalJSON(out)
		enrichNpmLatestConcurrently(ctx, rows, "https://registry.npmjs.org")
		return rows, nil

	case ManagerBun:
		out, err := runCmdCapture(ctx, env, exe, "pm", "ls", "-g")
		if err != nil && strings.TrimSpace(out) == "" {
			return nil, err
		}
		rows := ParseBunGlobalList(out)
		enrichNpmLatestConcurrently(ctx, rows, "https://registry.npmjs.org")
		return rows, nil

	case ManagerUv:
		out, err := runCmdCapture(ctx, env, exe, "tool", "list")
		if err != nil && strings.TrimSpace(out) == "" {
			return nil, err
		}
		rows := ParseUvToolList(out)
		enrichPyPILatestConcurrently(ctx, rows, "https://pypi.org/pypi")
		return rows, nil

	case ManagerPip:
		out, err := runCmdCapture(ctx, env, exe, "list", "--format=json", "--disable-pip-version-check")
		if err != nil && strings.TrimSpace(out) == "" {
			return nil, err
		}
		rows := ParsePipListJSON(out)
		if len(rows) > 0 {
			outdatedRaw, _ := runCmdCapture(ctx, env, exe, "list", "--outdated", "--format=json", "--disable-pip-version-check")
			updates := ParsePipOutdatedJSON(outdatedRaw)
			for i := range rows {
				if avail, ok := updates[strings.ToLower(rows[i].ID)]; ok && avail != "" && pathmgr.CompareSemver(avail, rows[i].Version) > 0 {
					rows[i].AvailableVer = avail
					rows[i].HasUpdate = true
				}
			}
		}
		return rows, nil

	case ManagerCargo:
		out, err := runCmdCapture(ctx, env, exe, "install", "--list")
		if err != nil && strings.TrimSpace(out) == "" {
			return nil, err
		}
		rows := ParseCargoInstallList(out)
		enrichCargoCratesConcurrently(ctx, rows, defaultCratesIOURL)
		return rows, nil

	case ManagerDotnet:
		out, err := runCmdCapture(ctx, env, exe, "tool", "list", "-g")
		if err != nil && strings.TrimSpace(out) == "" {
			return nil, err
		}
		rows := ParseDotnetToolList(out)
		enrichDotnetToolsConcurrently(ctx, rows, defaultNuGetSearchURL)
		return rows, nil

	case ManagerPSGallery:
		psCmd := `try { Get-InstalledPSResource -ErrorAction Stop | ForEach-Object { "$($_.Name)` + "`t" + `$($_.Version)` + "`t" + `$($_.Repository)" } } catch { Get-InstalledModule -ErrorAction SilentlyContinue | ForEach-Object { "$($_.Name)` + "`t" + `$($_.Version)` + "`t" + `$($_.Repository)" } }`
		out, err := runCmdCapture(ctx, env, exe, "-NoProfile", "-NonInteractive", "-Command", psCmd)
		if err != nil && strings.TrimSpace(out) == "" {
			return nil, err
		}
		rows := ParsePSGalleryInstalled(out)
		enrichPSGalleryUpdatesConcurrently(ctx, rows, DefaultPSGalleryFeedURL)
		return rows, nil

	default:
		return nil, fmt.Errorf("unknown manager %s", mgrID)
	}
}

func isSafeSimplePkgID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for _, r := range id {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '.' || r == '_' || r == '-'
		if !ok {
			return false
		}
	}
	return true
}

func runCmdCapture(ctx context.Context, env []string, exe string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil && len(out) == 0 {
		// Fall back to CombinedOutput when CLI prints warnings to stderr alongside stdout.
		cmd2 := exec.CommandContext(ctx, exe, args...)
		cmd2.Env = env
		cmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		out2, err2 := cmd2.CombinedOutput()
		return string(out2), err2
	}
	return string(out), err
}

// ParseScoopExport parses JSON from `scoop export`.
func ParseScoopExport(raw string) []PackageEntry {
	trimmed := strings.TrimSpace(raw)
	if start := strings.IndexByte(trimmed, '{'); start > 0 {
		trimmed = trimmed[start:]
	}
	var payload struct {
		Apps []struct {
			Name    string `json:"Name"`
			Version string `json:"Version"`
			Source  string `json:"Source"`
		} `json:"apps"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil
	}
	out := make([]PackageEntry, 0, len(payload.Apps))
	for _, a := range payload.Apps {
		name := strings.TrimSpace(a.Name)
		if name == "" {
			continue
		}
		out = append(out, PackageEntry{
			Manager: ManagerScoop,
			ID:      name,
			Name:    name,
			Version: strings.TrimSpace(a.Version),
			Source:  strings.TrimSpace(a.Source),
		})
	}
	return out
}

// ParseScoopStatus parses outdated rows from `scoop status` output.
func ParseScoopStatus(raw string) map[string]string {
	out := make(map[string]string)
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	pastHeader := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "----") {
			pastHeader = true
			continue
		}
		if !pastHeader {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) >= 3 && fields[2] != "" && !strings.EqualFold(fields[2], fields[1]) {
			out[strings.ToLower(fields[0])] = fields[2]
		}
	}
	return out
}

// ParseScoopSearch parses rows from `scoop search <query>`.
func ParseScoopSearch(raw string) []PackageEntry {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	pastHeader := false
	var out []PackageEntry
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "----") {
			pastHeader = true
			continue
		}
		if !pastHeader {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		src := ""
		if len(fields) >= 3 {
			src = fields[2]
		}
		desc := ""
		if len(fields) >= 4 {
			desc = "Binaries: " + strings.Join(fields[3:], " ")
		}
		out = append(out, PackageEntry{
			Manager:     ManagerScoop,
			ID:          fields[0],
			Name:        FormatDisplayName(fields[0], ""),
			Version:     fields[1],
			Source:      src,
			Description: desc,
			IconURL:     LookupIconMedia(ManagerScoop, fields[0], fields[0]).Icon,
		})
	}
	if len(out) > 30 {
		out = out[:30]
	}
	return out
}

// ParseChocoPipeList parses `choco list -r` or `choco search -r` lines (`id|version`).
func ParseChocoPipeList(raw string) []PackageEntry {
	var out []PackageEntry
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.Contains(trimmed, "|") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) < 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		ver := strings.TrimSpace(parts[1])
		if id == "" {
			continue
		}
		out = append(out, PackageEntry{
			Manager: ManagerChoco,
			ID:      id,
			Name:    FormatDisplayName(id, ""),
			Version: ver,
			Source:  ManagerChoco,
		})
	}
	if len(out) > 40 {
		out = out[:40]
	}
	return out
}

// ParseChocoOutdated parses `choco outdated -r` (`id|current|available|pinned`).
func ParseChocoOutdated(raw string) map[string]string {
	out := make(map[string]string)
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.Contains(trimmed, "|") {
			continue
		}
		parts := strings.Split(trimmed, "|")
		if len(parts) < 3 {
			continue
		}
		id := strings.ToLower(strings.TrimSpace(parts[0]))
		cur := strings.TrimSpace(parts[1])
		avail := strings.TrimSpace(parts[2])
		pinned := len(parts) >= 4 && strings.EqualFold(strings.TrimSpace(parts[3]), "true")
		if id != "" && avail != "" && avail != cur && !pinned {
			out[id] = avail
		}
	}
	return out
}

// ParseNpmGlobalJSON parses `npm ls -g --depth=0 --json`.
func ParseNpmGlobalJSON(raw string) []PackageEntry {
	var payload struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &payload); err != nil {
		return nil
	}
	return mapDependenciesToEntries(ManagerNpm, payload.Dependencies)
}

// ParseNpmOutdatedJSON parses `npm outdated -g --json`.
func ParseNpmOutdatedJSON(raw string) map[string]string {
	out := make(map[string]string)
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return out
	}
	var payload map[string]struct {
		Current string `json:"current"`
		Latest  string `json:"latest"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return out
	}
	for name, entry := range payload {
		latest := strings.TrimSpace(entry.Latest)
		if name != "" && latest != "" && latest != strings.TrimSpace(entry.Current) {
			out[strings.ToLower(name)] = latest
		}
	}
	return out
}

// ParsePnpmGlobalJSON parses `pnpm ls -g --depth=0 --json`.
func ParsePnpmGlobalJSON(raw string) []PackageEntry {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	type depsObj struct {
		Dependencies map[string]struct {
			Version string `json:"version"`
		} `json:"dependencies"`
	}
	if strings.HasPrefix(trimmed, "[") {
		var list []depsObj
		if err := json.Unmarshal([]byte(trimmed), &list); err != nil {
			return nil
		}
		merged := make(map[string]struct {
			Version string `json:"version"`
		})
		for _, item := range list {
			for k, v := range item.Dependencies {
				merged[k] = v
			}
		}
		return mapDependenciesToEntries(ManagerPnpm, merged)
	}
	var single depsObj
	if err := json.Unmarshal([]byte(trimmed), &single); err != nil {
		return nil
	}
	return mapDependenciesToEntries(ManagerPnpm, single.Dependencies)
}

func mapDependenciesToEntries(mgr string, deps map[string]struct {
	Version string `json:"version"`
}) []PackageEntry {
	names := make([]string, 0, len(deps))
	for k := range deps {
		names = append(names, k)
	}
	sort.Strings(names)
	out := make([]PackageEntry, 0, len(names))
	for _, name := range names {
		out = append(out, PackageEntry{
			Manager: mgr,
			ID:      name,
			Name:    name,
			Version: strings.TrimSpace(deps[name].Version),
			Source:  mgr,
		})
	}
	return out
}

// ParseBunGlobalList parses `bun pm ls -g` tree output (e.g. "├── typescript@5.8.2").
func ParseBunGlobalList(raw string) []PackageEntry {
	var out []PackageEntry
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		cleaned := strings.TrimSpace(line)
		cleaned = strings.TrimLeft(cleaned, "├└│─ \t")
		cleaned = strings.TrimSpace(cleaned)
		if cleaned == "" || !strings.Contains(cleaned, "@") || strings.Contains(cleaned, "node_modules") {
			continue
		}
		at := strings.LastIndex(cleaned, "@")
		if at <= 0 {
			continue
		}
		name := strings.TrimSpace(cleaned[:at])
		ver := strings.TrimSpace(cleaned[at+1:])
		if name == "" || ver == "" {
			continue
		}
		key := strings.ToLower(name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, PackageEntry{
			Manager: ManagerBun,
			ID:      name,
			Name:    name,
			Version: ver,
			Source:  ManagerBun,
		})
	}
	return out
}

// ParseUvToolList parses `uv tool list` lines (`ruff v0.8.0`), skipping `- executable` sub-rows.
func ParseUvToolList(raw string) []PackageEntry {
	var out []PackageEntry
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") || strings.HasPrefix(line, "-") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.EqualFold(trimmed, "No tools installed") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		name := fields[0]
		ver := strings.TrimPrefix(strings.TrimSpace(fields[1]), "v")
		out = append(out, PackageEntry{
			Manager: ManagerUv,
			ID:      name,
			Name:    name,
			Version: ver,
			Source:  "pypi",
		})
	}
	return out
}

func searchScoop(ctx context.Context, exe, baseDir, q string) ([]PackageEntry, error) {
	runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	// Prefer native scoop-search.exe when present (runs ~10x faster than PowerShell `scoop search`).
	if fastExe := findScoopSearchExe(baseDir); fastExe != "" {
		out, err := runCmdCapture(runCtx, tools.BuildMergedEnv(baseDir), fastExe, q)
		rows := ParseScoopSearchFast(out)
		if len(rows) > 0 || err == nil {
			return rows, nil
		}
	}

	out, err := runCmdCapture(runCtx, tools.BuildMergedEnv(baseDir), exe, "search", q)
	rows := ParseScoopSearch(out)
	if len(rows) == 0 && err != nil {
		return nil, fmt.Errorf("scoop search error: %w", err)
	}
	return rows, nil
}

func searchChoco(ctx context.Context, exe, baseDir, q string) ([]PackageEntry, error) {
	// Query Chocolatey NuGet v2 Search() over HTTPS first for popularity ordering, descriptions, and official icons.
	if rows, err := searchChocoOData(ctx, DefaultChocolateyFeedURL, q); err == nil {
		return rows, nil
	}

	runCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	out, err := runCmdCapture(runCtx, tools.BuildMergedEnv(baseDir), exe, "search", q, "-r", "--page-size=25")
	rows := ParseChocoPipeList(out)
	if len(rows) == 0 && err != nil {
		return nil, fmt.Errorf("choco search error: %w", err)
	}
	return rows, nil
}

func searchNpmRegistry(ctx context.Context, mgr, q string) ([]PackageEntry, error) {
	return searchNpmRegistryWithBaseURL(ctx, "https://registry.npmjs.org", mgr, q)
}

func searchNpmRegistryWithBaseURL(ctx context.Context, baseURL, mgr, q string) ([]PackageEntry, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/-/v1/search?text=" + url.QueryEscape(q) + "&size=25"
	var resp struct {
		Objects []struct {
			Package struct {
				Name        string `json:"name"`
				Version     string `json:"version"`
				Description string `json:"description"`
				Links       struct {
					Homepage   string `json:"homepage"`
					Repository string `json:"repository"`
					Npm        string `json:"npm"`
				} `json:"links"`
			} `json:"package"`
		} `json:"objects"`
	}
	if err := fetchJSON(ctx, endpoint, &resp); err != nil {
		return nil, err
	}
	out := make([]PackageEntry, 0, len(resp.Objects))
	for _, obj := range resp.Objects {
		p := obj.Package
		if strings.TrimSpace(p.Name) == "" {
			continue
		}
		hp := p.Links.Homepage
		if hp == "" {
			hp = p.Links.Npm
		}
		out = append(out, PackageEntry{
			Manager:     mgr,
			ID:          p.Name,
			Name:        p.Name,
			Version:     strings.TrimSpace(p.Version),
			Source:      mgr,
			Description: strings.TrimSpace(p.Description),
			Homepage:    strings.TrimSpace(hp),
		})
	}
	return out, nil
}

func searchPyPI(ctx context.Context, mgr, q string) ([]PackageEntry, error) {
	return searchPyPIWithBaseURL(ctx, "https://pypi.org/pypi", mgr, q)
}

func searchPyPIWithBaseURL(ctx context.Context, baseURL, mgr, q string) ([]PackageEntry, error) {
	slug := strings.TrimSpace(q)
	if slug == "" {
		return nil, nil
	}
	candidates := []string{slug}
	alt := strings.ReplaceAll(strings.ToLower(slug), " ", "-")
	if alt != strings.ToLower(slug) {
		candidates = append(candidates, alt)
	}

	var out []PackageEntry
	seen := make(map[string]bool)
	for _, cand := range candidates {
		info, err := fetchPyPIPackage(ctx, baseURL, cand)
		if err != nil {
			continue
		}
		key := strings.ToLower(info.Name)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, PackageEntry{
			Manager:     mgr,
			ID:          info.Name,
			Name:        info.Name,
			Version:     info.Version,
			Source:      "pypi",
			Description: info.Summary,
			Homepage:    info.HomePage,
			IconURL:     LookupIconMedia(mgr, info.Name, info.Name).Icon,
		})
	}
	return out, nil
}

type pypiPkgInfo struct {
	Name     string
	Version  string
	Summary  string
	HomePage string
}

func fetchPyPIPackage(ctx context.Context, baseURL, pkgName string) (pypiPkgInfo, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(pkgName) + "/json"
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return pypiPkgInfo{}, err
	}
	req.Header.Set("User-Agent", "UniDevUI/1.0 (Package Manager Hub)")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return pypiPkgInfo{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return pypiPkgInfo{}, fmt.Errorf("HTTP %d from %s", resp.StatusCode, endpoint)
	}
	return decodePyPIInfoOnly(io.LimitReader(resp.Body, 16*1024*1024))
}

func decodePyPIInfoOnly(r io.Reader) (pypiPkgInfo, error) {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return pypiPkgInfo{}, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return pypiPkgInfo{}, fmt.Errorf("expected JSON object from PyPI")
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return pypiPkgInfo{}, err
		}
		key, _ := keyTok.(string)
		if key == "info" {
			var info struct {
				Name     string `json:"name"`
				Version  string `json:"version"`
				Summary  string `json:"summary"`
				HomePage string `json:"home_page"`
				PkgURL   string `json:"package_url"`
			}
			if err := dec.Decode(&info); err != nil {
				return pypiPkgInfo{}, err
			}
			hp := strings.TrimSpace(info.HomePage)
			if hp == "" {
				hp = strings.TrimSpace(info.PkgURL)
			}
			return pypiPkgInfo{
				Name:     strings.TrimSpace(info.Name),
				Version:  strings.TrimSpace(info.Version),
				Summary:  strings.TrimSpace(info.Summary),
				HomePage: hp,
			}, nil
		}
		var discard json.RawMessage
		if err := dec.Decode(&discard); err != nil {
			return pypiPkgInfo{}, err
		}
	}
	return pypiPkgInfo{}, fmt.Errorf("missing info field in PyPI response")
}

func enrichNpmLatestConcurrently(ctx context.Context, rows []PackageEntry, baseURL string) {
	if len(rows) == 0 {
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := range rows {
		idx := i
		pkgName := rows[idx].ID
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			// Scoped npm names use %2f or raw @scope/name in registry/<name>/latest.
			endpoint := strings.TrimRight(baseURL, "/") + "/" + pkgName + "/latest"
			var resp struct {
				Version     string `json:"version"`
				Description string `json:"description"`
				Homepage    string `json:"homepage"`
			}
			if err := fetchJSON(ctx, endpoint, &resp); err != nil {
				return
			}
			latest := strings.TrimSpace(resp.Version)
			if rows[idx].Description == "" {
				rows[idx].Description = strings.TrimSpace(resp.Description)
			}
			if rows[idx].Homepage == "" {
				rows[idx].Homepage = strings.TrimSpace(resp.Homepage)
			}
			if latest != "" && rows[idx].Version != "" && pathmgr.CompareSemver(latest, rows[idx].Version) > 0 {
				rows[idx].AvailableVer = latest
				rows[idx].HasUpdate = true
			}
		}()
	}
	wg.Wait()
}

func enrichPyPILatestConcurrently(ctx context.Context, rows []PackageEntry, baseURL string) {
	if len(rows) == 0 {
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := range rows {
		idx := i
		pkgName := rows[idx].ID
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			info, err := fetchPyPIPackage(ctx, baseURL, pkgName)
			if err != nil {
				return
			}
			if rows[idx].Description == "" {
				rows[idx].Description = info.Summary
			}
			if rows[idx].Homepage == "" {
				rows[idx].Homepage = info.HomePage
			}
			if info.Version != "" && rows[idx].Version != "" && pathmgr.CompareSemver(info.Version, rows[idx].Version) > 0 {
				rows[idx].AvailableVer = info.Version
				rows[idx].HasUpdate = true
			}
		}()
	}
	wg.Wait()
}

func fetchJSON(ctx context.Context, endpoint string, dst any) error {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "UniDevUI/1.0 (Package Manager Hub)")
	req.Header.Set("Accept", "application/json")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d from %s", resp.StatusCode, endpoint)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}
