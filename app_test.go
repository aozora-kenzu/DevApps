package main

import "testing"

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
