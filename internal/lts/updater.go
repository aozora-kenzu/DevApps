package lts

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"DevApp/internal/config"
)

var sharedClient = &http.Client{Timeout: 20 * time.Second}

// FetchLatestVersions queries the official upstream feed for a given provider.
func FetchLatestVersions(ctx context.Context, provider string) ([]config.VersionEntry, error) {
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "nodejs":
		return fetchNodeLTS(ctx)
	case "php":
		return fetchPHPReleases(ctx)
	case "composer":
		return fetchComposer(ctx)
	case "go":
		return fetchGoReleases(ctx)
	case "git":
		return fetchGitLatest(ctx)
	case "pocketbase":
		return fetchGitHubZipRelease(ctx, "pocketbase/pocketbase", `(?i)^pocketbase_.*_windows_amd64\.zip$`)
	case "sqlite":
		return fetchSQLiteLatest(ctx)
	case "python":
		return fetchPythonStandalone(ctx)
	case "dotnet":
		return fetchDotnetLTS(ctx)
	case "rust-msvc":
		return fetchRustStable(ctx, "x86_64-pc-windows-msvc")
	case "rust-gnu":
		return fetchRustStable(ctx, "x86_64-pc-windows-gnu")
	case "winlibs":
		return fetchWinLibsLatest(ctx)
	case "cmake":
		return fetchGitHubZipRelease(ctx, "Kitware/CMake", `(?i)^cmake-.*-windows-x86_64\.zip$`)
	case "7zip":
		return fetch7ZipLatest(ctx)
	case "manual", "":
		return nil, nil
	default:
		return nil, fmt.Errorf("provider LTS tidak dikenal: %s", provider)
	}
}

func fetchNodeLTS(ctx context.Context) ([]config.VersionEntry, error) {
	type nodeItem struct {
		Version string   `json:"version"`
		Files   []string `json:"files"`
		LTS     any      `json:"lts"`
	}
	var items []nodeItem
	if err := getJSON(ctx, "https://nodejs.org/dist/index.json", &items); err != nil {
		return nil, err
	}

	seenMajor := make(map[string]bool)
	var out []config.VersionEntry
	for _, item := range items {
		if item.LTS == nil || item.LTS == false {
			continue
		}
		if !containsString(item.Files, "win-x64-zip") {
			continue
		}
		ver := strings.TrimPrefix(item.Version, "v")
		major := strings.Split(ver, ".")[0]
		if seenMajor[major] {
			continue
		}
		seenMajor[major] = true
		out = append(out, config.VersionEntry{
			Version: ver,
			URL:     fmt.Sprintf("https://nodejs.org/dist/%s/node-%s-win-x64.zip", item.Version, item.Version),
		})
		if len(out) >= 4 {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tidak menemukan versi Node.js LTS")
	}
	return out, nil
}

func fetchPHPReleases(ctx context.Context) ([]config.VersionEntry, error) {
	var raw map[string]json.RawMessage
	if err := getJSON(ctx, "https://windows.php.net/downloads/releases/releases.json", &raw); err != nil {
		return nil, err
	}

	type candidate struct {
		major   string
		version string
		url     string
	}
	var cands []candidate

	for majorKey, val := range raw {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(val, &obj); err != nil {
			continue
		}
		var fullVer string
		if rawVer, ok := obj["version"]; ok {
			_ = json.Unmarshal(rawVer, &fullVer)
		}
		if fullVer == "" {
			continue
		}

		// Pick the highest VS-runtime NTS x64 build available for this PHP minor.
		var bestKey string
		var zipPath string
		for k, v := range obj {
			if !strings.HasPrefix(k, "nts-") || !strings.Contains(k, "x64") {
				continue
			}
			var build struct {
				Zip struct {
					Path string `json:"path"`
				} `json:"zip"`
			}
			if err := json.Unmarshal(v, &build); err != nil || build.Zip.Path == "" {
				continue
			}
			if k > bestKey {
				bestKey = k
				zipPath = build.Zip.Path
			}
		}
		if zipPath != "" {
			cands = append(cands, candidate{
				major:   majorKey,
				version: fullVer,
				url:     "https://windows.php.net/downloads/releases/" + strings.TrimLeft(zipPath, "/"),
			})
		}
	}

	sort.Slice(cands, func(i, j int) bool {
		return compareVersionDesc(cands[i].version, cands[j].version)
	})

	if len(cands) > 4 {
		cands = cands[:4]
	}
	out := make([]config.VersionEntry, 0, len(cands))
	for _, c := range cands {
		out = append(out, config.VersionEntry{Version: c.version, URL: c.url})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tidak menemukan arsip PHP NTS x64")
	}
	return out, nil
}

func fetchComposer(ctx context.Context) ([]config.VersionEntry, error) {
	var resp struct {
		Stable []struct {
			Path    string `json:"path"`
			Version string `json:"version"`
		} `json:"stable"`
	}
	if err := getJSON(ctx, "https://getcomposer.org/versions", &resp); err != nil {
		return nil, err
	}

	var out []config.VersionEntry
	seen := make(map[string]bool)
	for _, s := range resp.Stable {
		if s.Version == "" || s.Path == "" || seen[s.Version] {
			continue
		}
		seen[s.Version] = true
		out = append(out, config.VersionEntry{
			Version: s.Version,
			URL:     "https://getcomposer.org" + s.Path,
		})
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tidak menemukan versi Composer stabil")
	}
	return out, nil
}

func fetchGoReleases(ctx context.Context) ([]config.VersionEntry, error) {
	var items []struct {
		Version string `json:"version"`
		Stable  bool   `json:"stable"`
		Files   []struct {
			Filename string `json:"filename"`
			OS       string `json:"os"`
			Arch     string `json:"arch"`
			Kind     string `json:"kind"`
		} `json:"files"`
	}
	if err := getJSON(ctx, "https://go.dev/dl/?mode=json", &items); err != nil {
		return nil, err
	}

	var out []config.VersionEntry
	for _, item := range items {
		if !item.Stable {
			continue
		}
		for _, f := range item.Files {
			if f.OS == "windows" && f.Arch == "amd64" && f.Kind == "archive" && strings.HasSuffix(f.Filename, ".zip") {
				out = append(out, config.VersionEntry{
					Version: strings.TrimPrefix(item.Version, "go"),
					URL:     "https://go.dev/dl/" + f.Filename,
				})
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tidak menemukan arsip Go Windows amd64")
	}
	return out, nil
}

func fetchGitLatest(ctx context.Context) ([]config.VersionEntry, error) {
	rel, err := fetchGitHubLatestRelease(ctx, "git-for-windows/git")
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`(?i)^MinGit-([0-9.]+)-64-bit\.zip$`)
	for _, a := range rel.Assets {
		if m := re.FindStringSubmatch(a.Name); len(m) == 2 {
			return []config.VersionEntry{{Version: m[1], URL: a.BrowserDownloadURL}}, nil
		}
	}
	return nil, fmt.Errorf("asset MinGit 64-bit zip tidak ditemukan di rilis %s", rel.TagName)
}

func fetchDotnetLTS(ctx context.Context) ([]config.VersionEntry, error) {
	var index struct {
		Channels []struct {
			ChannelVersion string `json:"channel-version"`
			ReleaseType    string `json:"release-type"`
			SupportPhase   string `json:"support-phase"`
			ReleasesJSON   string `json:"releases.json"`
		} `json:"releases-index"`
	}
	if err := getJSON(ctx, "https://dotnetcli.blob.core.windows.net/dotnet/release-metadata/releases-index.json", &index); err != nil {
		return nil, err
	}

	var out []config.VersionEntry
	for _, ch := range index.Channels {
		if !strings.EqualFold(ch.ReleaseType, "lts") || !strings.EqualFold(ch.SupportPhase, "active") {
			continue
		}
		var chRel struct {
			Releases []struct {
				SDK struct {
					Version string `json:"version"`
					Files   []struct {
						Name string `json:"name"`
						RID  string `json:"rid"`
						URL  string `json:"url"`
					} `json:"files"`
				} `json:"sdk"`
			} `json:"releases"`
		}
		if err := getJSON(ctx, ch.ReleasesJSON, &chRel); err != nil || len(chRel.Releases) == 0 {
			continue
		}
		latest := chRel.Releases[0]
		for _, f := range latest.SDK.Files {
			if f.RID == "win-x64" && strings.HasSuffix(strings.ToLower(f.Name), ".zip") {
				out = append(out, config.VersionEntry{
					Version: latest.SDK.Version,
					URL:     f.URL,
				})
				break
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tidak menemukan channel .NET SDK LTS aktif")
	}
	return out, nil
}

func fetchRustStable(ctx context.Context, targetTriple string) ([]config.VersionEntry, error) {
	body, err := getText(ctx, "https://static.rust-lang.org/dist/channel-rust-stable.toml")
	if err != nil {
		return nil, err
	}

	sectionHeader := fmt.Sprintf("[pkg.rust.target.%s]", targetTriple)
	idx := strings.Index(body, sectionHeader)
	if idx < 0 {
		return nil, fmt.Errorf("section %s tidak ditemukan di manifest Rust", targetTriple)
	}
	sub := body[idx:]
	if nextSec := strings.Index(sub[len(sectionHeader):], "\n["); nextSec >= 0 {
		sub = sub[:len(sectionHeader)+nextSec]
	}

	urlRe := regexp.MustCompile(`(?m)^url\s*=\s*"([^"]+)"`)
	m := urlRe.FindStringSubmatch(sub)
	if len(m) < 2 {
		return nil, fmt.Errorf("URL arsip Rust %s tidak ditemukan", targetTriple)
	}
	tarURL := m[1]

	verRe := regexp.MustCompile(`rust-([0-9]+\.[0-9]+\.[0-9]+)-`)
	vm := verRe.FindStringSubmatch(tarURL)
	ver := "stable"
	if len(vm) == 2 {
		ver = vm[1]
	}
	return []config.VersionEntry{{Version: ver, URL: tarURL}}, nil
}

func fetchWinLibsLatest(ctx context.Context) ([]config.VersionEntry, error) {
	rel, err := fetchGitHubLatestRelease(ctx, "brechtsanders/winlibs_mingw")
	if err != nil {
		return nil, err
	}
	reAsset := regexp.MustCompile(`(?i)^winlibs-x86_64-posix-seh-gcc-([0-9.]+).*ucrt.*\.zip$`)
	for _, a := range rel.Assets {
		if m := reAsset.FindStringSubmatch(a.Name); len(m) == 2 {
			return []config.VersionEntry{{Version: m[1], URL: a.BrowserDownloadURL}}, nil
		}
	}
	return nil, fmt.Errorf("asset WinLibs x86_64 UCRT zip tidak ditemukan di %s", rel.TagName)
}

func fetch7ZipLatest(ctx context.Context) ([]config.VersionEntry, error) {
	rel, err := fetchGitHubLatestRelease(ctx, "ip7z/7zip")
	if err != nil {
		return nil, err
	}
	for _, a := range rel.Assets {
		if strings.EqualFold(a.Name, "7zr.exe") {
			return []config.VersionEntry{{
				Version: strings.TrimPrefix(rel.TagName, "v"),
				URL:     a.BrowserDownloadURL,
			}}, nil
		}
	}
	return nil, fmt.Errorf("asset 7zr.exe tidak ditemukan di rilis %s", rel.TagName)
}

func fetchSQLiteLatest(ctx context.Context) ([]config.VersionEntry, error) {
	html, err := getText(ctx, "https://sqlite.org/download.html")
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(`(?m)^PRODUCT,([^,]+),([0-9]+/sqlite-tools-win-x64-[^,\r\n]+\.zip),`)
	m := re.FindStringSubmatch(html)
	if len(m) < 3 {
		return nil, fmt.Errorf("entri sqlite-tools-win-x64 tidak ditemukan di sqlite.org/download.html")
	}
	return []config.VersionEntry{{
		Version: strings.TrimSpace(m[1]),
		URL:     "https://sqlite.org/" + strings.TrimLeft(strings.TrimSpace(m[2]), "/"),
	}}, nil
}

func fetchPythonStandalone(ctx context.Context) ([]config.VersionEntry, error) {
	rel, err := fetchGitHubLatestRelease(ctx, "astral-sh/python-build-standalone")
	if err != nil {
		return nil, err
	}

	re := regexp.MustCompile(`^cpython-(\d+\.\d+\.\d+)\+[^-]+-x86_64-pc-windows-msvc-install_only_stripped\.tar\.gz$`)
	byMinor := make(map[string]config.VersionEntry)

	for _, a := range rel.Assets {
		m := re.FindStringSubmatch(a.Name)
		if len(m) != 2 {
			continue
		}
		ver := m[1]
		parts := strings.Split(ver, ".")
		if len(parts) < 2 {
			continue
		}
		minorKey := parts[0] + "." + parts[1]
		if existing, ok := byMinor[minorKey]; !ok || compareVersionDesc(ver, existing.Version) {
			byMinor[minorKey] = config.VersionEntry{
				Version: ver,
				URL:     a.BrowserDownloadURL,
			}
		}
	}

	out := make([]config.VersionEntry, 0, len(byMinor))
	for _, entry := range byMinor {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool {
		return compareVersionDesc(out[i].Version, out[j].Version)
	})
	if len(out) > 4 {
		out = out[:4]
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("tidak menemukan aset Windows x64 install_only_stripped di rilis %s", rel.TagName)
	}
	return out, nil
}

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

func fetchGitHubLatestRelease(ctx context.Context, repo string) (*githubRelease, error) {
	var rel githubRelease
	url := fmt.Sprintf("https://api.github.com/repos/%s/releases/latest", repo)
	if err := getJSON(ctx, url, &rel); err != nil {
		return nil, err
	}
	return &rel, nil
}

func fetchGitHubZipRelease(ctx context.Context, repo, pattern string) ([]config.VersionEntry, error) {
	rel, err := fetchGitHubLatestRelease(ctx, repo)
	if err != nil {
		return nil, err
	}
	re := regexp.MustCompile(pattern)
	for _, a := range rel.Assets {
		if re.MatchString(a.Name) {
			return []config.VersionEntry{{
				Version: strings.TrimPrefix(rel.TagName, "v"),
				URL:     a.BrowserDownloadURL,
			}}, nil
		}
	}
	return nil, fmt.Errorf("asset cocok %s tidak ditemukan di %s (%s)", pattern, repo, rel.TagName)
}

func getJSON(ctx context.Context, url string, dst any) error {
	body, err := getText(ctx, url)
	if err != nil {
		return err
	}
	if err := json.Unmarshal([]byte(body), dst); err != nil {
		return fmt.Errorf("gagal parsing JSON %s: %w", url, err)
	}
	return nil
}

func getText(ctx context.Context, url string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "DevApp/1.0 (LTS Updater)")
	resp, err := sharedClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d dari %s", resp.StatusCode, url)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func containsString(list []string, target string) bool {
	for _, item := range list {
		if item == target {
			return true
		}
	}
	return false
}

func compareVersionDesc(a, b string) bool {
	pa := strings.Split(a, ".")
	pb := strings.Split(b, ".")
	maxLen := len(pa)
	if len(pb) > maxLen {
		maxLen = len(pb)
	}
	for i := 0; i < maxLen; i++ {
		var va, vb int
		if i < len(pa) {
			va, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			vb, _ = strconv.Atoi(pb[i])
		}
		if va != vb {
			return va > vb
		}
	}
	return a > b
}
