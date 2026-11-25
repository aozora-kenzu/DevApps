package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultPackagesConfParsesCleanly(t *testing.T) {
	tmp := t.TempDir()
	confPath, err := EnsureConfig(tmp)
	if err != nil {
		t.Fatalf("EnsureConfig failed: %v", err)
	}

	pkgs, warnings, err := ParseFile(confPath)
	if err != nil {
		t.Fatalf("ParseFile failed: %v", err)
	}
	if len(warnings) != 0 {
		t.Fatalf("expected zero warnings in default conf, got: %v", warnings)
	}

	expected := []string{"node", "php", "composer", "go", "git", "pocketbase", "sqlite", "python", "dotnet", "rust", "gcc", "cmake", "7zip"}
	got := make(map[string]AppPackage)
	for _, p := range pkgs {
		got[p.ID] = p
	}
	for _, id := range expected {
		pkg, ok := got[id]
		if !ok {
			t.Errorf("missing expected package %q", id)
			continue
		}
		if len(pkg.Versions) == 0 {
			t.Errorf("package %q has no versions", id)
		}
	}
}

func TestUpdateVersionsPreservesCommentsAndCustomSections(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, "packages.conf")
	initial := strings.Join([]string{
		"# Header comment",
		"[node]",
		"name = Node.js",
		"lts_provider = nodejs",
		"version 22.0.0 = https://example.com/old.zip",
		"",
		"# User custom tool",
		"[customtool]",
		"name = Custom",
		"lts_provider = manual",
		"version 1.0.0 = https://example.com/custom.zip",
	}, "\r\n")

	if err := os.WriteFile(confPath, []byte(initial), 0o644); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	err := UpdateVersionsInFile(confPath, map[string][]VersionEntry{
		"node": {
			{Version: "24.1.0", URL: "https://example.com/node-24.1.0.zip"},
			{Version: "22.2.0", URL: "https://example.com/node-22.2.0.zip"},
		},
	})
	if err != nil {
		t.Fatalf("UpdateVersionsInFile failed: %v", err)
	}

	pkgs, warnings, err := ParseFile(confPath)
	if err != nil || len(warnings) > 0 {
		t.Fatalf("re-parse failed: err=%v warnings=%v", err, warnings)
	}
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}
	if len(pkgs[0].Versions) != 2 || pkgs[0].Versions[0].Version != "24.1.0" {
		t.Fatalf("unexpected node versions: %+v", pkgs[0].Versions)
	}
	if pkgs[1].ID != "customtool" || len(pkgs[1].Versions) != 1 || pkgs[1].Versions[0].Version != "1.0.0" {
		t.Fatalf("customtool section was modified unexpectedly: %+v", pkgs[1])
	}
}

func TestSyncMissingDefaultsAddsNewBuiltinBlocks(t *testing.T) {
	tmp := t.TempDir()
	confPath := filepath.Join(tmp, "packages.conf")
	initial := strings.Join([]string{
		"[node]",
		"name = Node.js (LTS)",
		"version 22.23.5 = https://example.com/node.zip",
	}, "\r\n")

	if err := os.WriteFile(confPath, []byte(initial), 0o644); err != nil {
		t.Fatal(err)
	}

	added, err := SyncMissingDefaults(confPath)
	if err != nil {
		t.Fatalf("SyncMissingDefaults failed: %v", err)
	}
	if len(added) == 0 {
		t.Fatal("expected missing defaults including sqlite to be added")
	}

	pkgs, _, err := ParseFile(confPath)
	if err != nil {
		t.Fatal(err)
	}
	foundSQLite := false
	for _, p := range pkgs {
		if p.ID == "sqlite" {
			foundSQLite = true
		}
		if p.ID == "node" && (len(p.Versions) != 1 || p.Versions[0].Version != "22.23.5") {
			t.Fatalf("existing node section should remain untouched, got: %+v", p.Versions)
		}
	}
	if !foundSQLite {
		t.Fatal("expected sqlite block to be added by SyncMissingDefaults")
	}
}
