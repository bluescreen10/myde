package buffer_test

import (
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluescreen10/myde/buffer"
)

func TestPieceTableEdits(t *testing.T) {
	b := buffer.New()
	b.Insert(0, []byte("hello world"))
	b.Insert(5, []byte(", brave"))
	b.Delete(0, 1)

	if got, want := string(b.Bytes()), "ello, brave world"; got != want {
		t.Fatalf("Bytes() = %q, want %q", got, want)
	}
}

func TestNewBuffersStartClean(t *testing.T) {
	for _, b := range []*buffer.Buffer{buffer.New(), buffer.NewNamed("scratch")} {
		if b.IsDirty() {
			t.Fatalf("new buffer %q is dirty", b.Name())
		}
	}
}

func TestPointsAndLines(t *testing.T) {
	b := buffer.New()
	b.Insert(0, []byte("one\ntwø\nthree"))

	if got, want := b.LineCount(), 3; got != want {
		t.Fatalf("LineCount() = %d, want %d", got, want)
	}
	point := buffer.Point{Line: 1, Column: 3}
	if got, want := b.Point(b.Offset(point)), point; got != want {
		t.Fatalf("point round trip = %+v, want %+v", got, want)
	}
}

func TestTrailingNewlineCreatesEmptyLine(t *testing.T) {
	b := buffer.New()
	b.Insert(0, []byte("one\n"))

	if got, want := b.LineCount(), 2; got != want {
		t.Fatalf("LineCount() = %d, want %d", got, want)
	}
	if got, want := b.Point(b.Len()), (buffer.Point{Line: 1}); got != want {
		t.Fatalf("Point(Len()) = %+v, want %+v", got, want)
	}
}

func TestLineIndexTracksEdits(t *testing.T) {
	b := buffer.New()
	b.Insert(0, []byte("alpha\ngamma"))
	_ = b.LineCount()
	b.Insert(6, []byte("beta\n"))

	if got, want := b.LineCount(), 3; got != want {
		t.Fatalf("LineCount() after insert = %d, want %d", got, want)
	}
	if got, want := string(b.Line(1)), "beta"; got != want {
		t.Fatalf("Line(1) after insert = %q, want %q", got, want)
	}
	b.Delete(5, 11)
	if got, want := string(b.Bytes()), "alphagamma"; got != want {
		t.Fatalf("Bytes() after delete = %q, want %q", got, want)
	}
	if got, want := b.LineCount(), 1; got != want {
		t.Fatalf("LineCount() after delete = %d, want %d", got, want)
	}
}

func TestPieceTableMatchesPlainTextAcrossEdits(t *testing.T) {
	random := rand.New(rand.NewSource(42))
	b := buffer.New()
	want := ""
	for operation := 0; operation < 500; operation++ {
		if len(want) == 0 || random.Intn(2) == 0 {
			offset := random.Intn(len(want) + 1)
			choices := []string{"x", "word", "\n", "two\nlines"}
			text := choices[random.Intn(len(choices))]
			b.Insert(offset, []byte(text))
			want = want[:offset] + text + want[offset:]
		} else {
			start := random.Intn(len(want))
			end := start + random.Intn(len(want)-start+1)
			b.Delete(start, end)
			want = want[:start] + want[end:]
		}

		if got := string(b.Bytes()); got != want {
			t.Fatalf("operation %d: Bytes() = %q, want %q", operation, got, want)
		}
		if got, lineCount := b.LineCount(), strings.Count(want, "\n")+1; got != lineCount {
			t.Fatalf("operation %d: LineCount() = %d, want %d", operation, got, lineCount)
		}
		offset := random.Intn(len(want) + 1)
		if got := b.Offset(b.Point(offset)); got != offset {
			t.Fatalf("operation %d: offset round trip = %d, want %d", operation, got, offset)
		}
	}
}

func TestUndoRedoTransaction(t *testing.T) {
	b := buffer.New()
	b.BeginTransaction()
	b.Insert(0, []byte("one"))
	b.Insert(3, []byte(" two"))
	b.EndTransaction()

	if !b.Undo() {
		t.Fatal("Undo() = false, want true")
	}
	if got := string(b.Bytes()); got != "" {
		t.Fatalf("Bytes() after undo = %q, want empty", got)
	}
	if !b.Redo() {
		t.Fatal("Redo() = false, want true")
	}
	if got, want := string(b.Bytes()), "one two"; got != want {
		t.Fatalf("Bytes() after redo = %q, want %q", got, want)
	}
}

func TestMergeNextEditGroup(t *testing.T) {
	b := buffer.New()
	b.Insert(0, []byte("h"))
	b.MergeNextEditGroup()
	b.Insert(1, []byte("ello"))

	if !b.Undo() {
		t.Fatal("Undo() = false, want true")
	}
	if got := string(b.Bytes()); got != "" {
		t.Fatalf("after Undo() = %q, want empty", got)
	}
	if b.IsDirty() {
		t.Fatal("buffer is dirty after undoing merged edit group")
	}
	if !b.Redo() {
		t.Fatal("Redo() = false, want true")
	}
	if got := string(b.Bytes()); got != "hello" {
		t.Fatalf("after Redo() = %q, want hello", got)
	}
	if !b.IsDirty() {
		t.Fatal("buffer is clean after redoing merged edit group")
	}
}

func TestUndoRestoresSavedState(t *testing.T) {
	b := buffer.New()
	b.Insert(0, []byte("saved"))
	if err := b.Save(filepath.Join(t.TempDir(), "state.txt")); err != nil {
		t.Fatal(err)
	}
	b.Insert(b.Len(), []byte(" changed"))
	if !b.IsDirty() {
		t.Fatal("edited buffer is not dirty")
	}
	b.Undo()
	if b.IsDirty() {
		t.Fatal("undo to saved state is dirty")
	}
}

func TestHistoryLimitAndRedoBranch(t *testing.T) {
	b := buffer.New()
	b.SetHistoryLimit(2)
	b.Insert(0, []byte("a"))
	b.Insert(1, []byte("b"))
	b.Insert(2, []byte("c"))

	b.Undo()
	b.Undo()
	if b.Undo() {
		t.Fatal("Undo() exceeded the configured history limit")
	}
	if got, want := string(b.Bytes()), "a"; got != want {
		t.Fatalf("Bytes() after limited undo = %q, want %q", got, want)
	}
	b.Insert(1, []byte("x"))
	if b.Redo() {
		t.Fatal("Redo() retained a discarded history branch")
	}
}

func TestUndoDeletion(t *testing.T) {
	b := buffer.New()
	b.Insert(0, []byte("before after"))
	b.Delete(6, 12)
	if !b.Undo() {
		t.Fatal("Undo() = false, want true")
	}
	if got, want := string(b.Bytes()), "before after"; got != want {
		t.Fatalf("Bytes() after undo deletion = %q, want %q", got, want)
	}
}

func TestSaveAndExternalChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	b := buffer.New()
	b.Insert(0, []byte("first"))
	if err := b.Save(path); err != nil {
		t.Fatal(err)
	}
	if b.IsDirty() {
		t.Fatal("saved buffer is dirty")
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "first" {
		t.Fatalf("saved file = %q, %v", got, err)
	}
}
