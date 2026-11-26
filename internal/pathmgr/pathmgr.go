package pathmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const (
	userEnvSubkey   = `Environment`
	systemEnvSubkey = `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`

	hwndBroadcast    = 0xffff
	wmSettingChange  = 0x001a
	smtoAbortIfHung  = 0x0002
	broadcastTimeout = 5000
)

var (
	user32              = syscall.NewLazyDLL("user32.dll")
	procSendMsgTimeoutW = user32.NewProc("SendMessageTimeoutW")
)

// ConflictInfo describes an external executable found outside UniDevUI's active directory.
type ConflictInfo struct {
	Scope    string `json:"scope"`    // "System PATH" or "User PATH"
	FullPath string `json:"fullPath"` // Full path to conflicting executable
}

// UserPathEntry describes a single entry inside HKCU\Environment\Path.
type UserPathEntry struct {
	Index     int    `json:"index"`
	Path      string `json:"path"`
	Expanded  string `json:"expanded"`
	Exists    bool   `json:"exists"`
	Managed   bool   `json:"managed"`
	Duplicate bool   `json:"duplicate"`
}

// GetUserPathEntries reads the raw User PATH from HKCU\Environment without expanding variables.
func GetUserPathEntries() ([]string, uint32, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, userEnvSubkey, registry.QUERY_VALUE)
	if err != nil {
		if err == registry.ErrNotExist {
			return nil, registry.EXPAND_SZ, nil
		}
		return nil, 0, fmt.Errorf("gagal membuka HKCU\\Environment: %w", err)
	}
	defer key.Close()

	val, valType, err := key.GetStringValue("Path")
	if err != nil {
		if err == registry.ErrNotExist {
			return nil, registry.EXPAND_SZ, nil
		}
		return nil, 0, fmt.Errorf("gagal membaca User PATH: %w", err)
	}

	return splitPathList(val), valType, nil
}

// SetAppPath inserts or replaces an application's managed entry in HKCU\Environment\Path
// and broadcasts WM_SETTINGCHANGE so newly opened terminals pick up the change immediately.
func SetAppPath(baseDir, appID, newBinDir string) ([]string, error) {
	cleanNew := normalizePath(newBinDir)
	if cleanNew == "" {
		return nil, fmt.Errorf("path target kosong")
	}

	entries, valType, err := GetUserPathEntries()
	if err != nil {
		return nil, err
	}

	appRoot := normalizePath(filepath.Join(baseDir, appID))
	updated := make([]string, 0, len(entries)+1)
	replaced := false

	for _, entry := range entries {
		norm := normalizePath(entry)
		if norm == "" {
			continue
		}
		if isManagedAppEntry(norm, appRoot) || strings.EqualFold(norm, cleanNew) {
			if !replaced {
				updated = append(updated, cleanNew)
				replaced = true
			}
			continue
		}
		updated = append(updated, entry)
	}

	// Prepend newly activated UniDevUI tool so it takes precedence over other User PATH entries.
	if !replaced {
		updated = append([]string{cleanNew}, updated...)
	}

	if err := writeUserPath(updated, valType); err != nil {
		return nil, err
	}
	BroadcastEnvironmentChange()
	return updated, nil
}

// RemoveAppPath deletes any User PATH entry belonging to baseDir\appID.
func RemoveAppPath(baseDir, appID string) (bool, error) {
	entries, valType, err := GetUserPathEntries()
	if err != nil {
		return false, err
	}

	appRoot := normalizePath(filepath.Join(baseDir, appID))
	updated := make([]string, 0, len(entries))
	removed := false

	for _, entry := range entries {
		norm := normalizePath(entry)
		if norm == "" {
			continue
		}
		if isManagedAppEntry(norm, appRoot) {
			removed = true
			continue
		}
		updated = append(updated, entry)
	}

	if !removed {
		return false, nil
	}

	if err := writeUserPath(updated, valType); err != nil {
		return false, err
	}
	BroadcastEnvironmentChange()
	return true, nil
}

// AddDirToUserPath appends an arbitrary directory (such as %USERPROFILE%\go\bin) to User PATH if missing.
func AddDirToUserPath(dir string) (bool, error) {
	clean := normalizePath(dir)
	if clean == "" {
		return false, fmt.Errorf("direktori kosong")
	}
	entries, valType, err := GetUserPathEntries()
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if strings.EqualFold(normalizePath(entry), clean) {
			return false, nil
		}
	}
	updated := append(entries, clean)
	if err := writeUserPath(updated, valType); err != nil {
		return false, err
	}
	BroadcastEnvironmentChange()
	return true, nil
}

// ExpandPathEntry resolves Windows %VAR% references in a User PATH entry.
func ExpandPathEntry(raw string) string {
	trimmed := strings.TrimSpace(strings.Trim(raw, `"`))
	if trimmed == "" {
		return ""
	}
	if expanded, err := registry.ExpandString(trimmed); err == nil && strings.TrimSpace(expanded) != "" {
		return normalizePath(expanded)
	}
	return normalizePath(os.ExpandEnv(trimmed))
}

// InspectUserPathEntries returns detailed status for every directory in HKCU\Environment\Path.
func InspectUserPathEntries(baseDir string) ([]UserPathEntry, error) {
	entries, _, err := GetUserPathEntries()
	if err != nil {
		return nil, err
	}
	baseNorm := normalizePath(baseDir)
	basePrefix := strings.ToLower(baseNorm) + `\`
	seen := make(map[string]bool, len(entries))
	out := make([]UserPathEntry, 0, len(entries))

	for idx, raw := range entries {
		expanded := ExpandPathEntry(raw)
		normKey := strings.ToLower(expanded)
		if normKey == "" {
			normKey = strings.ToLower(normalizePath(raw))
		}
		dup := normKey != "" && seen[normKey]
		if normKey != "" {
			seen[normKey] = true
		}
		managed := baseNorm != "" && (strings.EqualFold(expanded, baseNorm) || strings.HasPrefix(strings.ToLower(expanded), basePrefix))
		out = append(out, UserPathEntry{
			Index:     idx,
			Path:      raw,
			Expanded:  expanded,
			Exists:    dirExists(expanded),
			Managed:   managed,
			Duplicate: dup,
		})
	}
	return out, nil
}

// MoveUserPathEntry moves an entry up (delta < 0) or down (delta > 0) in HKCU\Environment\Path.
func MoveUserPathEntry(index, delta int) ([]string, error) {
	entries, valType, err := GetUserPathEntries()
	if err != nil {
		return nil, err
	}
	updated, moved, err := swapPathEntries(entries, index, delta)
	if err != nil {
		return nil, err
	}
	if !moved {
		return entries, nil
	}
	if err := writeUserPath(updated, valType); err != nil {
		return nil, err
	}
	BroadcastEnvironmentChange()
	return updated, nil
}

// RemoveUserPathEntry removes the entry matching index/expectedPath from HKCU\Environment\Path.
func RemoveUserPathEntry(index int, expectedPath string) ([]string, error) {
	entries, valType, err := GetUserPathEntries()
	if err != nil {
		return nil, err
	}
	updated, removed := removePathEntry(entries, index, expectedPath)
	if !removed {
		return nil, fmt.Errorf("User PATH entry not found: %s", expectedPath)
	}
	if err := writeUserPath(updated, valType); err != nil {
		return nil, err
	}
	BroadcastEnvironmentChange()
	return updated, nil
}

// CleanUserPathEntries removes duplicate or non-existent directory entries from HKCU\Environment\Path.
func CleanUserPathEntries() ([]string, []string, error) {
	entries, valType, err := GetUserPathEntries()
	if err != nil {
		return nil, nil, err
	}
	kept, removed := filterCleanPathEntries(entries, func(raw string) bool {
		return dirExists(ExpandPathEntry(raw))
	})
	if len(removed) == 0 {
		return nil, entries, nil
	}
	if err := writeUserPath(kept, valType); err != nil {
		return nil, nil, err
	}
	BroadcastEnvironmentChange()
	return removed, kept, nil
}

func swapPathEntries(entries []string, index, delta int) ([]string, bool, error) {
	if delta == 0 {
		return entries, false, nil
	}
	if index < 0 || index >= len(entries) {
		return nil, false, fmt.Errorf("invalid PATH index %d", index)
	}
	step := -1
	if delta > 0 {
		step = 1
	}
	target := index + step
	if target < 0 || target >= len(entries) {
		return entries, false, nil
	}
	out := append([]string(nil), entries...)
	out[index], out[target] = out[target], out[index]
	return out, true, nil
}

func removePathEntry(entries []string, index int, expectedPath string) ([]string, bool) {
	targetIdx := -1
	expNorm := normalizePath(expectedPath)
	if index >= 0 && index < len(entries) {
		if expNorm == "" || strings.EqualFold(normalizePath(entries[index]), expNorm) {
			targetIdx = index
		}
	}
	if targetIdx < 0 && expNorm != "" {
		for i, item := range entries {
			if strings.EqualFold(normalizePath(item), expNorm) {
				targetIdx = i
				break
			}
		}
	}
	if targetIdx < 0 {
		return entries, false
	}
	out := make([]string, 0, len(entries)-1)
	out = append(out, entries[:targetIdx]...)
	out = append(out, entries[targetIdx+1:]...)
	return out, true
}

func filterCleanPathEntries(entries []string, existsFn func(string) bool) (kept []string, removed []string) {
	seen := make(map[string]bool, len(entries))
	kept = make([]string, 0, len(entries))
	for _, entry := range entries {
		norm := strings.ToLower(normalizePath(ExpandPathEntry(entry)))
		if norm == "" {
			norm = strings.ToLower(normalizePath(entry))
		}
		if norm == "" || seen[norm] || (existsFn != nil && !existsFn(entry)) {
			removed = append(removed, entry)
			continue
		}
		seen[norm] = true
		kept = append(kept, entry)
	}
	return kept, removed
}

func dirExists(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

// ContainsDir checks whether entries (or effective PATH) contains dir case-insensitively.
func ContainsDir(entries []string, dir string) bool {
	target := normalizePath(dir)
	if target == "" {
		return false
	}
	for _, entry := range entries {
		if strings.EqualFold(normalizePath(entry), target) {
			return true
		}
	}
	return false
}

// ActiveVersions maps appID -> active version folder extracted from User PATH entries under baseDir.
func ActiveVersions(baseDir string) (map[string]string, error) {
	entries, _, err := GetUserPathEntries()
	if err != nil {
		return nil, err
	}

	baseNorm := normalizePath(baseDir)
	basePrefix := strings.ToLower(baseNorm) + `\`
	active := make(map[string]string)

	for _, entry := range entries {
		norm := normalizePath(entry)
		lower := strings.ToLower(norm)
		if !strings.HasPrefix(lower, basePrefix) {
			continue
		}
		rel := norm[len(basePrefix):]
		parts := strings.Split(rel, `\`)
		if len(parts) >= 2 && parts[0] != "" && parts[1] != "" {
			appKey := strings.ToLower(parts[0])
			if _, exists := active[appKey]; !exists {
				active[appKey] = parts[1]
			}
		}
	}
	return active, nil
}

// ResolveEffectiveBinary finds the first matching executable in Windows PATH evaluation order
// (System PATH HKLM first, then User PATH HKCU).
func ResolveEffectiveBinary(binaryName string) (scope string, fullPath string) {
	if strings.TrimSpace(binaryName) == "" {
		return "", ""
	}
	if sysPath, err := readExpandedPath(registry.LOCAL_MACHINE, systemEnvSubkey); err == nil {
		for _, dir := range splitPathList(sysPath) {
			if candidate := findExecutableInDir(dir, binaryName); candidate != "" {
				return "System PATH", candidate
			}
		}
	}
	if usrPath, err := readExpandedPath(registry.CURRENT_USER, userEnvSubkey); err == nil {
		for _, dir := range splitPathList(usrPath) {
			if candidate := findExecutableInDir(dir, binaryName); candidate != "" {
				return "User PATH", candidate
			}
		}
	}
	return "", ""
}

// CompareSemver compares two dotted version strings numerically (returns -1, 0, or 1).
func CompareSemver(a, b string) int {
	clean := func(s string) []int {
		s = strings.TrimSpace(strings.TrimPrefix(strings.ToLower(s), "v"))
		if s == "" {
			return nil
		}
		if dash := strings.IndexAny(s, "-+"); dash >= 0 {
			s = s[:dash]
		}
		parts := strings.Split(s, ".")
		nums := make([]int, 0, len(parts))
		for _, p := range parts {
			n, err := strconv.Atoi(p)
			if err != nil {
				n = 0
			}
			nums = append(nums, n)
		}
		return nums
	}

	pa := clean(a)
	pb := clean(b)
	maxLen := len(pa)
	if len(pb) > maxLen {
		maxLen = len(pb)
	}
	for i := 0; i < maxLen; i++ {
		va, vb := 0, 0
		if i < len(pa) {
			va = pa[i]
		}
		if i < len(pb) {
			vb = pb[i]
		}
		if va < vb {
			return -1
		}
		if va > vb {
			return 1
		}
	}
	return 0
}

// DetectBinaryConflict checks System PATH (HKLM) and User PATH (HKCU) in Windows evaluation order
// to warn if another executable shadows expectedDir\binaryName.
func DetectBinaryConflict(binaryName, expectedDir string) *ConflictInfo {
	if strings.TrimSpace(binaryName) == "" {
		return nil
	}

	expectedNorm := normalizePath(expectedDir)

	// Windows evaluates System PATH before User PATH.
	if sysPath, err := readExpandedPath(registry.LOCAL_MACHINE, systemEnvSubkey); err == nil {
		for _, dir := range splitPathList(sysPath) {
			candidate := findExecutableInDir(dir, binaryName)
			if candidate != "" && !strings.EqualFold(normalizePath(dir), expectedNorm) {
				return &ConflictInfo{Scope: "System PATH (HKLM)", FullPath: candidate}
			}
		}
	}

	if usrPath, err := readExpandedPath(registry.CURRENT_USER, userEnvSubkey); err == nil {
		for _, dir := range splitPathList(usrPath) {
			normDir := normalizePath(dir)
			if strings.EqualFold(normDir, expectedNorm) {
				// Reached our managed directory first within User PATH; no User PATH shadow.
				return nil
			}
			if candidate := findExecutableInDir(dir, binaryName); candidate != "" {
				return &ConflictInfo{Scope: "User PATH (HKCU)", FullPath: candidate}
			}
		}
	}
	return nil
}

// EffectiveProcessPath builds an up-to-date PATH string from HKLM + HKCU for running child tools.
func EffectiveProcessPath() string {
	var parts []string
	if sysPath, err := readExpandedPath(registry.LOCAL_MACHINE, systemEnvSubkey); err == nil {
		parts = append(parts, splitPathList(sysPath)...)
	}
	if usrPath, err := readExpandedPath(registry.CURRENT_USER, userEnvSubkey); err == nil {
		parts = append(parts, splitPathList(usrPath)...)
	}
	return strings.Join(parts, ";")
}

// BroadcastEnvironmentChange notifies Windows Explorer and future processes of Environment changes.
func BroadcastEnvironmentChange() {
	envPtr, err := syscall.UTF16PtrFromString("Environment")
	if err != nil {
		return
	}
	_, _, _ = procSendMsgTimeoutW.Call(
		uintptr(hwndBroadcast),
		uintptr(wmSettingChange),
		0,
		uintptr(unsafe.Pointer(envPtr)),
		uintptr(smtoAbortIfHung),
		uintptr(broadcastTimeout),
		0,
	)
}

func writeUserPath(entries []string, origType uint32) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, userEnvSubkey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("gagal membuka/membuat HKCU\\Environment: %w", err)
	}
	defer key.Close()

	joined := strings.Join(entries, ";")
	if origType == registry.SZ && !strings.Contains(joined, "%") {
		if err := key.SetStringValue("Path", joined); err != nil {
			return fmt.Errorf("gagal menulis User PATH (REG_SZ): %w", err)
		}
		return nil
	}

	if err := key.SetExpandStringValue("Path", joined); err != nil {
		return fmt.Errorf("gagal menulis User PATH (REG_EXPAND_SZ): %w", err)
	}
	return nil
}

func readExpandedPath(root registry.Key, subkey string) (string, error) {
	key, err := registry.OpenKey(root, subkey, registry.QUERY_VALUE)
	if err != nil {
		return "", err
	}
	defer key.Close()

	raw, _, err := key.GetStringValue("Path")
	if err != nil {
		return "", err
	}
	expanded, err := registry.ExpandString(raw)
	if err != nil {
		return raw, nil
	}
	return expanded, nil
}

func findExecutableInDir(dir, binaryName string) string {
	clean := strings.TrimSpace(dir)
	if clean == "" {
		return ""
	}
	candidate := filepath.Join(clean, binaryName)
	info, err := os.Stat(candidate)
	if err == nil && !info.IsDir() {
		return candidate
	}
	return ""
}

func splitPathList(raw string) []string {
	segments := strings.Split(raw, ";")
	out := make([]string, 0, len(segments))
	for _, seg := range segments {
		trimmed := strings.TrimSpace(seg)
		trimmed = strings.Trim(trimmed, `"`)
		if trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func normalizePath(p string) string {
	trimmed := strings.TrimSpace(p)
	trimmed = strings.Trim(trimmed, `"`)
	if trimmed == "" {
		return ""
	}
	cleaned := filepath.Clean(trimmed)
	if len(cleaned) > 3 {
		cleaned = strings.TrimRight(cleaned, `\`)
	}
	return cleaned
}

func isManagedAppEntry(normEntry, normAppRoot string) bool {
	entryLower := strings.ToLower(normEntry)
	rootLower := strings.ToLower(normAppRoot)
	return entryLower == rootLower || strings.HasPrefix(entryLower, rootLower+`\`)
}
