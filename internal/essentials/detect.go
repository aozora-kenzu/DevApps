package essentials

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows/registry"

	"github.com/aozora-kenzu/UniDevUI/internal/pathmgr"
)

// RegistryAppEntry represents an installed Windows desktop program discovered in Uninstall registry keys.
type RegistryAppEntry struct {
	DisplayName    string
	DisplayVersion string
	Publisher      string
}

// QueryRegistryUninstallApps scans standard Windows Uninstall registry hives for installed desktop apps.
func QueryRegistryUninstallApps() []RegistryAppEntry {
	type rootPath struct {
		root registry.Key
		path string
	}
	locations := []rootPath{
		{registry.LOCAL_MACHINE, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.LOCAL_MACHINE, `SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall`},
		{registry.CURRENT_USER, `SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall`},
	}

	var entries []RegistryAppEntry
	seen := make(map[string]bool)

	for _, loc := range locations {
		k, err := registry.OpenKey(loc.root, loc.path, registry.READ)
		if err != nil {
			continue
		}
		subNames, err := k.ReadSubKeyNames(-1)
		k.Close()
		if err != nil {
			continue
		}

		for _, sub := range subNames {
			sk, err := registry.OpenKey(loc.root, loc.path+`\`+sub, registry.READ)
			if err != nil {
				continue
			}
			sysComp, _, scErr := sk.GetIntegerValue("SystemComponent")
			if scErr == nil && sysComp == 1 {
				sk.Close()
				continue
			}
			dispName, _, dnErr := sk.GetStringValue("DisplayName")
			dispVer, _, _ := sk.GetStringValue("DisplayVersion")
			pub, _, _ := sk.GetStringValue("Publisher")
			sk.Close()

			if dnErr != nil {
				continue
			}
			dispName = strings.TrimSpace(dispName)
			if dispName == "" {
				continue
			}
			dispVer = CleanInstalledVersion(dispVer)
			pub = strings.TrimSpace(pub)

			dedupKey := strings.ToLower(dispName) + "|" + strings.ToLower(dispVer)
			if seen[dedupKey] {
				continue
			}
			seen[dedupKey] = true

			entries = append(entries, RegistryAppEntry{
				DisplayName:    dispName,
				DisplayVersion: dispVer,
				Publisher:      pub,
			})
		}
	}
	return entries
}

// CleanInstalledVersion normalizes empty or "Unknown" version labels into empty string.
func CleanInstalledVersion(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" || strings.EqualFold(v, "unknown") || v == "-" {
		return ""
	}
	return v
}

type aliasRule struct {
	prefixes  []string
	publisher string
}

var aliasRulesByKey = map[string]aliasRule{
	"firefox":                                   {prefixes: []string{"mozilla firefox"}, publisher: "mozilla"},
	"mozilla.firefox":                           {prefixes: []string{"mozilla firefox"}, publisher: "mozilla"},
	"chrome":                                    {prefixes: []string{"google chrome"}, publisher: "google"},
	"google.chrome":                             {prefixes: []string{"google chrome"}, publisher: "google"},
	"brave":                                     {prefixes: []string{"brave"}, publisher: "brave"},
	"brave.brave":                               {prefixes: []string{"brave"}, publisher: "brave"},
	"nanazip":                                   {prefixes: []string{"nanazip"}, publisher: ""},
	"m2team.nanazip":                            {prefixes: []string{"nanazip"}, publisher: ""},
	"winrar":                                    {prefixes: []string{"winrar"}, publisher: ""},
	"rarlab.winrar":                             {prefixes: []string{"winrar"}, publisher: ""},
	"revo":                                      {prefixes: []string{"revo uninstaller"}, publisher: "revo"},
	"revouninstaller.revouninstaller":           {prefixes: []string{"revo uninstaller"}, publisher: "revo"},
	"lightshot":                                 {prefixes: []string{"lightshot"}, publisher: "skillbrains"},
	"skillbrains.lightshot":                     {prefixes: []string{"lightshot"}, publisher: "skillbrains"},
	"stirlingpdf":                               {prefixes: []string{"stirling-pdf", "stirling pdf"}, publisher: ""},
	"stirlingtools.stirlingpdf":                 {prefixes: []string{"stirling-pdf", "stirling pdf"}, publisher: ""},
	"powershell7":                               {prefixes: []string{"powershell 7", "powershell-7"}, publisher: "microsoft"},
	"microsoft.powershell":                      {prefixes: []string{"powershell 7", "powershell-7"}, publisher: "microsoft"},
	"windowsterminal":                           {prefixes: []string{"windows terminal"}, publisher: ""},
	"microsoft.windowsterminal":                 {prefixes: []string{"windows terminal"}, publisher: ""},
	"notepadpp":                                 {prefixes: []string{"notepad++"}, publisher: "notepad++"},
	"notepad++.notepad++":                       {prefixes: []string{"notepad++"}, publisher: "notepad++"},
	"emeditor":                                  {prefixes: []string{"emeditor"}, publisher: "emurasoft"},
	"emurasoft.emeditor":                        {prefixes: []string{"emeditor"}, publisher: "emurasoft"},
	"sublimetext":                               {prefixes: []string{"sublime text"}, publisher: "sublime"},
	"sublimehq.sublimetext.4":                   {prefixes: []string{"sublime text"}, publisher: "sublime"},
	"vscode":                                    {prefixes: []string{"microsoft visual studio code"}, publisher: "microsoft"},
	"microsoft.visualstudiocode":                {prefixes: []string{"microsoft visual studio code"}, publisher: "microsoft"},
	"antigravity":                               {prefixes: []string{"antigravity"}, publisher: "google"},
	"google.antigravity":                        {prefixes: []string{"antigravity"}, publisher: "google"},
	"laragon":                                   {prefixes: []string{"laragon"}, publisher: ""},
	"lengockhoa.laragon":                        {prefixes: []string{"laragon"}, publisher: ""},
	"vlc":                                       {prefixes: []string{"vlc media player"}, publisher: "videolan"},
	"videolan.vlc":                              {prefixes: []string{"vlc media player"}, publisher: "videolan"},
	"vcredist":                                  {prefixes: []string{"microsoft visual c++ 2015", "microsoft visual c++ v14 redistributable (x64)"}, publisher: "microsoft"},
	"microsoft.vcredist.2015+.x64":              {prefixes: []string{"microsoft visual c++ 2015", "microsoft visual c++ v14 redistributable (x64)"}, publisher: "microsoft"},
	"libreoffice":                               {prefixes: []string{"libreoffice"}, publisher: "document foundation"},
	"thedocumentfoundation.libreoffice":         {prefixes: []string{"libreoffice"}, publisher: "document foundation"},
	"whatsapp":                                  {prefixes: []string{"whatsapp"}, publisher: ""},
	"9nksqgp7f2nh":                              {prefixes: []string{"whatsapp"}, publisher: ""},
	"telegram":                                  {prefixes: []string{"telegram desktop"}, publisher: "telegram"},
	"telegram.telegramdesktop":                  {prefixes: []string{"telegram desktop"}, publisher: "telegram"},
	"discord":                                   {prefixes: []string{"discord"}, publisher: "discord"},
	"discord.discord":                           {prefixes: []string{"discord"}, publisher: "discord"},
	"thunderbird":                               {prefixes: []string{"mozilla thunderbird"}, publisher: "mozilla"},
	"mozilla.thunderbird":                       {prefixes: []string{"mozilla thunderbird"}, publisher: "mozilla"},
	"googledrive":                               {prefixes: []string{"google drive"}, publisher: "google"},
	"google.googledrive":                        {prefixes: []string{"google drive"}, publisher: "google"},
	"idm":                                       {prefixes: []string{"internet download manager"}, publisher: "tonec"},
	"tonec.internetdownloadmanager":             {prefixes: []string{"internet download manager"}, publisher: "tonec"},
	"xdm":                                       {prefixes: []string{"xtreme download manager"}, publisher: ""},
	"subhra74.xtremedownloadmanager":            {prefixes: []string{"xtreme download manager"}, publisher: ""},
	"fdm":                                       {prefixes: []string{"free download manager"}, publisher: "softdeluxe"},
	"softdeluxe.freedownloadmanager":            {prefixes: []string{"free download manager"}, publisher: "softdeluxe"},
}

var channelMarkers = []string{
	"beta",
	"canary",
	"nightly",
	"insiders",
	"esr",
	"developer edition",
	"maintenance service",
	"helper",
	"updater",
}

// InstallMatch describes how an EssentialApp was detected on Windows.
type InstallMatch struct {
	Installed    bool
	InstalledVer string
	AvailableVer string
	ExactWinget  bool
	MatchedVia   string // "winget" | "registry" | "winget_name"
}

// MatchInstalledEssential determines install status via Winget ID first, then Registry Uninstall keys and normalized Winget names.
func MatchInstalledEssential(
	item EssentialApp,
	byID map[string]WingetTableRow,
	regApps []RegistryAppEntry,
	wingetRows []WingetTableRow,
) InstallMatch {
	if row, ok := byID[strings.ToLower(strings.TrimSpace(item.WingetID))]; ok {
		return InstallMatch{
			Installed:    true,
			InstalledVer: CleanInstalledVersion(row.Version),
			AvailableVer: strings.TrimSpace(row.Available),
			ExactWinget:  true,
			MatchedVia:   "winget",
		}
	}

	rule := buildAliasRule(item)

	// Check Windows Registry first (authoritative DisplayName + Publisher, immune to Winget ARP mis-mapping like Chrome -> Chrome.Beta.EXE).
	bestVer := ""
	foundReg := false
	for _, entry := range regApps {
		if matchesCandidateName(entry.DisplayName, entry.Publisher, item.Name, rule) {
			if !foundReg || (bestVer == "" && entry.DisplayVersion != "") {
				bestVer = entry.DisplayVersion
			}
			foundReg = true
		}
	}
	if foundReg {
		// Also check if winget list holds a non-empty version for the same display name.
		if bestVer == "" {
			for _, row := range wingetRows {
				if matchesCandidateName(row.Name, "", item.Name, rule) {
					if v := CleanInstalledVersion(row.Version); v != "" {
						bestVer = v
						break
					}
				}
			}
		}
		return InstallMatch{
			Installed:    true,
			InstalledVer: bestVer,
			MatchedVia:   "registry",
		}
	}

	// Fallback to winget list display names (covers MSIX/MS Store installs such as WhatsApp Desktop).
	for _, row := range wingetRows {
		if matchesCandidateName(row.Name, "", item.Name, rule) {
			return InstallMatch{
				Installed:    true,
				InstalledVer: CleanInstalledVersion(row.Version),
				AvailableVer: strings.TrimSpace(row.Available),
				MatchedVia:   "winget_name",
			}
		}
	}

	return InstallMatch{}
}

func buildAliasRule(item EssentialApp) aliasRule {
	if r, ok := aliasRulesByKey[strings.ToLower(item.WingetID)]; ok {
		return r
	}
	if r, ok := aliasRulesByKey[strings.ToLower(item.ID)]; ok {
		return r
	}
	return aliasRule{
		prefixes: []string{strings.ToLower(strings.TrimSpace(item.Name))},
	}
}

func matchesCandidateName(candName, candPublisher, targetName string, rule aliasRule) bool {
	cLower := strings.ToLower(strings.TrimSpace(candName))
	tLower := strings.ToLower(strings.TrimSpace(targetName))
	if cLower == "" {
		return false
	}

	if hasConflictingChannel(cLower, tLower) {
		return false
	}

	// Reject 32-bit x86 VCRedist when target explicitly requests x64.
	if strings.Contains(tLower, "x64") && strings.Contains(cLower, "(x86)") {
		return false
	}

	if rule.publisher != "" && candPublisher != "" {
		if !strings.Contains(strings.ToLower(candPublisher), rule.publisher) {
			return false
		}
	}

	for _, pref := range rule.prefixes {
		if pref == "" {
			continue
		}
		if cLower == pref {
			return true
		}
		for _, sep := range []string{" ", "-", "("} {
			if strings.HasPrefix(cLower, pref+sep) {
				return true
			}
		}
	}

	return false
}

func hasConflictingChannel(candLower, targetLower string) bool {
	for _, marker := range channelMarkers {
		if strings.Contains(candLower, marker) && !strings.Contains(targetLower, marker) {
			return true
		}
	}
	// Reject standalone "dev" channel markers like "Chrome Dev" or "Edge Dev" without matching "developer".
	for _, token := range strings.Fields(candLower) {
		if token == "dev" && !strings.Contains(targetLower, "dev") {
			return true
		}
	}
	return false
}

// DescribeDetectionSource returns a concise human-readable detection label.
func DescribeDetectionSource(via string) string {
	switch via {
	case "registry":
		return fmt.Sprintf("Windows Registry")
	case "winget_name":
		return "Winget / MSIX"
	default:
		return "Winget"
	}
}

// AnnotateSearchRowsInstalled enriches `winget search` rows with local Windows install/update detection
// using exact Winget IDs, Uninstall Registry display names, and `winget list` rows.
func AnnotateSearchRowsInstalled(
	rows []WingetTableRow,
	byID map[string]WingetTableRow,
	regApps []RegistryAppEntry,
	wingetRows []WingetTableRow,
	ignoredMap map[string]IgnoredApp,
) []WingetTableRow {
	out := make([]WingetTableRow, len(rows))
	for i, r := range rows {
		probe := EssentialApp{
			ID:       r.ID,
			Name:     r.Name,
			WingetID: r.ID,
			Source:   r.Source,
		}
		match := MatchInstalledEssential(probe, byID, regApps, wingetRows)
		if match.Installed {
			r.Installed = true
			r.InstalledVer = match.InstalledVer
			r.MatchedBy = DescribeDetectionSource(match.MatchedVia)

			ignEntry, hasIgn := ignoredMap[strings.ToLower(strings.TrimSpace(r.ID))]
			candidateAvail := strings.TrimSpace(match.AvailableVer)
			if candidateAvail == "" && !match.ExactWinget && match.InstalledVer != "" && strings.TrimSpace(r.Version) != "" {
				if pathmgr.CompareSemver(match.InstalledVer, r.Version) < 0 {
					candidateAvail = strings.TrimSpace(r.Version)
				}
			}
			fullIgn, verSkipped := EvaluateIgnoredPolicy(ignEntry, hasIgn, candidateAvail)
			if !fullIgn && !verSkipped && candidateAvail != "" {
				r.Available = candidateAvail
				r.HasUpdate = true
			}
		}
		out[i] = r
	}
	return out
}
