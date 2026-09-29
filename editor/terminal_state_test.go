package editor

import "testing"

func TestTerminalStateInterpretsRedrawAndSplitUTF8(t *testing.T) {
	state := newTerminalState(40, 10)
	state.Write([]byte("\x1b[31mwork $ first\x1b[0m"))
	state.Write([]byte("\r\x1b[Kwork $ second"))
	state.Write([]byte("\r\nvalue: \xe2\x82"))
	state.Write([]byte("\xac"))

	content, point := state.Snapshot()
	if got, want := string(content), "work $ second\nvalue: €"; got != want {
		t.Fatalf("terminal snapshot = %q, want %q", got, want)
	}
	if point.Line != 1 || point.Column != 8 {
		t.Fatalf("terminal cursor = %+v, want line 1 column 8", point)
	}
}

func TestTerminalStateHandlesCursorEditing(t *testing.T) {
	state := newTerminalState(40, 10)
	state.Write([]byte("abcdef\x1b[3D\x1b[2PXY"))
	content, point := state.Snapshot()
	if got, want := string(content), "abcXY"; got != want {
		t.Fatalf("terminal snapshot = %q, want %q", got, want)
	}
	if point.Column != 5 {
		t.Fatalf("terminal cursor column = %d, want 5", point.Column)
	}
}
