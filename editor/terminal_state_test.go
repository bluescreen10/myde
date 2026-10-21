package editor_test

import (
	"strings"
	"testing"
)

func TestTerminalRendersRedrawAndUTF8(t *testing.T) {
	h := scriptedTerminal(t, `printf '\033[31mwork $ first\033[0m\r\033[Kwork $ second\r\nvalue: \342\202'; printf '\254'`)
	h.wait("redrawn output", func() bool { return strings.Contains(h.content(), "work $ second\nvalue: €") })
	if strings.Contains(h.content(), "work $ first") || strings.Contains(h.content(), "\x1b") {
		t.Fatalf("terminal rendered raw escapes: %q", h.content())
	}
}
func TestTerminalHandlesCursorEditing(t *testing.T) {
	h := scriptedTerminal(t, `printf 'abcdef\033[3D\033[2PXY'`)
	h.wait("cursor edit", func() bool { return strings.Contains(h.content(), "abcXY") })
	if strings.Contains(h.content(), "abcdef") {
		t.Fatalf("cursor editing failed: %q", h.content())
	}
}
