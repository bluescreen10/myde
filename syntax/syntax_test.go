package syntax_test

import (
	"testing"

	"github.com/bluescreen10/myde/syntax"
)

func TestHighlightGo(t *testing.T) {
	highlighter := syntax.New("main.go")
	spans := highlighter.Highlight(0, `func main() { // hello`)
	if len(spans) != 6 || spans[0].Kind != syntax.Declaration || spans[1].Kind != syntax.Function ||
		spans[2].Kind != syntax.Delimiter || spans[3].Kind != syntax.Delimiter ||
		spans[4].Kind != syntax.Delimiter || spans[5].Kind != syntax.Comment {
		t.Fatalf("Highlight() = %+v", spans)
	}
}

func TestBlockCommentCarriesAcrossLines(t *testing.T) {
	highlighter := syntax.New("main.go")
	highlighter.Highlight(0, "/* open")
	spans := highlighter.Highlight(1, "still */ func")
	if len(spans) != 2 || spans[0].Kind != syntax.Comment || spans[1].Kind != syntax.Declaration {
		t.Fatalf("Highlight() = %+v", spans)
	}
}

func TestHighlightGoSemanticCategories(t *testing.T) {
	tests := []struct {
		line string
		want []syntax.Kind
	}{
		{line: `package syntax`, want: []syntax.Kind{syntax.Declaration}},
		{line: `import "net/http"`, want: []syntax.Kind{syntax.Declaration, syntax.Import}},
		{line: `type server struct {`, want: []syntax.Kind{
			syntax.Declaration, syntax.Type, syntax.Declaration, syntax.Delimiter,
		}},
		{line: `func serve(name string) {`, want: []syntax.Kind{
			syntax.Declaration, syntax.Function, syntax.Delimiter, syntax.Type, syntax.Delimiter, syntax.Delimiter,
		}},
		{line: `fmt.Println("hello")`, want: []syntax.Kind{
			syntax.Function, syntax.Delimiter, syntax.String, syntax.Delimiter,
		}},
	}
	for _, test := range tests {
		highlighter := syntax.New("main.go")
		spans := highlighter.Highlight(0, test.line)
		if len(spans) != len(test.want) {
			t.Errorf("Highlight(%q) = %+v, want kinds %v", test.line, spans, test.want)
			continue
		}
		for index, want := range test.want {
			if spans[index].Kind != want {
				t.Errorf("Highlight(%q)[%d] = %v, want %v", test.line, index, spans[index].Kind, want)
			}
		}
	}
}

func TestHighlightNestedDelimiters(t *testing.T) {
	highlighter := syntax.New("main.go")
	highlighter.Highlight(0, "func run() {")
	spans := highlighter.Highlight(1, "if ok { call() }")
	want := []syntax.Kind{
		syntax.Keyword,
		syntax.Delimiter2,
		syntax.Function,
		syntax.Delimiter3,
		syntax.Delimiter3,
		syntax.Delimiter2,
	}
	if len(spans) != len(want) {
		t.Fatalf("nested delimiters = %+v, want %v", spans, want)
	}
	for index, kind := range want {
		if spans[index].Kind != kind {
			t.Errorf("nested delimiters[%d] = %v, want %v", index, spans[index].Kind, kind)
		}
	}
}

func TestHighlightGoImportBlock(t *testing.T) {
	highlighter := syntax.New("main.go")
	highlighter.Highlight(0, "import (")
	spans := highlighter.Highlight(1, `    "fmt"`)
	if len(spans) != 1 || spans[0].Kind != syntax.Import {
		t.Fatalf("import path = %+v", spans)
	}
	highlighter.Highlight(2, ")")
	spans = highlighter.Highlight(3, `"ordinary string"`)
	if len(spans) != 1 || spans[0].Kind != syntax.String {
		t.Fatalf("ordinary string = %+v", spans)
	}
}

func TestHighlightRemembersDeclaredTypesUntilInvalidated(t *testing.T) {
	highlighter := syntax.New("main.go")
	highlighter.Highlight(0, "type server struct {}")
	spans := highlighter.Highlight(1, "var current server")
	if len(spans) != 2 || spans[1].Kind != syntax.Type {
		t.Fatalf("declared type reference = %+v", spans)
	}

	highlighter.Invalidate(0)
	highlighter.Highlight(0, "var server = 1")
	spans = highlighter.Highlight(1, "var current server")
	if len(spans) != 1 || spans[0].Kind != syntax.Declaration {
		t.Fatalf("invalidated type reference = %+v", spans)
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
