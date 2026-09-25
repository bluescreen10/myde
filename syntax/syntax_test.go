package syntax_test

import (
	"testing"

	"github.com/bluescreen10/myde/syntax"
)

func TestHighlightGo(t *testing.T) {
	highlighter := syntax.New("main.go")
	spans := highlighter.Highlight(0, `func main() { // hello`)
	if len(spans) != 2 || spans[0].Kind != syntax.Keyword || spans[1].Kind != syntax.Comment {
		t.Fatalf("Highlight() = %+v", spans)
	}
}

func TestBlockCommentCarriesAcrossLines(t *testing.T) {
	highlighter := syntax.New("main.go")
	highlighter.Highlight(0, "/* open")
	spans := highlighter.Highlight(1, "still */ func")
	if len(spans) != 2 || spans[0].Kind != syntax.Comment || spans[1].Kind != syntax.Keyword {
		t.Fatalf("Highlight() = %+v", spans)
	}
}

func TestDiffHighlighting(t *testing.T) {
	highlighter := syntax.New("changes.diff")
	tests := []struct {
		line string
		kind syntax.Kind
	}{
		{line: "+added", kind: syntax.Added},
		{line: "-removed", kind: syntax.Removed},
		{line: "@@ -1 +1 @@", kind: syntax.Keyword},
	}
	for line, test := range tests {
		spans := highlighter.Highlight(line, test.line)
		if len(spans) != 1 || spans[0].Kind != test.kind {
			t.Errorf("Highlight(%q) = %+v, want kind %v", test.line, spans, test.kind)
		}
	}
}
