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
	reader := terminal.NewReader(bytes.NewBufferString("\x1b[1;9C"))
	event, err := reader.ReadEvent()
	if err != nil {
		t.Fatal(err)
	}
	if event.Key != terminal.KeyRight || !event.Super || event.Shift || event.Alt || event.Control {
		t.Fatalf("ReadEvent() = %+v, want Super-Right", event)
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
