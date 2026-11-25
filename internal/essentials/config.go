package essentials

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// EssentialApp represents a Windows clean-install application entry in essentials.conf.
type EssentialApp struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"` // browsers | utilities | editors | office | chat | downloaders
	WingetID    string   `json:"wingetId"`
	Source      string   `json:"source"` // winget | msstore
	RequiresUAC bool     `json:"requiresUac"`
	Versions     []string `json:"versions,omitempty"`
	Description  string   `json:"description"`
	ExpectedSize int64    `json:"-"`
}

// ConfigPath returns essentials.conf inside appDir (alongside DevApps.exe).
func ConfigPath(appDir string) string {
	return filepath.Join(appDir, "essentials.conf")
}

// EnsureConfig creates essentials.conf with the curated 27-app list if missing.
func EnsureConfig(appDir string) (string, error) {
	if err := os.MkdirAll(appDir, 0o755); err != nil {
		return "", fmt.Errorf("failed creating app directory %s: %w", appDir, err)
	}
	confPath := ConfigPath(appDir)
	if _, err := os.Stat(confPath); os.IsNotExist(err) {
		if err := os.WriteFile(confPath, []byte(DefaultEssentialsConf()), 0o644); err != nil {
			return "", fmt.Errorf("failed creating %s: %w", confPath, err)
		}
	}
	return confPath, nil
}

// ParseFile reads essentials.conf into EssentialApp entries and syntax warnings.
func ParseFile(confPath string) ([]EssentialApp, []string, error) {
	f, err := os.Open(confPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed reading %s: %w", confPath, err)
	}
	defer f.Close()

	var apps []EssentialApp
	var warnings []string
	var current *EssentialApp

	flush := func() {
		if current != nil {
			if current.Name == "" {
				current.Name = current.ID
			}
			if current.Category == "" {
				current.Category = "utilities"
			}
			if current.Source == "" {
				current.Source = "winget"
			}
			if current.WingetID != "" {
				apps = append(apps, *current)
			} else {
				warnings = append(warnings, fmt.Sprintf("essentials [%s]: missing winget_id, skipped", current.ID))
			}
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
			flush()
			id := strings.TrimSpace(line[1 : len(line)-1])
			if id == "" {
				warnings = append(warnings, fmt.Sprintf("essentials line %d: empty section ID", lineNum))
				continue
			}
			current = &EssentialApp{ID: id, Category: "utilities", Source: "winget"}
			continue
		}
		if current == nil {
			continue
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			warnings = append(warnings, fmt.Sprintf("essentials line %d: invalid key=value format", lineNum))
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		val := strings.TrimSpace(line[eq+1:])

		switch key {
		case "name":
			current.Name = val
		case "category":
			current.Category = strings.ToLower(val)
		case "winget_id":
			current.WingetID = val
		case "source":
			current.Source = strings.ToLower(val)
		case "requires_uac":
			current.RequiresUAC = strings.EqualFold(val, "true") || val == "1" || strings.EqualFold(val, "yes")
		case "versions":
			for _, part := range strings.Split(val, ",") {
				if v := strings.TrimSpace(part); v != "" {
					current.Versions = append(current.Versions, v)
				}
			}
		case "description":
			current.Description = val
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, warnings, err
	}
	flush()
	return apps, warnings, nil
}

// SyncMissingDefaults appends any built-in essentials missing from confPath without overwriting edits.
func SyncMissingDefaults(confPath string) ([]string, error) {
	existing, _, err := ParseFile(confPath)
	if err != nil {
		return nil, err
	}
	seenID := make(map[string]bool, len(existing))
	seenWinget := make(map[string]bool, len(existing))
	for _, a := range existing {
		seenID[strings.ToLower(a.ID)] = true
		seenWinget[strings.ToLower(a.WingetID)] = true
	}

	defaults, _, err := ParseString(DefaultEssentialsConf())
	if err != nil {
		return nil, err
	}

	var added []string
	var buf strings.Builder
	for _, d := range defaults {
		if seenID[strings.ToLower(d.ID)] || seenWinget[strings.ToLower(d.WingetID)] {
			continue
		}
		added = append(added, d.ID)
		buf.WriteString("\r\n\r\n")
		buf.WriteString(formatEssentialBlock(d))
	}
	if len(added) == 0 {
		return nil, nil
	}

	raw, err := os.ReadFile(confPath)
	if err != nil {
		return nil, err
	}
	combined := strings.TrimRight(string(raw), "\r\n") + buf.String() + "\r\n"
	if err := os.WriteFile(confPath, []byte(combined), 0o644); err != nil {
		return nil, err
	}
	return added, nil
}

// AppendCustomApp appends a user-selected Winget search result into essentials.conf if not already present.
func AppendCustomApp(confPath string, app EssentialApp) error {
	existing, _, err := ParseFile(confPath)
	if err != nil {
		return err
	}
	for _, item := range existing {
		if strings.EqualFold(item.WingetID, app.WingetID) {
			return fmt.Errorf("%s (%s) is already in essentials.conf", item.Name, item.WingetID)
		}
	}

	if app.ID == "" {
		app.ID = sanitizeSectionID(app.WingetID)
	}
	// Ensure unique section ID.
	baseID := app.ID
	suffix := 2
	for sectionExists(existing, app.ID) {
		app.ID = fmt.Sprintf("%s-%d", baseID, suffix)
		suffix++
	}
	if app.Category == "" {
		app.Category = "utilities"
	}
	if app.Source == "" {
		app.Source = "winget"
	}

	raw, err := os.ReadFile(confPath)
	if err != nil {
		return err
	}
	combined := strings.TrimRight(string(raw), "\r\n") + "\r\n\r\n" + formatEssentialBlock(app) + "\r\n"
	return os.WriteFile(confPath, []byte(combined), 0o644)
}

// ParseString parses essentials.conf content directly from a string.
func ParseString(content string) ([]EssentialApp, []string, error) {
	tmp, err := os.CreateTemp("", "essentials-*.conf")
	if err != nil {
		return nil, nil, err
	}
	tmpPath := tmp.Name()
	_, _ = tmp.WriteString(content)
	_ = tmp.Close()
	defer os.Remove(tmpPath)
	return ParseFile(tmpPath)
}

func formatEssentialBlock(a EssentialApp) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("[%s]", a.ID))
	lines = append(lines, fmt.Sprintf("name = %s", a.Name))
	lines = append(lines, fmt.Sprintf("category = %s", a.Category))
	lines = append(lines, fmt.Sprintf("winget_id = %s", a.WingetID))
	lines = append(lines, fmt.Sprintf("source = %s", a.Source))
	if a.RequiresUAC {
		lines = append(lines, "requires_uac = true")
	} else {
		lines = append(lines, "requires_uac = false")
	}
	if len(a.Versions) > 0 {
		lines = append(lines, fmt.Sprintf("versions = %s", strings.Join(a.Versions, ", ")))
	}
	if a.Description != "" {
		lines = append(lines, fmt.Sprintf("description = %s", a.Description))
	}
	return strings.Join(lines, "\r\n")
}

func sectionExists(list []EssentialApp, id string) bool {
	for _, item := range list {
		if strings.EqualFold(item.ID, id) {
			return true
		}
	}
	return false
}

var reNonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func sanitizeSectionID(wingetID string) string {
	lower := strings.ToLower(wingetID)
	cleaned := strings.Trim(reNonAlnum.ReplaceAllString(lower, "-"), "-")
	if cleaned == "" {
		return "custom-app"
	}
	return cleaned
}

// DefaultEssentialsConf returns the curated 27-app Windows clean-install list.
func DefaultEssentialsConf() string {
	return `# ==============================================================================
# DevApps - Windows Clean Install Essentials (Winget)
# ==============================================================================
# File ini tersimpan di folder tempat DevApps.exe berada.
# Edit dengan Notepad, simpan, lalu klik "Reload" di DevApps.
#
# Category yang tersedia: browsers | utilities | editors | office | chat | downloaders
# ==============================================================================

# --- Browsers ---

[firefox]
name = Mozilla Firefox
category = browsers
winget_id = Mozilla.Firefox
source = winget
requires_uac = false
description = Open-source privacy-focused web browser

[chrome]
name = Google Chrome
category = browsers
winget_id = Google.Chrome
source = winget
requires_uac = true
description = Fast web browser by Google

[brave]
name = Brave Browser
category = browsers
winget_id = Brave.Brave
source = winget
requires_uac = false
description = Chromium browser with built-in ad & tracker blocker

# --- Archives & Utilities ---

[nanazip]
name = NanaZip
category = utilities
winget_id = M2Team.NanaZip
source = winget
requires_uac = false
description = Modern 7-Zip fork with native Windows 11 context menu

[winrar]
name = WinRAR
category = utilities
winget_id = RARLab.WinRAR
source = winget
requires_uac = true
description = Classic RAR & ZIP archive utility

[revo]
name = Revo Uninstaller
category = utilities
winget_id = RevoUninstaller.RevoUninstaller
source = winget
requires_uac = true
description = Deep Windows program uninstaller & leftover cleaner

[lightshot]
name = Lightshot
category = utilities
winget_id = Skillbrains.Lightshot
source = winget
requires_uac = true
description = Lightweight quick screenshot capture & annotation tool

[stirlingpdf]
name = Stirling-PDF
category = utilities
winget_id = StirlingTools.StirlingPDF
source = winget
requires_uac = false
description = Local self-hosted PDF merge, split, convert & edit suite

# --- Editors, Shells & Dev Environments ---

[powershell7]
name = PowerShell 7
category = editors
winget_id = Microsoft.PowerShell
source = winget
requires_uac = true
description = Cross-platform modern PowerShell 7 (pwsh)

[windowsterminal]
name = Windows Terminal
category = editors
winget_id = Microsoft.WindowsTerminal
source = winget
requires_uac = false
description = Modern tabbed terminal for CMD, PowerShell & WSL

[notepadpp]
name = Notepad++
category = editors
winget_id = Notepad++.Notepad++
source = winget
requires_uac = true
description = Fast lightweight source code & plain-text editor

[emeditor]
name = EmEditor
category = editors
winget_id = Emurasoft.EmEditor
source = winget
requires_uac = true
description = High-speed text & large CSV/log file editor

[sublimetext]
name = Sublime Text 4
category = editors
winget_id = SublimeHQ.SublimeText.4
source = winget
requires_uac = true
description = Ultra-fast code editor

[vscode]
name = Visual Studio Code
category = editors
winget_id = Microsoft.VisualStudioCode
source = winget
requires_uac = false
description = Extensible IDE & code editor by Microsoft

[antigravity]
name = Antigravity
category = editors
winget_id = Google.Antigravity
source = winget
requires_uac = false
description = Agentic AI IDE by Google

[laragon]
name = Laragon
category = editors
winget_id = LeNgocKhoa.Laragon
source = winget
requires_uac = true
versions = latest, 6.0.0
description = Portable universal dev environment (v6.0.0 has no license popup)

# --- Media, Office & System Runtimes ---

[vlc]
name = VLC Media Player
category = office
winget_id = VideoLAN.VLC
source = winget
requires_uac = true
description = Universal open-source audio & video player

[vcredist]
name = Visual C++ Redistributable 2015-2022 (x64)
category = office
winget_id = Microsoft.VCRedist.2015+.x64
source = winget
requires_uac = true
description = Required C++ runtime DLLs for PHP, Apache & desktop apps

[libreoffice]
name = LibreOffice
category = office
winget_id = TheDocumentFoundation.LibreOffice
source = winget
requires_uac = true
description = Free open-source Office suite (Writer, Calc, Impress)

# --- Chat, Mail & Cloud ---

[whatsapp]
name = WhatsApp
category = chat
winget_id = 9NKSQGP7F2NH
source = msstore
requires_uac = false
description = Official WhatsApp Desktop (via Microsoft Store source)

[telegram]
name = Telegram Desktop
category = chat
winget_id = Telegram.TelegramDesktop
source = winget
requires_uac = false
description = Fast desktop messaging client

[discord]
name = Discord
category = chat
winget_id = Discord.Discord
source = winget
requires_uac = false
description = Voice, video & community chat client

[thunderbird]
name = Mozilla Thunderbird
category = chat
winget_id = Mozilla.Thunderbird
source = winget
requires_uac = true
description = Open-source desktop email, calendar & RSS client

[googledrive]
name = Google Drive
category = chat
winget_id = Google.GoogleDrive
source = winget
requires_uac = true
description = Google Drive desktop sync & virtual drive client

# --- Download Managers ---

[idm]
name = Internet Download Manager (IDM)
category = downloaders
winget_id = Tonec.InternetDownloadManager
source = winget
requires_uac = true
description = Accelerated download manager with browser integration

[xdm]
name = Xtreme Download Manager (XDM)
category = downloaders
winget_id = subhra74.XtremeDownloadManager
source = winget
requires_uac = false
description = Open-source fast download accelerator & video grabber

[fdm]
name = Free Download Manager (FDM)
category = downloaders
winget_id = SoftDeluxe.FreeDownloadManager
source = winget
requires_uac = true
description = Free modern download & torrent manager
`
}
