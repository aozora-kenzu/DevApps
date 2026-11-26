package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/aozora-kenzu/UniDevUI/internal/archive"
	"github.com/aozora-kenzu/UniDevUI/internal/pathmgr"
)

const (
	// DefaultLatestReleaseURL points to the public GitHub latest-release endpoint for UniDevUI.
	DefaultLatestReleaseURL = "https://api.github.com/repos/aozora-kenzu/UniDevUI/releases/latest"

	// EnvUpdateURLOverride lets dev or local test environments override the release endpoint URL.
	EnvUpdateURLOverride = "UNIDEVUI_UPDATE_URL"

	// DefaultCheckCooldown limits automatic startup checks so GitHub API rate limits are respected.
	DefaultCheckCooldown = 6 * time.Hour

	expectedBinaryName = "UniDevUI.exe"
)

// UpdateInfo describes the latest UniDevUI release state compared to the running version.
type UpdateInfo struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	TagName        string `json:"tagName"`
	HasUpdate      bool   `json:"hasUpdate"`
	ReleaseURL     string `json:"releaseUrl"`
	ReleaseNotes   string `json:"releaseNotes"`
	PublishedAt    string `json:"publishedAt"`
	AssetName      string `json:"assetName"`
	DownloadURL    string `json:"downloadUrl"`
	DownloadSize   int64  `json:"downloadSize"`
	SHA256         string `json:"sha256"`
	CheckedAt      string `json:"checkedAt"`
	CheckError     string `json:"checkError,omitempty"`
}

type githubReleaseResponse struct {
	TagName     string                `json:"tag_name"`
	Name        string                `json:"name"`
	HTMLURL     string                `json:"html_url"`
	Body        string                `json:"body"`
	PublishedAt string                `json:"published_at"`
	Draft       bool                  `json:"draft"`
	Prerelease  bool                  `json:"prerelease"`
	Assets      []githubReleaseAsset  `json:"assets"`
}

type githubReleaseAsset struct {
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	Digest             string `json:"digest"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type cachedUpdateRecord struct {
	CheckedAtUnix int64      `json:"checkedAtUnix"`
	Info          UpdateInfo `json:"info"`
}

// EffectiveEndpointURL returns UNIDEVUI_UPDATE_URL when set, otherwise DefaultLatestReleaseURL.
func EffectiveEndpointURL() string {
	if override := strings.TrimSpace(os.Getenv(EnvUpdateURLOverride)); override != "" {
		return override
	}
	return DefaultLatestReleaseURL
}

// NormalizeVersion trims optional leading "v" and surrounding whitespace.
func NormalizeVersion(raw string) string {
	s := strings.TrimSpace(raw)
	if len(s) > 0 && (s[0] == 'v' || s[0] == 'V') {
		s = s[1:]
	}
	return strings.TrimSpace(s)
}

// IsNewerVersion returns true when candidate is strictly newer than current using semver rules.
func IsNewerVersion(candidate, current string) bool {
	candNorm := NormalizeVersion(candidate)
	curNorm := NormalizeVersion(current)
	if candNorm == "" || curNorm == "" {
		return false
	}
	return pathmgr.CompareSemver(candNorm, curNorm) > 0
}

// LoadCachedInfo reads cached release metadata from cachePath and evaluates HasUpdate against currentVersion.
func LoadCachedInfo(cachePath, currentVersion string, maxAge time.Duration) (UpdateInfo, bool) {
	cur := NormalizeVersion(currentVersion)
	base := UpdateInfo{CurrentVersion: cur}
	if strings.TrimSpace(cachePath) == "" {
		return base, false
	}

	raw, err := os.ReadFile(cachePath)
	if err != nil {
		return base, false
	}
	var rec cachedUpdateRecord
	if err := json.Unmarshal(raw, &rec); err != nil {
		return base, false
	}
	info := rec.Info
	info.CurrentVersion = cur
	info.HasUpdate = IsNewerVersion(info.LatestVersion, cur)

	if maxAge > 0 && rec.CheckedAtUnix > 0 {
		checkedTime := time.Unix(rec.CheckedAtUnix, 0)
		fresh := time.Since(checkedTime) < maxAge
		return info, fresh
	}
	return info, false
}

// SaveCachedInfo persists release metadata so repeated startups do not hammer the release API.
func SaveCachedInfo(cachePath string, info UpdateInfo) error {
	if strings.TrimSpace(cachePath) == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(cachePath), 0o755); err != nil {
		return err
	}
	rec := cachedUpdateRecord{
		CheckedAtUnix: time.Now().Unix(),
		Info:          info,
	}
	data, err := json.MarshalIndent(rec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(cachePath, data, 0o644)
}

// CheckLatestRelease queries EffectiveEndpointURL and compares the latest published version against currentVersion.
func CheckLatestRelease(ctx context.Context, currentVersion string) (UpdateInfo, error) {
	return CheckLatestReleaseWithURL(ctx, EffectiveEndpointURL(), currentVersion)
}

// CheckLatestReleaseWithURL queries endpointURL (in GitHub Releases JSON format) and evaluates update status.
func CheckLatestReleaseWithURL(ctx context.Context, endpointURL, currentVersion string) (UpdateInfo, error) {
	cur := NormalizeVersion(currentVersion)
	result := UpdateInfo{
		CurrentVersion: cur,
		CheckedAt:      time.Now().Format("15:04:05"),
	}
	if ctx == nil {
		ctx = context.Background()
	}

	reqCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimSpace(endpointURL), nil)
	if err != nil {
		result.CheckError = err.Error()
		return result, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "UniDevUI/"+cur)

	resp, err := (&http.Client{Timeout: 12 * time.Second}).Do(req)
	if err != nil {
		result.CheckError = err.Error()
		return result, fmt.Errorf("release check failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		err = fmt.Errorf("release endpoint returned HTTP %d (%s)", resp.StatusCode, resp.Status)
		result.CheckError = err.Error()
		return result, err
	}

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		result.CheckError = err.Error()
		return result, fmt.Errorf("failed reading release response: %w", err)
	}

	var rel githubReleaseResponse
	if err := json.Unmarshal(bodyBytes, &rel); err != nil {
		result.CheckError = err.Error()
		return result, fmt.Errorf("failed parsing release JSON: %w", err)
	}
	if rel.Draft || rel.Prerelease {
		err = fmt.Errorf("latest release %q is not a stable public release", rel.TagName)
		result.CheckError = err.Error()
		return result, err
	}

	latest := NormalizeVersion(rel.TagName)
	result.TagName = strings.TrimSpace(rel.TagName)
	result.LatestVersion = latest
	result.HasUpdate = IsNewerVersion(latest, cur)
	result.ReleaseURL = strings.TrimSpace(rel.HTMLURL)
	result.ReleaseNotes = strings.TrimSpace(rel.Body)
	result.PublishedAt = strings.TrimSpace(rel.PublishedAt)

	if asset, ok := pickExecutableAsset(rel.Assets); ok {
		result.AssetName = asset.Name
		result.DownloadURL = strings.TrimSpace(asset.BrowserDownloadURL)
		result.DownloadSize = asset.Size
		result.SHA256 = parseSHA256Digest(asset.Digest)
	}
	return result, nil
}

// DownloadVerifyAndSwap downloads the release executable alongside exePath, verifies PE header + SHA256,
// renames the currently running binary to <exePath>.old, and places the new executable at exePath.
func DownloadVerifyAndSwap(
	ctx context.Context,
	info UpdateInfo,
	exePath string,
	onProgress archive.ProgressCallback,
	log archive.Logger,
) (string, error) {
	if strings.TrimSpace(info.DownloadURL) == "" {
		return "", fmt.Errorf("release asset download URL is empty")
	}
	cleanExe := filepath.Clean(strings.TrimSpace(exePath))
	if cleanExe == "" {
		return "", fmt.Errorf("target executable path is empty")
	}

	exeDir := filepath.Dir(cleanExe)
	stagedName := filepath.Base(cleanExe) + ".new"
	stagedPath := filepath.Join(exeDir, stagedName)
	_ = os.Remove(stagedPath)

	downloadedPath, err := archive.DownloadFileNamed(ctx, info.DownloadURL, exeDir, stagedName, onProgress, log)
	if err != nil {
		_ = os.Remove(stagedPath)
		return "", err
	}

	backupPath, err := ApplyDownloadedExecutable(cleanExe, downloadedPath, info.SHA256)
	if err != nil {
		return "", err
	}
	return backupPath, nil
}

// VerifyExecutableFile confirms filePath is a non-trivial Windows PE ("MZ") executable matching expectedSHA256 when provided.
func VerifyExecutableFile(filePath, expectedSHA256 string) error {
	st, err := os.Stat(filePath)
	if err != nil {
		return fmt.Errorf("failed stating downloaded executable: %w", err)
	}
	if st.Size() < 1024 {
		return fmt.Errorf("downloaded file is unexpectedly small (%d bytes)", st.Size())
	}

	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed opening downloaded executable: %w", err)
	}
	defer f.Close()

	var magic [2]byte
	if _, err := io.ReadFull(f, magic[:]); err != nil {
		return fmt.Errorf("failed reading executable header: %w", err)
	}
	if magic[0] != 'M' || magic[1] != 'Z' {
		return fmt.Errorf("downloaded file is not a valid Windows executable (missing MZ header)")
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return fmt.Errorf("failed rewinding downloaded file: %w", err)
	}

	cleanExpected := strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.ToLower(expectedSHA256), "sha256:")))
	if cleanExpected == "" {
		return nil
	}

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return fmt.Errorf("failed hashing downloaded executable: %w", err)
	}
	actual := hex.EncodeToString(h.Sum(nil))
	if !strings.EqualFold(actual, cleanExpected) {
		return fmt.Errorf("SHA-256 mismatch: expected %s, got %s", cleanExpected, actual)
	}
	return nil
}

// ApplyDownloadedExecutable verifies stagedNewPath and replaces currentExePath while preserving backupPath (.old).
func ApplyDownloadedExecutable(currentExePath, stagedNewPath, expectedSHA256 string) (string, error) {
	if err := VerifyExecutableFile(stagedNewPath, expectedSHA256); err != nil {
		_ = os.Remove(stagedNewPath)
		return "", err
	}

	backupPath := currentExePath + ".old"
	if err := os.Remove(backupPath); err != nil && !os.IsNotExist(err) {
		_ = os.Remove(stagedNewPath)
		return "", fmt.Errorf("failed removing previous backup %s: %w", backupPath, err)
	}

	if err := os.Rename(currentExePath, backupPath); err != nil {
		_ = os.Remove(stagedNewPath)
		return "", fmt.Errorf("failed moving current executable to %s: %w", backupPath, err)
	}

	if err := os.Rename(stagedNewPath, currentExePath); err != nil {
		_ = os.Rename(backupPath, currentExePath)
		_ = os.Remove(stagedNewPath)
		return "", fmt.Errorf("failed activating updated executable %s: %w", currentExePath, err)
	}
	return backupPath, nil
}

// StartDetachedProcess launches exePath independently so the current process can exit immediately.
func StartDetachedProcess(exePath string) error {
	clean := filepath.Clean(strings.TrimSpace(exePath))
	if clean == "" {
		return fmt.Errorf("empty executable path")
	}
	cmd := exec.Command(clean)
	cmd.Dir = filepath.Dir(clean)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed launching updated executable: %w", err)
	}
	if cmd.Process != nil {
		_ = cmd.Process.Release()
	}
	return nil
}

// CleanupStaleArtifacts deletes leftover <exePath>.old and <exePath>.new files created by a self-update.
// It retries briefly so an exiting parent process has time to release its OS file lock.
func CleanupStaleArtifacts(exePath string) {
	clean := filepath.Clean(strings.TrimSpace(exePath))
	if clean == "" {
		return
	}
	targets := []string{clean + ".old", clean + ".new"}
	for attempt := 0; attempt < 20; attempt++ {
		remaining := false
		for _, target := range targets {
			err := os.Remove(target)
			if err != nil && !os.IsNotExist(err) {
				remaining = true
			}
		}
		if !remaining {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}
}

func pickExecutableAsset(assets []githubReleaseAsset) (githubReleaseAsset, bool) {
	for _, a := range assets {
		if strings.EqualFold(strings.TrimSpace(a.Name), expectedBinaryName) && strings.TrimSpace(a.BrowserDownloadURL) != "" {
			return a, true
		}
	}
	for _, a := range assets {
		if strings.HasSuffix(strings.ToLower(strings.TrimSpace(a.Name)), ".exe") && strings.TrimSpace(a.BrowserDownloadURL) != "" {
			return a, true
		}
	}
	return githubReleaseAsset{}, false
}

func parseSHA256Digest(raw string) string {
	trimmed := strings.TrimSpace(raw)
	lower := strings.ToLower(trimmed)
	if strings.HasPrefix(lower, "sha256:") {
		return strings.TrimSpace(trimmed[len("sha256:"):])
	}
	return trimmed
}
