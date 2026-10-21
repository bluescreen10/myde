package editor_test

import (
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

func mouseStage(t *testing.T) (*harness, ui.ViewHandle, ui.View) {
	h := newHarness(t, "")
	items := make([]ui.ListItem, 12)
	for i := range items {
		items[i] = ui.ListItem{Label: fmt.Sprintf("file%d.go", i), Value: fmt.Sprintf("file%d.go", i)}
	}
	def := ui.View{Title: "Git Stage", Layout: ui.Layout{Panes: []ui.Pane{
		{ID: "changes", Weight: 1, List: &ui.List{Sections: []ui.ListSection{{Title: "Unstaged", Items: items}}, PreviewPane: "diff", OnSelect: func(item ui.ListItem) (ui.Widget, error) {
			content := ui.WidgetContent{Title: "preview-" + item.Value, Lines: []ui.RichTextLine{{Spans: []ui.RichTextSpan{{Text: "+\t界", Style: ui.WidgetStyle{Background: ui.ToneSuccess}}, {Text: "abc", Style: ui.WidgetStyle{Background: ui.ToneWarning}}}}, {Spans: []ui.RichTextSpan{{Text: "-\tother"}}}}}
			for i := range 30 {
				content.Lines = append(content.Lines, ui.RichTextLine{Spans: []ui.RichTextSpan{{Text: fmt.Sprintf("line %d", i)}}})
			}
			return content, nil
		}}}, {ID: "diff", Weight: 2},
	}}}
	handle := h.app.NewView(def)
	h.wait("stage preview", func() bool { return h.contains("preview-file0.go") })
	return h, handle, def
}

func TestStageMouseSelectsFilesAndScrollsHoveredPane(t *testing.T) {
	for _, sidebar := range []bool{false, true} {
		t.Run(fmt.Sprint(sidebar), func(t *testing.T) {
			h, _, _ := mouseStage(t)
			listX, diffX := 2, 35
			if sidebar {
				h.app.OpenSidebar(ui.Sidebar{Width: 72, Input: &ui.TextInput{}})
				listX, diffX = 52, 69
			}
			h.mouse(listX, 4, 0, false, false)
			h.mouse(listX, 4, 0, false, true)
			h.wait("clicked file", func() bool { return strings.Contains(h.screen.row(1), "preview-file1.go") })
			h.mouse(listX, 4, 65, false, false)
			h.wait("wheel-selected file", func() bool { return strings.Contains(h.screen.row(1), "preview-file4.go") })
			h.mouse(diffX, 2, 0, false, false)
			h.mouse(diffX, 2, 0, false, true)
			h.mouse(diffX, 2, 65, false, false)
			if !strings.Contains(h.screen.row(2), "line 1") {
				t.Fatalf("diff wheel row=%q", h.screen.row(2))
			}
			h.mouse(diffX, 2, 64, false, false)
			if !strings.Contains(h.screen.row(2), "界abc") {
				t.Fatalf("diff wheel up=%q", h.screen.row(2))
			}
		})
	}
}

func TestStageMouseSelectsAndHighlightsDiffForCopy(t *testing.T) {
	h, _, _ := mouseStage(t)
	h.mouse(37, 2, 0, false, false)
	h.mouse(41, 2, 0, true, false)
	h.mouse(41, 2, 0, false, true)
	if h.selection() != "界ab" {
		t.Fatalf("selection=%q", h.selection())
	}
	if !strings.Contains(h.screen.raw.String(), "48;2;79;168;255m界ab") {
		t.Fatal("selection was not highlighted")
	}
	h.mouse(41, 3, 0, false, false)
	h.mouse(37, 2, 0, true, false)
	h.mouse(37, 2, 0, false, true)
	if h.selection() != "界abc\n-\tothe" {
		t.Fatalf("multi-line selection=%q", h.selection())
	}
	h.key(terminal.Event{Key: terminal.KeyEscape})
	if h.selection() != "" {
		t.Fatal("Escape did not clear selection")
	}
}

func TestStageSelectionSurvivesUnchangedRefreshAndClearsOnChange(t *testing.T) {
	h, handle, def := mouseStage(t)
	h.mouse(37, 2, 0, false, false)
	h.mouse(41, 2, 0, false, true)
	if !handle.Update(def) {
		t.Fatal("view update failed")
	}
	h.wait("preserved selection", func() bool { return h.selection() == "界ab" })
	var changed atomic.Bool
	def.Layout.Panes[0].List.OnSelect = func(item ui.ListItem) (ui.Widget, error) {
		changed.Store(true)
		return ui.TextWidget{Content: []byte("CHANGED_DIFF")}, nil
	}
	handle.Update(def)
	h.wait("changed preview", func() bool { return changed.Load() && h.contains("CHANGED_DIFF") })
	if h.selection() != "" {
		t.Fatal("changed preview kept stale selection")
	}
}

func TestStageSelectionRespectsHorizontalScroll(t *testing.T) {
	h := newHarness(t, "")
	h.app.NewView(ui.View{Title: "Wide", Layout: ui.Layout{Panes: []ui.Pane{{ID: "text", Widget: ui.TextWidget{Content: []byte(strings.Repeat("x", 100) + "界abc")}}}}})
	h.draw()
	for range 110 {
		h.key(terminal.Event{Key: terminal.KeyRight})
	}
	h.mouse(94, 2, 0, false, false)
	h.mouse(98, 2, 0, false, true)
	if h.selection() != "界ab" {
		t.Fatalf("scrolled selection=%q", h.selection())
	}
}
