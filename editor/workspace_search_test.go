package editor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/bluescreen10/myde/terminal"
)

func TestParseRipgrepSearchResultsAndGroupRows(t *testing.T) {
	output := []byte(
		`{"type":"begin","data":{"path":{"text":"folder/a.go"}}}` + "\n" +
			`{"type":"match","data":{"path":{"text":"./folder/a.go"},"lines":{"text":"alpha needle omega\n"},"line_number":7,"submatches":[{"match":{"text":"needle"},"start":6,"end":12}]}}` + "\n" +
			`{"type":"match","data":{"path":{"text":"./folder/a.go"},"lines":{"text":"needle again\n"},"line_number":9,"submatches":[{"match":{"text":"needle"},"start":0,"end":6}]}}` + "\n" +
			`{"type":"match","data":{"path":{"text":"./b.txt"},"lines":{"text":"last needle\n"},"line_number":2,"submatches":[{"match":{"text":"needle"},"start":5,"end":11}]}}` + "\n",
	)
	results := parseRipgrepSearchResults(output)
	if len(results) != 3 {
		t.Fatalf("results = %+v", results)
	}
	if results[0].path != filepath.Join("folder", "a.go") || results[0].line != 6 ||
		results[0].byteColumn != 6 || results[0].text != "alpha needle omega" {
		t.Fatalf("first result = %+v", results[0])
	}
	panel := &workspaceSearchPanel{results: results, selected: 2}
	rows := panel.rows()
	if len(rows) != 5 || rows[0].path != filepath.Join("folder", "a.go") ||
		!rows[1].match || rows[3].path != "b.txt" || rows[4].resultIndex != 2 {
		t.Fatalf("grouped rows = %+v", rows)
	}
	panel.ensureVisible(rows, 2)
	if panel.top != 3 {
		t.Fatalf("panel top = %d, want 3", panel.top)
	}
}

func TestWorkspaceSearchFallbackIsRecursiveAndSmartCase(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("first Needle\nsecond\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "b.txt"), []byte("another needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "binary"), []byte{'n', 0, 'x'}, 0o644); err != nil {
		t.Fatal(err)
	}
	files := []string{"a.txt", filepath.Join("nested", "b.txt"), "binary"}
	results, err := searchWorkspaceFiles(context.Background(), root, files, "needle")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 || results[0].path != "a.txt" || results[1].path != filepath.Join("nested", "b.txt") {
		t.Fatalf("case-insensitive results = %+v", results)
	}
	results, err = searchWorkspaceFiles(context.Background(), root, files, "Needle")
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].path != "a.txt" {
		t.Fatalf("case-sensitive results = %+v", results)
	}
}

func TestWorkspaceSearchKeyAndTrailingQuery(t *testing.T) {
	if got := keyName(terminal.Event{Key: terminal.KeyRune, Rune: 'f', Control: true, Shift: true}); got != "ctrl-shift-f" {
		t.Fatalf("key name = %q", got)
	}
	if got := trailingDisplayText([]rune("abcdefgh"), 4); got != "efgh" {
		t.Fatalf("trailing query = %q", got)
	}
}
