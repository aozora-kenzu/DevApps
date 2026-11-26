package selfupdate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/aozora-kenzu/UniDevUI/internal/archive"
)

func makeFakePEPayload(tag byte, size int) ([]byte, string) {
	if size < 2048 {
		size = 2048
	}
	buf := make([]byte, size)
	buf[0] = 'M'
	buf[1] = 'Z'
	buf[2] = tag
	sum := sha256.Sum256(buf)
	return buf, hex.EncodeToString(sum[:])
}

func TestCheckLatestReleaseAndCache(t *testing.T) {
	payload, digest := makeFakePEPayload('2', 4096)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(githubReleaseResponse{
				TagName:     "v1.0.2",
				HTMLURL:     "https://github.com/aozora-kenzu/UniDevUI/releases/tag/v1.0.2",
				Body:        "Bug fixes & self-update support",
				PublishedAt: "2026-11-26T08:00:00Z",
				Assets: []githubReleaseAsset{
					{
						Name:               "UniDevUI-windows-amd64.zip",
						Size:               1234,
						BrowserDownloadURL: "http://" + r.Host + "/download/UniDevUI.zip",
					},
					{
						Name:               "UniDevUI.exe",
						Size:               int64(len(payload)),
						Digest:             "sha256:" + digest,
						BrowserDownloadURL: "http://" + r.Host + "/download/UniDevUI.exe",
					},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	info, err := CheckLatestReleaseWithURL(context.Background(), srv.URL+"/latest", "1.0.1")
	if err != nil {
		t.Fatalf("CheckLatestReleaseWithURL failed: %v", err)
	}
	if !info.HasUpdate {
		t.Fatalf("expected HasUpdate=true for 1.0.1 -> v1.0.2")
	}
	if info.LatestVersion != "1.0.2" || info.AssetName != "UniDevUI.exe" || info.SHA256 != digest {
		t.Fatalf("unexpected release info parsed: %+v", info)
	}

	// Same or newer running version must not report an update.
	sameInfo, err := CheckLatestReleaseWithURL(context.Background(), srv.URL+"/latest", "1.0.2")
	if err != nil {
		t.Fatalf("CheckLatestReleaseWithURL same-ver failed: %v", err)
	}
	if sameInfo.HasUpdate {
		t.Fatalf("expected HasUpdate=false when already on 1.0.2")
	}

	cachePath := filepath.Join(t.TempDir(), "app_update_cache.json")
	if err := SaveCachedInfo(cachePath, info); err != nil {
		t.Fatalf("SaveCachedInfo failed: %v", err)
	}
	loadedOld, fresh := LoadCachedInfo(cachePath, "1.0.1", time.Hour)
	if !fresh || !loadedOld.HasUpdate {
		t.Fatalf("expected fresh cached update for 1.0.1, got fresh=%v hasUpdate=%v", fresh, loadedOld.HasUpdate)
	}
	loadedUpdated, _ := LoadCachedInfo(cachePath, "1.0.2", time.Hour)
	if loadedUpdated.HasUpdate {
		t.Fatalf("expected cached HasUpdate=false after version bumped to 1.0.2")
	}
}

func TestDownloadVerifySwapAndRejectCorruptSHA(t *testing.T) {
	newExeBytes, newDigest := makeFakePEPayload('N', 4096)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(newExeBytes)
	}))
	defer srv.Close()

	dir := t.TempDir()
	currentExe := filepath.Join(dir, "UniDevUI.exe")
	oldBytes, _ := makeFakePEPayload('O', 2048)
	if err := os.WriteFile(currentExe, oldBytes, 0o755); err != nil {
		t.Fatalf("write initial exe: %v", err)
	}

	// 1. Mismatch digest must fail cleanly without replacing currentExe.
	badInfo := UpdateInfo{
		DownloadURL: srv.URL + "/UniDevUI.exe",
		SHA256:      "0000000000000000000000000000000000000000000000000000000000000000",
	}
	if _, err := DownloadVerifyAndSwap(context.Background(), badInfo, currentExe, nil, nil); err == nil {
		t.Fatalf("expected SHA-256 mismatch error")
	}
	afterBad, err := os.ReadFile(currentExe)
	if err != nil || len(afterBad) != len(oldBytes) || afterBad[2] != 'O' {
		t.Fatalf("expected original exe untouched on SHA mismatch")
	}

	// 2. Matching digest swaps old -> .old and new -> UniDevUI.exe cleanly.
	goodInfo := UpdateInfo{
		DownloadURL: srv.URL + "/UniDevUI.exe",
		SHA256:      newDigest,
	}
	backupPath, err := DownloadVerifyAndSwap(context.Background(), goodInfo, currentExe, nil, nil)
	if err != nil {
		t.Fatalf("DownloadVerifyAndSwap failed: %v", err)
	}
	if backupPath != currentExe+".old" {
		t.Fatalf("unexpected backupPath %q", backupPath)
	}
	activeBytes, err := os.ReadFile(currentExe)
	if err != nil || len(activeBytes) != len(newExeBytes) || activeBytes[2] != 'N' {
		t.Fatalf("expected active exe replaced with new payload")
	}
	oldBackup, err := os.ReadFile(backupPath)
	if err != nil || oldBackup[2] != 'O' {
		t.Fatalf("expected old exe moved to %s", backupPath)
	}

	// 3. Startup cleanup removes .old artifact.
	CleanupStaleArtifacts(currentExe)
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("expected backup %s removed by CleanupStaleArtifacts, got err=%v", backupPath, err)
	}
}

func TestHelperHoldExecutableLock(t *testing.T) {
	if os.Getenv("UNIDEVUI_HOLD_LOCK") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
}

func TestLiveGitHubCara1SelfUpdate(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live GitHub self-update test in -short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	// Simulates running app on v1.0.0 checking real public GitHub latest release (>= v1.0.1).
	info, err := CheckLatestReleaseWithURL(ctx, DefaultLatestReleaseURL, "1.0.0")
	if err != nil {
		t.Fatalf("CheckLatestReleaseWithURL live failed: %v", err)
	}
	if !info.HasUpdate || info.DownloadURL == "" {
		t.Fatalf("expected live GitHub update available for 1.0.0, got: %+v", info)
	}
	t.Logf("Detected live release: %s (asset=%s, size=%d, sha256=%s)", info.TagName, info.AssetName, info.DownloadSize, info.SHA256)

	selfBin, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable failed: %v", err)
	}
	dir := t.TempDir()
	targetExe := filepath.Join(dir, "UniDevUI.exe")

	src, err := os.Open(selfBin)
	if err != nil {
		t.Fatalf("open test binary: %v", err)
	}
	dst, err := os.Create(targetExe)
	if err != nil {
		src.Close()
		t.Fatalf("create targetExe: %v", err)
	}
	if _, err := io.Copy(dst, src); err != nil {
		src.Close()
		dst.Close()
		t.Fatalf("copy targetExe: %v", err)
	}
	src.Close()
	dst.Close()

	// Start child process from targetExe so Windows actively holds an OS file lock on it during swap.
	lockCmd := exec.Command(targetExe, "-test.run=^TestHelperHoldExecutableLock$")
	lockCmd.Env = append(os.Environ(), "UNIDEVUI_HOLD_LOCK=1")
	if err := lockCmd.Start(); err != nil {
		t.Fatalf("start lock holder: %v", err)
	}
	defer func() {
		if lockCmd.Process != nil {
			_ = lockCmd.Process.Kill()
			_, _ = lockCmd.Process.Wait()
		}
	}()
	time.Sleep(250 * time.Millisecond)

	var lastPct float64
	backupPath, err := DownloadVerifyAndSwap(ctx, info, targetExe, func(ev archive.ProgressEvent) {
		lastPct = ev.Percent
	}, nil)
	if err != nil {
		t.Fatalf("live DownloadVerifyAndSwap while running failed: %v", err)
	}
	if err := VerifyExecutableFile(targetExe, info.SHA256); err != nil {
		t.Fatalf("swapped executable verification failed: %v", err)
	}
	st, err := os.Stat(targetExe)
	if err != nil {
		t.Fatalf("stat swapped executable: %v", err)
	}
	t.Logf("Live swap succeeded (lastPct=%.1f%%, newSize=%d, backup=%s)", lastPct, st.Size(), backupPath)

	// Terminate old lock holder (simulates runtime.Quit on old process), then run startup cleanup.
	_ = lockCmd.Process.Kill()
	_, _ = lockCmd.Process.Wait()
	CleanupStaleArtifacts(targetExe)
	if _, err := os.Stat(backupPath); !os.IsNotExist(err) {
		t.Fatalf("expected backup %s deleted after lock release, got err=%v", backupPath, err)
	}
}

