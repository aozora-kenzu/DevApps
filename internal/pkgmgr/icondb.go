package pkgmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	// IconDatabaseURL points to Devolutions/UniGetUI's curated package icon and screenshot database.
	IconDatabaseURL = "https://github.com/Devolutions/UniGetUI/raw/refs/heads/main/WebBasedData/screenshot-database-v2.json"
	// IconDatabaseTTL refreshes the cached icon database once per day.
	IconDatabaseTTL = 24 * time.Hour
)

// IconMedia stores resolved icon and preview screenshot URLs for a package.
type IconMedia struct {
	Icon        string   `json:"icon,omitempty"`
	Screenshots []string `json:"screenshots,omitempty"`
}

type rawIconDBPayload struct {
	IconsAndScreenshots map[string]struct {
		Icon   string   `json:"icon"`
		Images []string `json:"images"`
	} `json:"icons_and_screenshots"`
}

// IconStore holds the in-memory and on-disk cache of UniGetUI's icon/screenshot database.
type IconStore struct {
	mu          sync.RWMutex
	exact       map[string]IconMedia
	lower       map[string]IconMedia
	loaded      bool
	lastAttempt time.Time
}

var defaultIconStore = &IconStore{
	exact: make(map[string]IconMedia),
	lower: make(map[string]IconMedia),
}

// DefaultIconStore returns the shared process-wide IconStore instance.
func DefaultIconStore() *IconStore {
	return defaultIconStore
}

// LookupIconMedia returns an icon URL and screenshot list from the shared IconStore if present.
func LookupIconMedia(manager, id, name string) IconMedia {
	return defaultIconStore.Lookup(manager, id, name)
}

// LoadFromFile loads an existing icon database JSON cache into memory.
func (s *IconStore) LoadFromFile(cachePath string) error {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		return err
	}
	return s.LoadFromBytes(data)
}

// LoadFromBytes parses UniGetUI screenshot-database-v2 JSON bytes into the lookup maps.
func (s *IconStore) LoadFromBytes(data []byte) error {
	var payload rawIconDBPayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return fmt.Errorf("failed parsing icon database JSON: %w", err)
	}
	exact := make(map[string]IconMedia, len(payload.IconsAndScreenshots))
	lower := make(map[string]IconMedia, len(payload.IconsAndScreenshots))
	for rawKey, val := range payload.IconsAndScreenshots {
		key := strings.TrimSpace(rawKey)
		if key == "" {
			continue
		}
		icon := sanitizeHTTPURL(val.Icon)
		var shots []string
		for _, img := range val.Images {
			if cleanImg := sanitizeHTTPURL(img); cleanImg != "" {
				shots = append(shots, cleanImg)
			}
		}
		if icon == "" && len(shots) == 0 {
			continue
		}
		entry := IconMedia{Icon: icon, Screenshots: shots}
		exact[key] = entry
		lk := strings.ToLower(key)
		// Preserve more complete entry when duplicate case-variants exist (e.g. intellijidea-ultimate).
		if prev, exists := lower[lk]; !exists || (prev.Icon == "" && entry.Icon != "") {
			lower[lk] = entry
		}
	}

	s.mu.Lock()
	s.exact = exact
	s.lower = lower
	s.loaded = true
	s.mu.Unlock()
	return nil
}

// EntryCount returns the number of indexed package icon entries currently in memory.
func (s *IconStore) EntryCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.lower)
}

// EnsureSynced loads the cached database from cachePath if available and downloads a fresh copy
// from endpointURL when missing or older than ttl. Returns true when new remote data was downloaded.
func (s *IconStore) EnsureSynced(ctx context.Context, cachePath, endpointURL string, ttl time.Duration) (bool, error) {
	if strings.TrimSpace(cachePath) == "" {
		return false, fmt.Errorf("empty icon cache path")
	}
	if strings.TrimSpace(endpointURL) == "" {
		endpointURL = IconDatabaseURL
	}
	if ttl <= 0 {
		ttl = IconDatabaseTTL
	}

	if info, err := os.Stat(cachePath); err == nil && info.Size() > 1024 {
		if s.EntryCount() == 0 {
			_ = s.LoadFromFile(cachePath)
		}
		if time.Since(info.ModTime()) < ttl && s.EntryCount() > 0 {
			return false, nil
		}
	}

	s.mu.Lock()
	recentlyTried := !s.lastAttempt.IsZero() && time.Since(s.lastAttempt) < 2*time.Minute
	if !recentlyTried {
		s.lastAttempt = time.Now()
	}
	s.mu.Unlock()
	if recentlyTried {
		return false, nil
	}

	if ctx == nil {
		ctx = context.Background()
	}
	reqCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpointURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("User-Agent", "UniDevUI/1.0 (Icon Database Sync)")

	client := &http.Client{Timeout: 20 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return false, fmt.Errorf("HTTP %d from %s", resp.StatusCode, endpointURL)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return false, err
	}
	if err := s.LoadFromBytes(body); err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err == nil {
		tmp := cachePath + ".tmp"
		if wErr := os.WriteFile(tmp, body, 0o644); wErr == nil {
			_ = os.Rename(tmp, cachePath)
		}
	}
	return true, nil
}

// Lookup searches the in-memory database using UniGetUI-compatible normalized key precedence.
func (s *IconStore) Lookup(manager, id, name string) IconMedia {
	s.mu.RLock()
	loaded := s.loaded
	exact := s.exact
	lower := s.lower
	s.mu.RUnlock()

	if !loaded || len(lower) == 0 {
		return IconMedia{}
	}

	for _, key := range GenerateIconLookupKeys(manager, id, name) {
		if m, ok := exact[key]; ok {
			return IconMedia{Icon: m.Icon, Screenshots: append([]string(nil), m.Screenshots...)}
		}
		if m, ok := lower[strings.ToLower(key)]; ok {
			return IconMedia{Icon: m.Icon, Screenshots: append([]string(nil), m.Screenshots...)}
		}
	}
	return IconMedia{}
}

// GenerateIconLookupKeys produces candidate icon-database keys matching UniGetUI's GenerateIconId + lookup order.
func GenerateIconLookupKeys(manager, id, name string) []string {
	rawID := strings.TrimSpace(id)
	rawName := strings.TrimSpace(name)
	mgrNorm := NormalizeManager(manager)

	mgrTitle := "Winget"
	switch mgrNorm {
	case ManagerScoop:
		mgrTitle = "Scoop"
	case ManagerChoco:
		mgrTitle = "Chocolatey"
	case ManagerNpm:
		mgrTitle = "Npm"
	case ManagerPnpm:
		mgrTitle = "Pnpm"
	case ManagerBun:
		mgrTitle = "Bun"
	case ManagerUv:
		mgrTitle = "Pip"
	case ManagerPip:
		mgrTitle = "Pip"
	case ManagerCargo:
		mgrTitle = "Cargo"
	case ManagerDotnet:
		mgrTitle = ".NET Tool"
	case ManagerPSGallery:
		mgrTitle = "PowerShell7"
	}

	seen := make(map[string]bool)
	var keys []string
	add := func(k string) {
		trimmed := strings.TrimSpace(k)
		if trimmed == "" {
			return
		}
		lk := strings.ToLower(trimmed)
		if seen[lk] {
			return
		}
		seen[lk] = true
		keys = append(keys, trimmed)
	}

	if rawID != "" {
		add(mgrTitle + "." + rawID)
		add(rawID)
	}

	switch mgrNorm {
	case ManagerWinget:
		upper := strings.ToUpper(rawID)
		if strings.HasPrefix(upper, `ARP\`) || strings.HasPrefix(upper, `LOCAL\`) {
			parts := strings.Split(rawID, `\`)
			add(parts[len(parts)-1])
		} else if strings.HasPrefix(upper, `MSIX\`) {
			after := rawID[5:]
			if idx := strings.Index(after, "_"); idx > 0 {
				pkgPart := after[:idx]
				add(pkgPart)
				if dot := strings.Index(pkgPart, "."); dot >= 0 && dot+1 < len(pkgPart) {
					add(pkgPart[dot+1:])
				}
			}
		} else if dot := strings.Index(rawID, "."); dot >= 0 && dot+1 < len(rawID) {
			noPub := rawID[dot+1:]
			add(noPub)
			add(slugifyIconCandidate(noPub))
			// Handle multi-segment Winget IDs like `OpenJS.NodeJS.LTS` -> `NodeJS.LTS` and `NodeJS`.
			if dot2 := strings.Index(noPub, "."); dot2 > 0 {
				add(noPub[:dot2])
			}
		}
	case ManagerScoop:
		if slash := strings.LastIndex(rawID, "/"); slash >= 0 && slash+1 < len(rawID) {
			rawID = rawID[slash+1:]
		}
		base := trimSuffixFold(rawID, ".app")
		add(base)
	case ManagerChoco:
		base := trimSuffixFold(rawID, ".install")
		base = trimSuffixFold(base, ".portable")
		base = trimSuffixFold(base, ".commandline")
		add(base)
	}

	if rawID != "" {
		add(slugifyIconCandidate(rawID))
	}
	if rawName != "" {
		add(rawName)
		add(slugifyIconCandidate(rawName))
		// Strip trailing parenthetical suffixes such as `(Install)` or `(x64)`.
		if paren := strings.Index(rawName, "("); paren > 1 {
			cleanName := strings.TrimSpace(rawName[:paren])
			add(cleanName)
			add(slugifyIconCandidate(cleanName))
		}
	}

	return keys
}

func slugifyIconCandidate(raw string) string {
	cleaned := strings.TrimSpace(raw)
	cleaned = trimSuffixFold(cleaned, ".install")
	cleaned = trimSuffixFold(cleaned, ".portable")
	cleaned = strings.ToLower(cleaned)
	var b strings.Builder
	prevDash := false
	for _, r := range cleaned {
		isAlnum := (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
		if isAlnum {
			b.WriteRune(r)
			prevDash = false
		} else if b.Len() > 0 && !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func trimSuffixFold(s, suffix string) string {
	if len(s) >= len(suffix) && strings.EqualFold(s[len(s)-len(suffix):], suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}

func sanitizeHTTPURL(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if strings.HasPrefix(trimmed, "//") {
		trimmed = "https:" + trimmed
	}
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "https://") || strings.HasPrefix(lower, "http://") {
		return trimmed
	}
	return ""
}
