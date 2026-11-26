package archive

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestInstallPackageZipAutoFlattensSingleRootDir(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "node-test.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("node-v22.23.5-win-x64/node.exe")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("fake-node-binary"))
	_ = zw.Close()
	_ = f.Close()

	baseDir := filepath.Join(tmp, "AppPath")
	installDir, err := InstallPackage(context.Background(), zipPath, baseDir, "node", "22.23.5", nil, func(l, en, id string) {})
	if err != nil {
		t.Fatalf("InstallPackage failed: %v", err)
	}

	expectedBin := filepath.Join(installDir, "node.exe")
	if !fileExists(expectedBin) {
		t.Fatalf("expected flattened node.exe at %s", expectedBin)
	}
}

func TestExtractZipRejectsPathTraversal(t *testing.T) {
	tmp := t.TempDir()
	zipPath := filepath.Join(tmp, "evil.zip")

	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	w, err := zw.Create("../outside.txt")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = w.Write([]byte("bad"))
	_ = zw.Close()
	_ = f.Close()

	if err := extractZip(context.Background(), zipPath, filepath.Join(tmp, "dest")); err == nil {
		t.Fatal("expected zip-slip error for ../outside.txt")
	}
}

func TestMergeRustComponentsLayout(t *testing.T) {
	tmp := t.TempDir()
	src := filepath.Join(tmp, "rust-src")
	dst := filepath.Join(tmp, "rust-dst")

	mustWrite := func(rel, content string) {
		full := filepath.Join(src, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("components", "rustc\ncargo\nrust-std-x86_64-pc-windows-msvc\n")
	mustWrite("rustc/bin/rustc.exe", "rustc")
	mustWrite("cargo/bin/cargo.exe", "cargo")
	mustWrite("rust-std-x86_64-pc-windows-msvc/lib/rustlib/std.rlib", "rlib")

	if !isRustStandaloneLayout(src) {
		t.Fatal("expected isRustStandaloneLayout to return true")
	}
	if err := mergeRustComponents(src, dst); err != nil {
		t.Fatalf("mergeRustComponents failed: %v", err)
	}

	for _, rel := range []string{"bin/rustc.exe", "bin/cargo.exe", "lib/rustlib/std.rlib"} {
		if !fileExists(filepath.Join(dst, filepath.FromSlash(rel))) {
			t.Errorf("missing merged file %s", rel)
		}
	}
}

func TestInstallPharCreatesComposerBatchWrapper(t *testing.T) {
	tmp := t.TempDir()
	phar := filepath.Join(tmp, "composer.phar")
	if err := os.WriteFile(phar, []byte("phar"), 0o644); err != nil {
		t.Fatal(err)
	}

	baseDir := filepath.Join(tmp, "AppPath")
	installDir, err := InstallPackage(context.Background(), phar, baseDir, "composer", "2.11.3", nil, func(l, en, id string) {})
	if err != nil {
		t.Fatalf("InstallPackage phar failed: %v", err)
	}

	if !fileExists(filepath.Join(installDir, "composer.bat")) {
		t.Fatal("expected composer.bat wrapper to be generated")
	}
}

func TestInstallPythonTarGzCreatesPipWrapper(t *testing.T) {
	tmp := t.TempDir()
	tgzPath := filepath.Join(tmp, "python-standalone.tar.gz")

	f, err := os.Create(tgzPath)
	if err != nil {
		t.Fatal(err)
	}
	gw := gzip.NewWriter(f)
	tw := tar.NewWriter(gw)

	for _, name := range []string{"python/python.exe", "python/Scripts/pip.exe"} {
		data := []byte("stub")
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o755,
			Size: int64(len(data)),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatal(err)
		}
	}
	_ = tw.Close()
	_ = gw.Close()
	_ = f.Close()

	baseDir := filepath.Join(tmp, "AppPath")
	installDir, err := InstallPackage(context.Background(), tgzPath, baseDir, "python", "3.14.10", nil, func(l, en, id string) {})
	if err != nil {
		t.Fatalf("InstallPackage python tar.gz failed: %v", err)
	}
	if !fileExists(filepath.Join(installDir, "python.exe")) {
		t.Fatal("expected flattened python.exe at install root")
	}
	if !fileExists(filepath.Join(installDir, "pip.bat")) {
		t.Fatal("expected pip.bat wrapper at install root")
	}
}

func TestDownloadFileResolvesRedirectFilename(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/fwlink/p/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/cdn/MicrosoftEdgeWebview2Setup.exe", http.StatusFound)
	})
	mux.HandleFunc("/cdn/MicrosoftEdgeWebview2Setup.exe", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("MZ-stub-installer"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	cacheDir := filepath.Join(t.TempDir(), "_cache")
	got, err := DownloadFile(context.Background(), srv.URL+"/fwlink/p/?LinkId=2124703", cacheDir, nil, nil)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}
	if filepath.Base(got) != "MicrosoftEdgeWebview2Setup.exe" {
		t.Fatalf("expected resolved filename MicrosoftEdgeWebview2Setup.exe, got %q", filepath.Base(got))
	}
	if !fileExists(got) {
		t.Fatalf("expected downloaded file at %s", got)
	}
}
