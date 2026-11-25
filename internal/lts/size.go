package lts

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var sizeHTTPClient = &http.Client{
	Timeout: 6 * time.Second,
}

// ProbeURLSize returns the remote file size in bytes using HTTP HEAD, falling back to Range GET.
// Returns 0 if the URL is empty or the server does not report a known byte length.
func ProbeURLSize(ctx context.Context, rawURL string) int64 {
	u := strings.TrimSpace(rawURL)
	if u == "" || (!strings.HasPrefix(strings.ToLower(u), "https://") && !strings.HasPrefix(strings.ToLower(u), "http://")) {
		return 0
	}

	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	// 1. Try fast HTTP HEAD.
	if req, err := http.NewRequestWithContext(reqCtx, http.MethodHead, u, nil); err == nil {
		req.Header.Set("User-Agent", "DevApps/1.0")
		if resp, err := sizeHTTPClient.Do(req); err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.ContentLength > 0 {
				return resp.ContentLength
			}
		}
	}

	// 2. Fallback to Range GET (`bytes=0-0`) for CDNs that reject HEAD requests.
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, u, nil)
	if err != nil {
		return 0
	}
	req.Header.Set("User-Agent", "DevApps/1.0")
	req.Header.Set("Range", "bytes=0-0")
	resp, err := sizeHTTPClient.Do(req)
	if err != nil {
		return 0
	}
	_ = resp.Body.Close()

	if resp.StatusCode == http.StatusPartialContent {
		if total := ParseContentRangeTotal(resp.Header.Get("Content-Range")); total > 0 {
			return total
		}
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 && resp.ContentLength > 0 {
		return resp.ContentLength
	}
	return 0
}

// ParseContentRangeTotal parses the total length from an HTTP Content-Range header (e.g. "bytes 0-0/12345").
func ParseContentRangeTotal(header string) int64 {
	slash := strings.LastIndex(header, "/")
	if slash < 0 || slash+1 >= len(header) {
		return 0
	}
	totalStr := strings.TrimSpace(header[slash+1:])
	if totalStr == "" || totalStr == "*" {
		return 0
	}
	n, err := strconv.ParseInt(totalStr, 10, 64)
	if err != nil || n <= 0 {
		return 0
	}
	return n
}

// ProbeMissingSizes resolves download sizes in parallel for URLs not already cached (> 0).
func ProbeMissingSizes(ctx context.Context, urls []string, existing map[string]int64) map[string]int64 {
	seen := make(map[string]bool)
	var todo []string
	for _, u := range urls {
		trimmed := strings.TrimSpace(u)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		if existing != nil && existing[trimmed] > 0 {
			continue
		}
		todo = append(todo, trimmed)
	}
	if len(todo) == 0 {
		return nil
	}

	discovered := make(map[string]int64)
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)

	for _, u := range todo {
		targetURL := u
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			if sz := ProbeURLSize(ctx, targetURL); sz > 0 {
				mu.Lock()
				discovered[targetURL] = sz
				mu.Unlock()
			}
		}()
	}
	wg.Wait()
	return discovered
}

// LoadSizeCache loads persisted URL->bytes entries from disk, returning an initialized map.
func LoadSizeCache(cachePath string) map[string]int64 {
	out := make(map[string]int64)
	if cachePath == "" {
		return out
	}
	raw, err := os.ReadFile(cachePath)
	if err != nil {
		return out
	}
	var parsed map[string]int64
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return out
	}
	for k, v := range parsed {
		if k != "" && v > 0 {
			out[k] = v
		}
	}
	return out
}

// SaveSizeCache persists URL->bytes entries atomically if non-empty.
func SaveSizeCache(cachePath string, sizes map[string]int64) error {
	if cachePath == "" || len(sizes) == 0 {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}
	trimmed := sizes
	if len(sizes) > 500 {
		trimmed = make(map[string]int64, 500)
		for k, v := range sizes {
			trimmed[k] = v
			if len(trimmed) >= 500 {
				break
			}
		}
	}
	data, err := json.MarshalIndent(trimmed, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath, data, 0o644)
}
