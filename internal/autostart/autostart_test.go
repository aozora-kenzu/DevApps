//go:build windows

package autostart

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestFormatParseAndRegistrySync(t *testing.T) {
	exe := `C:\Program Files\UniDevUI\UniDevUI.exe`
	cmd := FormatCommand(exe, true)
	gotExe, gotTray := ParseCommand(cmd)
	if gotExe != exe || !gotTray {
		t.Fatalf("unexpected parse with tray: exe=%q tray=%v (cmd=%q)", gotExe, gotTray, cmd)
	}

	cmdVisible := FormatCommand(exe, false)
	gotExe2, gotTray2 := ParseCommand(cmdVisible)
	if gotExe2 != exe || gotTray2 {
		t.Fatalf("unexpected parse without tray: exe=%q tray=%v (cmd=%q)", gotExe2, gotTray2, cmdVisible)
	}

	if !HasTrayArg([]string{`UniDevUI.exe`, `--tray`}) || HasTrayArg([]string{`UniDevUI.exe`}) {
		t.Fatal("HasTrayArg check failed")
	}

	testSubkey := fmt.Sprintf(`Software\UniDevUI_Test_%d`, os.Getpid())
	defer func() { _ = registry.DeleteKey(registry.CURRENT_USER, testSubkey) }()

	if err := syncAt(registry.CURRENT_USER, testSubkey, "UniDevUI", exe, true, true); err != nil {
		t.Fatalf("enable syncAt failed: %v", err)
	}
	en, tr, err := isEnabledAt(registry.CURRENT_USER, testSubkey, "UniDevUI")
	if err != nil || !en || !tr {
		t.Fatalf("expected enabled+tray, got en=%v tr=%v err=%v", en, tr, err)
	}

	if err := syncAt(registry.CURRENT_USER, testSubkey, "UniDevUI", exe, false, false); err != nil {
		t.Fatalf("disable syncAt failed: %v", err)
	}
	en2, tr2, err := isEnabledAt(registry.CURRENT_USER, testSubkey, "UniDevUI")
	if err != nil || en2 || tr2 {
		t.Fatalf("expected disabled, got en=%v tr=%v err=%v", en2, tr2, err)
	}
}
