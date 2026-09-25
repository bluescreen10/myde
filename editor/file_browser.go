package editor

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/bluescreen10/myde/terminal"
)

type fileNode struct {
	name      string
	path      string
	directory bool
	parent    *fileNode
	children  []*fileNode
	byName    map[string]*fileNode
}

type fileEntry struct {
	node  *fileNode
	depth int
}

type fileBrowser struct {
	root     *fileNode
	expanded map[string]bool
	query    []rune
	entries  []fileEntry
	selected int
	top      int
	focused  bool
}

func newFileBrowser(root string, paths []string) *fileBrowser {
	rootNode := &fileNode{
		name:      filepath.Base(root),
		directory: true,
		byName:    make(map[string]*fileNode),
	}
	for _, path := range paths {
		parts := strings.Split(filepath.ToSlash(filepath.Clean(path)), "/")
		parent := rootNode
		currentPath := ""
		for index, part := range parts {
			if part == "" || part == "." {
				continue
			}
			currentPath = filepath.Join(currentPath, part)
			node := parent.byName[part]
			if node == nil {
				node = &fileNode{
					name:      part,
					path:      currentPath,
					directory: index < len(parts)-1,
					parent:    parent,
					byName:    make(map[string]*fileNode),
				}
				parent.children = append(parent.children, node)
				parent.byName[part] = node
			}
			parent = node
		}
	}
	sortFileNodes(rootNode)
	browser := &fileBrowser{
		root:     rootNode,
		expanded: map[string]bool{"": true},
	}
	browser.rebuild()
	return browser
}

func sortFileNodes(parent *fileNode) {
	sort.Slice(parent.children, func(i, j int) bool {
		left := parent.children[i]
		right := parent.children[j]
		if left.directory != right.directory {
			return left.directory
		}
		leftName := strings.ToLower(left.name)
		rightName := strings.ToLower(right.name)
		if leftName == rightName {
			return left.name < right.name
		}
		return leftName < rightName
	})
	for _, child := range parent.children {
		sortFileNodes(child)
		child.byName = nil
	}
}

func (b *fileBrowser) rebuild() {
	var selectedNode *fileNode
	if b.selected >= 0 && b.selected < len(b.entries) {
		selectedNode = b.entries[b.selected].node
	}
	query := strings.ToLower(string(b.query))
	included := make(map[*fileNode]bool)
	markIncludedFiles(b.root, query, included)
	b.entries = b.entries[:0]
	b.appendVisible(b.root, 0, query != "", included)
	b.selected = 0
	if selectedNode != nil {
		for index, entry := range b.entries {
			if entry.node == selectedNode {
				b.selected = index
				break
			}
		}
	}
	b.selected = min(max(0, b.selected), max(0, len(b.entries)-1))
}

func markIncludedFiles(node *fileNode, query string, included map[*fileNode]bool) bool {
	if !node.directory {
		_, matches := fuzzyScore(strings.ToLower(node.path), query)
		included[node] = matches
		return matches
	}
	matchedChild := false
	for _, child := range node.children {
		if markIncludedFiles(child, query, included) {
			matchedChild = true
		}
	}
	included[node] = matchedChild || node.parent == nil
	return included[node]
}

func (b *fileBrowser) appendVisible(node *fileNode, depth int, filtering bool, included map[*fileNode]bool) {
	if !included[node] {
		return
	}
	b.entries = append(b.entries, fileEntry{node: node, depth: depth})
	if !node.directory || (!filtering && !b.expanded[node.path]) {
		return
	}
	for _, child := range node.children {
		b.appendVisible(child, depth+1, filtering, included)
	}
}

func (b *fileBrowser) move(delta int) {
	b.selected = min(max(0, b.selected+delta), max(0, len(b.entries)-1))
}

func (b *fileBrowser) moveRight() {
	entry, ok := b.currentEntry()
	if !ok || !entry.node.directory {
		return
	}
	if !b.expanded[entry.node.path] {
		b.expanded[entry.node.path] = true
		b.rebuild()
		return
	}
	if b.selected+1 < len(b.entries) && b.entries[b.selected+1].depth > entry.depth {
		b.selected++
	}
}

func (b *fileBrowser) moveLeft() {
	entry, ok := b.currentEntry()
	if !ok {
		return
	}
	if entry.node.directory && b.expanded[entry.node.path] && entry.node.parent != nil {
		b.expanded[entry.node.path] = false
		b.rebuild()
		return
	}
	if entry.node.parent == nil {
		return
	}
	for index, candidate := range b.entries {
		if candidate.node == entry.node.parent {
			b.selected = index
			return
		}
	}
}

func (b *fileBrowser) toggleCurrentDirectory() bool {
	entry, ok := b.currentEntry()
	if !ok || !entry.node.directory {
		return false
	}
	b.expanded[entry.node.path] = !b.expanded[entry.node.path]
	b.rebuild()
	return true
}

func (b *fileBrowser) currentEntry() (fileEntry, bool) {
	if b.selected < 0 || b.selected >= len(b.entries) {
		return fileEntry{}, false
	}
	return b.entries[b.selected], true
}

func (b *fileBrowser) setQuery(query []rune) {
	b.query = query
	b.selected = 0
	b.top = 0
	b.rebuild()
	if len(query) == 0 {
		return
	}
	for index, entry := range b.entries {
		if !entry.node.directory {
			b.selected = index
			break
		}
	}
}

func (b *fileBrowser) ensureVisible(rows int) {
	if rows <= 0 {
		b.top = 0
		return
	}
	if b.selected < b.top {
		b.top = b.selected
	}
	if b.selected >= b.top+rows {
		b.top = b.selected - rows + 1
	}
	b.top = min(max(0, b.top), max(0, len(b.entries)-rows))
}

func (a *App) handleFileBrowserEvent(event terminal.Event) error {
	browser := a.browser
	switch event.Key {
	case terminal.KeyUp:
		browser.move(-1)
	case terminal.KeyDown:
		browser.move(1)
	case terminal.KeyPageUp:
		_, height := a.screen.Size()
		browser.move(-max(1, height-8))
	case terminal.KeyPageDown:
		_, height := a.screen.Size()
		browser.move(max(1, height-8))
	case terminal.KeyHome:
		browser.selected = 0
	case terminal.KeyEnd:
		browser.selected = max(0, len(browser.entries)-1)
	case terminal.KeyLeft:
		browser.moveLeft()
	case terminal.KeyRight:
		browser.moveRight()
	case terminal.KeyEnter:
		if browser.toggleCurrentDirectory() {
			return nil
		}
		entry, ok := browser.currentEntry()
		if !ok {
			return nil
		}
		if err := a.open(entry.node.path); err != nil {
			return err
		}
		a.closeFileBrowser()
	case terminal.KeyTab:
		a.closeFileBrowser()
	case terminal.KeyBackspace:
		if len(browser.query) > 0 {
			browser.setQuery(browser.query[:len(browser.query)-1])
		}
	case terminal.KeyRune:
		if !event.Control && !event.Alt && !event.Super && event.Rune != 0 {
			query := append([]rune(nil), browser.query...)
			browser.setQuery(append(query, event.Rune))
		}
	}
	return nil
}

func (a *App) closeFileBrowser() {
	a.showFiles = false
	a.browser.focused = false
	a.browser.setQuery(nil)
}
