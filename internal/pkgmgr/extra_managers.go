package pkgmgr

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/aozora-kenzu/UniDevUI/internal/essentials"
	"github.com/aozora-kenzu/UniDevUI/internal/pathmgr"
)

const (
	defaultCratesIOURL        = "https://crates.io"
	defaultNuGetSearchURL     = "https://azuresearch-usnc.nuget.org"
	DefaultPSGalleryFeedURL   = "https://www.powershellgallery.com/api/v2"
)

// ParseCargoInstallList parses output from `cargo install --list` lines (`ripgrep v15.0.0:`).
func ParseCargoInstallList(raw string) []PackageEntry {
	var out []PackageEntry
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		if line == "" || strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if !strings.HasSuffix(trimmed, ":") {
			continue
		}
		trimmed = strings.TrimSuffix(trimmed, ":")
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		id := strings.TrimSpace(fields[0])
		ver := strings.TrimPrefix(strings.TrimSpace(fields[1]), "v")
		if id == "" || ver == "" {
			continue
		}
		lk := strings.ToLower(id)
		if seen[lk] {
			continue
		}
		seen[lk] = true
		out = append(out, PackageEntry{
			Manager: ManagerCargo,
			ID:      id,
			Name:    FormatDisplayName(id, ""),
			Version: ver,
			Source:  "crates.io",
			IconURL: LookupIconMedia(ManagerCargo, id, id).Icon,
		})
	}
	return out
}

func searchCargoWithBaseURL(ctx context.Context, baseURL, q string) ([]PackageEntry, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/crates?q=" + url.QueryEscape(q) + "&per_page=25"
	var resp struct {
		Crates []struct {
			ID          string `json:"id"`
			Name        string `json:"name"`
			MaxStable   string `json:"max_stable_version"`
			NewestVer   string `json:"newest_version"`
			MaxVersion  string `json:"max_version"`
			Description string `json:"description"`
			Homepage    string `json:"homepage"`
			Repository  string `json:"repository"`
			Docs        string `json:"documentation"`
		} `json:"crates"`
	}
	if err := fetchJSON(ctx, endpoint, &resp); err != nil {
		return nil, err
	}
	out := make([]PackageEntry, 0, len(resp.Crates))
	for _, c := range resp.Crates {
		id := firstNonEmpty(c.ID, c.Name)
		if id == "" {
			continue
		}
		ver := firstNonEmpty(c.MaxStable, c.NewestVer, c.MaxVersion)
		hp := firstNonEmpty(sanitizeHTTPURL(c.Homepage), sanitizeHTTPURL(c.Repository), sanitizeHTTPURL(c.Docs))
		out = append(out, PackageEntry{
			Manager:     ManagerCargo,
			ID:          id,
			Name:        FormatDisplayName(id, ""),
			Version:     ver,
			Source:      "crates.io",
			Description: summarizeMarkdownText(c.Description, 200),
			Homepage:    hp,
			IconURL:     LookupIconMedia(ManagerCargo, id, id).Icon,
		})
	}
	return out, nil
}

func enrichCargoCratesConcurrently(ctx context.Context, rows []PackageEntry, baseURL string) {
	if len(rows) == 0 {
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := range rows {
		idx := i
		id := rows[idx].ID
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/crates/" + url.PathEscape(id)
			var resp struct {
				Crate struct {
					MaxStable   string `json:"max_stable_version"`
					NewestVer   string `json:"newest_version"`
					MaxVersion  string `json:"max_version"`
					Description string `json:"description"`
					Homepage    string `json:"homepage"`
					Repository  string `json:"repository"`
				} `json:"crate"`
			}
			if err := fetchJSON(ctx, endpoint, &resp); err != nil {
				return
			}
			latest := firstNonEmpty(resp.Crate.MaxStable, resp.Crate.NewestVer, resp.Crate.MaxVersion)
			if rows[idx].Description == "" {
				rows[idx].Description = summarizeMarkdownText(resp.Crate.Description, 200)
			}
			if rows[idx].Homepage == "" {
				rows[idx].Homepage = firstNonEmpty(sanitizeHTTPURL(resp.Crate.Homepage), sanitizeHTTPURL(resp.Crate.Repository))
			}
			if latest != "" && rows[idx].Version != "" && pathmgr.CompareSemver(latest, rows[idx].Version) > 0 {
				rows[idx].AvailableVer = latest
				rows[idx].HasUpdate = true
			}
		}()
	}
	wg.Wait()
}

func getCargoPackageDetails(ctx context.Context, baseURL, id string) (essentials.PackageDetails, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/api/v1/crates/" + url.PathEscape(id)
	var resp struct {
		Crate struct {
			ID          string   `json:"id"`
			Name        string   `json:"name"`
			MaxStable   string   `json:"max_stable_version"`
			MaxVersion  string   `json:"max_version"`
			Description string   `json:"description"`
			Homepage    string   `json:"homepage"`
			Repository  string   `json:"repository"`
			Downloads   int64    `json:"downloads"`
			Keywords    []string `json:"keywords"`
		} `json:"crate"`
		Versions []struct {
			Num     string `json:"num"`
			License string `json:"license"`
			CrateSz int64  `json:"crate_size"`
			DlPath  string `json:"dl_path"`
		} `json:"versions"`
	}
	if err := fetchJSON(ctx, endpoint, &resp); err != nil {
		return essentials.PackageDetails{}, err
	}
	lic := ""
	dlSize := int64(0)
	dlURL := ""
	if len(resp.Versions) > 0 {
		lic = strings.TrimSpace(resp.Versions[0].License)
		dlSize = resp.Versions[0].CrateSz
		if p := strings.TrimSpace(resp.Versions[0].DlPath); p != "" {
			dlURL = strings.TrimRight(baseURL, "/") + p
		}
	}
	pkgID := firstNonEmpty(resp.Crate.ID, id)
	return essentials.PackageDetails{
		Manager:       ManagerCargo,
		ID:            pkgID,
		Name:          FormatDisplayName(pkgID, ""),
		Version:       firstNonEmpty(resp.Crate.MaxStable, resp.Crate.MaxVersion),
		Source:        "crates.io",
		Description:   strings.TrimSpace(resp.Crate.Description),
		Homepage:      firstNonEmpty(sanitizeHTTPURL(resp.Crate.Homepage), sanitizeHTTPURL(resp.Crate.Repository)),
		License:       lic,
		InstallerType: "crate",
		InstallerURL:  dlURL,
		InstallerSize: dlSize,
		DownloadCount: resp.Crate.Downloads,
		Tags:          resp.Crate.Keywords,
		Loaded:        true,
	}, nil
}

// ParseDotnetToolList parses output of `dotnet tool list -g`.
func ParseDotnetToolList(raw string) []PackageEntry {
	var out []PackageEntry
	pastHeader := false
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "----") {
			pastHeader = true
			continue
		}
		if !pastHeader {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		id := strings.TrimSpace(fields[0])
		ver := strings.TrimSpace(fields[1])
		if id == "" || ver == "" {
			continue
		}
		out = append(out, PackageEntry{
			Manager: ManagerDotnet,
			ID:      id,
			Name:    FormatDisplayName(id, ""),
			Version: ver,
			Source:  "nuget.org",
			IconURL: LookupIconMedia(ManagerDotnet, id, id).Icon,
		})
	}
	return out
}

type nugetSearchResp struct {
	Data []struct {
		ID             string   `json:"id"`
		Title          string   `json:"title"`
		Version        string   `json:"version"`
		Description    string   `json:"description"`
		Summary        string   `json:"summary"`
		IconURL        string   `json:"iconUrl"`
		LicenseURL     string   `json:"licenseUrl"`
		ProjectURL     string   `json:"projectUrl"`
		Tags           []string `json:"tags"`
		Authors        []string `json:"authors"`
		TotalDownloads int64    `json:"totalDownloads"`
	} `json:"data"`
}

func searchDotnetNuGetWithBaseURL(ctx context.Context, baseURL, q string) ([]PackageEntry, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/query?q=" + url.QueryEscape(q) + "&packageType=DotnetTool&take=25"
	var resp nugetSearchResp
	if err := fetchJSON(ctx, endpoint, &resp); err != nil {
		return nil, err
	}
	out := make([]PackageEntry, 0, len(resp.Data))
	for _, d := range resp.Data {
		if strings.TrimSpace(d.ID) == "" {
			continue
		}
		icon := sanitizeHTTPURL(d.IconURL)
		if icon == "" {
			icon = LookupIconMedia(ManagerDotnet, d.ID, d.Title).Icon
		}
		out = append(out, PackageEntry{
			Manager:     ManagerDotnet,
			ID:          d.ID,
			Name:        FormatDisplayName(d.ID, d.Title),
			Version:     strings.TrimSpace(d.Version),
			Source:      "nuget.org",
			Description: firstNonEmpty(summarizeMarkdownText(d.Summary, 200), summarizeMarkdownText(d.Description, 200)),
			Homepage:    firstNonEmpty(sanitizeHTTPURL(d.ProjectURL), "https://www.nuget.org/packages/"+url.PathEscape(d.ID)),
			IconURL:     icon,
		})
	}
	return out, nil
}

func enrichDotnetToolsConcurrently(ctx context.Context, rows []PackageEntry, baseURL string) {
	if len(rows) == 0 {
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := range rows {
		idx := i
		id := rows[idx].ID
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			endpoint := strings.TrimRight(baseURL, "/") + "/query?q=packageid:" + url.QueryEscape(strings.ToLower(id)) + "&packageType=DotnetTool&take=3"
			var resp nugetSearchResp
			if err := fetchJSON(ctx, endpoint, &resp); err != nil {
				return
			}
			for _, d := range resp.Data {
				if !strings.EqualFold(d.ID, id) {
					continue
				}
				if icon := sanitizeHTTPURL(d.IconURL); icon != "" && rows[idx].IconURL == "" {
					rows[idx].IconURL = icon
				}
				if rows[idx].Description == "" {
					rows[idx].Description = firstNonEmpty(summarizeMarkdownText(d.Summary, 200), summarizeMarkdownText(d.Description, 200))
				}
				if rows[idx].Homepage == "" {
					rows[idx].Homepage = sanitizeHTTPURL(d.ProjectURL)
				}
				if latest := strings.TrimSpace(d.Version); latest != "" && rows[idx].Version != "" && pathmgr.CompareSemver(latest, rows[idx].Version) > 0 {
					rows[idx].AvailableVer = latest
					rows[idx].HasUpdate = true
				}
				break
			}
		}()
	}
	wg.Wait()
}

func getDotnetPackageDetails(ctx context.Context, baseURL, id string) (essentials.PackageDetails, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/query?q=packageid:" + url.QueryEscape(strings.ToLower(id)) + "&packageType=DotnetTool&take=3"
	var resp nugetSearchResp
	if err := fetchJSON(ctx, endpoint, &resp); err != nil {
		return essentials.PackageDetails{}, err
	}
	for _, d := range resp.Data {
		if !strings.EqualFold(d.ID, id) {
			continue
		}
		return essentials.PackageDetails{
			Manager:       ManagerDotnet,
			ID:            d.ID,
			Name:          FormatDisplayName(d.ID, d.Title),
			Version:       strings.TrimSpace(d.Version),
			Source:        "nuget.org",
			Author:        strings.Join(d.Authors, ", "),
			Description:   firstNonEmpty(strings.TrimSpace(d.Description), strings.TrimSpace(d.Summary)),
			Homepage:      firstNonEmpty(sanitizeHTTPURL(d.ProjectURL), "https://www.nuget.org/packages/"+url.PathEscape(d.ID)),
			LicenseURL:    sanitizeHTTPURL(d.LicenseURL),
			InstallerType: "dotnet tool",
			DownloadCount: d.TotalDownloads,
			Tags:          d.Tags,
			IconURL:       sanitizeHTTPURL(d.IconURL),
			Loaded:        true,
		}, nil
	}
	return essentials.PackageDetails{}, fmt.Errorf("dotnet tool %s not found on NuGet", id)
}

// ParsePipListJSON parses JSON output from `pip list --format=json`.
func ParsePipListJSON(raw string) []PackageEntry {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}
	var payload []struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return nil
	}
	out := make([]PackageEntry, 0, len(payload))
	for _, item := range payload {
		id := strings.TrimSpace(item.Name)
		ver := strings.TrimSpace(item.Version)
		if id == "" || ver == "" {
			continue
		}
		out = append(out, PackageEntry{
			Manager: ManagerPip,
			ID:      id,
			Name:    id,
			Version: ver,
			Source:  "pypi",
			IconURL: LookupIconMedia(ManagerPip, id, id).Icon,
		})
	}
	return out
}

// ParsePipOutdatedJSON parses JSON output from `pip list --outdated --format=json`.
func ParsePipOutdatedJSON(raw string) map[string]string {
	out := make(map[string]string)
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "[]" {
		return out
	}
	var payload []struct {
		Name          string `json:"name"`
		LatestVersion string `json:"latest_version"`
	}
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return out
	}
	for _, item := range payload {
		if id := strings.ToLower(strings.TrimSpace(item.Name)); id != "" && strings.TrimSpace(item.LatestVersion) != "" {
			out[id] = strings.TrimSpace(item.LatestVersion)
		}
	}
	return out
}

// ParsePSGalleryInstalled parses tab-separated `Name\tVersion\tRepository` rows from PowerShell 7.
func ParsePSGalleryInstalled(raw string) []PackageEntry {
	var out []PackageEntry
	seen := make(map[string]bool)
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.Contains(trimmed, "\t") {
			continue
		}
		parts := strings.Split(trimmed, "\t")
		if len(parts) < 2 {
			continue
		}
		id := strings.TrimSpace(parts[0])
		ver := strings.TrimSpace(parts[1])
		repo := "PSGallery"
		if len(parts) >= 3 && strings.TrimSpace(parts[2]) != "" {
			repo = strings.TrimSpace(parts[2])
		}
		if id == "" || ver == "" {
			continue
		}
		lk := strings.ToLower(id)
		if seen[lk] {
			continue
		}
		seen[lk] = true
		out = append(out, PackageEntry{
			Manager: ManagerPSGallery,
			ID:      id,
			Name:    FormatDisplayName(id, ""),
			Version: ver,
			Source:  repo,
			IconURL: LookupIconMedia(ManagerPSGallery, id, id).Icon,
		})
	}
	return out
}

func enrichPSGalleryUpdatesConcurrently(ctx context.Context, rows []PackageEntry, feedBaseURL string) {
	if len(rows) == 0 {
		return
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 6)
	for i := range rows {
		idx := i
		id := rows[idx].ID
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				return
			}
			det, err := getNuGetV2PackageDetails(ctx, feedBaseURL, ManagerPSGallery, id)
			if err != nil {
				return
			}
			if rows[idx].IconURL == "" && det.IconURL != "" {
				rows[idx].IconURL = det.IconURL
			}
			if rows[idx].Homepage == "" && det.Homepage != "" {
				rows[idx].Homepage = det.Homepage
			}
			if rows[idx].Description == "" && det.Description != "" {
				rows[idx].Description = summarizeMarkdownText(det.Description, 200)
			}
			if det.Version != "" && rows[idx].Version != "" && pathmgr.CompareSemver(det.Version, rows[idx].Version) > 0 {
				rows[idx].AvailableVer = det.Version
				rows[idx].HasUpdate = true
			}
		}()
	}
	wg.Wait()
}
