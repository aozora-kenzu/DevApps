package essentials

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aozora-kenzu/UniDevUI/internal/archive"
)

func TestDefaultEssentialsConfHasAll27Apps(t *testing.T) {
	tmp := t.TempDir()
	confPath, err := EnsureConfig(tmp)
	if err != nil {
		t.Fatalf("EnsureConfig failed: %v", err)
	}
	apps, warnings, err := ParseFile(confPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected 0 warnings, got: %v", warnings)
	}
	if len(apps) != 27 {
		t.Fatalf("expected 27 essential apps, got %d", len(apps))
	}

	byID := make(map[string]EssentialApp)
	for _, a := range apps {
		byID[a.ID] = a
	}
	if lara, ok := byID["laragon"]; !ok || len(lara.Versions) != 2 || lara.Versions[1] != "6.0.0" {
		t.Fatalf("unexpected laragon entry: %+v", lara)
	}
	if wa, ok := byID["whatsapp"]; !ok || wa.Source != "msstore" || wa.WingetID != "9NKSQGP7F2NH" {
		t.Fatalf("unexpected whatsapp entry: %+v", wa)
	}
	if st, ok := byID["stirlingpdf"]; !ok || st.WingetID != "StirlingTools.StirlingPDF" {
		t.Fatalf("unexpected stirlingpdf entry: %+v", st)
	}
}

func TestAppendCustomAppAndSyncDefaults(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, "essentials.conf")
	if err := os.WriteFile(confPath, []byte("[firefox]\nname = Firefox\nwinget_id = Mozilla.Firefox\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := AppendCustomApp(confPath, EssentialApp{
		Name:        "Everything",
		WingetID:    "voidtools.Everything",
		Source:      "winget",
		Category:    "utilities",
		Description: "Instant file search",
	})
	if err != nil {
		t.Fatalf("AppendCustomApp failed: %v", err)
	}

	added, err := SyncMissingDefaults(confPath)
	if err != nil {
		t.Fatalf("SyncMissingDefaults failed: %v", err)
	}
	if len(added) != 26 {
		t.Fatalf("expected 26 remaining defaults added, got %d", len(added))
	}
}

func TestParseWingetTableHandlesSpinnersAndAvailableColumn(t *testing.T) {
	sample := "- \r\\ \rName                             Id                         Version                       Available                     Source\r\n" +
		"------------------------------------------------------------------------------------------------------------------------------\r\n" +
		"EmEditor (MSI)                   Emurasoft.EmEditor         26.2.8                        26.4.2                        winget\r\n" +
		"Laragon 6.0.0                    LeNgocKhoa.Laragon         6.0.0.220916                  8.9.0                         winget\r\n" +
		"Antigravity 2.35.1               Google.Antigravity         2.35.1                                                      winget\r\n" +
		"3 upgrades available.\r\n"

	rows := ParseWingetTable(sample)
	if len(rows) != 3 {
		t.Fatalf("expected 3 rows, got %d: %+v", len(rows), rows)
	}
	if rows[0].ID != "Emurasoft.EmEditor" || rows[0].Version != "26.2.8" || rows[0].Available != "26.4.2" {
		t.Fatalf("unexpected row 0: %+v", rows[0])
	}
	if rows[1].ID != "LeNgocKhoa.Laragon" || rows[1].Version != "6.0.0.220916" || rows[1].Available != "8.9.0" {
		t.Fatalf("unexpected row 1: %+v", rows[1])
	}
	if rows[2].ID != "Google.Antigravity" || rows[2].Version != "2.35.1" || rows[2].Available != "" {
		t.Fatalf("unexpected row 2: %+v", rows[2])
	}
}

func TestParseWingetShowOutputWingetAndMSStore(t *testing.T) {
	firefoxRaw := "Found Mozilla Firefox (en-US) [Mozilla.Firefox]\r\n" +
		"Version: 160.0.1\r\n" +
		"Publisher: Mozilla\r\n" +
		"Publisher Url: https://www.mozilla.org/\r\n" +
		"Author: Mozilla Foundation\r\n" +
		"Moniker: firefox\r\n" +
		"Description: Firefox Browser is a free and open-source web browser.\r\n" +
		"  Supports Windows 10 and 11.\r\n" +
		"Homepage: https://www.mozilla.org/firefox/\r\n" +
		"License: MPL-2.0\r\n" +
		"License Url: https://www.mozilla.org/MPL/2.0\r\n" +
		"Release Notes:\r\n" +
		"  Fixed PDF bugs.\r\n" +
		"Release Notes Url: https://www.firefox.com/notes/\r\n" +
		"Tags:\r\n" +
		"  browser\r\n" +
		"  web-browser\r\n" +
		"Installer:\r\n" +
		"  Installer Type: nullsoft\r\n" +
		"  Installer Url: https://download-installer.cdn.mozilla.net/Firefox.exe\r\n" +
		"  Installer SHA256: abcdef123456\r\n" +
		"  Release Date: 2026-11-17\r\n"

	det := ParseWingetShowOutput(firefoxRaw, "Mozilla.Firefox", "winget")
	if !det.Loaded {
		t.Fatalf("expected Loaded=true")
	}
	if det.Publisher != "Mozilla" || det.Homepage != "https://www.mozilla.org/firefox/" {
		t.Fatalf("unexpected publisher/homepage: %+v", det)
	}
	if det.InstallerType != "nullsoft" || det.ReleaseDate != "2026-11-17" || len(det.Tags) != 2 {
		t.Fatalf("unexpected installer/tags: %+v", det)
	}
	if det.Description != "Firefox Browser is a free and open-source web browser.\nSupports Windows 10 and 11." {
		t.Fatalf("unexpected description: %q", det.Description)
	}

	msRaw := "Found WhatsApp [9NKSQGP7F2NH]\r\n" +
		"Version: Unknown\r\n" +
		"Publisher: WhatsApp Inc.\r\n" +
		"Publisher Url: http://whatsapp.com/\r\n" +
		"Description:\r\n" +
		"  WhatsApp from Meta is free.\r\n" +
		"\r\n" +
		"  End-to-end encrypted.\r\n" +
		"License: ms-windows-store://pdp/?ProductId=9NKSQGP7F2NH\r\n" +
		"Installer:\r\n" +
		"  Installer Type: msstore\r\n" +
		"  Store Product Id: 9NKSQGP7F2NH\r\n"

	msDet := ParseWingetShowOutput(msRaw, "9NKSQGP7F2NH", "msstore")
	if msDet.Publisher != "WhatsApp Inc." || msDet.InstallerType != "msstore" || msDet.Source != "msstore" {
		t.Fatalf("unexpected msstore details: %+v", msDet)
	}
	if msDet.Description != "WhatsApp from Meta is free.\n\nEnd-to-end encrypted." {
		t.Fatalf("unexpected msstore description: %q", msDet.Description)
	}
}

func TestLiveShowPackageDetailsIfEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live winget show test in -short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	for _, tc := range []struct {
		id     string
		source string
	}{
		{id: "Microsoft.VisualStudioCode", source: "winget"},
		{id: "9NKSQGP7F2NH", source: "msstore"},
		{id: "StirlingTools.StirlingPDF", source: "winget"},
	} {
		det, err := ShowPackageDetails(ctx, tc.id, tc.source, true)
		if err != nil {
			t.Fatalf("ShowPackageDetails(%s) failed: %v", tc.id, err)
		}
		if det.Publisher == "" || det.Description == "" {
			t.Fatalf("ShowPackageDetails(%s) missing publisher/desc: %+v", tc.id, det)
		}
		if !strings.HasPrefix(det.IconDataURL, "data:image/") {
			t.Fatalf("ShowPackageDetails(%s) expected data:image/ icon, got %q", tc.id, det.IconDataURL)
		}
		t.Logf("%s => Publisher=%q | Homepage=%q | IconBytes=%d | Installer=%s",
			tc.id, det.Publisher, det.Homepage, len(det.IconDataURL), det.InstallerType)
	}
}

func TestMatchInstalledEssentialHandlesWebsiteAndARPMismatches(t *testing.T) {
	byID := map[string]WingetTableRow{
		"google.chrome.beta.exe": {
			Name:    "Google Chrome Beta",
			ID:      "Google.Chrome.Beta.EXE",
			Version: "159.0.8154.33",
			Source:  "winget",
		},
		"microsoft.visualstudiocode": {
			Name:    "Microsoft Visual Studio Code (User)",
			ID:      "Microsoft.VisualStudioCode",
			Version: "1.147.0",
			Source:  "winget",
		},
	}
	wingetRows := []WingetTableRow{
		byID["google.chrome.beta.exe"],
		{Name: "Google Chrome", ID: "Google.Chrome.Beta.EXE", Version: "158.0.8127.37", Source: "winget"},
		{Name: "Mozilla Firefox (x64 en-US)", ID: `ARP\Machine\X64\Mozilla Firefox`, Version: "161.0"},
		{Name: "Sublime Text", ID: `ARP\Machine\X64\Sublime Text_is1`, Version: "Unknown"},
		{Name: "WhatsApp", ID: `MSIX\5319275A.WhatsAppDesktop_2.2645.101.0_x64__cv1g1gvanyjgm`, Version: "2.2645.101.0"},
	}
	regApps := []RegistryAppEntry{
		{DisplayName: "Mozilla Firefox (x64 en-US)", DisplayVersion: "161.0", Publisher: "Mozilla"},
		{DisplayName: "Mozilla Maintenance Service", DisplayVersion: "159.0", Publisher: "Mozilla"},
		{DisplayName: "Google Chrome", DisplayVersion: "158.0.8127.37", Publisher: "Google LLC"},
		{DisplayName: "Google Chrome Beta", DisplayVersion: "159.0.8154.33", Publisher: "Google LLC"},
		{DisplayName: "Sublime Text", DisplayVersion: "", Publisher: "Sublime HQ Pty Ltd"},
	}

	ff := EssentialApp{ID: "firefox", Name: "Mozilla Firefox", WingetID: "Mozilla.Firefox", Source: "winget"}
	mFF := MatchInstalledEssential(ff, byID, regApps, wingetRows)
	if !mFF.Installed || mFF.InstalledVer != "161.0" || mFF.MatchedVia != "registry" {
		t.Fatalf("expected Firefox 161.0 via registry, got %+v", mFF)
	}

	chrome := EssentialApp{ID: "chrome", Name: "Google Chrome", WingetID: "Google.Chrome", Source: "winget"}
	mChrome := MatchInstalledEssential(chrome, byID, regApps, wingetRows)
	if !mChrome.Installed || mChrome.InstalledVer != "158.0.8127.37" {
		t.Fatalf("expected Chrome Stable 158.0.8127.37 (not Beta), got %+v", mChrome)
	}

	sub := EssentialApp{ID: "sublimetext", Name: "Sublime Text 4", WingetID: "SublimeHQ.SublimeText.4", Source: "winget"}
	mSub := MatchInstalledEssential(sub, byID, regApps, wingetRows)
	if !mSub.Installed || mSub.InstalledVer != "" {
		t.Fatalf("expected Sublime Text installed with empty normalized version, got %+v", mSub)
	}

	wa := EssentialApp{ID: "whatsapp", Name: "WhatsApp", WingetID: "9NKSQGP7F2NH", Source: "msstore"}
	mWA := MatchInstalledEssential(wa, byID, regApps, wingetRows)
	if !mWA.Installed || mWA.InstalledVer != "2.2645.101.0" {
		t.Fatalf("expected WhatsApp via MSIX display name, got %+v", mWA)
	}
}

func TestLiveDetectFirefoxChromeHostIfPresent(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live host detection test in -short mode")
	}
	reg := QueryRegistryUninstallApps()
	if len(reg) == 0 {
		t.Fatal("expected non-empty Uninstall registry entries on Windows")
	}
	byID, _, rows, err := QueryInstalledEntries(context.Background())
	if err != nil {
		t.Fatalf("QueryInstalledEntries failed: %v", err)
	}

	defaults, _, _ := ParseString(DefaultEssentialsConf())
	for _, item := range defaults {
		m := MatchInstalledEssential(item, byID, reg, rows)
		if item.ID == "firefox" || item.ID == "chrome" || item.ID == "sublimetext" {
			t.Logf("%-12s => installed=%v ver=%q via=%s exact=%v avail=%q",
				item.ID, m.Installed, m.InstalledVer, m.MatchedVia, m.ExactWinget, m.AvailableVer)
			if !m.Installed {
				t.Errorf("expected host-installed %s to be detected", item.ID)
			}
		}
	}
}

func TestSplitWingetConsoleFramesAndParseProgress(t *testing.T) {
	raw := "\x1b[?25l\x1b[1;1HDownloading https://example.com/setup.exe\r\n" +
		"\x1b[3;1H  \x1b[94m██████░░░░░░\x1b[0m  12.5 MB / 50.0 MB\x1b[4;1H" +
		"  ████████████  100%\r\n" +
		"Successfully verified installer hash\r\n"

	frames := SplitWingetConsoleFrames(raw)
	if len(frames) < 4 {
		t.Fatalf("expected >=4 frames, got %v", frames)
	}
	dl, tot, pct, ok := ParseWingetProgressLine(frames[1])
	if !ok || tot != 50*1024*1024 || dl != int64(12.5*1024*1024) || pct < 24.9 || pct > 25.1 {
		t.Fatalf("unexpected parsed progress: dl=%d tot=%d pct=%.2f ok=%v (line=%q)", dl, tot, pct, ok, frames[1])
	}

	_, _, pctOnly, okPct := ParseWingetProgressLine("██████████ 65%")
	if !okPct || pctOnly != 65 {
		t.Fatalf("expected 65%% bar progress, got %.1f ok=%v", pctOnly, okPct)
	}

	// Simulate chunked winget output like Postman (single byte counts without "/ total").
	var logged []string
	var evs []archive.ProgressEvent
	st := &wingetStreamState{
		detailPrefix: "Postman",
		log:          func(lvl, en, _ string) { logged = append(logged, en) },
		progress:     func(ev archive.ProgressEvent) { evs = append(evs, ev) },
	}
	for _, ln := range []string{
		"Found Postman [Postman.Postman] Version 12.38.3",
		"Downloading https://dl.pstmn.io/download/version/12.38.3/windows_64",
		"40.1 MB",
		"40.2 MB",
		"███░░ 40.5 MB",
		"65%",
		"Successfully verified installer hash",
	} {
		st.handleLine(ln)
	}
	if len(logged) != 3 {
		t.Fatalf("expected only 3 meaningful log lines (no 40.x MB spam), got %d: %v", len(logged), logged)
	}
	wantBytes := int64(40*1024*1024) + (1024*1024)/10
	if len(evs) < 4 || evs[0].Stage != "download" || evs[0].Downloaded < wantBytes-2000 || evs[0].Downloaded > wantBytes+2000 || evs[len(evs)-1].Stage != "install" {
		t.Fatalf("unexpected progress events for chunked stream: %+v", evs)
	}

	// Simulate exact ConPTY differential cursor jumps where only changed digits (".4", "2") are written in place.
	var vtLogs []string
	var vtEvs []archive.ProgressEvent
	vtState := &wingetStreamState{
		detailPrefix: "Postman",
		log:          func(lvl, en, _ string) { vtLogs = append(vtLogs, en) },
		progress:     func(ev archive.ProgressEvent) { vtEvs = append(vtEvs, ev) },
	}
	vt := newVTTerminal(vtState)
	vt.Feed("\x1b[5;1HDownloading https://dl.pstmn.io/download/version/12.38.3/windows_64\r\n")
	vt.Feed("\x1b[6;1H  1.0 MB")
	vt.Feed("\x1b[6;4H.4 MB")
	vt.Feed("\x1b[6;3H25.0 MB")
	vt.Feed("\x1b[7;1HSuccessfully verified installer hash\r\n")
	vt.Flush()

	if len(vtLogs) != 2 {
		t.Fatalf("expected 2 clean log lines (no '1'/'.4 MB'/'MB' fragments), got %d: %v", len(vtLogs), vtLogs)
	}
	lastDL := int64(0)
	for _, ev := range vtEvs {
		if ev.Stage == "download" {
			lastDL = ev.Downloaded
		}
	}
	if lastDL != 25*1024*1024 {
		t.Fatalf("expected final ConPTY reconstructed bytes == 25 MB, got %d (events=%+v)", lastDL, vtEvs)
	}
}

func TestLiveConPTYWingetDownloadProgressIfEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live ConPTY winget download test in -short mode")
	}
	tmp := t.TempDir()
	exe := findWingetExe()
	if exe == "" {
		t.Skip("winget.exe not installed")
	}
	var events []archive.ProgressEvent
	err := runCommandWithConPTY(
		context.Background(),
		"7zip Probe",
		func(lvl, en, _ string) { t.Logf("[%s] %s", lvl, en) },
		func(ev archive.ProgressEvent) { events = append(events, ev) },
		exe, "download", "--id", "7zip.7zip", "-e", "-d", tmp, "--accept-source-agreements", "--accept-package-agreements",
	)
	if err != nil {
		t.Fatalf("runCommandWithConPTY failed: %v", err)
	}
	dlCount := 0
	for _, ev := range events {
		if ev.Stage == "download" && ev.Total > 0 {
			dlCount++
		}
	}
	t.Logf("Captured %d total progress events, %d with byte totals via ConPTY", len(events), dlCount)
	if dlCount == 0 {
		t.Errorf("expected at least 1 byte-level download progress event via ConPTY; got %d total events", len(events))
	}
}

func TestConPTYCapturesCmdAndWinget(t *testing.T) {
	var cmdGot []string
	if err := runCommandWithConPTY(
		context.Background(),
		"cmd",
		func(lvl, en, _ string) { cmdGot = append(cmdGot, lvl+":"+en) },
		nil,
		"cmd.exe", "/c", "echo HELLO_CONPTY_TEST",
	); err != nil {
		t.Fatalf("cmd ConPTY err: %v", err)
	}
	t.Logf("cmd.exe captured (%d lines): %v", len(cmdGot), cmdGot)
	if len(cmdGot) == 0 {
		t.Fatal("expected non-empty output captured from cmd.exe via ConPTY")
	}

	exe := findWingetExe()
	if exe == "" {
		t.Skip("winget.exe not installed")
	}
	var wgGot []string
	if err := runCommandWithConPTY(
		context.Background(),
		"winget-ver",
		func(lvl, en, _ string) { wgGot = append(wgGot, lvl+":"+en) },
		nil,
		exe, "--version",
	); err != nil {
		t.Fatalf("winget ConPTY err: %v", err)
	}
	t.Logf("winget --version (%s) captured (%d lines): %v", exe, len(wgGot), wgGot)
	if len(wgGot) == 0 {
		t.Fatal("expected non-empty output captured from winget via ConPTY")
	}
}

func TestIgnoredAppsConfigCRUDAndManageableIDCheck(t *testing.T) {
	tmp := t.TempDir()
	confPath, err := EnsureIgnoredConfig(tmp)
	if err != nil {
		t.Fatalf("EnsureIgnoredConfig failed: %v", err)
	}

	if err := AddIgnoredApp(confPath, IgnoredApp{
		ID:           "LeNgocKhoa.Laragon",
		Name:         "Laragon 6.0.0",
		Source:       "winget",
		WingetPinned: true,
		Reason:       "Keep v6.0.0 without popup",
	}); err != nil {
		t.Fatalf("AddIgnoredApp failed: %v", err)
	}
	// Duplicate add should update cleanly rather than creating duplicates.
	if err := AddIgnoredApp(confPath, IgnoredApp{
		ID:           "lengockhoa.laragon",
		Name:         "Laragon 6.0.0",
		Source:       "winget",
		WingetPinned: true,
		Reason:       "Keep v6.0.0",
	}); err != nil {
		t.Fatalf("AddIgnoredApp update failed: %v", err)
	}

	list, byID, err := ParseIgnoredFile(confPath)
	if err != nil {
		t.Fatalf("ParseIgnoredFile failed: %v", err)
	}
	if len(list) != 1 || byID["lengockhoa.laragon"].WingetPinned != true {
		t.Fatalf("unexpected ignored list: %+v", list)
	}

	removed, ok, err := RemoveIgnoredApp(confPath, "LeNgocKhoa.Laragon")
	if err != nil || !ok || removed.Name != "Laragon 6.0.0" {
		t.Fatalf("unexpected RemoveIgnoredApp: removed=%+v ok=%v err=%v", removed, ok, err)
	}
	listAfter, _, _ := ParseIgnoredFile(confPath)
	if len(listAfter) != 0 {
		t.Fatalf("expected 0 remaining after remove, got %d", len(listAfter))
	}

	if !IsManageableWingetID("LeNgocKhoa.Laragon") || IsManageableWingetID(`ARP\Machine\X64\Firefox`) || IsManageableWingetID(`MSIX\Foo`) {
		t.Fatal("unexpected IsManageableWingetID result")
	}
}

func TestWingetNoInstalledMatchErrHelper(t *testing.T) {
	if !isWingetNoInstalledMatchErr(errors.New("winget exited with code 2316632084 (0x8A150014)")) {
		t.Fatal("expected 0x8A150014 error to match")
	}
	if isWingetNoInstalledMatchErr(errors.New("winget exited with code 1 (0x1)")) {
		t.Fatal("unexpected match for generic error")
	}
}

func TestAnnotateSearchRowsInstalled(t *testing.T) {
	byID := map[string]WingetTableRow{
		"obsidian.obsidian": {ID: "Obsidian.Obsidian", Name: "Obsidian", Version: "1.7.0", Available: "1.8.0"},
	}
	regApps := []RegistryAppEntry{
		{DisplayName: "Mozilla Firefox (x64 en-US)", DisplayVersion: "160.0", Publisher: "Mozilla"},
	}
	ignored := map[string]IgnoredApp{
		"voidtools.everything": {ID: "voidtools.Everything", Name: "Everything"},
	}
	byID["voidtools.everything"] = WingetTableRow{ID: "voidtools.Everything", Name: "Everything", Version: "1.4.1", Available: "1.5.0"}

	searchRows := []WingetTableRow{
		{Name: "Obsidian", ID: "Obsidian.Obsidian", Version: "1.8.0", Source: "winget"},
		{Name: "Mozilla Firefox", ID: "Mozilla.Firefox", Version: "161.0", Source: "winget"},
		{Name: "Everything", ID: "voidtools.Everything", Version: "1.5.0", Source: "winget"},
		{Name: "7-Zip", ID: "7zip.7zip", Version: "26.06", Source: "winget"},
	}

	got := AnnotateSearchRowsInstalled(searchRows, byID, regApps, nil, ignored)
	if !got[0].Installed || got[0].InstalledVer != "1.7.0" || !got[0].HasUpdate || got[0].Available != "1.8.0" {
		t.Fatalf("expected Obsidian installed+update, got %+v", got[0])
	}
	if !got[1].Installed || got[1].MatchedBy != "Windows Registry" || !got[1].HasUpdate || got[1].Available != "161.0" {
		t.Fatalf("expected Firefox installed via registry+update, got %+v", got[1])
	}
	if !got[2].Installed || got[2].HasUpdate {
		t.Fatalf("expected ignored Everything installed without HasUpdate, got %+v", got[2])
	}
	if got[3].Installed {
		t.Fatalf("expected 7-Zip not installed, got %+v", got[3])
	}
}
