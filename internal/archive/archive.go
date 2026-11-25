package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/ulikunitz/xz"
)

const sevenZipStandaloneURL = "https://github.com/ip7z/7zip/releases/download/26.06/7zr.exe"

// ProgressEvent reports real-time download or extraction status.
type ProgressEvent struct {
	Stage      string  `json:"stage"`      // "download" | "extract"
	Percent    float64 `json:"percent"`    // 0..100, -1 if indeterminate
	Downloaded int64   `json:"downloaded"` // bytes downloaded
	Total      int64   `json:"total"`      // total bytes (-1 if unknown)
	SpeedBps   float64 `json:"speedBps"`   // bytes/sec
	Detail     string  `json:"detail"`     // human-readable detail
}

// Logger receives live bilingual status log lines for the terminal drawer.
type Logger func(level, enMsg, idMsg string)

// ProgressCallback receives download/extract progress updates.
type ProgressCallback func(ProgressEvent)

// DownloadFile downloads url into cacheDir and returns the local cached file path.
// For PHP release URLs, it automatically falls back to /downloads/releases/archives/ on 404.
func DownloadFile(ctx context.Context, rawURL, cacheDir string, onProgress ProgressCallback, log Logger) (string, error) {
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat folder cache: %w", err)
	}

	fileName := fileNameFromURL(rawURL)
	destPath := filepath.Join(cacheDir, fileName)
	partPath := destPath + ".part"

	tryURLs := []string{rawURL}
	if strings.Contains(rawURL, "windows.php.net/downloads/releases/") &&
		!strings.Contains(rawURL, "windows.php.net/downloads/releases/archives/") {
		fallback := strings.Replace(rawURL, "windows.php.net/downloads/releases/", "windows.php.net/downloads/releases/archives/", 1)
		tryURLs = append(tryURLs, fallback)
	}

	var lastErr error
	for i, targetURL := range tryURLs {
		if i > 0 && log != nil {
			log("WARN",
				fmt.Sprintf("Primary URL not found, trying archive fallback: %s", targetURL),
				fmt.Sprintf("URL utama tidak ditemukan, mencoba fallback archive: %s", targetURL),
			)
		}
		err := downloadSingle(ctx, targetURL, partPath, onProgress)
		if err == nil {
			_ = os.Remove(destPath)
			if err := os.Rename(partPath, destPath); err != nil {
				return "", fmt.Errorf("gagal memindahkan file download: %w", err)
			}
			return destPath, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			_ = os.Remove(partPath)
			return "", ctx.Err()
		}
	}

	_ = os.Remove(partPath)
	return "", lastErr
}

func downloadSingle(ctx context.Context, targetURL, partPath string, onProgress ProgressCallback) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, targetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "UniDevUI/1.0 (Windows Portable Package Manager)")

	client := &http.Client{
		Timeout: 0,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download gagal: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("server merespons HTTP %d (%s)", resp.StatusCode, resp.Status)
	}

	out, err := os.Create(partPath)
	if err != nil {
		return fmt.Errorf("gagal membuat file sementara: %w", err)
	}
	defer out.Close()

	total := resp.ContentLength
	var downloaded int64
	buf := make([]byte, 64*1024)
	started := time.Now()
	lastEmit := time.Now()

	emit := func(force bool) {
		if onProgress == nil {
			return
		}
		now := time.Now()
		if !force && now.Sub(lastEmit) < 150*time.Millisecond {
			return
		}
		lastEmit = now
		elapsed := now.Sub(started).Seconds()
		speed := 0.0
		if elapsed > 0 {
			speed = float64(downloaded) / elapsed
		}
		pct := -1.0
		if total > 0 {
			pct = (float64(downloaded) / float64(total)) * 100
			if pct > 100 {
				pct = 100
			}
		}
		onProgress(ProgressEvent{
			Stage:      "download",
			Percent:    pct,
			Downloaded: downloaded,
			Total:      total,
			SpeedBps:   speed,
		})
	}

	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			if _, wErr := out.Write(buf[:n]); wErr != nil {
				return fmt.Errorf("gagal menulis file download: %w", wErr)
			}
			downloaded += int64(n)
			emit(false)
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return fmt.Errorf("koneksi terputus saat download: %w", readErr)
		}
	}

	emit(true)
	return nil
}

// InstallPackage extracts or prepares archivePath into baseDir\appID\version.
func InstallPackage(ctx context.Context, archivePath, baseDir, appID, version string, onProgress ProgressCallback, log Logger) (string, error) {
	targetDir := filepath.Join(baseDir, appID, version)
	stagingRoot := filepath.Join(baseDir, "_staging")
	if err := os.MkdirAll(stagingRoot, 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat folder staging: %w", err)
	}

	stageDir := filepath.Join(stagingRoot, fmt.Sprintf("%s-%s-%s", appID, version, randHex(4)))
	if err := os.MkdirAll(stageDir, 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat staging dir: %w", err)
	}
	defer os.RemoveAll(stageDir)

	if onProgress != nil {
		onProgress(ProgressEvent{Stage: "extract", Percent: 10, Detail: "Mengekstrak arsip..."})
	}

	lowerName := strings.ToLower(archivePath)
	switch {
	case strings.HasSuffix(lowerName, ".zip"):
		if err := extractZip(ctx, archivePath, stageDir); err != nil {
			return "", err
		}
	case strings.HasSuffix(lowerName, ".tar.gz") || strings.HasSuffix(lowerName, ".tgz"):
		if err := extractTarGz(ctx, archivePath, stageDir); err != nil {
			return "", err
		}
	case strings.HasSuffix(lowerName, ".tar.xz") || strings.HasSuffix(lowerName, ".txz"):
		if err := extractTarXz(ctx, archivePath, stageDir); err != nil {
			return "", err
		}
	case strings.HasSuffix(lowerName, ".tar"):
		if err := extractPlainTar(ctx, archivePath, stageDir); err != nil {
			return "", err
		}
	case strings.HasSuffix(lowerName, ".7z"):
		if err := extractSevenZip(ctx, archivePath, baseDir, stageDir, log); err != nil {
			return "", err
		}
	case strings.HasSuffix(lowerName, ".phar"):
		if err := installPhar(archivePath, stageDir, appID); err != nil {
			return "", err
		}
	case strings.HasSuffix(lowerName, ".exe"):
		if err := installStandaloneExe(archivePath, stageDir, appID); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("format arsip tidak didukung: %s", filepath.Base(archivePath))
	}

	if onProgress != nil {
		onProgress(ProgressEvent{Stage: "extract", Percent: 75, Detail: "Merapikan struktur folder..."})
	}

	preparedDir := stageDir
	if unwrapped, ok := singleSubdir(stageDir); ok {
		preparedDir = unwrapped
	}

	// Rust standalone tarballs split binaries into component subdirectories (rustc/, cargo/, rust-std-*/).
	if isRustStandaloneLayout(preparedDir) {
		mergedDir := filepath.Join(stagingRoot, fmt.Sprintf("%s-merged-%s", appID, randHex(4)))
		defer os.RemoveAll(mergedDir)
		if log != nil {
			log("INFO",
				"Detected Rust standalone component layout, merging into bin/lib...",
				"Mendeteksi struktur komponen Rust standalone, menggabungkan ke folder bin/lib...",
			)
		}
		if err := mergeRustComponents(preparedDir, mergedDir); err != nil {
			return "", fmt.Errorf("gagal menggabungkan komponen Rust: %w", err)
		}
		preparedDir = mergedDir
	}

	if err := os.MkdirAll(filepath.Dir(targetDir), 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat direktori aplikasi: %w", err)
	}
	if err := os.RemoveAll(targetDir); err != nil {
		return "", fmt.Errorf("gagal membersihkan instalasi lama di %s: %w", targetDir, err)
	}

	if err := moveDirContents(preparedDir, targetDir); err != nil {
		return "", fmt.Errorf("gagal memindahkan hasil ekstrak ke %s: %w", targetDir, err)
	}

	// Ensure `pip` works directly from root alongside `python.exe` via a lightweight wrapper.
	if strings.EqualFold(appID, "python") && fileExists(filepath.Join(targetDir, "python.exe")) {
		pipBat := filepath.Join(targetDir, "pip.bat")
		_ = os.WriteFile(pipBat, []byte("@echo off\r\n\"%~dp0python.exe\" -m pip %*\r\n"), 0o644)
	}

	if onProgress != nil {
		onProgress(ProgressEvent{Stage: "extract", Percent: 100, Detail: "Selesai"})
	}
	return targetDir, nil
}

// ResolveBinDir locates the effective executable folder for an installed version.
func ResolveBinDir(installDir, configuredSubdir, binaryName string) string {
	if configuredSubdir != "" {
		candidate := filepath.Join(installDir, filepath.FromSlash(configuredSubdir))
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			if binaryName == "" || fileExists(filepath.Join(candidate, binaryName)) {
				return candidate
			}
		}
	}

	if binaryName != "" {
		probes := []string{
			installDir,
			filepath.Join(installDir, "bin"),
			filepath.Join(installDir, "cmd"),
			filepath.Join(installDir, "x64"),
			filepath.Join(installDir, "mingw64", "bin"),
			filepath.Join(installDir, "usr", "bin"),
		}
		for _, dir := range probes {
			if fileExists(filepath.Join(dir, binaryName)) {
				return dir
			}
		}
	}

	if configuredSubdir != "" {
		return filepath.Join(installDir, filepath.FromSlash(configuredSubdir))
	}
	return installDir
}

func extractZip(ctx context.Context, archivePath, destDir string) error {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("gagal membuka arsip zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		target, err := safeJoin(destDir, f.Name)
		if err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		err = writeFileStream(target, rc, f.Mode())
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(ctx context.Context, archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gz, err := gzip.NewReader(f)
	if err != nil {
		return fmt.Errorf("gagal membuka gzip stream: %w", err)
	}
	defer gz.Close()

	return extractTarReader(ctx, tar.NewReader(gz), destDir)
}

func extractTarXz(ctx context.Context, archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	xr, err := xz.NewReader(f)
	if err != nil {
		return fmt.Errorf("gagal membuka xz stream: %w", err)
	}

	return extractTarReader(ctx, tar.NewReader(xr), destDir)
}

func extractPlainTar(ctx context.Context, archivePath, destDir string) error {
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	return extractTarReader(ctx, tar.NewReader(f), destDir)
}

func extractTarReader(ctx context.Context, tr *tar.Reader, destDir string) error {
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("gagal membaca tar entry: %w", err)
		}

		target, err := safeJoin(destDir, hdr.Name)
		if err != nil {
			return err
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			if err := writeFileStream(target, tr, os.FileMode(hdr.Mode)); err != nil {
				return err
			}
		case tar.TypeSymlink:
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			_ = os.Symlink(hdr.Linkname, target)
		}
	}
	return nil
}

func extractSevenZip(ctx context.Context, archivePath, baseDir, destDir string, log Logger) error {
	sevenExe := findSevenZipBinary(baseDir)
	if sevenExe == "" {
		if log != nil {
			log("INFO",
				"7zr.exe helper not found for .7z extraction, downloading standalone 7zr.exe...",
				"Mencari helper 7zr.exe untuk ekstrak .7z, mengunduh standalone 7zr.exe...",
			)
		}
		cacheDir := filepath.Join(baseDir, "_cache")
		downloaded, err := DownloadFile(ctx, sevenZipStandaloneURL, cacheDir, nil, log)
		if err != nil {
			return fmt.Errorf("ekstrak .7z membutuhkan 7z/7zr.exe dan gagal mengunduh helper: %w", err)
		}
		sevenExe = downloaded
	}

	cmd := exec.CommandContext(ctx, sevenExe, "x", archivePath, "-o"+destDir, "-y")
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("gagal mengekstrak .7z (%s): %w\n%s", filepath.Base(sevenExe), err, string(out))
	}
	return nil
}

func findSevenZipBinary(baseDir string) string {
	candidates := []string{
		filepath.Join(baseDir, "_cache", "7zr.exe"),
		`C:\Program Files\7-Zip\7z.exe`,
	}
	if matches, err := filepath.Glob(filepath.Join(baseDir, "7zip", "*", "7zr.exe")); err == nil {
		candidates = append(matches, candidates...)
	}
	for _, c := range candidates {
		if fileExists(c) {
			return c
		}
	}
	for _, name := range []string{"7z.exe", "7za.exe", "7zr.exe"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	return ""
}

func installPhar(archivePath, stageDir, appID string) error {
	baseName := filepath.Base(archivePath)
	destPhar := filepath.Join(stageDir, baseName)
	if err := copyFile(archivePath, destPhar); err != nil {
		return err
	}
	if strings.EqualFold(appID, "composer") || strings.Contains(strings.ToLower(baseName), "composer") {
		batPath := filepath.Join(stageDir, "composer.bat")
		batContent := fmt.Sprintf("@echo off\r\nphp \"%%~dp0%s\" %%*\r\n", baseName)
		if err := os.WriteFile(batPath, []byte(batContent), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func installStandaloneExe(archivePath, stageDir, appID string) error {
	baseName := filepath.Base(archivePath)
	destExe := filepath.Join(stageDir, baseName)
	if err := copyFile(archivePath, destExe); err != nil {
		return err
	}
	if strings.EqualFold(appID, "7zip") && !strings.EqualFold(baseName, "7z.exe") {
		batPath := filepath.Join(stageDir, "7z.bat")
		batContent := fmt.Sprintf("@echo off\r\n\"%%~dp0%s\" %%*\r\n", baseName)
		if err := os.WriteFile(batPath, []byte(batContent), 0o644); err != nil {
			return err
		}
	}
	return nil
}

func isRustStandaloneLayout(dir string) bool {
	return fileExists(filepath.Join(dir, "components")) &&
		fileExists(filepath.Join(dir, "rustc", "bin", "rustc.exe"))
}

func mergeRustComponents(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}

	skipFiles := map[string]bool{
		"manifest.in":            true,
		"rust-installer-version": true,
		"components":             true,
		"install.sh":             true,
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		compDir := filepath.Join(srcDir, entry.Name())
		if strings.HasPrefix(strings.ToLower(entry.Name()), "rust-docs") {
			continue
		}
		err := filepath.WalkDir(compDir, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			rel, err := filepath.Rel(compDir, path)
			if err != nil || rel == "." {
				return err
			}
			if !d.IsDir() && skipFiles[strings.ToLower(d.Name())] && filepath.Dir(rel) == "." {
				return nil
			}
			target := filepath.Join(dstDir, rel)
			if d.IsDir() {
				return os.MkdirAll(target, 0o755)
			}
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			return moveOrCopyFile(path, target)
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func moveDirContents(srcDir, dstDir string) error {
	if err := os.Rename(srcDir, dstDir); err == nil {
		return nil
	}
	return filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return moveOrCopyFile(path, target)
	})
}

func singleSubdir(dir string) (string, bool) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || !entries[0].IsDir() {
		return "", false
	}
	return filepath.Join(dir, entries[0].Name()), true
}

func safeJoin(baseDir, entryPath string) (string, error) {
	cleanRel := filepath.Clean(filepath.FromSlash(entryPath))
	if filepath.IsAbs(cleanRel) || strings.HasPrefix(cleanRel, ".."+string(filepath.Separator)) || cleanRel == ".." {
		return "", fmt.Errorf("path berbahaya terdeteksi dalam arsip: %s", entryPath)
	}
	full := filepath.Join(baseDir, cleanRel)
	baseClean := filepath.Clean(baseDir)
	if full != baseClean && !strings.HasPrefix(strings.ToLower(full), strings.ToLower(baseClean)+string(filepath.Separator)) {
		return "", fmt.Errorf("path traversal terdeteksi: %s", entryPath)
	}
	return full, nil
}

func writeFileStream(target string, r io.Reader, mode os.FileMode) error {
	// Ensure owner write permission on Windows so read-only archive entries can still be overwritten or deleted.
	mode |= 0o600
	out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, r)
	return err
}

func moveOrCopyFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	return copyFile(src, dst)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func fileNameFromURL(rawURL string) string {
	trimmed := strings.Split(rawURL, "?")[0]
	base := filepath.Base(trimmed)
	if base == "" || base == "." || base == "/" {
		return "package.archive"
	}
	return base
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func randHex(bytesLen int) string {
	b := make([]byte, bytesLen)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
