package pkgmgr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIgnoreKeyAndSplitIgnoreKey(t *testing.T) {
	if got := IgnoreKey("winget", "Git.Git"); got != "Git.Git" {
		t.Fatalf("winget ignore key mismatch: %q", got)
	}
	if got := IgnoreKey("scoop", "git"); got != "scoop:git" {
		t.Fatalf("scoop ignore key mismatch: %q", got)
	}
	mgr, raw := SplitIgnoreKey("npm:@biomejs/biome", "")
	if mgr != "npm" || raw != "@biomejs/biome" {
		t.Fatalf("SplitIgnoreKey got mgr=%q raw=%q", mgr, raw)
	}
	mgrW, rawW := SplitIgnoreKey("Mozilla.Firefox", "winget")
	if mgrW != "winget" || rawW != "Mozilla.Firefox" {
		t.Fatalf("SplitIgnoreKey winget got mgr=%q raw=%q", mgrW, rawW)
	}
}

func TestParseScoopExportStatusAndSearch(t *testing.T) {
	exportJSON := `{
		"buckets": [{"Name": "main"}],
		"apps": [
			{"Name": "aria2", "Version": "1.37.0", "Source": "main"},
			{"Name": "jq", "Version": "1.7", "Source": "main"}
		]
	}`
	rows := ParseScoopExport(exportJSON)
	if len(rows) != 2 || rows[0].ID != "aria2" || rows[0].Version != "1.37.0" {
		t.Fatalf("unexpected ParseScoopExport: %+v", rows)
	}

	statusOut := `
Name   Installed Version Latest Version Missing Dependencies Info
----   ----------------- -------------- -------------------- ----
jq     1.7               1.7.1
`
	updates := ParseScoopStatus(statusOut)
	if updates["jq"] != "1.7.1" {
		t.Fatalf("expected jq update 1.7.1, got %+v", updates)
	}

	searchOut := `Results from local buckets...

Name                        Version      Source Binaries
----                        -------      ------ --------
git-cliff                   2.14.2       main   git-cliff.exe
`
	sRows := ParseScoopSearch(searchOut)
	if len(sRows) != 1 || sRows[0].ID != "git-cliff" || sRows[0].Source != "main" {
		t.Fatalf("unexpected ParseScoopSearch: %+v", sRows)
	}
}

func TestParseChocoNpmPnpmBunUv(t *testing.T) {
	chRows := ParseChocoPipeList("chocolatey|2.7.5\naria2|1.37.0\n")
	if len(chRows) != 2 || chRows[1].ID != "aria2" {
		t.Fatalf("unexpected choco rows: %+v", chRows)
	}
	chUpd := ParseChocoOutdated("aria2|1.37.0|1.38.0|false\npinnedpkg|1.0|2.0|true\n")
	if chUpd["aria2"] != "1.38.0" || chUpd["pinnedpkg"] != "" {
		t.Fatalf("unexpected choco outdated: %+v", chUpd)
	}

	npmRows := ParseNpmGlobalJSON(`{"dependencies":{"npm":{"version":"12.2.3"},"typescript":{"version":"5.7.2"}}}`)
	if len(npmRows) != 2 || npmRows[1].ID != "typescript" {
		t.Fatalf("unexpected npm rows: %+v", npmRows)
	}
	npmUpd := ParseNpmOutdatedJSON(`{"typescript":{"current":"5.7.2","latest":"5.8.0"}}`)
	if npmUpd["typescript"] != "5.8.0" {
		t.Fatalf("unexpected npm outdated: %+v", npmUpd)
	}

	pnpmRows := ParsePnpmGlobalJSON(`[{"dependencies":{"turbo":{"version":"2.4.0"}}}]`)
	if len(pnpmRows) != 1 || pnpmRows[0].ID != "turbo" || pnpmRows[0].Manager != "pnpm" {
		t.Fatalf("unexpected pnpm rows: %+v", pnpmRows)
	}

	bunOut := "/home/user/.bun/install/global/node_modules\n├── @biomejs/biome@1.9.4\n└── cowsay@1.6.0\n"
	bunRows := ParseBunGlobalList(bunOut)
	if len(bunRows) != 2 || bunRows[0].ID != "@biomejs/biome" || bunRows[0].Version != "1.9.4" {
		t.Fatalf("unexpected bun rows: %+v", bunRows)
	}

	uvOut := "ruff v0.8.0\n- ruff.exe\nyt-dlp v2026.1.1\n- yt-dlp.exe\n"
	uvRows := ParseUvToolList(uvOut)
	if len(uvRows) != 2 || uvRows[0].ID != "ruff" || uvRows[0].Version != "0.8.0" || uvRows[1].ID != "yt-dlp" {
		t.Fatalf("unexpected uv rows: %+v", uvRows)
	}
}

func TestSearchNpmRegistryAndPyPIEnrichment(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/-/v1/search":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"objects": []map[string]any{
					{
						"package": map[string]any{
							"name":        "typescript",
							"version":     "5.8.0",
							"description": "TypeScript compiler",
							"links":       map[string]string{"homepage": "https://www.typescriptlang.org/"},
						},
					},
				},
			})
		case "/ruff/json":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"info": map[string]any{
					"name":      "ruff",
					"version":   "0.9.1",
					"summary":   "An extremely fast Python linter",
					"home_page": "https://docs.astral.sh/ruff",
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	npmRes, err := searchNpmRegistryWithBaseURL(context.Background(), srv.URL, ManagerNpm, "typescript")
	if err != nil || len(npmRes) != 1 || npmRes[0].Version != "5.8.0" {
		t.Fatalf("unexpected npm search: err=%v rows=%+v", err, npmRes)
	}

	pyRows := []PackageEntry{{Manager: ManagerUv, ID: "ruff", Name: "ruff", Version: "0.8.0"}}
	enrichPyPILatestConcurrently(context.Background(), pyRows, srv.URL)
	if !pyRows[0].HasUpdate || pyRows[0].AvailableVer != "0.9.1" {
		t.Fatalf("expected PyPI update 0.9.1 detected, got %+v", pyRows[0])
	}
}

func TestAnnotateInstalledAcrossManagersDeduplicatedJS(t *testing.T) {
	rows := []PackageEntry{
		{Manager: ManagerNpm, ID: "typescript", Name: "typescript", Version: "5.8.0"},
		{Manager: ManagerNpm, ID: "vite", Name: "vite", Version: "6.1.0"},
	}
	installed := []PackageEntry{
		{Manager: ManagerPnpm, ID: "typescript", Name: "typescript", Version: "5.7.2"},
		{Manager: ManagerScoop, ID: "vite", Name: "vite", Version: "6.0.0"},
	}

	out := AnnotateInstalledAcrossManagers(rows, []string{ManagerNpm, ManagerPnpm, ManagerBun}, installed)
	if !out[0].Installed || out[0].Manager != ManagerPnpm || out[0].InstalledVer != "5.7.2" || !out[0].HasUpdate || out[0].AvailableVer != "5.8.0" {
		t.Fatalf("expected pnpm typescript installed with update 5.8.0, got %+v", out[0])
	}
	if out[1].Installed {
		t.Fatalf("scoop vite must not mark npm vite as installed: %+v", out[1])
	}
}

func TestLiveQueryInstalledAndSearchIfEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live multi-manager host scan & search test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	envs, rows := QueryAllInstalled(ctx, `C:\AppPath`)
	for _, env := range envs {
		t.Logf("Manager %-10s available=%v ver=%q exe=%q", env.ID, env.Available, env.Version, env.ExePath)
	}
	t.Logf("Total installed entries across extra managers: %d", len(rows))

	npmRes, err := SearchPackages(ctx, ManagerNpm, "typescript", `C:\AppPath`, rows)
	if err != nil || len(npmRes) == 0 {
		t.Fatalf("live npm SearchPackages failed: err=%v len=%d", err, len(npmRes))
	}
	t.Logf("Live npm search first hit: %s v%s", npmRes[0].ID, npmRes[0].Version)

	pyRes, err := SearchPackages(ctx, ManagerUv, "ruff", `C:\AppPath`, rows)
	if err != nil || len(pyRes) == 0 {
		t.Fatalf("live PyPI SearchPackages failed: err=%v len=%d", err, len(pyRes))
	}
	t.Logf("Live uv/PyPI hit: %s v%s", pyRes[0].ID, pyRes[0].Version)
}
