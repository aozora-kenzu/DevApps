package tools

import (
	"context"
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
