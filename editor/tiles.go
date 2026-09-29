package editor

import (
	"fmt"

	"github.com/bluescreen10/myde/terminal"
)

// splitDirection describes the divider introduced by a split. A horizontal
// divider produces top and bottom panes; a vertical divider produces left and
// right panes.
type splitDirection uint8

const (
	splitHorizontally splitDirection = iota + 1
	splitVertically
)

// editorTile is either a leaf with a buffer and viewport, or a branch with two
// children. Tiling exists only while tileRoot is a branch; a single remaining
// leaf is folded back into the normal, unsplit editor state.
type editorTile struct {
	parent     *editorTile
	direction  splitDirection
	first      *editorTile
	second     *editorTile
	buffer     *editorBuffer
	topLine    int
	leftColumn int
}

func (t *editorTile) isLeaf() bool {
	return t != nil && t.first == nil && t.second == nil
}

func (t *editorTile) leaves(result *[]*editorTile) {
	if t == nil {
		return
	}
	if t.isLeaf() {
		*result = append(*result, t)
		return
	}
	t.first.leaves(result)
	t.second.leaves(result)
}

func (t *editorTile) contains(target *editorTile) bool {
	if t == nil || target == nil {
		return false
	}
	if t == target {
		return true
	}
	return t.first.contains(target) || t.second.contains(target)
}

func (a *App) activateBufferIndex(index int, resetViewport bool) {
	if index < 0 || index >= len(a.buffers) {
		return
	}
	// Generic plugin views currently own the whole editor canvas. Raising one
	// therefore leaves tiled text-buffer mode cleanly.
	if a.buffers[index].view != nil && a.tileRoot != nil {
		a.unsplit()
	}
	a.active = index
	if resetViewport {
		a.topLine = 0
		a.leftColumn = 0
	}
	if a.focusedTile != nil {
		a.focusedTile.buffer = a.buffers[index]
		a.focusedTile.topLine = a.topLine
		a.focusedTile.leftColumn = a.leftColumn
	}
}

func (a *App) syncFocusedTile() {
	if a.focusedTile == nil || a.focusedTile.buffer == nil || len(a.buffers) == 0 {
		return
	}
	a.focusedTile.buffer = a.currentEditorBuffer()
	a.focusedTile.topLine = a.topLine
	a.focusedTile.leftColumn = a.leftColumn
}

func (a *App) focusEditorTile(tile *editorTile) {
	if tile == nil || !tile.isLeaf() || tile.buffer == nil {
		return
	}
	a.syncFocusedTile()
	index := a.editorBufferIndex(tile.buffer)
	if index < 0 {
		return
	}
	a.focusedTile = tile
	a.active = index
	a.topLine = tile.topLine
	a.leftColumn = tile.leftColumn
	a.activateCurrentMode()
	if a.screen != nil {
		a.ensureCursorVisible()
	}
}

func (a *App) editorBufferIndex(target *editorBuffer) int {
	for index, candidate := range a.buffers {
		if candidate == target {
			return index
		}
	}
	return -1
}

func (a *App) replaceEditorBufferInTiles(previous, replacement *editorBuffer) {
	if a.tileRoot == nil || previous == nil || replacement == nil {
		return
	}
	leaves := make([]*editorTile, 0, 4)
	a.tileRoot.leaves(&leaves)
	for _, tile := range leaves {
		if tile.buffer == previous {
			tile.buffer = replacement
		}
	}
}

func (a *App) splitHorizontally(arguments string) error {
	return a.splitFocusedTile(splitHorizontally)
}

func (a *App) splitVertically(arguments string) error {
	return a.splitFocusedTile(splitVertically)
}

func (a *App) splitFocusedTile(direction splitDirection) error {
	if a.currentView() != nil {
		return fmt.Errorf("plugin views cannot be split")
	}
	if a.tileRoot == nil {
		a.tileRoot = &editorTile{
			buffer: a.currentEditorBuffer(), topLine: a.topLine, leftColumn: a.leftColumn,
		}
		a.focusedTile = a.tileRoot
	}
	leaf := a.focusedTile
	if leaf == nil || !leaf.isLeaf() {
		return fmt.Errorf("no editor pane is focused")
	}
	first := &editorTile{
		parent: leaf, buffer: leaf.buffer, topLine: leaf.topLine, leftColumn: leaf.leftColumn,
	}
	second := &editorTile{
		parent: leaf, buffer: leaf.buffer, topLine: leaf.topLine, leftColumn: leaf.leftColumn,
	}
	leaf.direction = direction
	leaf.buffer = nil
	leaf.first = first
	leaf.second = second
	a.focusedTile = second
	a.topLine = second.topLine
	a.leftColumn = second.leftColumn
	return nil
}

func (a *App) nextView(arguments string) error {
	return a.moveViewFocus(1)
}

func (a *App) previousView(arguments string) error {
	return a.moveViewFocus(-1)
}

func (a *App) moveViewFocus(delta int) error {
	if a.tileRoot == nil {
		return fmt.Errorf("view is not split")
	}
	leaves := make([]*editorTile, 0, 4)
	a.tileRoot.leaves(&leaves)
	if len(leaves) < 2 {
		return fmt.Errorf("view is not split")
	}
	index := 0
	for current, tile := range leaves {
		if tile == a.focusedTile {
			index = current
			break
		}
	}
	index = (index + delta + len(leaves)) % len(leaves)
	a.focusEditorTile(leaves[index])
	return nil
}

func (a *App) closeView(arguments string) error {
	if a.tileRoot == nil || a.focusedTile == nil {
		return fmt.Errorf("view is not split")
	}
	a.syncFocusedTile()
	closing := a.focusedTile
	parent := closing.parent
	if parent == nil {
		return fmt.Errorf("view is not split")
	}
	sibling := parent.first
	if sibling == closing {
		sibling = parent.second
	}
	grandparent := parent.parent
	if grandparent == nil {
		sibling.parent = nil
		a.tileRoot = sibling
	} else if grandparent.first == parent {
		grandparent.first = sibling
		sibling.parent = grandparent
	} else {
		grandparent.second = sibling
		sibling.parent = grandparent
	}
	leaf := firstEditorTile(sibling)
	if a.tileRoot.isLeaf() {
		a.tileRoot = nil
		a.focusedTile = nil
		if index := a.editorBufferIndex(leaf.buffer); index >= 0 {
			a.active = index
			a.topLine = leaf.topLine
			a.leftColumn = leaf.leftColumn
			a.activateCurrentMode()
		}
		return nil
	}
	a.focusEditorTile(leaf)
	return nil
}

func firstEditorTile(tile *editorTile) *editorTile {
	for tile != nil && !tile.isLeaf() {
		tile = tile.first
	}
	return tile
}

func (a *App) unsplit() {
	if a.tileRoot == nil {
		return
	}
	a.syncFocusedTile()
	if a.focusedTile != nil {
		if index := a.editorBufferIndex(a.focusedTile.buffer); index >= 0 {
			a.active = index
			a.topLine = a.focusedTile.topLine
			a.leftColumn = a.focusedTile.leftColumn
		}
	}
	a.tileRoot = nil
	a.focusedTile = nil
}

// removeBufferFromTiles removes every pane displaying a buffer that is being
// destroyed. Unary branches collapse, and a lone survivor becomes the normal
// full-screen editor again.
func (a *App) removeBufferFromTiles(removed *editorBuffer) bool {
	if a.tileRoot == nil {
		return false
	}
	root := pruneEditorTiles(a.tileRoot, removed)
	if root == nil {
		a.tileRoot = nil
		a.focusedTile = nil
		return false
	}
	root.parent = nil
	if root.isLeaf() {
		a.tileRoot = nil
		a.focusedTile = nil
		if index := a.editorBufferIndex(root.buffer); index >= 0 {
			a.active = index
			a.topLine = root.topLine
			a.leftColumn = root.leftColumn
		}
		return true
	}
	a.tileRoot = root
	if !root.contains(a.focusedTile) {
		a.focusedTile = firstEditorTile(root)
	}
	if a.focusedTile != nil {
		if index := a.editorBufferIndex(a.focusedTile.buffer); index >= 0 {
			a.active = index
			a.topLine = a.focusedTile.topLine
			a.leftColumn = a.focusedTile.leftColumn
		}
	}
	return a.focusedTile != nil
}

func pruneEditorTiles(tile *editorTile, removed *editorBuffer) *editorTile {
	if tile == nil {
		return nil
	}
	if tile.isLeaf() {
		if tile.buffer == removed {
			return nil
		}
		return tile
	}
	first := pruneEditorTiles(tile.first, removed)
	second := pruneEditorTiles(tile.second, removed)
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	tile.first = first
	tile.second = second
	first.parent = tile
	second.parent = tile
	return tile
}

func (a *App) renderEditorTiles(region editorRegion) {
	a.renderEditorTile(a.tileRoot, region)
}

func (a *App) renderEditorTile(tile *editorTile, region editorRegion) {
	if tile == nil || region.width <= 0 || region.height <= 0 {
		return
	}
	if tile.isLeaf() {
		a.renderBufferRegion(tile.buffer, region, tile.topLine, tile.leftColumn)
		return
	}
	first, second, separator, vertical := splitEditorRegion(region, tile.direction)
	a.renderEditorTile(tile.first, first)
	a.renderEditorTile(tile.second, second)
	style := terminal.Style{Foreground: a.theme.PanelBorder, Background: a.theme.Background}
	if tile.contains(a.focusedTile) {
		style.Foreground = a.theme.Accent
	}
	if vertical {
		for y := separator.y; y < separator.y+separator.height; y++ {
			a.screen.Set(separator.x, y, a.theme.Borders.Separator.Vertical, style)
		}
		return
	}
	for x := separator.x; x < separator.x+separator.width; x++ {
		a.screen.Set(x, separator.y, a.theme.Borders.Separator.Horizontal, style)
	}
}

func splitEditorRegion(region editorRegion, direction splitDirection) (
	first editorRegion,
	second editorRegion,
	separator editorRegion,
	vertical bool,
) {
	if direction == splitVertically {
		available := max(0, region.width-1)
		firstWidth := available / 2
		first = editorRegion{x: region.x, y: region.y, width: firstWidth, height: region.height}
		separator = editorRegion{x: region.x + firstWidth, y: region.y, width: 1, height: region.height}
		second = editorRegion{
			x: separator.x + 1, y: region.y,
			width: available - firstWidth, height: region.height,
		}
		return first, second, separator, true
	}
	available := max(0, region.height-1)
	firstHeight := available / 2
	first = editorRegion{x: region.x, y: region.y, width: region.width, height: firstHeight}
	separator = editorRegion{x: region.x, y: region.y + firstHeight, width: region.width, height: 1}
	second = editorRegion{
		x: region.x, y: separator.y + 1,
		width: region.width, height: available - firstHeight,
	}
	return first, second, separator, false
}

func (a *App) editorTileRegion(target *editorTile, root editorRegion) (editorRegion, bool) {
	return findEditorTileRegion(a.tileRoot, target, root)
}

func findEditorTileRegion(tile, target *editorTile, region editorRegion) (editorRegion, bool) {
	if tile == nil || target == nil {
		return editorRegion{}, false
	}
	if tile == target {
		return region, true
	}
	if tile.isLeaf() {
		return editorRegion{}, false
	}
	first, second, _, _ := splitEditorRegion(region, tile.direction)
	if found, ok := findEditorTileRegion(tile.first, target, first); ok {
		return found, true
	}
	return findEditorTileRegion(tile.second, target, second)
}
