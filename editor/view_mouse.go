package editor

import (
	"slices"
	"strings"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

// Use the same geometry for painting, mouse input, and selection overlays.
func (a *App) viewBounds(view *viewPanel, statusRow int) []viewPaneBounds {
	width, _ := a.screen.Size()
	left := a.sidebarWidth(width)
	return layoutViewPanes(view, viewPaneBounds{left: left, top: 1, width: width - left, height: statusRow - 1})
}

func (a *App) handleViewMouseEvent(view *viewPanel, event terminal.Event) error {
	_, height := a.screen.Size()
	bounds := a.viewBounds(view, height-1)
	index := -1
	// A drag remains in its original pane even when the pointer leaves it.
	for i, pane := range view.panes {
		if pane.selection != nil && pane.selection.dragging {
			index = i
			break
		}
	}
	if index < 0 {
		for i, region := range bounds {
			if event.MouseX > region.left && event.MouseX < region.left+region.width-1 &&
				event.MouseY > region.top && event.MouseY < region.top+region.height-1 {
				index = i
				break
			}
		}
	}
	if index < 0 || index >= len(bounds) {
		return nil
	}
	pane := view.panes[index]
	region := bounds[index]
	rows := max(0, region.height-4)
	if pane.list != nil {
		rows = max(0, rows-pane.list.contentOffset())
	}
	if rows == 0 {
		return nil
	}
	if event.MouseButton == 64 || event.MouseButton == 65 {
		delta := 3
		if event.MouseButton == 64 {
			delta = -delta
		}
		if pane.list != nil {
			before := pane.list.selected
			pane.list.move(delta)
			pane.list.ensureVisible(rows)
			if before != pane.list.selected {
				a.requestViewSelection(view, pane)
			}
		} else {
			pane.top = min(max(0, pane.top+delta), max(0, len(pane.content.Lines)-rows))
		}
		return nil
	}
	if event.MouseButton != 0 {
		return nil
	}
	if pane.list != nil {
		if event.MouseMotion || event.MouseRelease {
			return nil
		}
		view.focus = index
		pane.list.ensureVisible(rows)
		row := event.MouseY - region.top - 1 - pane.list.contentOffset()
		selected := pane.list.top + row
		if row >= 0 && row < rows && selected < len(pane.list.rows) && pane.list.rows[selected].set {
			if pane.list.selected != selected {
				pane.list.selected = selected
				a.requestViewSelection(view, pane)
			}
		}
		return nil
	}
	dragging := pane.selection != nil && pane.selection.dragging
	if (event.MouseMotion || event.MouseRelease) && !dragging {
		return nil
	}
	if !dragging && (event.MouseY < region.top+1 || event.MouseY >= region.top+1+rows) {
		view.focus = index
		return nil
	}
	view.focus = index
	if len(pane.content.Lines) == 0 || pane.err != nil {
		return nil
	}
	pane.top = min(max(0, pane.top), max(0, len(pane.content.Lines)-rows))
	line := min(len(pane.content.Lines)-1, pane.top+min(max(0, event.MouseY-region.top-1), rows-1))
	cell := pane.left + min(max(0, event.MouseX-region.left-1), max(0, region.width-2))
	values := widgetLineText(pane.content.Lines[line])
	column, display := 0, 0
	for _, value := range values {
		width := terminal.RuneWidth(value)
		if value == '\t' {
			width = 4 - display%4
		}
		if display+width > cell {
			break
		}
		display += width
		column++
	}
	point := buffer.Point{Line: line, Column: column}
	anchor := point
	if pane.selection != nil && (dragging || event.Shift) {
		anchor = pane.selection.anchor
	}
	pane.selection = &viewTextSelection{anchor: anchor, point: point, dragging: !event.MouseRelease}
	return nil
}

// Reconstruct widget text without borders, styles, or unused panel padding.
func widgetLineText(line ui.RichTextLine) []rune {
	var values []rune
	column := 0
	for _, span := range line.Spans {
		if span.Positioned {
			target := max(0, span.Column)
			if target < column {
				for len(values) > 0 && sourceDisplayWidth(values) > target {
					values = values[:len(values)-1]
				}
				column = sourceDisplayWidth(values)
			}
			for column < target {
				values = append(values, ' ')
				column++
			}
		}
		for _, value := range span.Text {
			value = safeWidgetRune(value)
			values = append(values, value)
			if value == '\t' {
				column += 4 - column%4
			} else {
				column += terminal.RuneWidth(value)
			}
		}
	}
	return values
}

func sameWidgetText(left, right ui.WidgetContent) bool {
	if len(left.Lines) != len(right.Lines) {
		return false
	}
	for i, line := range left.Lines {
		if !slices.Equal(widgetLineText(line), widgetLineText(right.Lines[i])) {
			return false
		}
	}
	return true
}

func (p *viewPane) selectionRange() (buffer.Point, buffer.Point, bool) {
	if p.selection == nil || p.selection.anchor == p.selection.point {
		return buffer.Point{}, buffer.Point{}, false
	}
	start, end := p.selection.anchor, p.selection.point
	if start.Line > end.Line || start.Line == end.Line && start.Column > end.Column {
		start, end = end, start
	}
	return start, end, true
}

func (p *viewPane) selectedText() string {
	start, end, ok := p.selectionRange()
	if !ok || start.Line < 0 || end.Line >= len(p.content.Lines) {
		return ""
	}
	var text strings.Builder
	for line := start.Line; line <= end.Line; line++ {
		values := widgetLineText(p.content.Lines[line])
		from, to := 0, len(values)
		if line == start.Line {
			from = min(max(0, start.Column), len(values))
		}
		if line == end.Line {
			to = min(max(0, end.Column), len(values))
		}
		if line > start.Line {
			text.WriteByte('\n')
		}
		text.WriteString(string(values[from:to]))
	}
	return text.String()
}

func (a *App) renderViewSelection(pane *viewPane, bounds viewPaneBounds) {
	start, end, ok := pane.selectionRange()
	if !ok {
		return
	}
	style := terminal.Style{Foreground: a.theme.SelectionText, Background: a.theme.Selection}
	rows := max(0, bounds.height-4)
	for line := max(start.Line, pane.top); line <= end.Line && line < pane.top+rows && line < len(pane.content.Lines); line++ {
		values := widgetLineText(pane.content.Lines[line])
		from, to := 0, len(values)
		if line == start.Line {
			from = min(max(0, start.Column), len(values))
		}
		if line == end.Line {
			to = min(max(0, end.Column), len(values))
		}
		left := max(0, sourceDisplayWidth(values[:from])-pane.left)
		right := min(bounds.width-2, sourceDisplayWidth(values[:to])-pane.left)
		for x := left; x < right; x++ {
			a.screen.Restyle(bounds.left+1+x, bounds.top+1+line-pane.top, style)
		}
	}
}
