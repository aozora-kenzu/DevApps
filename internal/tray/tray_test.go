//go:build windows

package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"testing"
	"unsafe"
)

func TestNotifyIconDataLayoutBadgesAndLocalization(t *testing.T) {
	const expectedWin64NotifyIconDataSize = 976
	if got := unsafe.Sizeof(notifyIconDataW{}); got != expectedWin64NotifyIconDataSize {
		t.Fatalf("unexpected notifyIconDataW size: got %d, want %d", got, expectedWin64NotifyIconDataSize)
	}

	var buf [8]uint16
	copyUTF16(buf[:], "UniDevUI-Long-String")
	if buf[len(buf)-1] != 0 {
		t.Fatal("expected NUL-terminated UTF-16 buffer")
	}

	idLabels := menuLabelsForLang("id", 3)
	enLabels := menuLabelsForLang("en", 0)
	if idLabels.Open != "Buka UniDevUI" || idLabels.Updates != "Pembaruan (3)" {
		t.Fatalf("unexpected ID menu labels: %+v", idLabels)
	}
	if enLabels.Open != "Open UniDevUI" || enLabels.Updates != "Updates" {
		t.Fatalf("unexpected EN menu labels: %+v", enLabels)
	}

	src := image.NewNRGBA(image.Rect(0, 0, 16, 16))
	icoIdle, err := buildBadgeICOBytes(src, nil)
	if err != nil {
		t.Fatalf("buildBadgeICOBytes(idle) failed: %v", err)
	}
	dot := color.NRGBA{R: 34, G: 197, B: 94, A: 255}
	icoGreen, err := buildBadgeICOBytes(src, &dot)
	if err != nil {
		t.Fatalf("buildBadgeICOBytes(green) failed: %v", err)
	}
	if len(icoIdle) <= 22 || binary.LittleEndian.Uint16(icoIdle[2:4]) != 1 || binary.LittleEndian.Uint16(icoIdle[4:6]) != 1 {
		t.Fatalf("invalid ICO header: len=%d", len(icoIdle))
	}
	if !bytes.HasPrefix(icoGreen[22:], []byte{0x89, 'P', 'N', 'G'}) {
		t.Fatal("expected PNG payload inside generated ICO badge")
	}
	if bytes.Equal(icoIdle, icoGreen) {
		t.Fatal("expected green badge ICO to differ from idle ICO")
	}
}
