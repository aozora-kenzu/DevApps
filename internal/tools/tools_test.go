package tools

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestComputeToolStatusAndNormalize(t *testing.T) {
	if got := normalizeToolVer("tauri-cli 2.12.6"); got != "2.12.6" {
		t.Fatalf("expected 2.12.6, got %q", got)
	}
	if got := normalizeToolVer("v2.17.1"); got != "2.17.1" {
		t.Fatalf("expected 2.17.1, got %q", got)
	}
	if got := normalizeToolVer("bun-v1.4.5"); got != "1.4.5" {
		t.Fatalf("expected 1.4.5, got %q", got)
	}
	scoopOut := "Current Scoop version:\nv0.6.0 - Released at 2026-09-30\n\n'main' bucket:\nabc123\n"
	if got := parseScoopVersion(scoopOut); got != "0.6.0" {
		t.Fatalf("expected scoop version 0.6.0, got %q", got)
	}

	st := ToolStatus{Installed: true, RawVersion: "2.17.0", LatestVersion: "2.17.1"}
	if got := computeToolStatus(&st); got != "outdated" {
		t.Fatalf("expected outdated, got %q", got)
	}
	st.RawVersion = "2.17.1"
	if got := computeToolStatus(&st); got != "uptodate" {
		t.Fatalf("expected uptodate, got %q", got)
	}
}

func TestExtractNamedExesFromZip(t *testing.T) {
	tmpDir := t.TempDir()
	zipPath := filepath.Join(tmpDir, "bun-windows-x64.zip")

	zf, err := os.Create(zipPath)
	if err != nil {
		t.Fatalf("create zip: %v", err)
	}
	zw := zip.NewWriter(zf)
	w, err := zw.Create("bun-windows-x64/bun.exe")
	if err != nil {
		t.Fatalf("create entry: %v", err)
	}
	if _, err := w.Write([]byte("MZ-dummy-bun")); err != nil {
		t.Fatalf("write entry: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close zw: %v", err)
	}
	_ = zf.Close()

	destDir := filepath.Join(tmpDir, "out")
	if err := extractNamedExesFromZip(zipPath, destDir, map[string]bool{"bun.exe": true}); err != nil {
		t.Fatalf("extractNamedExesFromZip failed: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(destDir, "bun.exe"))
	if err != nil || string(got) != "MZ-dummy-bun" {
		t.Fatalf("unexpected extracted exe: err=%v got=%q", err, string(got))
	}
}

func TestParseWingetShowAndExitCodeAndStderrVersion(t *testing.T) {
	rawShow := "Found Microsoft Edge WebView2 Runtime [Microsoft.EdgeWebView2Runtime]\nVersion: 158.0.4370.33\nInstaller:\n  Installer Type: exe\n  Installer Url: https://example.com/MicrosoftEdgeWebView2RuntimeInstallerX64.exe\n"
	ver, urlStr := parseWingetShowVersionAndURL(rawShow)
	if ver != "158.0.4370.33" || urlStr != "https://example.com/MicrosoftEdgeWebView2RuntimeInstallerX64.exe" {
		t.Fatalf("unexpected parseWingetShowVersionAndURL result: ver=%q url=%q", ver, urlStr)
	}

	if !isExitCode(errors.New("command failed: exit status 0x80040828"), 0x80040828) {
		t.Fatal("expected 0x80040828 hex string match")
	}
	if !isExitCode(fmt.Errorf("exit code %d", 3010), 3010) {
		t.Fatal("expected 3010 decimal match")
	}

	// wails3 outputs its version on stderr; runVersionCmd should capture stderr cleanly.
	if got := runVersionCmd("cmd.exe", "/c", "echo v3.1.0 1>&2"); got != "v3.1.0" {
		t.Fatalf("expected stderr version v3.1.0, got %q", got)
	}
}

func TestFindPipAndPythonDetectsBatchWrappers(t *testing.T) {
	baseDir := t.TempDir()
	pyDir := filepath.Join(baseDir, "python", "3.14.10")
	if err := os.MkdirAll(pyDir, 0o755); err != nil {
		t.Fatalf("mkdir pyDir: %v", err)
	}
	pyExe := filepath.Join(pyDir, "python.exe")
	pipBat := filepath.Join(pyDir, "pip.bat")
	if err := os.WriteFile(pyExe, []byte("MZ-dummy-python"), 0o644); err != nil {
		t.Fatalf("write python.exe: %v", err)
	}
	if err := os.WriteFile(pipBat, []byte("@echo off\r\necho pip 26.3 from %~dp0\r\n"), 0o644); err != nil {
		t.Fatalf("write pip.bat: %v", err)
	}

	// Isolate PATH to a temp dir with a 0-byte WindowsApps stub and no real python/pip so fallback paths are exercised.
	stubDir := filepath.Join(t.TempDir(), "Microsoft", "WindowsApps")
	if err := os.MkdirAll(stubDir, 0o755); err != nil {
		t.Fatalf("mkdir stubDir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stubDir, "python.exe"), nil, 0o644); err != nil {
		t.Fatalf("write stub python.exe: %v", err)
	}
	emptyLocal := t.TempDir()
	emptyPF := t.TempDir()

	if gotPy := findPythonExeIn(stubDir, baseDir, emptyLocal, emptyPF); gotPy != pyExe {
		t.Fatalf("expected findPythonExeIn to skip WindowsApps stub and return %q, got %q", pyExe, gotPy)
	}
	if gotPip := findPipExeIn(stubDir, baseDir, emptyLocal, emptyPF); gotPip != pipBat {
		t.Fatalf("expected findPipExeIn to detect %q, got %q", pipBat, gotPip)
	}
	if gotPathPip := findPipExeIn(pyDir, filepath.Join(baseDir, "other"), emptyLocal, emptyPF); gotPathPip != pipBat {
		t.Fatalf("expected findPipExeIn on PATH to detect %q, got %q", pipBat, gotPathPip)
	}
	if ver := normalizeToolVer(runVersionCmd(pipBat, "--version")); ver != "26.3" {
		t.Fatalf("expected pip.bat version 26.3, got %q", ver)
	}
}

func TestLiveToolLatestVersionsIfEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live tool version test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	res := RefreshLatestToolVersions(ctx, func(lvl, en, id string) {
		t.Logf("[%s] %s", lvl, en)
	})
	for _, id := range []string{"wails2", "wails3", "tauri", "vsbuildtools", "webview2", "scoop", "choco", "npm", "pnpm", "bun", "uv"} {
		if res[id] == "" {
			t.Errorf("missing latest version for tool %s", id)
		} else {
			t.Logf("%-12s -> %s", id, res[id])
		}
	}
}
