package editor

import (
	"strconv"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/terminal"
)

func (a *App) mouseEditorRegion() editorRegion {
	width, height := a.screen.Size()
	left := a.sidebarWidth(width)
	return editorRegion{x: left, y: 1, width: width - left, height: height - 2}
}

func (a *App) handleMouseEvent(event terminal.Event) error {
	if a.minibuffer != nil || a.palette != nil {
		return nil
	}
	if view := a.currentView(); view != nil {
		a.mouseSelecting = false
		return a.handleViewMouseEvent(view, event)
	}
	region := a.mouseEditorRegion()
	if !a.mouseSelecting {
		if !region.contains(event.MouseX, event.MouseY) {
			return nil
		}
		if a.tileRoot != nil {
			leaves := make([]*editorTile, 0, 4)
			a.tileRoot.leaves(&leaves)
			for _, tile := range leaves {
				bounds, ok := a.editorTileRegion(tile, region)
				if ok && bounds.contains(event.MouseX, event.MouseY) {
					a.focusEditorTile(tile)
					break
				}
			}
		}
	}
	if a.tileRoot != nil {
		if bounds, ok := a.editorTileRegion(a.focusedTile, region); ok {
			region = bounds
		}
	}
	if event.MouseButton == 64 || event.MouseButton == 65 {
		delta := 3
		if event.MouseButton == 64 {
			delta = -delta
		}
		a.topLine = min(max(0, a.topLine+delta), max(0, a.current().LineCount()-region.height))
		a.syncFocusedTile()
		return nil
	}
	if event.MouseButton != 0 {
		return nil
	}
	if (event.MouseMotion || event.MouseRelease) && !a.mouseSelecting {
		return nil
	}
	if a.currentEditorBuffer().terminal != nil {
		return nil
	}
	if a.browser != nil {
		a.browser.focused = false
	}
	current := a.current()
	gutter := len(strconv.Itoa(max(1, current.LineCount()))) + 2
	line := min(current.LineCount()-1, a.topLine+min(max(0, event.MouseY-region.y), max(0, region.height-1)))
	cell := max(0, event.MouseX-region.x-gutter+a.leftColumn)
	column, display := 0, 0
	for _, value := range []rune(string(current.Line(line))) {
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
	if a.mouseSelecting || event.Shift {
		anchor = current.Cursors()[0].Anchor
	}
	current.SetCursors([]buffer.Cursor{{Anchor: anchor, Point: point}})
	a.mouseSelecting = !event.MouseRelease
	a.showDiagnostic = false
	if event.MouseRelease && a.sidebar != nil && a.sidebar.input != nil {
		a.CloseSidebar()
	}
	return nil
}

func (r editorRegion) contains(x, y int) bool {
	return x >= r.x && x < r.x+r.width && y >= r.y && y < r.y+r.height
}
