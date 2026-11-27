package pkgmgr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseCargoDotnetPipPSGalleryInstalledParsers(t *testing.T) {
	cargoRaw := `
ripgrep v14.1.0:
    rg.exe
fd-find v10.2.0:
    fd.exe
`
	cRows := ParseCargoInstallList(cargoRaw)
	if len(cRows) != 2 || cRows[0].ID != "ripgrep" || cRows[0].Version != "14.1.0" || cRows[1].ID != "fd-find" {
		t.Fatalf("unexpected cargo parsed list: %+v", cRows)
	}

	dotnetRaw := `
Package Id      Version      Commands
-------------------------------------------
dotnet-ef       10.0.0       dotnet-ef
csharpier       0.30.6       dotnet-csharpier
`
	dRows := ParseDotnetToolList(dotnetRaw)
	if len(dRows) != 2 || dRows[0].ID != "dotnet-ef" || dRows[0].Version != "10.0.0" || dRows[1].ID != "csharpier" {
		t.Fatalf("unexpected dotnet tool parsed list: %+v", dRows)
	}

	pipList := `[{"name":"pip","version":"24.3.1"},{"name":"ruff","version":"0.8.0"}]`
	pipOutdated := `[{"name":"ruff","latest_version":"0.9.5"}]`
	pRows := ParsePipListJSON(pipList)
	pUpd := ParsePipOutdatedJSON(pipOutdated)
	if len(pRows) != 2 || pUpd["ruff"] != "0.9.5" {
		t.Fatalf("unexpected pip parsed rows/updates: rows=%+v upd=%+v", pRows, pUpd)
	}
	if !IsCoreProtectedPackage(ManagerPip, "pip") || IsCoreProtectedPackage(ManagerPip, "ruff") {
		t.Fatal("pip core protection check failed")
	}

	psgRaw := "Posh-SSH\t3.2.0\tPSGallery\nTerminal-Icons\t0.11.0\tPSGallery\n"
	psRows := ParsePSGalleryInstalled(psgRaw)
	if len(psRows) != 2 || psRows[0].ID != "Posh-SSH" || psRows[0].Version != "3.2.0" || psRows[1].Source != "PSGallery" {
		t.Fatalf("unexpected PSGallery parsed rows: %+v", psRows)
	}
}

func TestCargoAndDotnetHTTPAPISearchEnrichAndDetails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/api/v1/crates/ripgrep"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"crate": {
					"id": "ripgrep",
					"name": "ripgrep",
					"max_stable_version": "16.0.0",
					"description": "Fast line search tool",
					"repository": "https://github.com/BurntSushi/ripgrep",
					"downloads": 1691378,
					"keywords": ["search", "cli"]
				},
				"versions": [{"num": "16.0.0", "license": "MIT", "crate_size": 250000, "dl_path": "/api/v1/crates/ripgrep/16.0.0/download"}]
			}`))
		case strings.HasPrefix(r.URL.Path, "/api/v1/crates"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"crates": [{
					"id": "ripgrep",
					"name": "ripgrep",
					"max_stable_version": "16.0.0",
					"description": "Fast line search tool",
					"repository": "https://github.com/BurntSushi/ripgrep"
				}]
			}`))
		case r.URL.Path == "/query":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{
				"data": [{
					"id": "dotnet-ef",
					"title": "dotnet-ef",
					"version": "11.0.0",
					"description": "Entity Framework Core CLI",
					"iconUrl": "https://api.nuget.org/v3-flatcontainer/dotnet-ef/11.0.0/icon",
					"projectUrl": "https://docs.microsoft.com/ef/core/",
					"licenseUrl": "https://www.nuget.org/packages/dotnet-ef/11.0.0/license",
					"authors": ["Microsoft"],
					"tags": ["ef", "cli"],
					"totalDownloads": 143049295
				}]
			}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	cSearch, err := searchCargoWithBaseURL(context.Background(), srv.URL, "ripgrep")
	if err != nil || len(cSearch) != 1 || cSearch[0].Version != "16.0.0" {
		t.Fatalf("cargo search failed: err=%v rows=%+v", err, cSearch)
	}
	cInstalled := []PackageEntry{{Manager: ManagerCargo, ID: "ripgrep", Name: "Ripgrep", Version: "14.1.0"}}
	enrichCargoCratesConcurrently(context.Background(), cInstalled, srv.URL)
	if !cInstalled[0].HasUpdate || cInstalled[0].AvailableVer != "16.0.0" {
		t.Fatalf("expected cargo update to 16.0.0, got %+v", cInstalled[0])
	}
	cDet, err := getCargoPackageDetails(context.Background(), srv.URL, "ripgrep")
	if err != nil || !cDet.Loaded || cDet.License != "MIT" || cDet.DownloadCount != 1691378 {
		t.Fatalf("cargo details failed: err=%v det=%+v", err, cDet)
	}

	dSearch, err := searchDotnetNuGetWithBaseURL(context.Background(), srv.URL, "ef")
	if err != nil || len(dSearch) != 1 || dSearch[0].IconURL == "" {
		t.Fatalf("dotnet search failed: err=%v rows=%+v", err, dSearch)
	}
	dInstalled := []PackageEntry{{Manager: ManagerDotnet, ID: "dotnet-ef", Name: "Dotnet Ef", Version: "10.0.0"}}
	enrichDotnetToolsConcurrently(context.Background(), dInstalled, srv.URL)
	if !dInstalled[0].HasUpdate || dInstalled[0].AvailableVer != "11.0.0" || dInstalled[0].IconURL == "" {
		t.Fatalf("expected dotnet update 11.0.0 + icon, got %+v", dInstalled[0])
	}
	dDet, err := getDotnetPackageDetails(context.Background(), srv.URL, "dotnet-ef")
	if err != nil || !dDet.Loaded || dDet.DownloadCount != 143049295 || dDet.Author != "Microsoft" {
		t.Fatalf("dotnet details failed: err=%v det=%+v", err, dDet)
	}
}

func TestLiveNewPackageManagersAPIsIfEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live Cargo / NuGet DotnetTool / PSGallery API tests in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	cRows, err := searchCargoWithBaseURL(ctx, defaultCratesIOURL, "ripgrep")
	if err != nil || len(cRows) == 0 {
		t.Fatalf("live crates.io search failed: err=%v len=%d", err, len(cRows))
	}
	t.Logf("Live crates.io hit: %s v%s", cRows[0].ID, cRows[0].Version)

	dRows, err := searchDotnetNuGetWithBaseURL(ctx, defaultNuGetSearchURL, "dotnet-ef")
	if err != nil || len(dRows) == 0 {
		t.Fatalf("live NuGet DotnetTool search failed: err=%v len=%d", err, len(dRows))
	}
	t.Logf("Live NuGet DotnetTool hit: %s v%s icon=%q", dRows[0].ID, dRows[0].Version, dRows[0].IconURL)

	psRows, err := searchNuGetV2OData(ctx, DefaultPSGalleryFeedURL, ManagerPSGallery, "posh")
	if err != nil || len(psRows) == 0 {
		t.Fatalf("live PSGallery search failed: err=%v len=%d", err, len(psRows))
	}
	t.Logf("Live PSGallery hit: %s v%s source=%s", psRows[0].ID, psRows[0].Version, psRows[0].Source)
}
