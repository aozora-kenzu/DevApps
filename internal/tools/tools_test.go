package tools

import (
	"context"
	"errors"
	"fmt"
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

	st := ToolStatus{Installed: true, RawVersion: "2.17.0", LatestVersion: "2.17.1"}
	if got := computeToolStatus(&st); got != "outdated" {
		t.Fatalf("expected outdated, got %q", got)
	}
	st.RawVersion = "2.17.1"
	if got := computeToolStatus(&st); got != "uptodate" {
		t.Fatalf("expected uptodate, got %q", got)
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

func TestLiveToolLatestVersionsIfEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live tool version test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	res := RefreshLatestToolVersions(ctx, func(lvl, en, id string) {
		t.Logf("[%s] %s", lvl, en)
	})
	for _, id := range []string{"wails2", "wails3", "tauri", "vsbuildtools", "webview2"} {
		if res[id] == "" {
			t.Errorf("missing latest version for tool %s", id)
		} else {
			t.Logf("%-12s -> %s", id, res[id])
		}
	}
}
