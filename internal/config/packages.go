package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultBaseDir = `C:\AppPath`

// VersionEntry represents a single downloadable version of an application.
type VersionEntry struct {
	Version string `json:"version"`
	URL     string `json:"url"`
}

// AppPackage represents a package block inside packages.conf.
type AppPackage struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Category    string         `json:"category"` // "core" or "stack"
	BinSubdir   string         `json:"binSubdir"`
	Binary      string         `json:"binary"`
	LTSProvider string         `json:"ltsProvider"`
	Description string         `json:"description"`
	Versions    []VersionEntry `json:"versions"`
}

// ConfigPath returns the canonical packages.conf path inside baseDir.
func ConfigPath(baseDir string) string {
	return filepath.Join(baseDir, "packages.conf")
}

// EnsureConfig creates baseDir and a default packages.conf if missing.
func EnsureConfig(baseDir string) (string, error) {
	if err := os.MkdirAll(baseDir, 0o755); err != nil {
		return "", fmt.Errorf("gagal membuat folder %s: %w", baseDir, err)
	}
	confPath := ConfigPath(baseDir)
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		if err := os.WriteFile(confPath, []byte(DefaultPackagesConf()), 0o644); err != nil {
			return "", fmt.Errorf("gagal membuat %s: %w", confPath, err)
		}
	}
	return confPath, nil
}

// ParseFile reads packages.conf into structured AppPackage blocks and warning messages.
func ParseFile(confPath string) ([]AppPackage, []string, error) {
	f, err := os.Open(confPath)
	if err != nil {
		return nil, nil, fmt.Errorf("gagal membaca %s: %w", confPath, err)
	}
	defer f.Close()

	var packages []AppPackage
	var warnings []string
	var current *AppPackage

	flushCurrent := func() {
		if current != nil {
			if current.Name == "" {
				current.Name = current.ID
			}
			if current.Category == "" {
				current.Category = "core"
			}
			if current.LTSProvider == "" {
				current.LTSProvider = "manual"
			}
			packages = append(packages, *current)
			current = nil
		}
	}

	scanner := bufio.NewScanner(f)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") || line == "---" {
			continue
		}

		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			flushCurrent()
			id := strings.TrimSpace(line[1 : len(line)-1])
			if id == "" {
				warnings = append(warnings, fmt.Sprintf("Baris %d: section ID kosong diabaikan", lineNum))
				continue
			}
			current = &AppPackage{ID: id, Category: "core", LTSProvider: "manual"}
			continue
		}

		if current == nil {
			warnings = append(warnings, fmt.Sprintf("Baris %d: baris di luar blok [app_id] diabaikan", lineNum))
			continue
		}

		eqIdx := strings.Index(line, "=")
		if eqIdx <= 0 {
			warnings = append(warnings, fmt.Sprintf("Baris %d: format tidak valid (tidak ada '=')", lineNum))
			continue
		}

		key := strings.TrimSpace(line[:eqIdx])
		val := strings.TrimSpace(line[eqIdx+1:])
		lowerKey := strings.ToLower(key)

		switch {
		case lowerKey == "name":
			current.Name = val
		case lowerKey == "category":
			current.Category = strings.ToLower(val)
		case lowerKey == "bin_subdir":
			current.BinSubdir = val
		case lowerKey == "binary":
			current.Binary = val
		case lowerKey == "lts_provider":
			current.LTSProvider = strings.ToLower(val)
		case lowerKey == "description":
			current.Description = val
		case strings.HasPrefix(lowerKey, "version "):
			ver := strings.TrimSpace(key[len("version "):])
			if ver == "" || !isLikelyURL(val) {
				warnings = append(warnings, fmt.Sprintf("Baris %d: versi atau URL tidak valid", lineNum))
				continue
			}
			current.Versions = append(current.Versions, VersionEntry{Version: ver, URL: val})
		default:
			// Also accept Laragon-like "name-version = https://..." inside a block for convenience.
			if isLikelyURL(val) {
				ver := strings.TrimPrefix(key, "*")
				if dashIdx := strings.Index(ver, "-"); dashIdx >= 0 && dashIdx+1 < len(ver) {
					ver = ver[dashIdx+1:]
				}
				current.Versions = append(current.Versions, VersionEntry{Version: ver, URL: val})
			}
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, warnings, fmt.Errorf("error membaca %s: %w", confPath, err)
	}
	flushCurrent()
	return packages, warnings, nil
}

// UpdateVersionsInFile replaces "version ... = ..." lines inside matching [app_id] sections
// while preserving comments, custom sections, and formatting.
func UpdateVersionsInFile(confPath string, updatedByID map[string][]VersionEntry) error {
	raw, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}

	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	var out []string
	currentID := ""
	wroteUpdatedForSection := false

	emitVersions := func(appID string) {
		versions, ok := updatedByID[appID]
		if !ok {
			return
		}
		for _, v := range versions {
			out = append(out, fmt.Sprintf("version %s = %s", v.Version, v.URL))
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			if currentID != "" && !wroteUpdatedForSection {
				emitVersions(currentID)
			}
			currentID = strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			wroteUpdatedForSection = false
			out = append(out, line)
			continue
		}

		if currentID != "" {
			if _, hasUpdate := updatedByID[currentID]; hasUpdate && isVersionLine(trimmed) {
				if !wroteUpdatedForSection {
					emitVersions(currentID)
					wroteUpdatedForSection = true
				}
				continue
			}
		}

		out = append(out, line)
	}

	if currentID != "" && !wroteUpdatedForSection {
		emitVersions(currentID)
	}

	content := strings.Join(out, "\r\n")
	return os.WriteFile(confPath, []byte(content), 0o644)
}

// SyncMissingDefaults appends any built-in [app_id] blocks missing from confPath
// without altering existing sections, versions, or user comments.
func SyncMissingDefaults(confPath string) ([]string, error) {
	existingPkgs, _, err := ParseFile(confPath)
	if err != nil {
		return nil, err
	}
	existingIDs := make(map[string]bool, len(existingPkgs))
	for _, p := range existingPkgs {
		existingIDs[strings.ToLower(p.ID)] = true
	}

	rawBlocks := parseRawBlocks(DefaultPackagesConf())
	var added []string
	var appendText strings.Builder

	for _, blk := range rawBlocks {
		if existingIDs[strings.ToLower(blk.ID)] {
			continue
		}
		added = append(added, blk.ID)
		appendText.WriteString("\r\n\r\n")
		appendText.WriteString(strings.Join(blk.Lines, "\r\n"))
	}

	if len(added) == 0 {
		return nil, nil
	}

	existingBytes, err := os.ReadFile(confPath)
	if err != nil {
		return nil, err
	}
	combined := strings.TrimRight(string(existingBytes), "\r\n") + appendText.String() + "\r\n"
	if err := os.WriteFile(confPath, []byte(combined), 0o644); err != nil {
		return nil, err
	}
	return added, nil
}

type rawBlock struct {
	ID    string
	Lines []string
}

func parseRawBlocks(content string) []rawBlock {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var blocks []rawBlock
	var current *rawBlock

	flush := func() {
		if current != nil {
			for len(current.Lines) > 0 && strings.TrimSpace(current.Lines[len(current.Lines)-1]) == "" {
				current.Lines = current.Lines[:len(current.Lines)-1]
			}
			blocks = append(blocks, *current)
			current = nil
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.HasSuffix(trimmed, "]") {
			flush()
			id := strings.TrimSpace(trimmed[1 : len(trimmed)-1])
			current = &rawBlock{ID: id, Lines: []string{line}}
			continue
		}
		if current != nil {
			current.Lines = append(current.Lines, line)
		}
	}
	flush()
	return blocks
}

func isVersionLine(trimmed string) bool {
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || strings.HasPrefix(trimmed, ";") {
		return false
	}
	eqIdx := strings.Index(trimmed, "=")
	if eqIdx <= 0 {
		return false
	}
	key := strings.TrimSpace(trimmed[:eqIdx])
	val := strings.TrimSpace(trimmed[eqIdx+1:])
	return strings.HasPrefix(strings.ToLower(key), "version ") || isLikelyURL(val)
}

func isLikelyURL(s string) bool {
	ls := strings.ToLower(strings.TrimSpace(s))
	return strings.HasPrefix(ls, "https://") || strings.HasPrefix(ls, "http://")
}

// DefaultPackagesConf returns the initial Notepad-editable packages.conf template.
func DefaultPackagesConf() string {
	return `# ==============================================================================
# DevApps - Portable Packages Configuration
# ==============================================================================
# File ini tersimpan di folder tempat DevApps.exe berada.
# Edit file ini dengan Notepad, simpan, lalu klik tombol "Reload" di DevApps.
# Semua hasil ekstrak akan disatukan ke folder Output Path pilihan Anda (default: C:\AppPath\<app_id>\<version>\).
#
# Format setiap blok aplikasi:
#   [app_id]
#   name = Nama Tampilan
#   category = core | stack
#   bin_subdir = (kosong jika .exe di root, atau isi subfolder misal: bin / cmd)
#   binary = nama_executable.exe (untuk mendeteksi konflik PATH Windows)
#   lts_provider = nodejs | php | composer | go | git | pocketbase | sqlite | python | dotnet | rust-msvc | rust-gnu | winlibs | cmake | 7zip | manual
#   description = Keterangan singkat
#   version <versi> = <url_archive>
# ==============================================================================

[node]
name = Node.js (LTS)
category = core
bin_subdir =
binary = node.exe
lts_provider = nodejs
description = JavaScript runtime (otomatis filter hanya jalur LTS resmi)
version 26.13.1 = https://nodejs.org/dist/v26.13.1/node-v26.13.1-win-x64.zip
version 24.22.3 = https://nodejs.org/dist/v24.22.3/node-v24.22.3-win-x64.zip
version 22.23.5 = https://nodejs.org/dist/v22.23.5/node-v22.23.5-win-x64.zip

[php]
name = PHP (NTS x64)
category = core
bin_subdir =
binary = php.exe
lts_provider = php
description = PHP Windows Non-Thread-Safe (CLI & FastCGI)
version 8.5.13 = https://windows.php.net/downloads/releases/php-8.5.13-nts-Win32-vs17-x64.zip
version 8.4.28 = https://windows.php.net/downloads/releases/php-8.4.28-nts-Win32-vs17-x64.zip
version 8.3.37 = https://windows.php.net/downloads/releases/php-8.3.37-nts-Win32-vs16-x64.zip

[composer]
name = Composer
category = core
bin_subdir =
binary = composer.bat
lts_provider = composer
description = PHP dependency manager (auto-buat composer.bat wrapper)
version 2.11.3 = https://getcomposer.org/download/2.11.3/composer.phar
version 2.2.33 = https://getcomposer.org/download/2.2.33/composer.phar

[go]
name = Go (Golang)
category = core
bin_subdir = bin
binary = go.exe
lts_provider = go
description = Go compiler & toolchain (stabil aktif)
version 1.27.4 = https://go.dev/dl/go1.27.4.windows-amd64.zip
version 1.26.11 = https://go.dev/dl/go1.26.11.windows-amd64.zip

[git]
name = Git Portable (MinGit)
category = core
bin_subdir = cmd
binary = git.exe
lts_provider = git
description = Git for Windows portable archive
version 2.56.1 = https://github.com/git-for-windows/git/releases/download/v2.56.1.windows.1/MinGit-2.56.1-64-bit.zip

[pocketbase]
name = PocketBase
category = core
bin_subdir =
binary = pocketbase.exe
lts_provider = pocketbase
description = Open source Go backend in 1 file
version 0.40.6 = https://github.com/pocketbase/pocketbase/releases/download/v0.40.6/pocketbase_0.40.6_windows_amd64.zip

[sqlite]
name = SQLite Tools
category = core
bin_subdir =
binary = sqlite3.exe
lts_provider = sqlite
description = SQLite CLI shell (sqlite3.exe), sqldiff & analyzer portable
version 3.54.1 = https://sqlite.org/2026/sqlite-tools-win-x64-3540100.zip

[python]
name = Python (Standalone)
category = core
bin_subdir =
binary = python.exe
lts_provider = python
description = CPython portable build by Astral (includes pip & venv)
version 3.15.2 = https://github.com/astral-sh/python-build-standalone/releases/download/20261124/cpython-3.15.2%2B20261124-x86_64-pc-windows-msvc-install_only_stripped.tar.gz
version 3.14.10 = https://github.com/astral-sh/python-build-standalone/releases/download/20261124/cpython-3.14.10%2B20261124-x86_64-pc-windows-msvc-install_only_stripped.tar.gz
version 3.13.18 = https://github.com/astral-sh/python-build-standalone/releases/download/20261124/cpython-3.13.18%2B20261124-x86_64-pc-windows-msvc-install_only_stripped.tar.gz
version 3.12.16 = https://github.com/astral-sh/python-build-standalone/releases/download/20261124/cpython-3.12.16%2B20261124-x86_64-pc-windows-msvc-install_only_stripped.tar.gz

[dotnet]
name = .NET SDK (LTS)
category = stack
bin_subdir =
binary = dotnet.exe
lts_provider = dotnet
description = .NET SDK jalur LTS resmi (portal portable zip)
version 10.0.404 = https://builds.dotnet.microsoft.com/dotnet/Sdk/10.0.404/dotnet-sdk-10.0.404-win-x64.zip

[rust]
name = Rust Toolchain (MSVC)
category = stack
bin_subdir = bin
binary = rustc.exe
lts_provider = rust-msvc
description = rustc + cargo standalone archive (ganti lts_provider = rust-gnu jika tanpa VS Build Tools)
version 1.100.0 = https://static.rust-lang.org/dist/2026-11-12/rust-1.100.0-x86_64-pc-windows-msvc.tar.gz

[gcc]
name = WinLibs GCC (MinGW-w64)
category = stack
bin_subdir = bin
binary = gcc.exe
lts_provider = winlibs
description = Portable GCC, G++, LLVM & MinGW-w64 UCRT toolchain
version 16.2.0 = https://github.com/brechtsanders/winlibs_mingw/releases/download/16.2.0posix-23.1.3-14.0.0-ucrt-r3/winlibs-x86_64-posix-seh-gcc-16.2.0-llvm-23.1.3-mingw-w64ucrt-14.0.0-r3.zip

[cmake]
name = CMake
category = stack
bin_subdir = bin
binary = cmake.exe
lts_provider = cmake
description = Cross-platform C/C++ build system generator
version 4.5.1 = https://github.com/Kitware/CMake/releases/download/v4.5.1/cmake-4.5.1-windows-x86_64.zip

[7zip]
name = 7-Zip CLI
category = stack
bin_subdir =
binary = 7zr.exe
lts_provider = 7zip
description = Standalone 7zr.exe CLI archiver (disertai wrapper 7z.bat)
version 26.06 = https://github.com/ip7z/7zip/releases/download/26.06/7zr.exe
`
}
