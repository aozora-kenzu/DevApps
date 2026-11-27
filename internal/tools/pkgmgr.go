package tools

import (
	"archive/zip"
	"context"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/aozora-kenzu/UniDevUI/internal/archive"
	"github.com/aozora-kenzu/UniDevUI/internal/pathmgr"
)

const (
	bunWinZipURL      = "https://github.com/oven-sh/bun/releases/latest/download/bun-windows-x64.zip"
	uvWinZipURL       = "https://github.com/astral-sh/uv/releases/latest/download/uv-x86_64-pc-windows-msvc.zip"
	pnpmWinExeURL     = "https://github.com/pnpm/pnpm/releases/latest/download/pnpm-win-x64.exe"
	scoopInstallURL   = "https://get.scoop.sh"
	chocoInstallURL   = "https://community.chocolatey.org/install.ps1"
	statusCacheMaxAge = 20 * time.Second
)

var (
	statusCacheMu   sync.RWMutex
	cachedStatuses  []ToolStatus
	cachedStatusAt  time.Time
	cachedStatusDir string
)

// InvalidateStatusCache clears the short-lived local tool & package manager probe cache.
func InvalidateStatusCache() {
	statusCacheMu.Lock()
	cachedStatuses = nil
	cachedStatusAt = time.Time{}
	cachedStatusDir = ""
	statusCacheMu.Unlock()
}

func cloneToolStatuses(src []ToolStatus) []ToolStatus {
	out := make([]ToolStatus, len(src))
	copy(out, src)
	return out
}

func detectAllToolsConcurrently(baseDir string) []ToolStatus {
	detectors := []func() ToolStatus{
		detectWailsV2,
		detectWailsV3,
		detectTauriCLI,
		detectVSBuildTools,
		detectWebView2,
		detectScoop,
		detectChoco,
		func() ToolStatus { return detectNpm(baseDir) },
		func() ToolStatus { return detectPnpm(baseDir) },
		detectBun,
		detectUv,
		func() ToolStatus { return detectPip(baseDir) },
		func() ToolStatus { return detectCargo(baseDir) },
		func() ToolStatus { return detectDotnet(baseDir) },
		detectPSGallery,
	}

	results := make([]ToolStatus, len(detectors))
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for i, fn := range detectors {
		idx := i
		detectFn := fn
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			results[idx] = detectFn()
		}()
	}
	wg.Wait()
	return results
}

func pkgMgrLatestFetchers() map[string]func(context.Context) (string, error) {
	return map[string]func(context.Context) (string, error){
		"scoop": func(c context.Context) (string, error) {
			return fetchGitHubLatestTag(c, "https://api.github.com/repos/ScoopInstaller/Scoop/releases/latest")
		},
		"choco": func(c context.Context) (string, error) {
			return fetchGitHubLatestTag(c, "https://api.github.com/repos/chocolatey/choco/releases/latest")
		},
		"bun": func(c context.Context) (string, error) {
			return fetchGitHubLatestTag(c, "https://api.github.com/repos/oven-sh/bun/releases/latest")
		},
		"uv": func(c context.Context) (string, error) {
			return fetchGitHubLatestTag(c, "https://api.github.com/repos/astral-sh/uv/releases/latest")
		},
		"npm": func(c context.Context) (string, error) {
			return fetchNpmRegistryLatest(c, "https://registry.npmjs.org/npm/latest")
		},
		"pnpm": func(c context.Context) (string, error) {
			return fetchNpmRegistryLatest(c, "https://registry.npmjs.org/pnpm/latest")
		},
		"pip": func(c context.Context) (string, error) {
			var resp struct {
				Info struct {
					Version string `json:"version"`
				} `json:"info"`
			}
			if err := fetchToolJSON(c, "https://pypi.org/pypi/pip/json", &resp); err != nil {
				return "", err
			}
			return resp.Info.Version, nil
		},
		"psgallery": func(c context.Context) (string, error) {
			return fetchGitHubLatestTag(c, "https://api.github.com/repos/PowerShell/PowerShell/releases/latest")
		},
	}
}

func installPkgMgrTool(ctx context.Context, toolID, mode, baseDir string, log Logger, onProgress archive.ProgressCallback) (bool, error) {
	switch toolID {
	case "scoop":
		return true, installScoop(ctx, baseDir, mode, log)
	case "choco":
		return true, installChocolatey(ctx, baseDir, mode, log)
	case "npm":
		return true, installNpmCLI(ctx, baseDir, mode, log)
	case "pnpm":
		return true, installPnpmCLI(ctx, baseDir, mode, log, onProgress)
	case "bun":
		return true, installBunCLI(ctx, baseDir, mode, log, onProgress)
	case "uv":
		return true, installUvCLI(ctx, baseDir, mode, log, onProgress)
	case "pip":
		return true, installPipCLI(ctx, baseDir, mode, log)
	case "cargo":
		return true, installCargoCLI(ctx, baseDir, log)
	case "dotnet":
		return true, installDotnetCLI(ctx, baseDir, log)
	case "psgallery":
		return true, installPSGalleryCLI(ctx, baseDir, mode, log)
	default:
		return false, nil
	}
}

func detectScoop() ToolStatus {
	st := ToolStatus{
		ID:          "scoop",
		Name:        "Scoop",
		Category:    "pkgmgr",
		Description: "User-space Windows CLI package & bucket installer (no Admin needed)",
		CommandHint: "irm get.scoop.sh | iex",
		Requires:    "PowerShell + Git",
	}
	exe := findScoopCmd()
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		rawOut := runFullOutputCmd(exe, "--version")
		ver := parseScoopVersion(rawOut)
		if ver != "" {
			st.Version = "v" + ver
			st.RawVersion = ver
		} else {
			st.Version = "Installed"
		}
	}
	return st
}

func detectChoco() ToolStatus {
	st := ToolStatus{
		ID:          "choco",
		Name:        "Chocolatey",
		Category:    "pkgmgr",
		Description: "Classic Windows software automation & CLI package manager",
		CommandHint: "choco upgrade chocolatey -y",
		Requires:    "Admin (UAC)",
	}
	exe := findChocoExe()
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		verOut := runVersionCmd(exe, "--version")
		st.Version = verOut
		st.RawVersion = normalizeToolVer(verOut)
	}
	return st
}

func detectNpm(baseDir string) ToolStatus {
	st := ToolStatus{
		ID:          "npm",
		Name:        "npm",
		Category:    "pkgmgr",
		Description: "Node.js global package manager (bundled with Node.js)",
		CommandHint: "npm install -g npm@latest",
		Requires:    "Node.js",
	}
	exe := findNpmCmd(baseDir)
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		verOut := runVersionCmd(exe, "--version")
		st.Version = verOut
		st.RawVersion = normalizeToolVer(verOut)
	}
	return st
}

func detectPnpm(baseDir string) ToolStatus {
	st := ToolStatus{
		ID:          "pnpm",
		Name:        "pnpm",
		Category:    "pkgmgr",
		Description: "Fast, disk-space efficient Node.js package manager",
		CommandHint: "npm install -g pnpm@latest",
		Requires:    "Node.js (or Standalone)",
	}
	exe := findPnpmBin(baseDir)
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		verOut := runVersionCmd(exe, "--version")
		st.Version = verOut
		st.RawVersion = normalizeToolVer(verOut)
	}
	return st
}

func detectBun() ToolStatus {
	st := ToolStatus{
		ID:          "bun",
		Name:        "Bun",
		Category:    "pkgmgr",
		Description: "All-in-one ultra-fast JavaScript package manager & runtime",
		CommandHint: "bun.exe (GitHub official windows-x64 release)",
	}
	exe := findBunExe()
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		verOut := runVersionCmd(exe, "--version")
		st.Version = verOut
		st.RawVersion = normalizeToolVer(verOut)
	}
	return st
}

func detectUv() ToolStatus {
	st := ToolStatus{
		ID:          "uv",
		Name:        "uv (Astral)",
		Category:    "pkgmgr",
		Description: "Ultra-fast Python package, pipx & venv manager written in Rust",
		CommandHint: "uv.exe (GitHub official windows-msvc release)",
	}
	exe := findUvExe()
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		verOut := runVersionCmd(exe, "--version")
		st.Version = verOut
		st.RawVersion = normalizeToolVer(verOut)
	}
	return st
}

func detectPip(baseDir string) ToolStatus {
	st := ToolStatus{
		ID:          "pip",
		Name:        "pip (Python)",
		Category:    "pkgmgr",
		Description: "Standard Python package installer from PyPI (bundled with Python)",
		CommandHint: "python -m pip install --upgrade pip",
		Requires:    "Python",
	}
	exe := findPipExe(baseDir)
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		verOut := runVersionCmd(exe, "--version")
		st.Version = verOut
		st.RawVersion = normalizeToolVer(verOut)
	}
	return st
}

func detectCargo(baseDir string) ToolStatus {
	st := ToolStatus{
		ID:          "cargo",
		Name:        "Cargo (Rust)",
		Category:    "pkgmgr",
		Description: "Rust package & CLI binary manager from crates.io (bundled with Rust)",
		CommandHint: "cargo install <crate>",
		Requires:    "Rust Toolchain",
	}
	exe := findCargoExe(baseDir)
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		verOut := runVersionCmd(exe, "--version")
		st.Version = verOut
		st.RawVersion = normalizeToolVer(verOut)
	}
	return st
}

func detectDotnet(baseDir string) ToolStatus {
	st := ToolStatus{
		ID:          "dotnet",
		Name:        ".NET Tool",
		Category:    "pkgmgr",
		Description: "NuGet global CLI tools manager built into the .NET SDK",
		CommandHint: "dotnet tool install -g <tool>",
		Requires:    ".NET SDK",
	}
	exe := findDotnetExe(baseDir)
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		verOut := runVersionCmd(exe, "--version")
		st.Version = verOut
		st.RawVersion = normalizeToolVer(verOut)
	}
	return st
}

func detectPSGallery() ToolStatus {
	st := ToolStatus{
		ID:          "psgallery",
		Name:        "PSGallery (PS7)",
		Category:    "pkgmgr",
		Description: "PowerShell Gallery module & script manager powered by PowerShell 7",
		CommandHint: "Install-PSResource / Install-Module",
		Requires:    "PowerShell 7",
	}
	exe := findPwshExe()
	if exe != "" {
		st.Installed = true
		st.Location = exe
		st.BinDir = filepath.Dir(exe)
		st.InPath = pathContainsDir(pathmgr.EffectiveProcessPath(), st.BinDir)
		verOut := runVersionCmd(exe, "--version")
		st.Version = verOut
		st.RawVersion = normalizeToolVer(verOut)
	}
	return st
}

func installScoop(ctx context.Context, baseDir, mode string, log Logger) error {
	env := buildMergedEnv(baseDir)
	if scoopCmd := findScoopCmd(); scoopCmd != "" && mode == "update" {
		log("CMD",
			fmt.Sprintf("%s update", scoopCmd),
			fmt.Sprintf("%s update", scoopCmd),
		)
		return runStreamingCmd(ctx, env, log, scoopCmd, "update")
	}

	psScript := fmt.Sprintf("$ProgressPreference = 'SilentlyContinue'; Invoke-RestMethod -Uri '%s' | Invoke-Expression", scoopInstallURL)
	if windows.GetCurrentProcessToken().IsElevated() {
		psScript = fmt.Sprintf("$ProgressPreference = 'SilentlyContinue'; & ([scriptblock]::Create((Invoke-RestMethod -Uri '%s'))) -RunAsAdmin", scoopInstallURL)
	}
	log("CMD",
		"powershell -NoProfile -ExecutionPolicy Bypass (irm https://get.scoop.sh | iex)",
		"powershell -NoProfile -ExecutionPolicy Bypass (irm https://get.scoop.sh | iex)",
	)
	if err := runEncodedPowerShell(ctx, env, log, false, psScript); err != nil {
		return err
	}
	log("OK",
		"Scoop installed successfully.",
		"Scoop berhasil diinstal.",
	)
	return nil
}

func installChocolatey(ctx context.Context, baseDir, mode string, log Logger) error {
	env := buildMergedEnv(baseDir)
	if chocoExe := findChocoExe(); chocoExe != "" && mode == "update" {
		log("CMD",
			fmt.Sprintf("%s upgrade chocolatey -y --no-progress", chocoExe),
			fmt.Sprintf("%s upgrade chocolatey -y --no-progress", chocoExe),
		)
		return runInstallerWithElevationFallback(ctx, env, log, chocoExe, "upgrade", "chocolatey", "-y", "--no-progress")
	}

	psScript := fmt.Sprintf(
		"Set-ExecutionPolicy Bypass -Scope Process -Force; [System.Net.ServicePointManager]::SecurityProtocol = [System.Net.ServicePointManager]::SecurityProtocol -bor 3072; iex ((New-Object System.Net.WebClient).DownloadString('%s'))",
		chocoInstallURL,
	)
	elevate := !windows.GetCurrentProcessToken().IsElevated()
	if elevate {
		log("WARN",
			"Chocolatey installation requires Administrator elevation, launching UAC prompt...",
			"Instalasi Chocolatey membutuhkan hak Administrator, menampilkan prompt UAC...",
		)
	} else {
		log("CMD",
			"Installing Chocolatey via official install.ps1...",
			"Menginstal Chocolatey melalui install.ps1 resmi...",
		)
	}
	if err := runEncodedPowerShell(ctx, env, log, elevate, psScript); err != nil {
		return err
	}
	log("OK",
		"Chocolatey installation completed.",
		"Instalasi Chocolatey selesai.",
	)
	return nil
}

func installNpmCLI(ctx context.Context, baseDir, mode string, log Logger) error {
	npmCmd := findNpmCmd(baseDir)
	if npmCmd == "" {
		return fmt.Errorf("npm.cmd not found; download & activate Node.js in Core Runtimes first")
	}
	args := []string{"install", "-g", "npm@latest"}
	if mode == "fix" {
		args = []string{"install", "-g", "npm@latest", "--force"}
	}
	log("CMD",
		fmt.Sprintf("%s %s", npmCmd, strings.Join(args, " ")),
		fmt.Sprintf("%s %s", npmCmd, strings.Join(args, " ")),
	)
	if err := runStreamingCmd(ctx, buildMergedEnv(baseDir), log, npmCmd, args...); err != nil {
		return err
	}
	log("OK", "npm updated to latest release.", "npm selesai diperbarui ke rilis terbaru.")
	return nil
}

func installPnpmCLI(ctx context.Context, baseDir, mode string, log Logger, onProgress archive.ProgressCallback) error {
	if npmCmd := findNpmCmd(baseDir); npmCmd != "" && mode != "fix" {
		log("CMD",
			fmt.Sprintf("%s install -g pnpm@latest", npmCmd),
			fmt.Sprintf("%s install -g pnpm@latest", npmCmd),
		)
		if err := runStreamingCmd(ctx, buildMergedEnv(baseDir), log, npmCmd, "install", "-g", "pnpm@latest"); err == nil {
			log("OK", "pnpm installed/updated via npm.", "pnpm selesai dipasang/diperbarui lewat npm.")
			return nil
		}
		log("WARN",
			"npm global install failed, trying standalone pnpm-win-x64.exe binary...",
			"Install global via npm gagal, mencoba binary standalone pnpm-win-x64.exe...",
		)
	}

	targetDir := pnpmInstallDir()
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	cacheDir := filepath.Join(baseDir, "_cache")
	downloaded, err := archive.DownloadFileNamed(ctx, pnpmWinExeURL, cacheDir, "pnpm-win-x64.exe", wrapToolProgress(onProgress, "pnpm"), func(lvl, en, id string) { log(lvl, en, id) })
	if err != nil {
		return err
	}
	destExe := filepath.Join(targetDir, "pnpm.exe")
	if err := copyFileAtomic(downloaded, destExe); err != nil {
		return err
	}
	log("OK",
		fmt.Sprintf("pnpm standalone installed at: %s", destExe),
		fmt.Sprintf("pnpm standalone selesai dipasang di: %s", destExe),
	)
	return nil
}

func installBunCLI(ctx context.Context, baseDir, _ string, log Logger, onProgress archive.ProgressCallback) error {
	cacheDir := filepath.Join(baseDir, "_cache")
	log("INFO",
		"Downloading official Bun Windows x64 archive from GitHub...",
		"Mengunduh arsip resmi Bun Windows x64 dari GitHub...",
	)
	zipPath, err := archive.DownloadFileNamed(ctx, bunWinZipURL, cacheDir, "bun-windows-x64.zip", wrapToolProgress(onProgress, "Bun"), func(lvl, en, id string) { log(lvl, en, id) })
	if err != nil {
		return err
	}
	targetDir := bunInstallDir()
	if err := extractNamedExesFromZip(zipPath, targetDir, map[string]bool{"bun.exe": true}); err != nil {
		return err
	}
	log("OK",
		fmt.Sprintf("Bun installed at: %s", filepath.Join(targetDir, "bun.exe")),
		fmt.Sprintf("Bun selesai dipasang di: %s", filepath.Join(targetDir, "bun.exe")),
	)
	return nil
}

func installUvCLI(ctx context.Context, baseDir, _ string, log Logger, onProgress archive.ProgressCallback) error {
	cacheDir := filepath.Join(baseDir, "_cache")
	log("INFO",
		"Downloading official uv Windows x64 archive from GitHub (Astral)...",
		"Mengunduh arsip resmi uv Windows x64 dari GitHub (Astral)...",
	)
	zipPath, err := archive.DownloadFileNamed(ctx, uvWinZipURL, cacheDir, "uv-x86_64-pc-windows-msvc.zip", wrapToolProgress(onProgress, "uv"), func(lvl, en, id string) { log(lvl, en, id) })
	if err != nil {
		return err
	}
	targetDir := uvInstallDir()
	wanted := map[string]bool{"uv.exe": true, "uvx.exe": true, "uvw.exe": true}
	if err := extractNamedExesFromZip(zipPath, targetDir, wanted); err != nil {
		return err
	}
	log("OK",
		fmt.Sprintf("uv installed at: %s", filepath.Join(targetDir, "uv.exe")),
		fmt.Sprintf("uv selesai dipasang di: %s", filepath.Join(targetDir, "uv.exe")),
	)
	return nil
}

func installPipCLI(ctx context.Context, baseDir, mode string, log Logger) error {
	pyExe := findPythonExe(baseDir)
	if pipExe := findPipExe(baseDir); pipExe != "" && (mode == "update" || pyExe == "") {
		args := []string{"install", "--upgrade", "pip"}
		if mode == "fix" {
			args = append(args, "--force-reinstall")
		}
		cmdLine := fmt.Sprintf("%s %s", pipExe, strings.Join(args, " "))
		log("CMD", cmdLine, cmdLine)
		return runStreamingCmd(ctx, buildMergedEnv(baseDir), log, pipExe, args...)
	}
	if pyExe == "" {
		return fmt.Errorf("python.exe not found; download & activate Python in Core Runtimes first")
	}
	cmdLine := fmt.Sprintf("%s -m ensurepip --upgrade", pyExe)
	log("CMD", cmdLine, cmdLine)
	if err := runStreamingCmd(ctx, buildMergedEnv(baseDir), log, pyExe, "-m", "ensurepip", "--upgrade"); err != nil {
		return err
	}
	if err := runStreamingCmd(ctx, buildMergedEnv(baseDir), log, pyExe, "-m", "pip", "install", "--upgrade", "pip"); err != nil {
		return err
	}
	if findPipExe(baseDir) == "" {
		pipBat := filepath.Join(filepath.Dir(pyExe), "pip.bat")
		_ = os.WriteFile(pipBat, []byte("@echo off\r\n\"%~dp0python.exe\" -m pip %*\r\n"), 0o644)
	}
	return nil
}

func installCargoCLI(ctx context.Context, baseDir string, log Logger) error {
	if rustup := findRustupExe(baseDir); rustup != "" {
		log("CMD", rustup+" update", rustup+" update")
		return runStreamingCmd(ctx, buildMergedEnv(baseDir), log, rustup, "update")
	}
	return fmt.Errorf("cargo.exe comes with Rust Toolchain; install & activate Rust in Core Runtimes & Dev Stack first")
}

func installDotnetCLI(_ context.Context, baseDir string, _ Logger) error {
	if findDotnetExe(baseDir) != "" {
		return fmt.Errorf(".NET Tool CLI is bundled with .NET SDK; manage SDK versions from Core Runtimes & Dev Stack")
	}
	return fmt.Errorf("dotnet.exe not found; install & activate .NET SDK in Core Runtimes & Dev Stack first")
}

func installPSGalleryCLI(ctx context.Context, baseDir, mode string, log Logger) error {
	wingetExe := findWingetExe()
	if wingetExe == "" {
		return fmt.Errorf("winget.exe not found; install PowerShell 7 from Windows Apps or microsoft.com/powershell")
	}
	sub := "install"
	if mode == "update" {
		sub = "upgrade"
	}
	args := []string{sub, "--id", "Microsoft.PowerShell", "-e", "--silent", "--accept-package-agreements", "--accept-source-agreements"}
	cmdLine := "winget " + strings.Join(args, " ")
	log("CMD", cmdLine, cmdLine)
	return runStreamingCmd(ctx, buildMergedEnv(baseDir), log, wingetExe, args...)
}

func extractNamedExesFromZip(zipPath, destDir string, wantedBaseNames map[string]bool) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("failed opening %s: %w", filepath.Base(zipPath), err)
	}
	defer r.Close()

	extracted := 0
	for _, f := range r.File {
		if f.FileInfo().IsDir() {
			continue
		}
		base := strings.ToLower(filepath.Base(strings.ReplaceAll(f.Name, `\`, "/")))
		if !wantedBaseNames[base] {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		outPath := filepath.Join(destDir, filepath.Base(base))
		tmpPath := outPath + ".tmp"
		outFile, createErr := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if createErr != nil {
			rc.Close()
			return createErr
		}
		_, copyErr := io.Copy(outFile, rc)
		closeErr := outFile.Close()
		rc.Close()
		if copyErr != nil {
			_ = os.Remove(tmpPath)
			return copyErr
		}
		if closeErr != nil {
			_ = os.Remove(tmpPath)
			return closeErr
		}
		_ = os.Remove(outPath)
		if err := os.Rename(tmpPath, outPath); err != nil {
			_ = os.Remove(tmpPath)
			return err
		}
		extracted++
	}
	if extracted == 0 {
		return fmt.Errorf("no expected executables found inside %s", filepath.Base(zipPath))
	}
	return nil
}

func copyFileAtomic(srcPath, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	tmpPath := dstPath + ".tmp"
	dst, err := os.OpenFile(tmpPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dst, src); err != nil {
		dst.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := dst.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	_ = os.Remove(dstPath)
	if err := os.Rename(tmpPath, dstPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func runEncodedPowerShell(ctx context.Context, env []string, log Logger, elevate bool, script string) error {
	encoded := encodePSUTF16LE(script)
	if elevate {
		return runElevatedViaPowerShell(ctx, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encoded)
	}
	return runStreamingCmd(ctx, env, log, "powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encoded)
}

func encodePSUTF16LE(script string) string {
	u16 := utf16.Encode([]rune(script))
	raw := make([]byte, len(u16)*2)
	for i, v := range u16 {
		binary.LittleEndian.PutUint16(raw[i*2:], v)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

func fetchGitHubLatestTag(ctx context.Context, apiURL string) (string, error) {
	var resp struct {
		TagName string `json:"tag_name"`
	}
	if err := fetchToolJSON(ctx, apiURL, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.TagName) == "" {
		return "", fmt.Errorf("empty tag_name")
	}
	return resp.TagName, nil
}

func fetchNpmRegistryLatest(ctx context.Context, apiURL string) (string, error) {
	var resp struct {
		Version string `json:"version"`
	}
	if err := fetchToolJSON(ctx, apiURL, &resp); err != nil {
		return "", err
	}
	if strings.TrimSpace(resp.Version) == "" {
		return "", fmt.Errorf("empty version")
	}
	return resp.Version, nil
}

func parseScoopVersion(raw string) string {
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(strings.ToLower(trimmed), "current scoop version") {
			continue
		}
		if v := normalizeToolVer(trimmed); v != "" {
			return v
		}
	}
	return normalizeToolVer(raw)
}

func runFullOutputCmd(exe string, args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, _ := cmd.CombinedOutput()
	return strings.TrimSpace(string(out))
}

func findScoopCmd() string {
	if p := findInEffectivePath("scoop.cmd"); p != "" {
		return p
	}
	if root := strings.TrimSpace(os.Getenv("SCOOP")); root != "" {
		if cand := filepath.Join(root, "shims", "scoop.cmd"); fileExists(cand) {
			return cand
		}
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		if cand := filepath.Join(home, "scoop", "shims", "scoop.cmd"); fileExists(cand) {
			return cand
		}
	}
	return ""
}

func findChocoExe() string {
	if p := findInEffectivePath("choco.exe"); p != "" {
		return p
	}
	if root := strings.TrimSpace(os.Getenv("ChocolateyInstall")); root != "" {
		if cand := filepath.Join(root, "bin", "choco.exe"); fileExists(cand) {
			return cand
		}
	}
	if cand := `C:\ProgramData\chocolatey\bin\choco.exe`; fileExists(cand) {
		return cand
	}
	return ""
}

func findNpmCmd(baseDir string) string {
	if p := findInEffectivePath("npm.cmd"); p != "" {
		return p
	}
	return findExecutableWithFallback(baseDir, "node", "", "npm.cmd")
}

func findPnpmBin(baseDir string) string {
	for _, name := range []string{"pnpm.exe", "pnpm.cmd"} {
		if p := findInEffectivePath(name); p != "" {
			return p
		}
	}
	if cand := filepath.Join(pnpmInstallDir(), "pnpm.exe"); fileExists(cand) {
		return cand
	}
	if npmCmd := findNpmCmd(baseDir); npmCmd != "" {
		if cand := filepath.Join(filepath.Dir(npmCmd), "pnpm.cmd"); fileExists(cand) {
			return cand
		}
	}
	return ""
}

func findBunExe() string {
	if p := findInEffectivePath("bun.exe"); p != "" {
		return p
	}
	if cand := filepath.Join(bunInstallDir(), "bun.exe"); fileExists(cand) {
		return cand
	}
	return ""
}

func findUvExe() string {
	if p := findInEffectivePath("uv.exe"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	for _, dir := range []string{uvInstallDir(), filepath.Join(home, ".cargo", "bin")} {
		if cand := filepath.Join(dir, "uv.exe"); fileExists(cand) {
			return cand
		}
	}
	return ""
}

func bunInstallDir() string {
	if custom := strings.TrimSpace(os.Getenv("BUN_INSTALL")); custom != "" {
		return filepath.Join(custom, "bin")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".bun", "bin")
}

func uvInstallDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "bin")
}

func pnpmInstallDir() string {
	if local := strings.TrimSpace(os.Getenv("LOCALAPPDATA")); local != "" {
		return filepath.Join(local, "pnpm")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "AppData", "Local", "pnpm")
}

// FindPkgMgrExe locates the executable for a supported Tab 1/Tab 2 package manager.
func FindPkgMgrExe(managerID, baseDir string) string {
	switch strings.ToLower(strings.TrimSpace(managerID)) {
	case "scoop":
		return findScoopCmd()
	case "choco":
		return findChocoExe()
	case "npm":
		return findNpmCmd(baseDir)
	case "pnpm":
		return findPnpmBin(baseDir)
	case "bun":
		return findBunExe()
	case "uv":
		return findUvExe()
	case "pip":
		return findPipExe(baseDir)
	case "cargo":
		return findCargoExe(baseDir)
	case "dotnet":
		return findDotnetExe(baseDir)
	case "psgallery":
		return findPwshExe()
	default:
		return ""
	}
}

var pipBinaryNames = []string{"pip.exe", "pip.bat", "pip.cmd", "pip3.exe", "pip3.bat", "pip3.cmd"}

func isUsableBinary(path string) bool {
	info, err := os.Stat(path)
	if err != nil || info.IsDir() {
		return false
	}
	// Windows App Execution Aliases in WindowsApps are 0-byte reparse points that only prompt to install from MS Store.
	if info.Size() == 0 && strings.Contains(strings.ToLower(path), `\microsoft\windowsapps\`) {
		return false
	}
	return true
}

func firstUsableInDirs(dirs, names []string) string {
	for _, dir := range dirs {
		d := strings.TrimSpace(dir)
		if d == "" {
			continue
		}
		for _, name := range names {
			cand := filepath.Join(d, name)
			if isUsableBinary(cand) {
				return cand
			}
		}
	}
	return ""
}

func standardPythonDirs(localAppData, programFiles string) []string {
	var dirs []string
	if local := strings.TrimSpace(localAppData); local != "" {
		if matches, _ := filepath.Glob(filepath.Join(local, "Programs", "Python", "Python*")); len(matches) > 0 {
			for i := len(matches) - 1; i >= 0; i-- {
				dirs = append(dirs, matches[i])
			}
		}
	}
	if pf := strings.TrimSpace(programFiles); pf != "" {
		if matches, _ := filepath.Glob(filepath.Join(pf, "Python*")); len(matches) > 0 {
			for i := len(matches) - 1; i >= 0; i-- {
				dirs = append(dirs, matches[i])
			}
		}
	}
	return dirs
}

func findPythonExe(baseDir string) string {
	return findPythonExeIn(pathmgr.EffectiveProcessPath(), baseDir, os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"))
}

func findPythonExeIn(pathEnv, baseDir, localAppData, programFiles string) string {
	if p := firstUsableInDirs(strings.Split(pathEnv, ";"), []string{"python.exe", "python3.exe"}); p != "" {
		return p
	}
	var fallbackDirs []string
	if matches, _ := filepath.Glob(filepath.Join(baseDir, "python", "*")); len(matches) > 0 {
		for i := len(matches) - 1; i >= 0; i-- {
			fallbackDirs = append(fallbackDirs, matches[i])
		}
	}
	fallbackDirs = append(fallbackDirs, standardPythonDirs(localAppData, programFiles)...)
	return firstUsableInDirs(fallbackDirs, []string{"python.exe", "python3.exe"})
}

func findPipExe(baseDir string) string {
	return findPipExeIn(pathmgr.EffectiveProcessPath(), baseDir, os.Getenv("LOCALAPPDATA"), os.Getenv("ProgramFiles"))
}

func findPipExeIn(pathEnv, baseDir, localAppData, programFiles string) string {
	if p := firstUsableInDirs(strings.Split(pathEnv, ";"), pipBinaryNames); p != "" {
		return p
	}
	if py := findPythonExeIn(pathEnv, baseDir, localAppData, programFiles); py != "" {
		pyDir := filepath.Dir(py)
		if p := firstUsableInDirs([]string{filepath.Join(pyDir, "Scripts"), pyDir}, pipBinaryNames); p != "" {
			return p
		}
	}
	var candidateDirs []string
	if matches, _ := filepath.Glob(filepath.Join(baseDir, "python", "*")); len(matches) > 0 {
		for i := len(matches) - 1; i >= 0; i-- {
			candidateDirs = append(candidateDirs, filepath.Join(matches[i], "Scripts"), matches[i])
		}
	}
	for _, d := range standardPythonDirs(localAppData, programFiles) {
		candidateDirs = append(candidateDirs, filepath.Join(d, "Scripts"), d)
	}
	return firstUsableInDirs(candidateDirs, pipBinaryNames)
}

func findCargoExe(baseDir string) string {
	if p := findInEffectivePath("cargo.exe"); p != "" {
		return p
	}
	if ch := strings.TrimSpace(os.Getenv("CARGO_HOME")); ch != "" {
		if cand := filepath.Join(ch, "bin", "cargo.exe"); fileExists(cand) {
			return cand
		}
	}
	return findExecutableWithFallback(baseDir, "rust", "bin", "cargo.exe")
}

func findRustupExe(baseDir string) string {
	if p := findInEffectivePath("rustup.exe"); p != "" {
		return p
	}
	return findExecutableWithFallback(baseDir, "rust", "bin", "rustup.exe")
}

func findDotnetExe(baseDir string) string {
	if p := findInEffectivePath("dotnet.exe"); p != "" {
		return p
	}
	if pf := strings.TrimSpace(os.Getenv("ProgramFiles")); pf != "" {
		if cand := filepath.Join(pf, "dotnet", "dotnet.exe"); fileExists(cand) {
			return cand
		}
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		if cand := filepath.Join(home, ".dotnet", "dotnet.exe"); fileExists(cand) {
			return cand
		}
	}
	return findExecutableWithFallback(baseDir, "dotnet", "", "dotnet.exe")
}

func findPwshExe() string {
	if p := findInEffectivePath("pwsh.exe"); p != "" {
		return p
	}
	if pf := strings.TrimSpace(os.Getenv("ProgramFiles")); pf != "" {
		if cand := filepath.Join(pf, "PowerShell", "7", "pwsh.exe"); fileExists(cand) {
			return cand
		}
	}
	return ""
}

// BuildMergedEnv returns the current environment with Tab 1 portable runtime bin directories prepended to PATH.
func BuildMergedEnv(baseDir string) []string {
	return buildMergedEnv(baseDir)
}

// RunStreamingCmd executes exe with args and streams stdout/stderr lines to log.
func RunStreamingCmd(ctx context.Context, env []string, log Logger, exe string, args ...string) error {
	return runStreamingCmd(ctx, env, log, exe, args...)
}

// RunWithElevationFallback runs exe directly and falls back to UAC PowerShell elevation when Windows requires it.
func RunWithElevationFallback(ctx context.Context, env []string, log Logger, exe string, args ...string) error {
	return runInstallerWithElevationFallback(ctx, env, log, exe, args...)
}
