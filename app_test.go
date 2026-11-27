package main

import (
	"strings"
	"testing"
	"time"

	"github.com/aozora-kenzu/UniDevUI/internal/config"
)

func TestVersionBranchPrefix(t *testing.T) {
	cases := []struct {
		appID string
		ver   string
		want  string
	}{
		{"node", "22.3.1", "22"},
		{"node", "v26.13.1", "26"},
		{"php", "8.3.33", "8.3"},
		{"go", "1.26.11", "1.26"},
		{"composer", "2.2.33", "2.2"},
		{"python", "3.14.10", "3.14"},
		{"dotnet", "10.0.404", "10"},
	}
	for _, tc := range cases {
		if got := versionBranchPrefix(tc.appID, tc.ver); got != tc.want {
			t.Errorf("versionBranchPrefix(%q, %q) = %q, want %q", tc.appID, tc.ver, got, tc.want)
		}
	}
}

func TestStartupDelayAndNotificationDedup(t *testing.T) {
	s := config.DefaultSettings()
	if got := computeStartupScanDelay(true, s); got != 45*time.Second {
		t.Fatalf("expected 45s when startedHidden, got %v", got)
	}
	if got := computeStartupScanDelay(false, s); got != 6*time.Second {
		t.Fatalf("expected 6s when DelayStartupScan=true, got %v", got)
	}
	s.DelayStartupScan = false
	if got := computeStartupScanDelay(false, s); got != 1*time.Second {
		t.Fatalf("expected 1s when DelayStartupScan=false, got %v", got)
	}

	items1 := []updateNoticeItem{
		{Key: "pkg:node", Label: "Node.js (LTS)", Version: "26.14.0"},
		{Key: "app:winget:mozilla.firefox", Label: "Mozilla Firefox", Version: "162.0"},
	}
	fresh1, cache1 := diffNewNotifications(nil, items1)
	if len(fresh1) != 2 || len(cache1) != 2 {
		t.Fatalf("expected 2 initial notifications, got fresh=%d cache=%d", len(fresh1), len(cache1))
	}
	// Identical items on second cycle must not re-trigger notification.
	fresh2, cache2 := diffNewNotifications(cache1, items1)
	if len(fresh2) != 0 || len(cache2) != 2 {
		t.Fatalf("expected 0 duplicate notifications, got fresh=%d", len(fresh2))
	}
	// Newer version on same key triggers fresh notification; resolved key is pruned.
	items3 := []updateNoticeItem{
		{Key: "app:winget:mozilla.firefox", Label: "Mozilla Firefox", Version: "163.0"},
		{Key: "tool:pip", Label: "pip", Version: "27.0"},
		{Key: "pkg:go", Label: "Go", Version: "1.27.0"},
		{Key: "app:choco:git", Label: "Git", Version: "2.57.0"},
	}
	fresh3, cache3 := diffNewNotifications(cache2, items3)
	if len(fresh3) != 4 || cache3["pkg:node"] != "" {
		t.Fatalf("expected 4 new updates and pruned pkg:node, got fresh=%d cache=%v", len(fresh3), cache3)
	}

	titleID, bodyID := formatUpdateNotificationMessage("id", items3)
	if !strings.Contains(titleID, "4 Pembaruan") || !strings.Contains(bodyID, "1 lainnya") {
		t.Fatalf("unexpected ID notification format: title=%q body=%q", titleID, bodyID)
	}
	titleEN, bodyEN := formatUpdateNotificationMessage("en", items3)
	if !strings.Contains(titleEN, "4 Update(s)") || !strings.Contains(bodyEN, "1 more") {
		t.Fatalf("unexpected EN notification format: title=%q body=%q", titleEN, bodyEN)
	}
}

func TestCooldownAndUpdateNoticeExtraction(t *testing.T) {
	now := time.Unix(1800000000, 0)
	s := config.DefaultSettings()
	s.AutoCheckIntervalHours = 6

	if shouldSkipStartupScanByCooldown(s, now) {
		t.Fatalf("expected false when LastCheckUnix is zero")
	}
	s.LastCheckUnix = now.Add(-2 * time.Hour).Unix()
	if !shouldSkipStartupScanByCooldown(s, now) {
		t.Fatalf("expected true within 6h cooldown window")
	}
	s.LastCheckUnix = now.Add(-7 * time.Hour).Unix()
	if shouldSkipStartupScanByCooldown(s, now) {
		t.Fatalf("expected false once 6h cooldown elapsed")
	}
	s.LastCheckUnix = now.Add(10 * time.Minute).Unix()
	if shouldSkipStartupScanByCooldown(s, now) {
		t.Fatalf("expected false when persisted clock is in the future")
	}

	items := extractUpdateNoticeItems(DashboardState{
		Packages: []PackageView{
			{ID: "node", Name: "Node.js", ActiveVer: "24.0.0", LatestVer: "26.1.0"},
		},
		InstalledApps: []InstalledAppView{
			{Manager: "winget", ID: "Git.Git", Name: "Git", AvailableVer: "2.50.0", HasUpdate: true, CanUpdate: true},
			{Manager: "winget", ID: "Ignored.App", Name: "Ignored", AvailableVer: "3.0.0", HasUpdate: true, CanUpdate: true, Ignored: true},
		},
	})
	if len(items) != 2 {
		t.Fatalf("expected 2 non-ignored update notice items, got %d (%+v)", len(items), items)
	}
}
