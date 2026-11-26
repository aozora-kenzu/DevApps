package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"

	"github.com/aozora-kenzu/UniDevUI/internal/archive"
	"github.com/aozora-kenzu/UniDevUI/internal/pathmgr"
)

// ToolStatus represents the install state of a CLI or System SDK tool.
type ToolStatus struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Category      string `json:"category"` // "cli" | "sdk"
	Installed     bool   `json:"installed"`
	Version       string `json:"version"`
	RawVersion    string `json:"rawVersion"`
	LatestVersion string `json:"latestVersion"`
	VersionStatus string `json:"versionStatus"` // "uptodate" | "outdated" | "ahead" | "not_installed" | "unknown"
	Location      string `json:"location"`
	BinDir        string `json:"binDir,omitempty"`
	InPath        bool   `json:"inPath"`
	Description   string `json:"description"`
	CommandHint   string `json:"commandHint"`
	Requires      string `json:"requires,omitempty"`
	DownloadSize  int64  `json:"downloadSize"`
}

// Logger receives live bilingual streaming output lines from tool installations.
type Logger func(level, enMsg, idMsg string)

const (
	VSBootstrapURL   = "https://aka.ms/vs/17/release/vs_BuildTools.exe"
	WebView2WingetID = "Microsoft.EdgeWebView2Runtime"

	vsBuildToolsWingetID = "Microsoft.VisualStudio.2022.BuildTools"
	webview2WingetID     = WebView2WingetID
	vsBootstrapURL       = VSBootstrapURL
	webview2BootstrapURL = "https://go.microsoft.com/fwlink/p/?LinkId=2124703"
)

var (
	toolHTTPClient = &http.Client{Timeout: 15 * time.Second}
	reCleanVer     = regexp.MustCompile(`v?(\d+\.\d+(?:\.\d+)*)`)

	toolLatestMu    sync.RWMutex
	toolLatestCache = make(map[string]string)
)

// ListToolStatuses inspects current installation status for all supported CLI and System tools,
// enriched with any cached latest upstream versions.
func ListToolStatuses(baseDir string) []ToolStatus {
	list := []ToolStatus{
		detectWailsV2(),
		detectWailsV3(),
		detectTauriCLI(),
		detectVSBuildTools(),
		detectWebView2(),
	}

	toolLatestMu.RLock()
	for i := range list {
		if latest, ok := toolLatestCache[list[i].ID]; ok {
			list[i].LatestVersion = latest
		}
		list[i].VersionStatus = computeToolStatus(&list[i])
	}
	toolLatestMu.RUnlock()

	_ = baseDir
	return list
}

// RefreshLatestToolVersions fetches the latest upstream versions for all CLI and SDK tools concurrently
// and updates the in-memory cache.
func RefreshLatestToolVersions(ctx context.Context, log Logger) map[string]string {
	fetchers := map[string]func(context.Context) (string, error){
		"wails2": func(c context.Context) (string, error) {
			return fetchGoProxyLatest(c, "https://proxy.golang.org/github.com/wailsapp/wails/v2/@latest")
		},
		"wails3": func(c context.Context) (string, error) {
			return fetchGoProxyLatest(c, "https://proxy.golang.org/github.com/wailsapp/wails/v3/@latest")
		},
		"tauri":        fetchCratesTauriLatest,
		"vsbuildtools": fetchVSBuildToolsLatest,
		"webview2":     fetchWebView2Latest,
	}

	results := make(map[string]string)
	var mu sync.Mutex
	var wg sync.WaitGroup

	for id, fn := range fetchers {
		toolID := id
		fetchFn := fn
		wg.Add(1)
		go func() {
			defer wg.Done()
			ver, err := fetchFn(ctx)
			if err != nil {
				if log != nil {
					log("WARN",
						fmt.Sprintf("[%s] failed to check latest version: %v", toolID, err),
						fmt.Sprintf("[%s] gagal cek versi terbaru: %v", toolID, err),
					)
				}
				return
			}
			ver = normalizeToolVer(ver)
			if ver == "" {
				return
			}
			mu.Lock()
			results[toolID] = ver
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(results) > 0 {
		toolLatestMu.Lock()
		for k, v := range results {
			toolLatestCache[k] = v
		}
		toolLatestMu.Unlock()
	}
	return results
}

func computeToolStatus(st *ToolStatus) string {
	if !st.Installed {
		return "not_installed"
	}
	if st.RawVersion == "" || st.LatestVersion == "" {
		return "unknown"
	}
	cmp := pathmgr.CompareSemver(st.RawVersion, st.LatestVersion)
	switch {
	case cmp == 0:
		return "uptodate"
	case cmp < 0:
		return "outdated"
	default:
		return "ahead"
	}
}

// InstallTool executes the requested tool installation ("install" | "update" | "fix") and streams live output.
func InstallTool(ctx context.Context, toolID, mode, baseDir string, log Logger) error {
	return InstallToolWithProgress(ctx, toolID, mode, baseDir, log, nil)
}

// InstallToolWithProgress executes the requested tool installation while forwarding download/install progress events.
func InstallToolWithProgress(ctx context.Context, toolID, mode, baseDir string, log Logger, onProgress archive.ProgressCallback) error {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "update", "fix":
	default:
		mode = "install"
	}
	switch toolID {
	case "wails2":
		return installGoTool(ctx, baseDir, mode, "github.com/wailsapp/wails/v2/cmd/wails@latest", "wails.exe", log)
	case "wails3":
		return installGoTool(ctx, baseDir, mode, "github.com/wailsapp/wails/v3/cmd/wails3@latest", "wails3.exe", log)
	case "tauri":
		return installCargoTauri(ctx, baseDir, mode, log)
	case "vsbuildtools":
		return installVSBuildTools(ctx, baseDir, mode, log, onProgress)
	case "webview2":
		return installWebView2(ctx, baseDir, mode, log, onProgress)
	default:
		return fmt.Errorf("unknown tool: %s", toolID)
	}
}

func detectWailsV2() ToolStatus {
	st := ToolStatus{
		ID:          "wails2",
		Name:        "Wails CLI v2",
		Category:    "cli",
		Description: "Build desktop Go + WebView2 apps (wails.exe)",
		CommandHint: "go install github.com/wailsapp/wails/v2/cmd/wails@latest",
		Requires:    "Go",
		InPath:      true,
	}
	if exe := findGoBinTool("wails.exe"); exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		st.Version = runVersionCmd(exe, "version")
		st.RawVersion = normalizeToolVer(st.Version)
	}
	return st
}

func detectWailsV3() ToolStatus {
	st := ToolStatus{
		ID:          "wails3",
		Name:        "Wails CLI v3",
		Category:    "cli",
		Description: "Next-gen Wails CLI (wails3.exe)",
		CommandHint: "go install github.com/wailsapp/wails/v3/cmd/wails3@latest",
		Requires:    "Go",
		InPath:      true,
	}
	if exe := findGoBinTool("wails3.exe"); exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		st.Version = runVersionCmd(exe, "version")
		st.RawVersion = normalizeToolVer(st.Version)
	}
	return st
}

func detectTauriCLI() ToolStatus {
	st := ToolStatus{
		ID:          "tauri",
		Name:        "Tauri CLI (cargo-tauri)",
		Category:    "cli",
		Description: "Build Rust + WebView2 desktop apps (cargo tauri)",
		CommandHint: "cargo install tauri-cli",
		Requires:    "Rust + VS Build Tools",
		InPath:      true,
	}
	if exe := findCargoBinTool("cargo-tauri.exe"); exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		st.Version = runVersionCmd(exe, "tauri", "--version")
		st.RawVersion = normalizeToolVer(st.Version)
	}
	return st
}

func detectVSBuildTools() ToolStatus {
	st := ToolStatus{
		ID:          "vsbuildtools",
		Name:        "Visual Studio Build Tools 2022",
		Category:    "sdk",
		Description: "MSVC compiler (cl.exe), linker & Windows SDK for Rust/CGo",
		CommandHint: "winget install Microsoft.VisualStudio.2022.BuildTools (VCTools)",
		InPath:      true,
	}
	vswhere := filepath.Join(os.Getenv("ProgramFiles(x86)"), "Microsoft Visual Studio", "Installer", "vswhere.exe")
	if fileExists(vswhere) {
		name := runVsWhere(vswhere, "displayName")
		if name != "" {
			st.Installed = true
			instVer := runVsWhere(vswhere, "installationVersion")
			dispVer := runVsWhere(vswhere, "catalog_productDisplayVersion")
			st.Location = runVsWhere(vswhere, "installationPath")
			st.RawVersion = normalizeToolVer(instVer)
			if dispVer != "" && instVer != "" {
				st.Version = fmt.Sprintf("%s (%s)", dispVer, instVer)
			} else if instVer != "" {
				st.Version = instVer
			} else {
				st.Version = name
			}
		}
	}
	return st
}

func runVsWhere(vswherePath, prop string) string {
	cmd := exec.Command(vswherePath, "-all", "-products", "*", "-requires", "Microsoft.VisualStudio.Component.VC.Tools.x86.x64", "-property", prop)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if t := strings.TrimSpace(line); t != "" {
			return t
		}
	}
	return ""
}

func detectWebView2() ToolStatus {
	st := ToolStatus{
		ID:          "webview2",
		Name:        "Microsoft Edge WebView2 Runtime",
		Category:    "sdk",
		Description: "Evergreen WebView2 runtime required by Wails & Tauri apps",
		CommandHint: "winget install Microsoft.EdgeWebView2Runtime",
		InPath:      true,
	}
	regLocations := []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
		{registry.CURRENT_USER, `Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}`},
	}
	for _, loc := range regLocations {
		k, err := registry.OpenKey(loc.root, loc.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		pv, _, vErr := k.GetStringValue("pv")
		_ = k.Close()
		if vErr == nil && strings.TrimSpace(pv) != "" {
			st.Installed = true
			st.Version = strings.TrimSpace(pv)
			st.RawVersion = normalizeToolVer(pv)
			st.Location = "Windows Registry (Evergreen)"
			return st
		}
	}
	return st
}

func fetchGoProxyLatest(ctx context.Context, url string) (string, error) {
	var resp struct {
		Version string `json:"Version"`
	}
	if err := fetchToolJSON(ctx, url, &resp); err != nil {
		return "", err
	}
	return resp.Version, nil
}

func fetchCratesTauriLatest(ctx context.Context) (string, error) {
	var resp struct {
		Crate struct {
			MaxStableVersion string `json:"max_stable_version"`
			MaxVersion       string `json:"max_version"`
		} `json:"crate"`
	}
	if err := fetchToolJSON(ctx, "https://crates.io/api/v1/crates/tauri-cli", &resp); err != nil {
		return "", err
	}
	if resp.Crate.MaxStableVersion != "" {
		return resp.Crate.MaxStableVersion, nil
	}
	return resp.Crate.MaxVersion, nil
}

func fetchVSBuildToolsLatest(ctx context.Context) (string, error) {
	var resp struct {
		ChannelItems []struct {
			ID      string `json:"id"`
			Version string `json:"version"`
		} `json:"channelItems"`
	}
	if err := fetchToolJSON(ctx, "https://aka.ms/vs/17/release/channel", &resp); err != nil {
		return "", err
	}
	for _, item := range resp.ChannelItems {
		if item.ID == "Microsoft.VisualStudio.Product.BuildTools" && item.Version != "" {
			return item.Version, nil
		}
	}
	return "", fmt.Errorf("BuildTools channel item not found")
}

func fetchWebView2Latest(ctx context.Context) (string, error) {
	// Query official Winget Evergreen manifest first so WebView2 Runtime isn't compared against Edge Browser versions.
	if ver, _, err := resolveWingetPackageShow(ctx, webview2WingetID); err == nil && ver != "" {
		return ver, nil
	}

	var products []struct {
		Product  string `json:"Product"`
		Releases []struct {
			Platform       string `json:"Platform"`
			Architecture   string `json:"Architecture"`
			ProductVersion string `json:"ProductVersion"`
		} `json:"Releases"`
	}
	if err := fetchToolJSON(ctx, "https://edgeupdates.microsoft.com/api/products?view=enterprise", &products); err != nil {
		return "", err
	}
	for _, p := range products {
		if p.Product != "Stable" {
			continue
		}
		for _, r := range p.Releases {
			if r.Platform == "Windows" && r.Architecture == "x64" && r.ProductVersion != "" {
				return r.ProductVersion, nil
			}
		}
	}
	return "", fmt.Errorf("WebView2 Stable Windows x64 release not found")
}

func resolveWingetPackageShow(ctx context.Context, wingetID string) (version string, installerURL string, err error) {
	wingetExe := findWingetExe()
	if wingetExe == "" {
		return "", "", fmt.Errorf("winget.exe not found")
	}
	runCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(runCtx, wingetExe, "show", "--id", wingetID, "-e", "--source", "winget", "--accept-source-agreements")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, cmdErr := cmd.CombinedOutput()
	ver, urlStr := parseWingetShowVersionAndURL(string(out))
	if ver == "" && urlStr == "" && cmdErr != nil {
		return "", "", cmdErr
	}
	return ver, urlStr, nil
}

func parseWingetShowVersionAndURL(raw string) (version string, installerURL string) {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		if idx := strings.LastIndex(line, "\r"); idx >= 0 {
			line = line[idx+1:]
		}
		trimmed := strings.TrimSpace(line)
		colon := strings.IndexByte(trimmed, ':')
		if colon <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(trimmed[:colon]))
		val := strings.TrimSpace(trimmed[colon+1:])
		switch key {
		case "version":
			if version == "" {
				version = val
			}
		case "installer url":
			if installerURL == "" {
				installerURL = val
			}
		}
	}
	return version, installerURL
}

func fetchToolJSON(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "UniDevUI/1.0 (Version Check)")
	resp, err := toolHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, dst)
}

func normalizeToolVer(raw string) string {
	m := reCleanVer.FindStringSubmatch(strings.TrimSpace(raw))
	if len(m) == 2 {
		return m[1]
	}
	return ""
}

func installGoTool(ctx context.Context, baseDir, mode, pkgSpec, binName string, log Logger) error {
	goExe := findExecutableWithFallback(baseDir, "go", "bin", "go.exe")
	if goExe == "" {
		return fmt.Errorf("go.exe not found in PATH or %s; install & activate Go first", filepath.Join(baseDir, "go"))
	}
	args := []string{"install", pkgSpec}
	if mode == "fix" {
		args = []string{"install", "-a", pkgSpec}
	}
	cmdLine := fmt.Sprintf("%s %s", goExe, strings.Join(args, " "))
	log("CMD", cmdLine, cmdLine)
	env := buildMergedEnv(baseDir)
	if err := runStreamingCmd(ctx, env, log, goExe, args...); err != nil {
		return err
	}

	home, _ := os.UserHomeDir()
	goBinDir := filepath.Join(home, "go", "bin")
	installedExe := filepath.Join(goBinDir, binName)
	log("OK",
		fmt.Sprintf("Installed at: %s", installedExe),
		fmt.Sprintf("Berhasil diinstal di: %s", installedExe),
	)
	if !pathContainsDir(pathmgr.EffectiveProcessPath(), goBinDir) {
		log("WARN",
			fmt.Sprintf("%s is not in PATH yet. Click Add to PATH if terminal cannot find '%s'.", goBinDir, strings.TrimSuffix(binName, ".exe")),
			fmt.Sprintf("%s belum ada di PATH. Klik tombol Add to PATH jika terminal belum mengenali '%s'.", goBinDir, strings.TrimSuffix(binName, ".exe")),
		)
	}
	return nil
}

func installCargoTauri(ctx context.Context, baseDir, mode string, log Logger) error {
	cargoExe := findExecutableWithFallback(baseDir, "rust", "bin", "cargo.exe")
	if cargoExe == "" {
		return fmt.Errorf("cargo.exe not found in PATH or %s; install & activate Rust first", filepath.Join(baseDir, "rust"))
	}
	args := []string{"install", "tauri-cli"}
	if mode == "fix" || mode == "update" {
		args = append(args, "--force")
	}
	log("CMD",
		fmt.Sprintf("%s %s (compiling crates may take several minutes)", cargoExe, strings.Join(args, " ")),
		fmt.Sprintf("%s %s (proses kompilasi mungkin memakan waktu beberapa menit)", cargoExe, strings.Join(args, " ")),
	)
	env := buildMergedEnv(baseDir)
	if err := runStreamingCmd(ctx, env, log, cargoExe, args...); err != nil {
		return err
	}
	home, _ := os.UserHomeDir()
	cargoBin := filepath.Join(home, ".cargo", "bin")
	installedExe := filepath.Join(cargoBin, "cargo-tauri.exe")
	log("OK",
		fmt.Sprintf("Tauri CLI installed at: %s", installedExe),
		fmt.Sprintf("Tauri CLI selesai diinstal di: %s", installedExe),
	)
	return nil
}

func installVSBuildTools(ctx context.Context, baseDir, mode string, log Logger, onProgress archive.ProgressCallback) error {
	override := "--wait --passive --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended"
	if wingetExe := findWingetExe(); wingetExe != "" {
		subCmd := "install"
		args := []string{"install", "--id", vsBuildToolsWingetID, "-e", "--accept-source-agreements", "--accept-package-agreements", "--override", override}
		if mode == "update" {
			subCmd = "upgrade"
			args = []string{"upgrade", "--id", vsBuildToolsWingetID, "-e", "--accept-source-agreements", "--accept-package-agreements", "--override", override}
		} else if mode == "fix" {
			args = []string{"install", "--id", vsBuildToolsWingetID, "-e", "--force", "--accept-source-agreements", "--accept-package-agreements", "--override", override}
		}
		log("CMD",
			fmt.Sprintf("winget %s Microsoft.VisualStudio.2022.BuildTools (VCTools Workload)...", subCmd),
			fmt.Sprintf("winget %s Microsoft.VisualStudio.2022.BuildTools (VCTools Workload)...", subCmd),
		)
		err := runStreamingCmd(ctx, buildMergedEnv(baseDir), log, wingetExe, args...)
		if err == nil {
			log("OK", "Visual Studio Build Tools 2022 completed.", "Visual Studio Build Tools 2022 selesai diproses.")
			return nil
		}
		log("WARN",
			fmt.Sprintf("winget %s failed (%v), downloading official Microsoft installer...", subCmd, err),
			fmt.Sprintf("winget %s gagal (%v), mengunduh installer resmi Microsoft...", subCmd, err),
		)
	}

	log("INFO",
		"Downloading vs_BuildTools.exe bootstrapper from Microsoft...",
		"Mengunduh bootstrapper vs_BuildTools.exe dari Microsoft...",
	)
	cacheDir := filepath.Join(baseDir, "_cache")
	installer, err := archive.DownloadFileNamed(ctx, vsBootstrapURL, cacheDir, "vs_BuildTools.exe", wrapToolProgress(onProgress, "VS Build Tools"), func(lvl, en, id string) { log(lvl, en, id) })
	if err != nil {
		return err
	}
	bootArgs := []string{"--wait", "--passive", "--add", "Microsoft.VisualStudio.Workload.VCTools", "--includeRecommended"}
	if mode == "fix" {
		bootArgs = []string{"--repair", "--wait", "--passive"}
	} else if mode == "update" {
		bootArgs = []string{"--update", "--wait", "--passive"}
	}
	if onProgress != nil {
		onProgress(archive.ProgressEvent{Stage: "install", Percent: -1, Detail: "VS Build Tools..."})
	}
	cmdDesc := fmt.Sprintf("Running %s %s", installer, strings.Join(bootArgs, " "))
	cmdDescID := fmt.Sprintf("Menjalankan %s %s", installer, strings.Join(bootArgs, " "))
	log("CMD", cmdDesc, cmdDescID)
	err = runInstallerWithElevationFallback(ctx, buildMergedEnv(baseDir), log, installer, bootArgs...)
	if isExitCode(err, 3010) {
		log("OK",
			"Visual Studio Build Tools completed; restart Windows to finish applying updates.",
			"Visual Studio Build Tools selesai; restart Windows untuk menyelesaikan pembaruan.",
		)
		return nil
	}
	return err
}

func installWebView2(ctx context.Context, baseDir, mode string, log Logger, onProgress archive.ProgressCallback) error {
	showVer, showURL, _ := resolveWingetPackageShow(ctx, webview2WingetID)
	if showVer != "" {
		cleanVer := normalizeToolVer(showVer)
		if cleanVer != "" {
			toolLatestMu.Lock()
			toolLatestCache["webview2"] = cleanVer
			toolLatestMu.Unlock()
		}
		if mode == "update" {
			cur := detectWebView2()
			if cur.Installed && cur.RawVersion != "" && cleanVer != "" && pathmgr.CompareSemver(cur.RawVersion, cleanVer) >= 0 {
				log("OK",
					fmt.Sprintf("WebView2 Runtime v%s is already up to date (latest Winget Evergreen: v%s).", cur.RawVersion, cleanVer),
					fmt.Sprintf("WebView2 Runtime v%s sudah versi terbaru (Winget Evergreen terbaru: v%s).", cur.RawVersion, cleanVer),
				)
				return nil
			}
		}
	}

	if wingetExe := findWingetExe(); wingetExe != "" {
		subCmd := "install"
		args := []string{"install", "--id", webview2WingetID, "-e", "--silent", "--accept-source-agreements", "--accept-package-agreements"}
		if mode == "update" {
			subCmd = "upgrade"
			args = []string{"upgrade", "--id", webview2WingetID, "-e", "--silent", "--accept-source-agreements", "--accept-package-agreements"}
		} else if mode == "fix" {
			args = []string{"install", "--id", webview2WingetID, "-e", "--force", "--silent", "--accept-source-agreements", "--accept-package-agreements"}
		}
		log("CMD",
			fmt.Sprintf("winget %s Microsoft.EdgeWebView2Runtime...", subCmd),
			fmt.Sprintf("winget %s Microsoft.EdgeWebView2Runtime...", subCmd),
		)
		err := runStreamingCmd(ctx, buildMergedEnv(baseDir), log, wingetExe, args...)
		if err == nil {
			log("OK", "WebView2 Runtime completed.", "WebView2 Runtime selesai diproses.")
			return nil
		}
		log("WARN",
			fmt.Sprintf("winget %s failed (%v), downloading official WebView2 installer...", subCmd, err),
			fmt.Sprintf("winget %s gagal (%v), mengunduh installer resmi WebView2...", subCmd, err),
		)
	}

	cacheDir := filepath.Join(baseDir, "_cache")
	// Remove any legacy extensionless "p" artifact created by earlier fwlink downloads without redirect resolution.
	_ = os.Remove(filepath.Join(cacheDir, "p"))

	dlURL := webview2BootstrapURL
	dlFile := "MicrosoftEdgeWebview2Setup.exe"
	dlMsgEN := "Downloading official MicrosoftEdgeWebview2Setup.exe (~1.8 MB bootstrapper)..."
	dlMsgID := "Mengunduh installer resmi MicrosoftEdgeWebview2Setup.exe (~1.8 MB bootstrapper)..."
	// Prefer the standalone offline installer when updating/repairing so EdgeUpdate doesn't reject with 0x80040828 ("already installed").
	if showURL != "" && (mode == "update" || mode == "fix" || detectWebView2().Installed) {
		dlURL = showURL
		dlFile = "MicrosoftEdgeWebView2RuntimeInstallerX64.exe"
		dlMsgEN = "Downloading official standalone WebView2 Runtime x64 installer..."
		dlMsgID = "Mengunduh installer standalone resmi WebView2 Runtime x64..."
	}

	log("INFO", dlMsgEN, dlMsgID)
	installer, err := archive.DownloadFileNamed(ctx, dlURL, cacheDir, dlFile, wrapToolProgress(onProgress, "WebView2"), func(lvl, en, id string) { log(lvl, en, id) })
	if err != nil {
		return err
	}
	if onProgress != nil {
		onProgress(archive.ProgressEvent{Stage: "install", Percent: -1, Detail: "Installing WebView2 Runtime..."})
	}
	log("CMD",
		fmt.Sprintf("Running %s /silent /install", installer),
		fmt.Sprintf("Menjalankan %s /silent /install", installer),
	)
	err = runInstallerWithElevationFallback(ctx, buildMergedEnv(baseDir), log, installer, "/silent", "/install")
	if err == nil {
		return nil
	}
	if isExitCode(err, 3010) {
		log("OK",
			"WebView2 update staged; restart UniDevUI or Windows to finish applying the new runtime version.",
			"Pembaruan WebView2 sudah disiapkan; restart UniDevUI atau Windows untuk menyelesaikan aktivasi versi baru.",
		)
		return nil
	}
	// EdgeUpdate bootstrapper returns 0x80040828 when an equal/newer Evergreen WebView2 Runtime is already present.
	if isExitCode(err, 0x80040828, -2147219416) {
		cur := detectWebView2()
		if cur.Installed && mode != "fix" {
			log("OK",
				fmt.Sprintf("WebView2 Runtime (v%s) is already installed on this machine (EdgeUpdate returned 0x80040828).", cur.RawVersion),
				fmt.Sprintf("WebView2 Runtime (v%s) sudah terpasang di sistem (EdgeUpdate kode 0x80040828).", cur.RawVersion),
			)
			return nil
		}
		return fmt.Errorf("EdgeUpdate reported WebView2 Runtime is already installed (0x80040828); repair via Windows Settings -> Installed Apps -> Microsoft Edge WebView2 Runtime -> Modify")
	}
	return err
}

func isExitCode(err error, targets ...int64) bool {
	if err == nil {
		return false
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code := int64(exitErr.ExitCode())
		uCode := int64(uint32(exitErr.ExitCode()))
		for _, t := range targets {
			if code == t || uCode == t || int64(uint32(t)) == uCode {
				return true
			}
		}
	}
	upper := strings.ToUpper(err.Error())
	for _, t := range targets {
		if t != 0 {
			hexUpper := fmt.Sprintf("0X%X", uint32(t))
			decStr := strconv.FormatInt(t, 10)
			if strings.Contains(upper, hexUpper) || strings.Contains(upper, decStr) {
				return true
			}
		}
	}
	return false
}

func wrapToolProgress(onProgress archive.ProgressCallback, detail string) archive.ProgressCallback {
	if onProgress == nil {
		return nil
	}
	return func(ev archive.ProgressEvent) {
		ev.Detail = detail
		onProgress(ev)
	}
}

func runInstallerWithElevationFallback(ctx context.Context, env []string, log Logger, installer string, args ...string) error {
	err := runStreamingCmd(ctx, env, log, installer, args...)
	if err == nil {
		return nil
	}
	if !errors.Is(err, windows.ERROR_ELEVATION_REQUIRED) && !strings.Contains(strings.ToLower(err.Error()), "requires elevation") {
		return err
	}
	log("WARN",
		"Installer requires Administrator elevation, launching UAC prompt...",
		"Installer membutuhkan hak Administrator, menampilkan prompt UAC...",
	)
	return runElevatedViaPowerShell(ctx, installer, args...)
}

func runElevatedViaPowerShell(ctx context.Context, installer string, args ...string) error {
	quotedArgs := make([]string, 0, len(args))
	for _, a := range args {
		quotedArgs = append(quotedArgs, fmt.Sprintf("'%s'", strings.ReplaceAll(a, "'", "''")))
	}
	argArrayLiteral := "@()"
	if len(quotedArgs) > 0 {
		argArrayLiteral = "@(" + strings.Join(quotedArgs, ", ") + ")"
	}
	psScript := fmt.Sprintf(
		"$p = Start-Process -FilePath '%s' -ArgumentList %s -Verb RunAs -Wait -PassThru; exit $p.ExitCode",
		strings.ReplaceAll(installer, "'", "''"),
		argArrayLiteral,
	)
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", psScript)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(out))
		if msg != "" {
			return fmt.Errorf("%w (%s)", err, msg)
		}
		return err
	}
	return nil
}

func runStreamingCmd(ctx context.Context, env []string, log Logger, exe string, args ...string) error {
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.Env = env
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start %s: %w", filepath.Base(exe), err)
	}

	var wg sync.WaitGroup
	stream := func(r io.Reader, level string) {
		defer wg.Done()
		sc := bufio.NewScanner(r)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			text := strings.TrimSpace(sc.Text())
			if text != "" {
				log(level, text, text)
			}
		}
	}

	wg.Add(2)
	go stream(stdout, "STDOUT")
	go stream(stderr, "STDERR")
	wg.Wait()

	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("command failed: %w", err)
	}
	return nil
}

func buildMergedEnv(baseDir string) []string {
	mergedPath := pathmgr.EffectiveProcessPath()
	extraDirs := findInstalledAppBinDirs(baseDir)
	if len(extraDirs) > 0 {
		mergedPath = strings.Join(extraDirs, ";") + ";" + mergedPath
	}
	env := os.Environ()
	out := make([]string, 0, len(env)+1)
	replaced := false
	for _, kv := range env {
		if len(kv) >= 5 && strings.EqualFold(kv[:5], "PATH=") {
			out = append(out, "PATH="+mergedPath)
			replaced = true
		} else {
			out = append(out, kv)
		}
	}
	if !replaced {
		out = append(out, "PATH="+mergedPath)
	}
	return out
}

func findInstalledAppBinDirs(baseDir string) []string {
	var dirs []string
	for _, spec := range []struct {
		app, sub, bin string
	}{
		{"go", "bin", "go.exe"},
		{"rust", "bin", "cargo.exe"},
		{"gcc", "bin", "gcc.exe"},
	} {
		matches, _ := filepath.Glob(filepath.Join(baseDir, spec.app, "*", spec.sub, spec.bin))
		for _, m := range matches {
			dirs = append(dirs, filepath.Dir(m))
		}
	}
	return dirs
}

func findExecutableWithFallback(baseDir, appID, subDir, binaryName string) string {
	for _, dir := range strings.Split(pathmgr.EffectiveProcessPath(), ";") {
		cand := filepath.Join(strings.TrimSpace(dir), binaryName)
		if fileExists(cand) {
			return cand
		}
	}
	home, _ := os.UserHomeDir()
	for _, extra := range []string{
		filepath.Join(home, "go", "bin", binaryName),
		filepath.Join(home, ".cargo", "bin", binaryName),
	} {
		if fileExists(extra) {
			return extra
		}
	}
	matches, _ := filepath.Glob(filepath.Join(baseDir, appID, "*", subDir, binaryName))
	if len(matches) > 0 {
		return matches[len(matches)-1]
	}
	return ""
}

func findGoBinTool(binName string) string {
	home, _ := os.UserHomeDir()
	cand := filepath.Join(home, "go", "bin", binName)
	if fileExists(cand) {
		return cand
	}
	return findInEffectivePath(binName)
}

func findCargoBinTool(binName string) string {
	home, _ := os.UserHomeDir()
	cand := filepath.Join(home, ".cargo", "bin", binName)
	if fileExists(cand) {
		return cand
	}
	return findInEffectivePath(binName)
}

func findInEffectivePath(binName string) string {
	for _, dir := range strings.Split(pathmgr.EffectiveProcessPath(), ";") {
		cand := filepath.Join(strings.TrimSpace(dir), binName)
		if fileExists(cand) {
			return cand
		}
	}
	return ""
}

func runVersionCmd(exe string, args ...string) string {
	cmd := exec.Command(exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil && len(bytesTrimSpace(out)) == 0 {
		return "Installed"
	}
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t != "" {
			return t
		}
	}
	return "Installed"
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

func findWingetExe() string {
	if p, err := exec.LookPath("winget.exe"); err == nil {
		return p
	}
	localAppData := os.Getenv("LOCALAPPDATA")
	if localAppData != "" {
		cand := filepath.Join(localAppData, "Microsoft", "WindowsApps", "winget.exe")
		if fileExists(cand) {
			return cand
		}
	}
	return ""
}

func pathContainsDir(pathEnv, dir string) bool {
	target := strings.TrimRight(strings.ToLower(filepath.Clean(dir)), `\`)
	for _, part := range strings.Split(pathEnv, ";") {
		norm := strings.TrimRight(strings.ToLower(filepath.Clean(strings.TrimSpace(part))), `\`)
		if norm == target {
			return true
		}
	}
	return false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
