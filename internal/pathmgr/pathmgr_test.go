package pathmgr

import "testing"

func TestSplitAndNormalizePathHelpers(t *testing.T) {
	raw := `C:\Windows\system32; "C:\AppPath\node\22.23.5\" ; ; %USERPROFILE%\go\bin`
	parts := splitPathList(raw)
	if len(parts) != 3 {
		t.Fatalf("expected 3 path entries, got %d: %v", len(parts), parts)
	}

	norm := normalizePath(`"C:\AppPath\node\22.23.5\"`)
	if norm != `C:\AppPath\node\22.23.5` {
		t.Fatalf("unexpected normalized path: %q", norm)
	}

	if !isManagedAppEntry(`c:\apppath\node\24.22.3`, `C:\AppPath\node`) {
		t.Fatal("expected case-insensitive managed prefix match")
	}
	if isManagedAppEntry(`C:\AppPath\nodejs\24.22.3`, `C:\AppPath\node`) {
		t.Fatal("should not match sibling app folder with shared prefix")
	}
	if !ContainsDir(parts, `c:\apppath\node\22.23.5`) {
		t.Fatal("expected ContainsDir to match case-insensitively")
	}
}

func TestCompareSemverAndExtractVersionString(t *testing.T) {
	if CompareSemver("22.23.5", "26.13.1") >= 0 {
		t.Fatal("expected 22.23.5 < 26.13.1")
	}
	if CompareSemver("10.0.404", "10.0.404") != 0 {
		t.Fatal("expected equal versions")
	}
	if CompareSemver("1.27.4", "1.26.11") <= 0 {
		t.Fatal("expected 1.27.4 > 1.26.11")
	}

	cases := []struct {
		appID string
		out   string
		want  string
	}{
		{"node", "v22.23.5\n", "22.23.5"},
		{"go", "go version go1.27.4 windows/amd64", "1.27.4"},
		{"php", "PHP 8.3.33 (cli) (built: Jul 28 2026)", "8.3.33"},
		{"git", "git version 2.37.3.windows.1", "2.37.3"},
		{"dotnet", "10.0.404", "10.0.404"},
		{"rust", "rustc 1.99.1 (58cf8f9c3 2026-10-13)", "1.99.1"},
		{"sqlite", "3.54.1 2026-01-01", "3.54.1"},
		{"7zip", "7-Zip (r) 26.06 (x64) : Copyright (c) 1999-2026", "26.06"},
		{"gcc", "16.2.0", "16.2.0"},
	}
	for _, tc := range cases {
		got := extractVersionString(tc.appID, tc.out)
		if got != tc.want {
			t.Errorf("extractVersionString(%q, %q) = %q, want %q", tc.appID, tc.out, got, tc.want)
		}
	}
}
