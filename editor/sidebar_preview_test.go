package editor

import (
	"io"
	"testing"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/terminal"
)

func TestFullScreenSidebarLoadsPreviewAndTogglesFocus(t *testing.T) {
	panel := newSidebarPanel(plugin.Sidebar{
		Title:      "Review",
		FullScreen: true,
		Sections: []plugin.SidebarSection{{Title: "Files", Items: []plugin.SidebarItem{
			{Label: "a.go", Value: "a.go"},
			{Label: "b.go", Value: "b.go"},
		}}},
		OnPreview: func(item plugin.SidebarItem) (plugin.SidebarPreview, error) {
			return plugin.SidebarPreview{Title: item.Value, Content: []byte("one\ntwo\nthree\nfour\n"), Syntax: "diff"}, nil
		},
	})
	app := &App{sidebar: panel, screen: terminal.NewScreen(io.Discard, 100, 8)}
	app.requestSidebarPreview()
	if panel.preview.Title != "a.go" || len(panel.previewLines) != 4 {
		t.Fatalf("initial preview = %+v, lines = %d", panel.preview, len(panel.previewLines))
	}
	if err := app.handleSidebarEvent(terminal.Event{Key: terminal.KeyDown}); err != nil {
		t.Fatal(err)
	}
	if selected, _ := panel.selectedItem(); selected.Value != "b.go" || panel.preview.Title != "b.go" {
		t.Fatalf("selection/preview = %+v / %+v", selected, panel.preview)
	}
	if err := app.handleSidebarEvent(terminal.Event{Key: terminal.KeyEnter}); err != nil {
		t.Fatal(err)
	}
	if !panel.previewFocused {
		t.Fatal("Enter did not focus the preview")
	}
	if err := app.handleSidebarEvent(terminal.Event{Key: terminal.KeyDown}); err != nil {
		t.Fatal(err)
	}
	if panel.previewTop != 1 {
		t.Fatalf("preview top = %d, want 1", panel.previewTop)
	}
	if err := app.handleSidebarEvent(terminal.Event{Key: terminal.KeyTab}); err != nil {
		t.Fatal(err)
	}
	if panel.previewFocused {
		t.Fatal("Tab did not return focus to the changes pane")
	}
}

func TestDiffPreviewHighlightsLinesAndChangedCharacters(t *testing.T) {
	lines := buildSidebarPreviewLines(plugin.SidebarPreview{
		Syntax:  "diff",
		Content: []byte("--- a/file.go\n+++ b/file.go\n@@ -1 +1 @@\n-hello world\n+hello there\n"),
	})
	if len(lines) != 5 || lines[0].kind != previewMeta || lines[1].kind != previewMeta ||
		lines[2].kind != previewHunk || lines[3].kind != previewRemoved || lines[4].kind != previewAdded {
		t.Fatalf("diff line kinds = %+v", lines)
	}
	if lines[3].changeStart != 7 || lines[3].changeEnd != 12 ||
		lines[4].changeStart != 7 || lines[4].changeEnd != 12 {
		t.Fatalf("changed ranges = removed %d:%d, added %d:%d",
			lines[3].changeStart, lines[3].changeEnd, lines[4].changeStart, lines[4].changeEnd)
	}
	base := terminal.Color{R: 10, G: 20, B: 30}
	tint := terminal.Color{R: 110, G: 120, B: 130}
	if got := blendColor(base, tint, 25); got != (terminal.Color{R: 35, G: 45, B: 55}) {
		t.Fatalf("blended color = %+v", got)
	}
}
