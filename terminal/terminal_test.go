package terminal_test

import (
	"bytes"
	"testing"

	"github.com/bluescreen10/myde/terminal"
)

func TestScreenOnlyWritesChangedRows(t *testing.T) {
	var output bytes.Buffer
	screen := terminal.NewScreen(&output, 4, 2)
	style := terminal.Style{Foreground: terminal.Color{R: 255}}
	screen.Clear(style)
	screen.Text(0, 0, "test", style)
	if err := screen.Flush(0, 0); err != nil {
		t.Fatal(err)
	}
	output.Reset()
	if err := screen.Flush(0, 0); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); got != "\x1b[0m\x1b[1;1H" {
		t.Fatalf("unchanged flush = %q", got)
	}
}

func TestParseColor(t *testing.T) {
	color, err := terminal.ParseColor("#1E2F3A")
	if err != nil {
		t.Fatal(err)
	}
	if want := (terminal.Color{R: 30, G: 47, B: 58}); color != want {
		t.Fatalf("ParseColor() = %+v, want %+v", color, want)
	}
}

func TestReaderDecodesStandaloneEscape(t *testing.T) {
	reader := terminal.NewReader(bytes.NewBufferString("\x1b"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyEscape {
		t.Fatalf("ReadEvent().Key = %v, want Escape", event.Key)
	}
}

func TestReaderDecodesArrowSequence(t *testing.T) {
	reader := terminal.NewReader(bytes.NewBufferString("\x1b[A"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyUp || event.Shift || event.Alt || event.Control || event.Super {
		t.Fatalf("ReadEvent().Key = %v, want Up", event.Key)
	}
}

func TestReaderDecodesControlDigit(t *testing.T) {
	reader := terminal.NewReader(bytes.NewBufferString("\x1b[49;5u"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyRune || event.Rune != '1' || !event.Control {
		t.Fatalf("ReadEvent() = %+v, want Control-1", event)
	}
}

func TestReaderDecodesExtendedArrow(t *testing.T) {
	reader := terminal.NewReader(bytes.NewBufferString("\x1b[1;1A"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyUp {
		t.Fatalf("ReadEvent() = %+v, want Up", event)
	}
}

func TestReaderDecodesShiftArrow(t *testing.T) {
	reader := terminal.NewReader(bytes.NewBufferString("\x1b[1;2D"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyLeft || !event.Shift || event.Alt || event.Control || event.Super {
		t.Fatalf("ReadEvent() = %+v, want Shift-Left", event)
	}
}

func TestReaderDecodesSuperArrow(t *testing.T) {
	tests := []struct {
		sequence string
		key      terminal.Key
	}{
		{sequence: "\x1b[1;9A", key: terminal.KeyUp},
		{sequence: "\x1b[1;9B", key: terminal.KeyDown},
		{sequence: "\x1b[1;9C", key: terminal.KeyRight},
		{sequence: "\x1b[1;9D", key: terminal.KeyLeft},
		{sequence: "\x1b[1;9:1A", key: terminal.KeyUp},
		{sequence: "\x1b[1;9:2B", key: terminal.KeyDown},
		{sequence: "\x1b[1;17C", key: terminal.KeyRight},
		{sequence: "\x1b[1;33D", key: terminal.KeyLeft},
	}
	for _, test := range tests {
		reader := terminal.NewReader(bytes.NewBufferString(test.sequence))
		event, err := reader.ReadEvent()
		if err != nil {
			t.Fatal(err)
		}
		if event.Key != test.key || !event.Super || event.Shift || event.Alt || event.Control {
			t.Fatalf("ReadEvent(%q) = %+v, want Super modifier and key %v", test.sequence, event, test.key)
		}
	}
}

func TestReaderDecodesReportAllRune(t *testing.T) {
	reader := terminal.NewReader(bytes.NewBufferString("\x1b[97u"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyRune || event.Rune != 'a' ||
		event.Super || event.Shift || event.Alt || event.Control {
		t.Fatalf("ReadEvent() = %+v, want unmodified a", event)
	}
}

func TestReaderIgnoresKittyModifierKeys(t *testing.T) {
	for _, codepoint := range []string{"57441", "57447", "57454"} {
		reader := terminal.NewReader(bytes.NewBufferString("\x1b[" + codepoint + ";2:1u"))
		event, err := reader.ReadEvent()
		if err != nil {
			t.Fatal(err)
		}
		if event.Key != terminal.KeyIgnored || event.Rune != 0 {
			t.Errorf("ReadEvent(%s) = %+v, want ignored key", codepoint, event)
		}
	}
}

func TestReaderIgnoresKittyReleaseEvent(t *testing.T) {
	reader := terminal.NewReader(bytes.NewBufferString("\x1b[1;9:3A"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyIgnored {
		t.Fatalf("ReadEvent() = %+v, want ignored key", event)
	}
}

func TestReaderDecodesKittyAlternateKeyFields(t *testing.T) {
	reader := terminal.NewReader(bytes.NewBufferString("\x1b[97:65;2:1u"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyRune || event.Rune != 'a' || !event.Shift {
		t.Fatalf("ReadEvent() = %+v, want Shift-A key event", event)
	}
}

func TestReaderDecodesSuperRune(t *testing.T) {
	reader := terminal.NewReader(bytes.NewBufferString("\x1b[115;9u"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyRune || event.Rune != 's' || !event.Super ||
		event.Shift || event.Alt || event.Control {
		t.Fatalf("ReadEvent() = %+v, want Super-S", event)
	}
}

func TestScreenWritesCurlyColoredUnderline(t *testing.T) {
	var output bytes.Buffer
	red := terminal.Color{R: 255}
	screen := terminal.NewScreen(&output, 1, 1)
	style := terminal.Style{
		CurlyUnderline: true, UnderlineColor: red, HasUnderlineColor: true,
	}
	screen.Clear(style)
	screen.Text(0, 0, "x", style)
	if err := screen.Flush(0, 0); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !bytes.Contains([]byte(got), []byte(";4:3;58;2;255;0;0")) {
		t.Fatalf("curly underline frame = %q", got)
	}
}

func TestScreenKeepsWideRunesAligned(t *testing.T) {
	var output bytes.Buffer
	screen := terminal.NewScreen(&output, 4, 1)
	style := terminal.Style{}
	screen.Clear(style)
	screen.Text(0, 0, "界x", style)
	if err := screen.Flush(0, 0); err != nil {
		t.Fatal(err)
	}
	if got := output.String(); !bytes.Contains([]byte(got), []byte("界x ")) {
		t.Fatalf("wide-rune frame = %q", got)
	}
}
