package pkgmgr

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIconStoreLookupAndCaseInsensitiveKeys(t *testing.T) {
	raw := `{
		"icons_and_screenshots": {
			"firefox": {
				"icon": "https://cdn.example.org/firefox.png",
				"images": ["https://cdn.example.org/firefox-1.png", "https://cdn.example.org/firefox-2.png"]
			},
			"git": {
				"icon": "https://cdn.example.org/git.svg",
				"images": []
			},
			"obsidian": {
				"icon": "https://cdn.example.org/obsidian.png",
				"images": ["https://cdn.example.org/obsidian-shot.png"]
			},
			"intellijidea-ultimate": {
				"icon": "",
				"images": []
			},
			"IntelliJIDEA-Ultimate": {
				"icon": "https://cdn.example.org/idea.png",
				"images": []
			}
		}
	}`

	var store IconStore
	if err := store.LoadFromBytes([]byte(raw)); err != nil {
		t.Fatalf("LoadFromBytes failed: %v", err)
	}

	mWinget := store.Lookup(ManagerWinget, "Mozilla.Firefox", "Mozilla Firefox")
	if mWinget.Icon != "https://cdn.example.org/firefox.png" || len(mWinget.Screenshots) != 2 {
		t.Fatalf("unexpected Winget icon lookup: %+v", mWinget)
	}

	mChoco := store.Lookup(ManagerChoco, "git.install", "Git (Install)")
	if mChoco.Icon != "https://cdn.example.org/git.svg" {
		t.Fatalf("unexpected Choco .install icon lookup: %+v", mChoco)
	}

	mScoop := store.Lookup(ManagerScoop, "extras/obsidian", "Obsidian")
	if mScoop.Icon != "https://cdn.example.org/obsidian.png" || len(mScoop.Screenshots) != 1 {
		t.Fatalf("unexpected Scoop icon lookup: %+v", mScoop)
	}

	mIdea := store.Lookup(ManagerWinget, "JetBrains.IntelliJIDEA.Ultimate", "IntelliJ IDEA Ultimate")
	if mIdea.Icon != "https://cdn.example.org/idea.png" {
		t.Fatalf("expected case-insensitive deduped IntelliJ icon, got %+v", mIdea)
	}
}

func TestFormatDisplayNameAndParseScoopSearchFast(t *testing.T) {
	if got := FormatDisplayName("git.install", ""); got != "Git" {
		t.Fatalf("expected Git, got %q", got)
	}
	if got := FormatDisplayName("extras/lazygit", ""); got != "Lazygit" {
		t.Fatalf("expected Lazygit, got %q", got)
	}
	if got := FormatDisplayName("googlechrome", "Google Chrome (Install)"); got != "Google Chrome (Install)" {
		t.Fatalf("expected title preserved, got %q", got)
	}

	raw := `
Results from local buckets...
'main' bucket:
  git (2.56.1) --> includes 'git.exe'
'extras' bucket:
  obsidian (1.8.10) A knowledge base app
`
	rows := ParseScoopSearchFast(raw)
	if len(rows) != 2 {
		t.Fatalf("expected 2 fast scoop results, got %d: %+v", len(rows), rows)
	}
	if rows[0].ID != "git" || rows[0].Source != "main" || rows[0].Version != "2.56.1" {
		t.Fatalf("unexpected first scoop row: %+v", rows[0])
	}
	if rows[1].ID != "obsidian" || rows[1].Source != "extras" || rows[1].Name != "Obsidian" {
		t.Fatalf("unexpected second scoop row: %+v", rows[1])
	}
}

func TestChocoODataSearchAndDetails(t *testing.T) {
	atomXML := `<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<feed xml:base="https://community.chocolatey.org/api/v2/" xmlns:d="http://schemas.microsoft.com/ado/2007/08/dataservices" xmlns:m="http://schemas.microsoft.com/ado/2007/08/dataservices/metadata" xmlns="http://www.w3.org/2005/Atom">
  <title type="text">Search</title>
  <entry>
    <id>https://community.chocolatey.org/api/v2/Packages(Id='git.install',Version='2.56.1')</id>
    <title type="text">git.install</title>
    <summary type="text">Git for Windows SCM</summary>
    <author><name>Git Community</name></author>
    <content type="application/zip" src="https://community.chocolatey.org/api/v2/package/git.install/2.56.1" />
    <m:properties>
      <d:Version>2.56.1</d:Version>
      <d:Title>Git (Install)</d:Title>
      <d:Description>Full Git package description here</d:Description>
      <d:Tags>git vcs cli</d:Tags>
      <d:Dependencies>chocolatey-core.extension:1.3.3:</d:Dependencies>
      <d:DownloadCount>25146970</d:DownloadCount>
      <d:IconUrl>https://cdn.example.org/git.svg</d:IconUrl>
      <d:LicenseUrl>https://www.gnu.org/licenses/gpl-2.0.html</d:LicenseUrl>
      <d:PackageHash>ABC123SHA==</d:PackageHash>
      <d:PackageSize>66004695</d:PackageSize>
      <d:ProjectUrl>https://git-for-windows.github.io/</d:ProjectUrl>
      <d:Published>2026-11-11T00:14:31Z</d:Published>
    </m:properties>
  </entry>
</feed>`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/atom+xml; charset=utf-8")
		_, _ = w.Write([]byte(atomXML))
	}))
	defer srv.Close()

	rows, err := searchChocoOData(context.Background(), srv.URL, "git")
	if err != nil || len(rows) != 1 {
		t.Fatalf("searchChocoOData failed: err=%v rows=%+v", err, rows)
	}
	if rows[0].ID != "git.install" || rows[0].Name != "Git (Install)" || rows[0].IconURL != "https://cdn.example.org/git.svg" {
		t.Fatalf("unexpected Choco row: %+v", rows[0])
	}

	det, err := getChocoPackageDetails(context.Background(), srv.URL, "git.install")
	if err != nil || !det.Loaded {
		t.Fatalf("getChocoPackageDetails failed: err=%v det=%+v", err, det)
	}
	if det.DownloadCount != 25146970 || det.InstallerSize != 66004695 || det.Author != "Git Community" || len(det.Dependencies) != 1 {
		t.Fatalf("unexpected Choco details: %+v", det)
	}
}

func TestParseScoopManifestFileDetails(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "obsidian.json")
	manifest := `{
		"version": "1.8.10",
		"description": "A powerful knowledge base on top of local Markdown files",
		"homepage": "https://obsidian.md",
		"license": {"identifier": "Proprietary"},
		"architecture": {
			"64bit": {
				"url": "https://github.com/obsidianmd/obsidian-releases/releases/download/v1.8.10/Obsidian.1.8.10.exe",
				"hash": "deadbeef1234"
			}
		},
		"depends": ["7zip"]
	}`
	if err := os.WriteFile(path, []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	det, err := parseScoopManifestFile(path, "obsidian", "extras")
	if err != nil || !det.Loaded {
		t.Fatalf("parseScoopManifestFile failed: err=%v det=%+v", err, det)
	}
	if det.License != "Proprietary" || det.Homepage != "https://obsidian.md" || det.InstallerSHA256 != "deadbeef1234" || len(det.Dependencies) != 1 {
		t.Fatalf("unexpected Scoop manifest details: %+v", det)
	}
}

func TestLiveUniGetUIIconDBAndChocoODataIfEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live UniGetUI icon DB sync & Chocolatey OData search in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	tmp := t.TempDir()
	cachePath := filepath.Join(tmp, "icon_db.json")
	var store IconStore
	downloaded, err := store.EnsureSynced(ctx, cachePath, IconDatabaseURL, 24*time.Hour)
	if err != nil || !downloaded || store.EntryCount() < 1000 {
		t.Fatalf("live IconDB EnsureSynced failed: downloaded=%v count=%d err=%v", downloaded, store.EntryCount(), err)
	}
	m := store.Lookup(ManagerWinget, "Mozilla.Firefox", "Firefox")
	t.Logf("Live IconDB entries=%d Firefox icon=%q shots=%d", store.EntryCount(), m.Icon, len(m.Screenshots))
	if m.Icon == "" {
		t.Fatal("expected live Firefox icon URL in UniGetUI database")
	}

	chRows, err := searchChocoOData(ctx, DefaultChocolateyFeedURL, "git")
	if err != nil || len(chRows) == 0 {
		t.Fatalf("live Chocolatey OData Search failed: err=%v len=%d", err, len(chRows))
	}
	t.Logf("Live Choco OData top hit: id=%s name=%q v%s icon=%q", chRows[0].ID, chRows[0].Name, chRows[0].Version, chRows[0].IconURL)
}
