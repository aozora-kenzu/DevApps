package pkgmgr

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/aozora-kenzu/UniDevUI/internal/essentials"
	"github.com/aozora-kenzu/UniDevUI/internal/tools"
)

// DefaultChocolateyFeedURL is the official Chocolatey NuGet v2 feed endpoint used for fast search & metadata.
const DefaultChocolateyFeedURL = "https://community.chocolatey.org/api/v2"

// FormatDisplayName mirrors UniGetUI's FormatAsName helper to make Scoop/Chocolatey IDs human-readable
// while preserving clean scoping for developer packages.
func FormatDisplayName(rawID, rawTitle string) string {
	title := strings.TrimSpace(rawTitle)
	if title != "" && !strings.EqualFold(title, strings.TrimSpace(rawID)) {
		return title
	}
	name := strings.TrimSpace(rawID)
	if name == "" {
		return ""
	}
	if slash := strings.LastIndex(name, "/"); slash >= 0 && !strings.HasPrefix(name, "@") {
		name = name[slash+1:]
	}
	name = trimSuffixFold(name, ".install")
	name = trimSuffixFold(name, ".portable")
	name = trimSuffixFold(name, ".app")
	name = strings.ReplaceAll(name, "-", " ")
	name = strings.ReplaceAll(name, "_", " ")

	words := strings.Fields(name)
	for i, w := range words {
		runes := []rune(w)
		if len(runes) > 0 {
			runes[0] = unicode.ToUpper(runes[0])
			words[i] = string(runes)
		}
	}
	return strings.Join(words, " ")
}

type chocoAtomFeed struct {
	Entries []chocoAtomEntry `xml:"entry"`
}

type chocoAtomEntry struct {
	AtomID  string          `xml:"id"`
	Title   string          `xml:"title"`
	Summary string          `xml:"summary"`
	Author  chocoAtomAuthor `xml:"author"`
	Content struct {
		Src string `xml:"src,attr"`
	} `xml:"content"`
	Props chocoAtomProps `xml:"properties"`
}

type chocoAtomAuthor struct {
	Name string `xml:"name"`
}

type chocoAtomProps struct {
	Version          string `xml:"Version"`
	Title            string `xml:"Title"`
	Description      string `xml:"Description"`
	Summary          string `xml:"Summary"`
	Tags             string `xml:"Tags"`
	Dependencies     string `xml:"Dependencies"`
	DownloadCount    int64  `xml:"DownloadCount"`
	GalleryDetails   string `xml:"GalleryDetailsUrl"`
	IconURL          string `xml:"IconUrl"`
	LicenseURL       string `xml:"LicenseUrl"`
	PackageHash      string `xml:"PackageHash"`
	PackageSize      int64  `xml:"PackageSize"`
	ProjectURL       string `xml:"ProjectUrl"`
	ReleaseNotesURL  string `xml:"ReleaseNotesUrl"`
	ReleaseNotes     string `xml:"ReleaseNotes"`
	Published        string `xml:"Published"`
	Copyright        string `xml:"Copyright"`
}

// searchNuGetV2OData queries a NuGet v2 Search() endpoint (Chocolatey / PowerShell Gallery) over HTTPS,
// returning packages ordered by download popularity together with descriptions, homepages, and official icon URLs.
func searchNuGetV2OData(ctx context.Context, feedBaseURL, mgrID, q string) ([]PackageEntry, error) {
	escapedTerm := url.QueryEscape(strings.ReplaceAll(strings.TrimSpace(q), "'", "''"))
	endpoint := fmt.Sprintf(
		"%s/Search()?%%24filter=IsLatestVersion&%%24orderby=DownloadCount%%20desc&searchTerm='%s'&targetFramework=''&includePrerelease=false&%%24top=35",
		strings.TrimRight(feedBaseURL, "/"),
		escapedTerm,
	)

	entries, err := fetchChocoAtomFeed(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	out := make([]PackageEntry, 0, len(entries))
	srcLabel := mgrID
	if mgrID == ManagerPSGallery {
		srcLabel = "PSGallery"
	}
	for _, e := range entries {
		pkgID := parseChocoEntryID(e)
		if pkgID == "" {
			continue
		}
		summary := firstNonEmpty(e.Summary, e.Props.Summary, summarizeMarkdownText(e.Props.Description, 200))
		hp := firstNonEmpty(sanitizeHTTPURL(e.Props.ProjectURL), sanitizeHTTPURL(e.Props.GalleryDetails))
		iconURL := sanitizeHTTPURL(e.Props.IconURL)
		if iconURL == "" {
			iconURL = LookupIconMedia(mgrID, pkgID, e.Props.Title).Icon
		}
		out = append(out, PackageEntry{
			Manager:     mgrID,
			ID:          pkgID,
			Name:        FormatDisplayName(pkgID, e.Props.Title),
			Version:     strings.TrimSpace(e.Props.Version),
			Source:      srcLabel,
			Description: summary,
			Homepage:    hp,
			IconURL:     iconURL,
		})
	}
	return out, nil
}

func searchChocoOData(ctx context.Context, feedBaseURL, q string) ([]PackageEntry, error) {
	return searchNuGetV2OData(ctx, feedBaseURL, ManagerChoco, q)
}

func fetchChocoAtomFeed(ctx context.Context, endpoint string) ([]chocoAtomEntry, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "UniDevUI/1.0 (Chocolatey NuGet Client)")
	req.Header.Set("Accept", "application/atom+xml,application/xml,text/xml")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, endpoint)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 4*1024*1024))
	if err != nil {
		return nil, err
	}

	var feed chocoAtomFeed
	if err := xml.Unmarshal(data, &feed); err != nil {
		// Fallback for single-<entry> endpoints.
		var single chocoAtomEntry
		if err2 := xml.Unmarshal(data, &single); err2 == nil && parseChocoEntryID(single) != "" {
			return []chocoAtomEntry{single}, nil
		}
		return nil, fmt.Errorf("failed parsing Chocolatey XML feed: %w", err)
	}
	return feed.Entries, nil
}

func parseChocoEntryID(e chocoAtomEntry) string {
	if t := strings.TrimSpace(e.Title); t != "" {
		return t
	}
	// Fallback from Atom <id>: .../Packages(Id='git.install',Version='2.56.1')
	const marker = "Id='"
	if idx := strings.Index(e.AtomID, marker); idx >= 0 {
		rest := e.AtomID[idx+len(marker):]
		if end := strings.Index(rest, "'"); end > 0 {
			return strings.TrimSpace(rest[:end])
		}
	}
	return ""
}

// ParseScoopSearchFast parses output produced by native `scoop-search.exe <query>`.
// Typical format groups results under `'extras' bucket:` headings with lines like `obsidian (1.8.10)`.
func ParseScoopSearchFast(raw string) []PackageEntry {
	var out []PackageEntry
	currentBucket := ""
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if strings.HasPrefix(trimmed, "'") {
			if endQuote := strings.Index(trimmed[1:], "'"); endQuote >= 0 {
				currentBucket = strings.TrimSpace(trimmed[1 : 1+endQuote])
			}
			continue
		}
		if strings.HasPrefix(trimmed, "Results from ") || strings.HasPrefix(trimmed, "----") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}
		id := strings.TrimSpace(fields[0])
		ver := strings.Trim(strings.TrimSpace(fields[1]), "()")
		if id == "" || strings.EqualFold(id, "Name") || ver == "" {
			continue
		}
		desc := ""
		if len(fields) >= 3 {
			extra := strings.Join(fields[2:], " ")
			if extra != "" && !strings.HasPrefix(extra, "-->") {
				desc = extra
			}
		}
		icon := LookupIconMedia(ManagerScoop, id, id).Icon
		out = append(out, PackageEntry{
			Manager:     ManagerScoop,
			ID:          id,
			Name:        FormatDisplayName(id, ""),
			Version:     ver,
			Source:      currentBucket,
			Description: desc,
			IconURL:     icon,
		})
	}
	if len(out) > 35 {
		out = out[:35]
	}
	return out
}

func findScoopSearchExe(baseDir string) string {
	var extra []string
	if profile := strings.TrimSpace(os.Getenv("USERPROFILE")); profile != "" {
		extra = append(extra, filepath.Join(profile, "scoop", "shims", "scoop-search.exe"))
	}
	if scoopRoot := strings.TrimSpace(os.Getenv("SCOOP")); scoopRoot != "" {
		extra = append(extra, filepath.Join(scoopRoot, "shims", "scoop-search.exe"))
	}
	if baseDir != "" {
		extra = append(extra, filepath.Join(baseDir, "scoop", "shims", "scoop-search.exe"))
	}
	for _, cand := range extra {
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return cand
		}
	}
	lp, _ := exec.LookPath("scoop-search.exe")
	return lp
}

// GetPackageDetails resolves rich package metadata (description, license, dependencies, installer URL/SHA,
// icon, and screenshots) for Scoop, Chocolatey, npm/pnpm/Bun, and uv.
func GetPackageDetails(ctx context.Context, manager, pkgID, source, baseDir string) (essentials.PackageDetails, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	mgr := NormalizeManager(manager)
	id := strings.TrimSpace(pkgID)
	if id == "" {
		return essentials.PackageDetails{}, fmt.Errorf("empty package ID")
	}

	var det essentials.PackageDetails
	var err error
	switch mgr {
	case ManagerScoop:
		det, err = getScoopPackageDetails(ctx, id, source, baseDir)
	case ManagerChoco:
		det, err = getNuGetV2PackageDetails(ctx, DefaultChocolateyFeedURL, ManagerChoco, id)
	case ManagerPSGallery:
		det, err = getNuGetV2PackageDetails(ctx, DefaultPSGalleryFeedURL, ManagerPSGallery, id)
	case ManagerNpm, ManagerPnpm, ManagerBun:
		det, err = getNpmPackageDetails(ctx, "https://registry.npmjs.org", mgr, id)
	case ManagerUv, ManagerPip:
		det, err = getPyPIPackageDetails(ctx, "https://pypi.org/pypi", id)
		det.Manager = mgr
	case ManagerCargo:
		det, err = getCargoPackageDetails(ctx, defaultCratesIOURL, id)
	case ManagerDotnet:
		det, err = getDotnetPackageDetails(ctx, defaultNuGetSearchURL, id)
	default:
		return essentials.PackageDetails{}, fmt.Errorf("unsupported package manager: %s", manager)
	}

	if err != nil {
		// Fall back cleanly if the icon DB still has media or at least return a helpful error.
		return essentials.PackageDetails{}, err
	}

	media := LookupIconMedia(mgr, det.ID, det.Name)
	if det.IconURL == "" {
		det.IconURL = media.Icon
	}
	if len(det.Screenshots) == 0 && len(media.Screenshots) > 0 {
		det.Screenshots = media.Screenshots
	}
	if det.Loaded {
		return det, nil
	}
	return det, nil
}

func getScoopPackageDetails(ctx context.Context, id, source, baseDir string) (essentials.PackageDetails, error) {
	// Locate `<id>.json` inside local Scoop bucket clones first for sub-millisecond reads.
	manifestID := id
	bucketHint := strings.TrimSpace(source)
	if slash := strings.Index(id, "/"); slash > 0 && slash+1 < len(id) {
		if bucketHint == "" {
			bucketHint = strings.TrimSpace(id[:slash])
		}
		manifestID = strings.TrimSpace(id[slash+1:])
	}

	var roots []string
	if sr := strings.TrimSpace(os.Getenv("SCOOP")); sr != "" {
		roots = append(roots, sr)
	}
	if up := strings.TrimSpace(os.Getenv("USERPROFILE")); up != "" {
		roots = append(roots, filepath.Join(up, "scoop"))
	}
	if sg := strings.TrimSpace(os.Getenv("SCOOP_GLOBAL")); sg != "" {
		roots = append(roots, sg)
	}

	var manifestPath string
	var resolvedBucket string
	for _, root := range roots {
		bucketsDir := filepath.Join(root, "buckets")
		if bucketHint != "" {
			cand := filepath.Join(bucketsDir, bucketHint, "bucket", manifestID+".json")
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				manifestPath = cand
				resolvedBucket = bucketHint
				break
			}
		}
		entries, _ := os.ReadDir(bucketsDir)
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			cand := filepath.Join(bucketsDir, e.Name(), "bucket", manifestID+".json")
			if st, err := os.Stat(cand); err == nil && !st.IsDir() {
				manifestPath = cand
				resolvedBucket = e.Name()
				break
			}
		}
		if manifestPath != "" {
			break
		}
	}

	if manifestPath != "" {
		if det, err := parseScoopManifestFile(manifestPath, id, resolvedBucket); err == nil {
			return det, nil
		}
	}

	// Fallback to `scoop info <id>` when manifest isn't locally cached.
	exe := tools.FindPkgMgrExe(ManagerScoop, baseDir)
	if exe == "" {
		return essentials.PackageDetails{}, fmt.Errorf("scoop manifest not found locally and scoop CLI unavailable")
	}
	target := id
	if bucketHint != "" && !strings.Contains(id, "/") {
		target = bucketHint + "/" + id
	}
	out, err := runCmdCapture(ctx, tools.BuildMergedEnv(baseDir), exe, "info", target)
	if err != nil && strings.TrimSpace(out) == "" {
		return essentials.PackageDetails{}, err
	}
	return parseScoopInfoText(out, id, bucketHint), nil
}

func parseScoopManifestFile(path, id, bucket string) (essentials.PackageDetails, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return essentials.PackageDetails{}, err
	}
	var raw struct {
		Version     string `json:"version"`
		Description string `json:"description"`
		Homepage    string `json:"homepage"`
		License     any    `json:"license"`
		URL         any    `json:"url"`
		Hash        any    `json:"hash"`
		Depends     any    `json:"depends"`
		Architecture map[string]struct {
			URL  any `json:"url"`
			Hash any `json:"hash"`
		} `json:"architecture"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return essentials.PackageDetails{}, err
	}

	det := essentials.PackageDetails{
		Manager:     ManagerScoop,
		ID:          id,
		Name:        FormatDisplayName(id, ""),
		Version:     strings.TrimSpace(raw.Version),
		Source:      firstNonEmpty(bucket, ManagerScoop),
		Description: strings.TrimSpace(raw.Description),
		Homepage:    sanitizeHTTPURL(raw.Homepage),
		License:     extractScoopLicense(raw.License),
		Loaded:      true,
	}
	det.InstallerURL = firstStringFromAny(raw.Architecture["64bit"].URL, raw.URL)
	det.InstallerSHA256 = firstStringFromAny(raw.Architecture["64bit"].Hash, raw.Hash)
	det.Dependencies = extractStringSliceAny(raw.Depends)
	return det, nil
}

func parseScoopInfoText(raw, id, bucket string) essentials.PackageDetails {
	det := essentials.PackageDetails{
		Manager: ManagerScoop,
		ID:      id,
		Name:    FormatDisplayName(id, ""),
		Source:  firstNonEmpty(bucket, ManagerScoop),
		Loaded:  strings.TrimSpace(raw) != "",
	}
	for _, line := range strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n") {
		colon := strings.Index(line, ":")
		if colon <= 0 {
			continue
		}
		k := strings.ToLower(strings.TrimSpace(line[:colon]))
		v := strings.TrimSpace(line[colon+1:])
		if v == "" {
			continue
		}
		switch k {
		case "description":
			det.Description = v
		case "version":
			det.Version = v
		case "website":
			det.Homepage = sanitizeHTTPURL(v)
		case "license":
			det.License = v
		case "bucket":
			det.Source = v
		}
	}
	return det
}

func getChocoPackageDetails(ctx context.Context, feedBaseURL, id string) (essentials.PackageDetails, error) {
	return getNuGetV2PackageDetails(ctx, feedBaseURL, ManagerChoco, id)
}

func getNuGetV2PackageDetails(ctx context.Context, feedBaseURL, mgrID, id string) (essentials.PackageDetails, error) {
	escapedID := url.QueryEscape(strings.ReplaceAll(strings.ToLower(strings.TrimSpace(id)), "'", "''"))
	endpoint := fmt.Sprintf(
		"%s/Search()?%%24filter=IsLatestVersion&%%24orderby=DownloadCount%%20desc&searchTerm='%s'&targetFramework=''&includePrerelease=false&%%24top=10",
		strings.TrimRight(feedBaseURL, "/"),
		escapedID,
	)
	entries, err := fetchChocoAtomFeed(ctx, endpoint)
	if err != nil {
		return essentials.PackageDetails{}, err
	}
	var chosen *chocoAtomEntry
	for i := range entries {
		if strings.EqualFold(parseChocoEntryID(entries[i]), id) {
			chosen = &entries[i]
			break
		}
	}
	if chosen == nil && len(entries) > 0 {
		chosen = &entries[0]
	}
	if chosen == nil {
		return essentials.PackageDetails{}, fmt.Errorf("package %s not found on %s feed", id, mgrID)
	}

	pkgID := parseChocoEntryID(*chosen)
	var deps []string
	for _, part := range strings.Split(chosen.Props.Dependencies, "|") {
		fields := strings.Split(part, ":")
		depID := strings.TrimSpace(fields[0])
		if depID != "" {
			if len(fields) > 1 && strings.TrimSpace(fields[1]) != "" {
				deps = append(deps, depID+" "+strings.TrimSpace(fields[1]))
			} else {
				deps = append(deps, depID)
			}
		}
	}
	var tags []string
	for _, t := range strings.Fields(chosen.Props.Tags) {
		tags = append(tags, t)
	}

	published := strings.TrimSpace(chosen.Props.Published)
	if len(published) > 10 {
		published = published[:10]
	}

	srcLabel := mgrID
	if mgrID == ManagerPSGallery {
		srcLabel = "PSGallery"
	}
	return essentials.PackageDetails{
		Manager:         mgrID,
		ID:              pkgID,
		Name:            FormatDisplayName(pkgID, chosen.Props.Title),
		Version:         strings.TrimSpace(chosen.Props.Version),
		Source:          srcLabel,
		Publisher:       strings.TrimSpace(chosen.Author.Name),
		Author:          strings.TrimSpace(chosen.Author.Name),
		Description:     firstNonEmpty(strings.TrimSpace(chosen.Props.Description), strings.TrimSpace(chosen.Summary)),
		Homepage:        firstNonEmpty(sanitizeHTTPURL(chosen.Props.ProjectURL), sanitizeHTTPURL(chosen.Props.GalleryDetails)),
		LicenseURL:      sanitizeHTTPURL(chosen.Props.LicenseURL),
		ReleaseDate:     published,
		ReleaseNotesURL: sanitizeHTTPURL(chosen.Props.ReleaseNotesURL),
		InstallerType:   "nupkg",
		InstallerURL:    sanitizeHTTPURL(chosen.Content.Src),
		InstallerSize:   chosen.Props.PackageSize,
		InstallerSHA256: strings.TrimSpace(chosen.Props.PackageHash),
		Tags:            tags,
		Dependencies:    deps,
		DownloadCount:   chosen.Props.DownloadCount,
		IconURL:         sanitizeHTTPURL(chosen.Props.IconURL),
		Loaded:          true,
	}, nil
}

func getNpmPackageDetails(ctx context.Context, baseURL, mgr, id string) (essentials.PackageDetails, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/" + id + "/latest"
	var resp struct {
		Name        string `json:"name"`
		Version     string `json:"version"`
		Description string `json:"description"`
		Homepage    string `json:"homepage"`
		License     any    `json:"license"`
		Keywords    []string `json:"keywords"`
		Author      any    `json:"author"`
		Dependencies map[string]string `json:"dependencies"`
		Dist        struct {
			Tarball      string `json:"tarball"`
			Integrity    string `json:"integrity"`
			UnpackedSize int64  `json:"unpackedSize"`
		} `json:"dist"`
	}
	if err := fetchJSON(ctx, endpoint, &resp); err != nil {
		return essentials.PackageDetails{}, err
	}

	var deps []string
	for k, v := range resp.Dependencies {
		deps = append(deps, fmt.Sprintf("%s@%s", k, v))
	}
	if len(deps) > 25 {
		deps = deps[:25]
	}

	return essentials.PackageDetails{
		Manager:         mgr,
		ID:              firstNonEmpty(resp.Name, id),
		Name:            firstNonEmpty(resp.Name, id),
		Version:         strings.TrimSpace(resp.Version),
		Source:          mgr,
		Author:          extractPersonName(resp.Author),
		Description:     strings.TrimSpace(resp.Description),
		Homepage:        firstNonEmpty(sanitizeHTTPURL(resp.Homepage), "https://www.npmjs.com/package/"+url.PathEscape(id)),
		License:         extractScoopLicense(resp.License),
		InstallerType:   "npm tarball",
		InstallerURL:    sanitizeHTTPURL(resp.Dist.Tarball),
		InstallerSize:   resp.Dist.UnpackedSize,
		InstallerSHA256: strings.TrimSpace(resp.Dist.Integrity),
		Tags:            resp.Keywords,
		Dependencies:    deps,
		Loaded:          true,
	}, nil
}

func getPyPIPackageDetails(ctx context.Context, baseURL, id string) (essentials.PackageDetails, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/" + url.PathEscape(id) + "/json"
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return essentials.PackageDetails{}, err
	}
	req.Header.Set("User-Agent", "UniDevUI/1.0 (Package Manager Hub)")
	resp, err := httpClient.Do(req)
	if err != nil {
		return essentials.PackageDetails{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return essentials.PackageDetails{}, fmt.Errorf("HTTP %d from %s", resp.StatusCode, endpoint)
	}
	info, err := decodePyPIInfoExtended(io.LimitReader(resp.Body, 16*1024*1024))
	if err != nil {
		return essentials.PackageDetails{}, err
	}
	return info, nil
}

func decodePyPIInfoExtended(r io.Reader) (essentials.PackageDetails, error) {
	dec := json.NewDecoder(r)
	tok, err := dec.Token()
	if err != nil {
		return essentials.PackageDetails{}, err
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return essentials.PackageDetails{}, fmt.Errorf("expected JSON object from PyPI")
	}
	for dec.More() {
		keyTok, err := dec.Token()
		if err != nil {
			return essentials.PackageDetails{}, err
		}
		key, _ := keyTok.(string)
		if key == "info" {
			var info struct {
				Name         string            `json:"name"`
				Version      string            `json:"version"`
				Summary      string            `json:"summary"`
				Description  string            `json:"description"`
				Author       string            `json:"author"`
				AuthorEmail  string            `json:"author_email"`
				License      string            `json:"license"`
				Keywords     string            `json:"keywords"`
				HomePage     string            `json:"home_page"`
				PkgURL       string            `json:"package_url"`
				ProjectURLs  map[string]string `json:"project_urls"`
				RequiresDist []string          `json:"requires_dist"`
			}
			if err := dec.Decode(&info); err != nil {
				return essentials.PackageDetails{}, err
			}
			hp := sanitizeHTTPURL(info.HomePage)
			if hp == "" {
				for _, k := range []string{"Homepage", "Repository", "Source", "Documentation"} {
					if hp = sanitizeHTTPURL(info.ProjectURLs[k]); hp != "" {
						break
					}
				}
			}
			if hp == "" {
				hp = sanitizeHTTPURL(info.PkgURL)
			}
			var tags []string
			for _, part := range strings.FieldsFunc(info.Keywords, func(r rune) bool { return r == ',' || r == ' ' }) {
				if p := strings.TrimSpace(part); p != "" {
					tags = append(tags, p)
				}
			}
			deps := info.RequiresDist
			if len(deps) > 25 {
				deps = deps[:25]
			}
			desc := firstNonEmpty(strings.TrimSpace(info.Summary), summarizeMarkdownText(info.Description, 600))
			return essentials.PackageDetails{
				Manager:      ManagerUv,
				ID:           strings.TrimSpace(info.Name),
				Name:         strings.TrimSpace(info.Name),
				Version:      strings.TrimSpace(info.Version),
				Source:       "pypi",
				Author:       firstNonEmpty(strings.TrimSpace(info.Author), strings.TrimSpace(info.AuthorEmail)),
				Description:  desc,
				Homepage:     hp,
				License:      strings.TrimSpace(info.License),
				Tags:         tags,
				Dependencies: deps,
				Loaded:       true,
			}, nil
		}
		var discard json.RawMessage
		if err := dec.Decode(&discard); err != nil {
			return essentials.PackageDetails{}, err
		}
	}
	return essentials.PackageDetails{}, fmt.Errorf("missing info field in PyPI response")
}

func extractScoopLicense(val any) string {
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if id, ok := v["identifier"].(string); ok {
			return strings.TrimSpace(id)
		}
		if typ, ok := v["type"].(string); ok {
			return strings.TrimSpace(typ)
		}
	}
	return ""
}

func extractPersonName(val any) string {
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case map[string]any:
		if n, ok := v["name"].(string); ok {
			return strings.TrimSpace(n)
		}
	}
	return ""
}

func firstStringFromAny(vals ...any) string {
	for _, val := range vals {
		switch v := val.(type) {
		case string:
			if s := sanitizeHTTPURL(v); s != "" {
				return s
			}
			if s := strings.TrimSpace(v); s != "" {
				return s
			}
		case []any:
			for _, item := range v {
				if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
					return strings.TrimSpace(s)
				}
			}
		}
	}
	return ""
}

func extractStringSliceAny(val any) []string {
	switch v := val.(type) {
	case string:
		if strings.TrimSpace(v) != "" {
			return []string{strings.TrimSpace(v)}
		}
	case []any:
		var out []string
		for _, item := range v {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}

func summarizeMarkdownText(raw string, maxRunes int) string {
	joined := strings.Join(strings.Fields(raw), " ")
	if joined == "" {
		return ""
	}
	r := []rune(joined)
	if len(r) <= maxRunes {
		return joined
	}
	return strings.TrimSpace(string(r[:maxRunes-1])) + "…"
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}
