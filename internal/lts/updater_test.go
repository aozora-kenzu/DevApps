package lts

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"
)

func TestCompareVersionDesc(t *testing.T) {
	if !compareVersionDesc("8.5.13", "8.4.28") {
		t.Fatal("expected 8.5.13 > 8.4.28")
	}
	if !compareVersionDesc("8.10.0", "8.9.5") {
		t.Fatal("expected 8.10.0 > 8.9.5")
	}
	if compareVersionDesc("8.3.37", "8.4.0") {
		t.Fatal("expected 8.3.37 < 8.4.0")
	}
}

func TestLiveProvidersIfEnabled(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live provider test in -short mode")
	}
	providers := []string{
		"nodejs", "php", "composer", "go", "git",
		"pocketbase", "sqlite", "python", "dotnet", "rust-msvc", "winlibs", "cmake", "7zip",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	for _, p := range providers {
		vers, err := FetchLatestVersions(ctx, p)
		if err != nil {
			t.Errorf("provider %s failed: %v", p, err)
			continue
		}
		if len(vers) == 0 {
			t.Errorf("provider %s returned 0 versions", p)
			continue
		}
		t.Logf("%-12s -> %s (%s)", p, vers[0].Version, vers[0].URL)
	}
}

func TestParseContentRangeAndProbeMissingSizesAndCache(t *testing.T) {
	if got := ParseContentRangeTotal("bytes 0-0/41166246"); got != 41166246 {
		t.Fatalf("expected 41166246, got %d", got)
	}
	if got := ParseContentRangeTotal("bytes 0-0/*"); got != 0 {
		t.Fatalf("expected 0 for unknown total, got %d", got)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/head-ok.zip":
			if r.Method == http.MethodHead {
				w.Header().Set("Content-Length", "10485760")
				w.WriteHeader(http.StatusOK)
				return
			}
		case "/range-only.tar.gz":
			if r.Method == http.MethodHead {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Range", "bytes 0-0/26214400")
			w.WriteHeader(http.StatusPartialContent)
			_, _ = w.Write([]byte("x"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	cachedURL := srv.URL + "/already-cached.zip"
	headURL := srv.URL + "/head-ok.zip"
	rangeURL := srv.URL + "/range-only.tar.gz"

	existing := map[string]int64{cachedURL: 1234}
	got := ProbeMissingSizes(context.Background(), []string{cachedURL, headURL, rangeURL}, existing)
	if got[headURL] != 10485760 {
		t.Fatalf("expected HEAD size 10485760, got %d", got[headURL])
	}
	if got[rangeURL] != 26214400 {
		t.Fatalf("expected Range fallback size 26214400, got %d", got[rangeURL])
	}
	if _, exists := got[cachedURL]; exists {
		t.Fatal("should not re-probe URL already present in cache")
	}

	cachePath := filepath.Join(t.TempDir(), "size_cache.json")
	if err := SaveSizeCache(cachePath, got); err != nil {
		t.Fatalf("SaveSizeCache failed: %v", err)
	}
	loaded := LoadSizeCache(cachePath)
	if loaded[headURL] != 10485760 || loaded[rangeURL] != 26214400 {
		t.Fatalf("unexpected reloaded cache: %+v", loaded)
	}
}
