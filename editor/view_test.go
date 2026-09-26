package editor

import (
	"io"
	"testing"
	"time"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/terminal"
)

func TestViewListLoadsPreviewAndSwitchesPaneFocus(t *testing.T) {
	view := plugin.View{
		Title: "Review",
		Layout: plugin.Layout{Direction: plugin.LayoutHorizontal, Panes: []plugin.Pane{
			{ID: "files", List: &plugin.List{
				Sections: []plugin.ListSection{{Title: "Files", Items: []plugin.ListItem{
					{Label: "a.go", Value: "a.go", Kind: "unstaged"},
					{Label: "b.go", Value: "b.go", Kind: "unstaged"},
				}}},
				PreviewPane: "preview",
				OnSelect: func(item plugin.ListItem) (plugin.ViewDocument, error) {
					return plugin.ViewDocument{Title: item.Value, Content: []byte(item.Value + "\n"), Syntax: "diff"}, nil
				},
			}},
			{ID: "preview", Document: &plugin.ViewDocument{Syntax: "diff"}},
		}},
	}
	panel := newViewPanel(1, view)
	app := &App{
		buffers: []*editorBuffer{{text: buffer.NewReadOnly("Review", nil), view: panel}},
		screen:  terminal.NewScreen(io.Discard, 100, 30),
	}

	app.requestViewSelections(panel)
	preview := panel.paneByID("preview")
	if preview.document.Title != "a.go" || len(preview.lines) != 1 || preview.lines[0].text != "a.go" {
		t.Fatalf("initial preview = %+v, lines = %+v", preview.document, preview.lines)
	}
	if err := app.handleViewEvent(terminal.Event{Key: terminal.KeyDown}); err != nil {
		t.Fatal(err)
	}
	if preview.document.Title != "b.go" || preview.lines[0].text != "b.go" {
		t.Fatalf("selected preview = %+v, lines = %+v", preview.document, preview.lines)
	}
	if err := app.handleViewEvent(terminal.Event{Key: terminal.KeyEnter}); err != nil {
		t.Fatal(err)
	}
	if panel.focus != 1 {
		t.Fatalf("Enter focused pane %d, want preview pane 1", panel.focus)
	}
	if err := app.handleViewEvent(terminal.Event{Key: terminal.KeyTab}); err != nil {
		t.Fatal(err)
	}
	if panel.focus != 0 {
		t.Fatalf("Tab focused pane %d, want list pane 0", panel.focus)
	}
}

func TestViewUpdateHonorsExplicitSelectionAndPreservesImplicitSelection(t *testing.T) {
	definition := func(selected string) plugin.View {
		return plugin.View{Layout: plugin.Layout{Panes: []plugin.Pane{{
			ID: "files",
			List: &plugin.List{
				Sections: []plugin.ListSection{{Items: []plugin.ListItem{
					{Value: "a", Kind: "unstaged"},
					{Value: "b", Kind: "unstaged"},
					{Value: "c", Kind: "unstaged"},
				}}},
				SelectedValue: selected,
				SelectedKind:  "unstaged",
			},
		}}}}
	}
	previous := newViewPanel(1, definition("a"))
	replacement := newViewPanel(1, definition("b"))
	preserveViewState(previous, replacement)
	selected, _ := replacement.panes[0].list.selectedItem()
	if selected.Value != "b" {
		t.Fatalf("explicit replacement selection = %q, want b", selected.Value)
	}

	previous.panes[0].list.selectItem("unstaged", "c")
	replacement = newViewPanel(1, definition(""))
	preserveViewState(previous, replacement)
	selected, _ = replacement.panes[0].list.selectedItem()
	if selected.Value != "c" {
		t.Fatalf("preserved replacement selection = %q, want c", selected.Value)
	}
}

func TestViewHandleUsesBufferTabLifecycle(t *testing.T) {
	app := &App{
		screen:         terminal.NewScreen(io.Discard, 80, 24),
		browser:        newFileBrowser("", nil, nil),
		modes:          map[string]plugin.Mode{modeKey("Plain Text"): {Name: "Plain Text", Syntax: "plain"}},
		extensionModes: make(map[string]string),
	}
	app.addBuffer(buffer.New())
	handle := app.NewView(plugin.View{Title: "Review", Layout: plugin.Layout{Panes: []plugin.Pane{{ID: "one"}}}})
	if len(app.buffers) != 2 || app.active != 1 || app.currentView() == nil {
		t.Fatalf("view tab state: buffers=%d active=%d view=%v", len(app.buffers), app.active, app.currentView())
	}
	if !handle.Show() {
		t.Fatal("live view handle could not raise its tab")
	}
	handle.Destroy()
	if len(app.buffers) != 1 || handle.Show() {
		t.Fatalf("destroyed view state: buffers=%d show=%v", len(app.buffers), handle.Show())
	}
}

func TestViewBackgroundRefreshPreservesSelection(t *testing.T) {
	refreshes := 0
	view := plugin.View{
		Layout: plugin.Layout{Panes: []plugin.Pane{{ID: "files", List: &plugin.List{
			Sections:      []plugin.ListSection{{Items: []plugin.ListItem{{Value: "a"}, {Value: "b"}}}},
			SelectedValue: "b",
		}}}},
		RefreshInterval: time.Millisecond,
		OnRefresh: func() (plugin.View, error) {
			refreshes++
			return plugin.View{
				Layout: plugin.Layout{Panes: []plugin.Pane{{ID: "files", List: &plugin.List{
					Sections: []plugin.ListSection{{Items: []plugin.ListItem{{Value: "b"}, {Value: "c"}}}},
				}}}},
				RefreshInterval: time.Millisecond,
			}, nil
		},
	}
	panel := newViewPanel(1, view)
	panel.nextRefresh = time.Time{}
	app := &App{
		buffers: []*editorBuffer{{text: buffer.NewReadOnly("Review", nil), view: panel}},
		servers: make(chan serverEvent, 1),
	}
	app.pollViewRefreshes()
	select {
	case event := <-app.servers:
		app.handleServerEvent(event)
	case <-time.After(time.Second):
		t.Fatal("view refresh did not complete")
	}
	selected, ok := app.currentView().panes[0].list.selectedItem()
	if refreshes != 1 || !ok || selected.Value != "b" {
		t.Fatalf("refreshes=%d selected=%+v ok=%v", refreshes, selected, ok)
	}
}

func TestViewDiffLinesMarkChangedCharacters(t *testing.T) {
	lines := buildViewDocumentLines(plugin.ViewDocument{
		Syntax:  "diff",
		Content: []byte("@@ -1 +1 @@\n-old value\n+new value\n"),
	})
	if len(lines) != 3 || lines[0].kind != viewLineHunk || lines[1].kind != viewLineRemoved || lines[2].kind != viewLineAdded {
		t.Fatalf("diff lines = %+v", lines)
	}
	if lines[1].changeStart == lines[1].changeEnd || lines[2].changeStart == lines[2].changeEnd {
		t.Fatalf("changed character ranges were not marked: %+v", lines)
	}
}
