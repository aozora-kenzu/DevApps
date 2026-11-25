package essentials

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"DevApp/internal/lts"
)

// PackageDetails holds rich metadata parsed from `winget show` plus a resolved Data-URI icon.
type PackageDetails struct {
	ID              string   `json:"id"`
	Name            string   `json:"name"`
	Version         string   `json:"version"`
	Source          string   `json:"source"`
	Publisher       string   `json:"publisher"`
	PublisherURL    string   `json:"publisherUrl"`
	Author          string   `json:"author"`
	Moniker         string   `json:"moniker"`
	Description     string   `json:"description"`
	Homepage        string   `json:"homepage"`
	License         string   `json:"license"`
	LicenseURL      string   `json:"licenseUrl"`
	ReleaseDate     string   `json:"releaseDate"`
	ReleaseNotesURL string   `json:"releaseNotesUrl"`
	InstallerType   string   `json:"installerType"`
	InstallerURL    string   `json:"installerUrl"`
	InstallerSize   int64    `json:"installerSize,omitempty"`
	InstallerSHA256 string   `json:"installerSha256"`
	Tags            []string `json:"tags,omitempty"`
	IconDataURL     string   `json:"iconDataUrl,omitempty"`
	Loaded          bool     `json:"loaded"`
}

var (
	detailsCache sync.Map // key: "lower(id)|lower(source)" -> PackageDetails
	iconCache    sync.Map // key: cacheKey -> string (data URI or "")
)

var metadataHTTPClient = &http.Client{
	Timeout: 4500 * time.Millisecond,
}

// ShowPackageDetails runs `winget show` (cached in memory) and resolves publisher/website/Store icons.
func ShowPackageDetails(ctx context.Context, wingetID, source string, fetchIcon bool) (PackageDetails, error) {
	id := strings.TrimSpace(wingetID)
	src := strings.TrimSpace(strings.ToLower(source))
	if id == "" {
		return PackageDetails{}, fmt.Errorf("empty winget ID")
	}

	cacheKey := strings.ToLower(id) + "|" + src
	if cached, ok := detailsCache.Load(cacheKey); ok {
		det := cached.(PackageDetails)
		if !fetchIcon || (det.IconDataURL != "" && (det.InstallerURL == "" || det.InstallerSize > 0)) {
			return det, nil
		}
		if det.IconDataURL == "" {
			det.IconDataURL = resolvePackageIcon(ctx, det)
		}
		if det.InstallerURL != "" && det.InstallerSize == 0 {
			det.InstallerSize = lts.ProbeURLSize(ctx, det.InstallerURL)
		}
		detailsCache.Store(cacheKey, det)
		return det, nil
	}

	exe := findWingetExe()
	if exe == "" {
		return PackageDetails{}, fmt.Errorf("winget.exe not found")
	}

	runCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	args := []string{"show", "--id", id, "-e", "--accept-source-agreements"}
	if src == "winget" || src == "msstore" {
		args = append(args, "--source", src)
	}
	cmd := exec.CommandContext(runCtx, exe, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	rawText := string(out)

	// Retry without exact source if the package moved or caller omitted exact source.
	if err != nil && !strings.Contains(rawText, "Found ") && src != "" {
		argsFallback := []string{"show", "--id", id, "-e", "--accept-source-agreements"}
		cmd2 := exec.CommandContext(runCtx, exe, argsFallback...)
		cmd2.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if out2, err2 := cmd2.CombinedOutput(); err2 == nil {
			rawText = string(out2)
			err = nil
		}
	}

	det := ParseWingetShowOutput(rawText, id, src)
	if !det.Loaded && err != nil {
		return PackageDetails{}, fmt.Errorf("winget show failed for %s: %w", id, err)
	}
	if fetchIcon {
		var wg sync.WaitGroup
		wg.Add(1)
		go func() {
			defer wg.Done()
			det.IconDataURL = resolvePackageIcon(ctx, det)
		}()
		if det.InstallerURL != "" {
			wg.Add(1)
			go func() {
				defer wg.Done()
				det.InstallerSize = lts.ProbeURLSize(ctx, det.InstallerURL)
			}()
		}
		wg.Wait()
	}

	detailsCache.Store(cacheKey, det)
	return det, nil
}

// EnrichSearchRows fetches live `winget show` metadata and icons in parallel for up to maxItems search results.
func EnrichSearchRows(ctx context.Context, rows []WingetTableRow, maxItems int) []PackageDetails {
	if len(rows) == 0 {
		return nil
	}
	if maxItems <= 0 || maxItems > len(rows) {
		maxItems = len(rows)
	}

	results := make([]PackageDetails, maxItems)
	sem := make(chan struct{}, 6)
	var wg sync.WaitGroup

	for i := 0; i < maxItems; i++ {
		idx := i
		row := rows[idx]
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}

			det, err := ShowPackageDetails(ctx, row.ID, row.Source, true)
			if err != nil {
				results[idx] = PackageDetails{
					ID:      row.ID,
					Name:    row.Name,
					Version: row.Version,
					Source:  row.Source,
				}
				return
			}
			if det.Name == "" {
				det.Name = row.Name
			}
			if det.Version == "" {
				det.Version = row.Version
			}
			if det.Source == "" {
				det.Source = row.Source
			}
			results[idx] = det
		}()
	}
	wg.Wait()
	return results
}

// ParseWingetShowOutput parses structured metadata from `winget show --id ...` stdout.
func ParseWingetShowOutput(raw, fallbackID, fallbackSource string) PackageDetails {
	det := PackageDetails{
		ID:     fallbackID,
		Source: fallbackSource,
	}

	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")

	section := "" // "", "description", "tags", "release_notes", "installer", "documentation", "agreements"
	var descLines []string

	for _, rawLine := range lines {
		if idx := strings.LastIndex(rawLine, "\r"); idx >= 0 {
			rawLine = rawLine[idx+1:]
		}
		trimmed := strings.TrimSpace(rawLine)
		if trimmed == "" {
			if section == "description" && len(descLines) > 0 {
				descLines = append(descLines, "")
			}
			continue
		}

		// Top header: `Found Mozilla Firefox (en-US) [Mozilla.Firefox]`
		if strings.HasPrefix(trimmed, "Found ") {
			det.Loaded = true
			rest := strings.TrimPrefix(trimmed, "Found ")
			if lb := strings.LastIndex(rest, "["); lb >= 0 && strings.HasSuffix(rest, "]") {
				parsedID := strings.TrimSpace(rest[lb+1 : len(rest)-1])
				if parsedID != "" {
					det.ID = parsedID
				}
				det.Name = strings.TrimSpace(rest[:lb])
			} else {
				det.Name = strings.TrimSpace(rest)
			}
			continue
		}

		isIndented := strings.HasPrefix(rawLine, " ") || strings.HasPrefix(rawLine, "\t")
		if isIndented {
			switch section {
			case "description":
				descLines = append(descLines, trimmed)
				continue
			case "tags":
				det.Tags = append(det.Tags, trimmed)
				continue
			case "release_notes":
				continue
			case "installer":
				if k, v, ok := splitKV(trimmed); ok {
					assignInstallerField(&det, k, v)
				}
				continue
			case "agreements", "documentation":
				if k, v, ok := splitKV(trimmed); ok {
					if strings.EqualFold(k, "Category") && det.InstallerType == "msstore" && len(det.Tags) == 0 {
						det.Tags = append(det.Tags, v)
					}
				}
				continue
			}
		}

		section = ""
		k, v, ok := splitKV(trimmed)
		if !ok {
			continue
		}
		det.Loaded = true

		switch strings.ToLower(k) {
		case "version":
			det.Version = v
		case "publisher":
			det.Publisher = v
		case "publisher url":
			det.PublisherURL = v
		case "author":
			det.Author = v
		case "moniker":
			det.Moniker = v
		case "homepage":
			det.Homepage = v
		case "license":
			det.License = v
		case "license url":
			det.LicenseURL = v
		case "release notes url":
			det.ReleaseNotesURL = v
		case "release date":
			det.ReleaseDate = v
		case "description":
			section = "description"
			if v != "" {
				descLines = append(descLines, v)
			}
		case "tags":
			section = "tags"
		case "release notes":
			section = "release_notes"
		case "installer":
			section = "installer"
		case "documentation":
			section = "documentation"
		case "agreements":
			section = "agreements"
		}
	}

	det.Description = strings.TrimSpace(strings.Join(descLines, "\n"))
	if det.Source == "" && det.InstallerType == "msstore" {
		det.Source = "msstore"
	} else if det.Source == "" {
		det.Source = "winget"
	}
	return det
}

func splitKV(line string) (string, string, bool) {
	colon := strings.Index(line, ":")
	if colon <= 0 {
		return "", "", false
	}
	return strings.TrimSpace(line[:colon]), strings.TrimSpace(line[colon+1:]), true
}

func assignInstallerField(det *PackageDetails, key, val string) {
	switch strings.ToLower(key) {
	case "installer type":
		det.InstallerType = val
	case "installer url":
		det.InstallerURL = val
	case "installer sha256":
		det.InstallerSHA256 = val
	case "release date":
		det.ReleaseDate = val
	case "release notes url":
		if det.ReleaseNotesURL == "" {
			det.ReleaseNotesURL = val
		}
	}
}

func resolvePackageIcon(ctx context.Context, det PackageDetails) string {
	// 1. Official MS Store app logo via Store DisplayCatalog API.
	if strings.EqualFold(det.Source, "msstore") || isStoreProductID(det.ID) {
		if dataURL := fetchMSStoreIconDataURL(ctx, det.ID); dataURL != "" {
			return dataURL
		}
	}

	// 2. If hosted on GitHub (not generic Octocat favicon), prefer the project owner's avatar first.
	for _, raw := range []string{det.Homepage, det.PublisherURL} {
		if dataURL := resolveGitHubRepoAvatar(ctx, raw); dataURL != "" {
			return dataURL
		}
	}

	// 3. Favicon from official Homepage / Publisher URL / Installer URL domain.
	candidates := []string{det.Homepage, det.PublisherURL, det.InstallerURL}
	seenHost := make(map[string]bool)
	for _, raw := range candidates {
		host := extractIconHost(raw)
		if host == "" || seenHost[host] || host == "github.com" {
			continue
		}
		seenHost[host] = true
		if dataURL := fetchHostFaviconDataURL(ctx, host); dataURL != "" {
			return dataURL
		}
	}
	return ""
}

func isStoreProductID(id string) bool {
	if len(id) != 12 || strings.Contains(id, ".") {
		return false
	}
	return strings.HasPrefix(strings.ToUpper(id), "9N") || strings.HasPrefix(strings.ToUpper(id), "XP")
}

func extractIconHost(rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	if !strings.Contains(trimmed, "://") {
		trimmed = "https://" + trimmed
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	if host == "" {
		return ""
	}
	// Skip CDN/object-storage hosts that have generic icons instead of app logos.
	if strings.HasSuffix(host, "cloudfront.net") ||
		strings.HasSuffix(host, "amazonaws.com") ||
		strings.HasSuffix(host, "googleapis.com") {
		return ""
	}
	return host
}

func resolveGitHubRepoAvatar(ctx context.Context, rawURL string) string {
	trimmed := strings.TrimSpace(rawURL)
	if trimmed == "" {
		return ""
	}
	u, err := url.Parse(trimmed)
	if err != nil {
		return ""
	}
	if !strings.EqualFold(u.Hostname(), "github.com") {
		return ""
	}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		return ""
	}
	owner := parts[0]
	cacheKey := "gh:" + strings.ToLower(owner)
	if cached, ok := iconCache.Load(cacheKey); ok {
		return cached.(string)
	}
	avatarURL := fmt.Sprintf("https://github.com/%s.png?size=64", url.PathEscape(owner))
	dataURL := downloadImageAsDataURI(ctx, avatarURL)
	iconCache.Store(cacheKey, dataURL)
	return dataURL
}

func fetchMSStoreIconDataURL(ctx context.Context, storeID string) string {
	cacheKey := "msstore:" + strings.ToUpper(storeID)
	if cached, ok := iconCache.Load(cacheKey); ok {
		return cached.(string)
	}

	apiURL := fmt.Sprintf("https://displaycatalog.mp.microsoft.com/v7.0/products?bigIds=%s&market=US&languages=en-US", url.QueryEscape(storeID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return ""
	}
	resp, err := metadataHTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}

	var payload struct {
		Products []struct {
			LocalizedProperties []struct {
				Images []struct {
					ImagePurpose string `json:"ImagePurpose"`
					Width        int    `json:"Width"`
					Height       int    `json:"Height"`
					URI          string `json:"Uri"`
				} `json:"Images"`
			} `json:"LocalizedProperties"`
		} `json:"Products"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 512*1024)).Decode(&payload); err != nil {
		return ""
	}
	if len(payload.Products) == 0 || len(payload.Products[0].LocalizedProperties) == 0 {
		iconCache.Store(cacheKey, "")
		return ""
	}

	var bestURI string
	bestScore := -1
	for _, img := range payload.Products[0].LocalizedProperties[0].Images {
		purpose := strings.ToLower(img.ImagePurpose)
		if purpose != "logo" && purpose != "tile" {
			continue
		}
		if img.Width != img.Height || img.URI == "" {
			continue
		}
		score := 0
		if purpose == "logo" {
			score += 100
		}
		if img.Width >= 48 && img.Width <= 150 {
			score += 50
		}
		if score > bestScore {
			bestScore = score
			bestURI = img.URI
		}
	}
	if bestURI == "" {
		iconCache.Store(cacheKey, "")
		return ""
	}
	if strings.HasPrefix(bestURI, "//") {
		bestURI = "https:" + bestURI
	}

	dataURL := downloadImageAsDataURI(ctx, bestURI)
	iconCache.Store(cacheKey, dataURL)
	return dataURL
}

func fetchHostFaviconDataURL(ctx context.Context, host string) string {
	cacheKey := "host:" + host
	if cached, ok := iconCache.Load(cacheKey); ok {
		return cached.(string)
	}

	urls := []string{
		fmt.Sprintf("https://www.google.com/s2/favicons?domain=%s&sz=64", url.QueryEscape(host)),
		fmt.Sprintf("https://icons.duckduckgo.com/ip3/%s.ico", url.PathEscape(host)),
	}
	for _, u := range urls {
		if dataURL := downloadImageAsDataURI(ctx, u); dataURL != "" {
			iconCache.Store(cacheKey, dataURL)
			return dataURL
		}
	}
	iconCache.Store(cacheKey, "")
	return ""
}

func downloadImageAsDataURI(ctx context.Context, rawURL string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return ""
	}
	req.Header.Set("User-Agent", "DevApps-Windows-Setup/1.0")
	resp, err := metadataHTTPClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return ""
	}
	ct := strings.ToLower(strings.TrimSpace(resp.Header.Get("Content-Type")))
	if semi := strings.Index(ct, ";"); semi >= 0 {
		ct = strings.TrimSpace(ct[:semi])
	}
	if !strings.HasPrefix(ct, "image/") {
		return ""
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256*1024))
	if err != nil || len(data) < 80 {
		return ""
	}
	return fmt.Sprintf("data:%s;base64,%s", ct, base64.StdEncoding.EncodeToString(data))
}
