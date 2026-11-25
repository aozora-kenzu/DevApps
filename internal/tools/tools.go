package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/windows/registry"

	"DevApp/internal/archive"
	"DevApp/internal/pathmgr"
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
		return installVSBuildTools(ctx, baseDir, mode, log)
	case "webview2":
		return installWebView2(ctx, baseDir, mode, log)
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

func fetchToolJSON(ctx context.Context, url string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "DevApp/1.0 (Version Check)")
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

func installVSBuildTools(ctx context.Context, baseDir, mode string, log Logger) error {
	override := "--wait --passive --add Microsoft.VisualStudio.Workload.VCTools --includeRecommended"
	if wingetExe, err := exec.LookPath("winget.exe"); err == nil {
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
	installer, err := archive.DownloadFile(ctx, vsBootstrapURL, cacheDir, nil, func(lvl, en, id string) { log(lvl, en, id) })
	if err != nil {
		return err
	}
	bootArgs := []string{"--wait", "--passive", "--add", "Microsoft.VisualStudio.Workload.VCTools", "--includeRecommended"}
	if mode == "fix" {
		bootArgs = []string{"--repair", "--wait", "--passive"}
	} else if mode == "update" {
		bootArgs = []string{"--update", "--wait", "--passive"}
	}
	cmdDesc := fmt.Sprintf("Running %s %s", installer, strings.Join(bootArgs, " "))
	cmdDescID := fmt.Sprintf("Menjalankan %s %s", installer, strings.Join(bootArgs, " "))
	log("CMD", cmdDesc, cmdDescID)
	return runStreamingCmd(ctx, buildMergedEnv(baseDir), log, installer, bootArgs...)
}

func installWebView2(ctx context.Context, baseDir, mode string, log Logger) error {
	if wingetExe, err := exec.LookPath("winget.exe"); err == nil {
		subCmd := "install"
		args := []string{"install", "--id", webview2WingetID, "-e", "--accept-source-agreements", "--accept-package-agreements"}
		if mode == "update" {
			subCmd = "upgrade"
			args = []string{"upgrade", "--id", webview2WingetID, "-e", "--accept-source-agreements", "--accept-package-agreements"}
		} else if mode == "fix" {
			args = []string{"install", "--id", webview2WingetID, "-e", "--force", "--accept-source-agreements", "--accept-package-agreements"}
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
	installer, err := archive.DownloadFile(ctx, webview2BootstrapURL, cacheDir, nil, func(lvl, en, id string) { log(lvl, en, id) })
	if err != nil {
		return err
	}
	log("CMD",
		fmt.Sprintf("Running %s /silent /install", installer),
		fmt.Sprintf("Menjalankan %s /silent /install", installer),
	)
	return runStreamingCmd(ctx, buildMergedEnv(baseDir), log, installer, "/silent", "/install")
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
	out, err := cmd.Output()
	if err != nil {
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
