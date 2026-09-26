package editor

import (
	"io"
	"testing"
	"time"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

func TestViewListLoadsPreviewAndSwitchesPaneFocus(t *testing.T) {
	view := ui.View{
		Title: "Review",
		Layout: ui.Layout{Direction: ui.LayoutHorizontal, Panes: []ui.Pane{
			{ID: "files", List: &ui.List{
				Sections: []ui.ListSection{{Title: "Files", Items: []ui.ListItem{
					{Label: "a.go", Value: "a.go", Kind: "unstaged"},
					{Label: "b.go", Value: "b.go", Kind: "unstaged"},
				}}},
				PreviewPane: "preview",
				OnSelect: func(item ui.ListItem) (ui.Widget, error) {
					return ui.TextWidget{Title: item.Value, Content: []byte(item.Value + "\n")}, nil
				},
			}},
			{ID: "preview"},
		}},
	}
	panel := newViewPanel(1, view)
	app := &App{
		buffers: []*editorBuffer{{text: buffer.NewReadOnly("Review", nil), view: panel}},
		screen:  terminal.NewScreen(io.Discard, 100, 30),
	}

	app.requestViewSelections(panel)
	preview := panel.paneByID("preview")
	if preview.content.Title != "a.go" || len(preview.content.Lines) != 1 ||
		preview.content.Lines[0].Spans[0].Text != "a.go" {
		t.Fatalf("initial preview = %+v", preview.content)
	}
	if err := app.handleViewEvent(terminal.Event{Key: terminal.KeyDown}); err != nil {
		t.Fatal(err)
	}
	if preview.content.Title != "b.go" || preview.content.Lines[0].Spans[0].Text != "b.go" {
		t.Fatalf("selected preview = %+v", preview.content)
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
	definition := func(selected string) ui.View {
		return ui.View{Layout: ui.Layout{Panes: []ui.Pane{{
			ID: "files",
			List: &ui.List{
				Sections: []ui.ListSection{{Items: []ui.ListItem{
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
		modes:          map[string]plugin.Mode{modeKey("Plain Text"): {Name: "Plain Text", Syntax: "plain"}},
		extensionModes: make(map[string]string),
	}
	app.addBuffer(buffer.New())
	handle := app.NewView(ui.View{Title: "Review", Layout: ui.Layout{Panes: []ui.Pane{{ID: "one"}}}})
	if len(app.buffers) != 2 || app.active != 1 || app.currentView() == nil {
		t.Fatalf("view tab state: buffers=%d active=%d view=%v", len(app.buffers), app.active, app.currentView())
	}
	if !handle.Show() {
		t.Fatal("live view handle could not raise its tab")
	}
	handle.Destroy()
	shown := handle.Show()
	if len(app.buffers) != 1 || shown {
		t.Fatalf("destroyed view state: buffers=%d show=%v", len(app.buffers), shown)
	}
}

func TestViewIgnoresResponsesFromReplacedPanes(t *testing.T) {
	old := newViewPanel(1, ui.View{Layout: ui.Layout{Panes: []ui.Pane{
		{ID: "preview"},
	}}})
	oldPane := old.panes[0]
	oldPane.generation = 1
	replacement := newViewPanel(1, ui.View{Layout: ui.Layout{Panes: []ui.Pane{
		{ID: "preview"},
	}}})
	replacement.panes[0].generation = 1
	app := &App{buffers: []*editorBuffer{{text: buffer.NewReadOnly("Review", nil), view: replacement}}}

	app.applyViewPreviewEvent(&viewPreviewEvent{
		viewID: 1, paneID: "preview", pane: oldPane, generation: 1,
		widget: ui.TextWidget{Content: []byte("stale")},
	})
	if len(replacement.panes[0].content.Lines) != 0 {
		t.Fatalf("stale preview replaced current content: %+v", replacement.panes[0].content)
	}

	app.applyViewRefreshEvent(&viewRefreshEvent{
		viewID: 1, view: old,
		updated: ui.View{Title: "Stale", Layout: ui.Layout{Panes: []ui.Pane{{ID: "preview"}}}},
	})
	if app.currentView() != replacement || app.currentView().title == "Stale" {
		t.Fatal("stale refresh replaced the current view")
	}
}

func TestViewClearsPreviewWhenListBecomesEmpty(t *testing.T) {
	panel := newViewPanel(1, ui.View{Layout: ui.Layout{Panes: []ui.Pane{
		{ID: "files", List: &ui.List{
			Sections:    []ui.ListSection{{Title: "Files"}},
			PreviewPane: "preview",
			OnSelect: func(ui.ListItem) (ui.Widget, error) {
				return ui.TextWidget{}, nil
			},
		}},
		{ID: "preview", Widget: ui.TextWidget{Content: []byte("old")}},
	}}})
	app := &App{buffers: []*editorBuffer{{text: buffer.NewReadOnly("Review", nil), view: panel}}}
	app.requestViewSelections(panel)
	preview := panel.paneByID("preview")
	if preview.widget != nil || len(preview.content.Lines) != 0 {
		t.Fatalf("empty list retained preview %+v", preview.content)
	}
}

func TestViewBackgroundRefreshPreservesSelection(t *testing.T) {
	refreshes := 0
	view := ui.View{
		Layout: ui.Layout{Panes: []ui.Pane{{ID: "files", List: &ui.List{
			Sections:      []ui.ListSection{{Items: []ui.ListItem{{Value: "a"}, {Value: "b"}}}},
			SelectedValue: "b",
		}}}},
		RefreshInterval: time.Millisecond,
		OnRefresh: func() (ui.View, error) {
			refreshes++
			return ui.View{
				Layout: ui.Layout{Panes: []ui.Pane{{ID: "files", List: &ui.List{
					Sections: []ui.ListSection{{Items: []ui.ListItem{{Value: "b"}, {Value: "c"}}}},
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

func TestRichTextWidgetPlacementAndSemanticStyle(t *testing.T) {
	content := ui.WidgetContent{Lines: []ui.RichTextLine{{Spans: []ui.RichTextSpan{
		{Text: "ab"},
		{Text: "x", Positioned: true, Column: 8},
		{Text: "\tZ", Style: ui.WidgetStyle{Foreground: ui.ToneAccent}},
	}}}}
	panel := newViewPanel(1, ui.View{Layout: ui.Layout{Panes: []ui.Pane{{
		ID: "rich", Widget: content,
	}}}})
	if width := panel.panes[0].widgetWidth(); width != 13 {
		t.Fatalf("rich-text width = %d, want 13", width)
	}
	if got := safeWidgetRune('\x1b'); got != '�' {
		t.Fatalf("escape rune rendered as %q", got)
	}

	app := &App{theme: Theme{
		Foreground: terminal.Color{R: 200, G: 200, B: 200},
		Panel:      terminal.Color{R: 10, G: 20, B: 30},
		Danger:     terminal.Color{R: 210, G: 20, B: 30},
	}}
	resolved := app.resolveWidgetStyle(terminal.Style{
		Foreground: app.theme.Foreground, Background: app.theme.Panel,
	}, ui.WidgetStyle{Background: ui.ToneDanger, BackgroundIntensity: 50})
	want := blendColor(app.theme.Panel, app.theme.Danger, 50)
	if resolved.Background != want {
		t.Fatalf("semantic background = %+v, want %+v", resolved.Background, want)
	}
}
