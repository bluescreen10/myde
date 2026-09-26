package editor

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bluescreen10/myde/buffer"
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
	root              *fileNode
	expanded          map[string]bool
	directoryVersions map[string]directoryVersion
	query             []rune
	entries           []fileEntry
	selected          int
	top               int
	focused           bool
}

type directoryVersion struct {
	modified time.Time
	size     int64
}

func newFileBrowser(root string, paths, directories []string) *fileBrowser {
	rootNode := &fileNode{
		name:      filepath.Base(root),
		directory: true,
		byName:    make(map[string]*fileNode),
	}
	addPath := func(path string, directory bool) {
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
					directory: index < len(parts)-1 || directory,
					parent:    parent,
					byName:    make(map[string]*fileNode),
				}
				parent.children = append(parent.children, node)
				parent.byName[part] = node
			} else if index < len(parts)-1 || directory {
				node.directory = true
			}
			parent = node
		}
	}
	for _, path := range directories {
		addPath(path, true)
	}
	for _, path := range paths {
		addPath(path, false)
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
	included[node] = query == "" || matchedChild || node.parent == nil
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
	case terminal.KeyDelete:
		return a.confirmFileDelete()
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

func (a *App) newFile(arguments string) error {
	if !a.showFiles || !a.browser.focused {
		if strings.TrimSpace(arguments) != "" {
			return a.createFile(a.root, arguments)
		}
		a.CloseSidebar()
		a.addBuffer(buffer.New())
		a.topLine = 0
		a.leftColumn = 0
		a.message = "new buffer"
		return nil
	}
	directory, relative, err := a.browserDirectory()
	if err != nil {
		return err
	}
	if strings.TrimSpace(arguments) != "" {
		return a.createFile(directory, arguments)
	}
	title := "New file"
	if relative != "." {
		title += " in " + relative
	}
	a.promptMinibuffer(title, func(name string) {
		if err := a.createFile(directory, name); err != nil {
			a.message = err.Error()
		}
	})
	return nil
}

func (a *App) createFile(directory, name string) error {
	name, err := validFileName(name)
	if err != nil {
		return err
	}
	path := filepath.Join(directory, name)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", name, err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %s: %w", name, err)
	}
	a.refreshFileBrowser(relativeToRoot(a.root, path))
	if err := a.open(path); err != nil {
		return err
	}
	a.message = "created " + relativeToRoot(a.root, path)
	return nil
}

func (a *App) renameFile(arguments string) error {
	if !a.showFiles || !a.browser.focused {
		return fmt.Errorf("file.rename requires the focused file browser")
	}
	entry, ok := a.browser.currentEntry()
	if !ok || entry.node.parent == nil {
		return fmt.Errorf("select a file to rename")
	}
	if entry.node.directory {
		return fmt.Errorf("directory renaming is not supported")
	}
	oldRelative := entry.node.path
	if strings.TrimSpace(arguments) != "" {
		return a.renameBrowserFile(oldRelative, arguments)
	}
	a.promptMinibuffer("Rename "+entry.node.name+" to", func(name string) {
		if err := a.renameBrowserFile(oldRelative, name); err != nil {
			a.message = err.Error()
		}
	})
	return nil
}

func (a *App) renameBrowserFile(oldRelative, name string) error {
	name, err := validFileName(name)
	if err != nil {
		return err
	}
	oldPath := filepath.Join(a.root, oldRelative)
	newPath := filepath.Join(filepath.Dir(oldPath), name)
	if oldPath == newPath {
		return nil
	}
	oldInfo, err := os.Lstat(oldPath)
	if err != nil {
		return fmt.Errorf("rename %s: %w", filepath.Base(oldPath), err)
	}
	newInfo, newErr := os.Lstat(newPath)
	if newErr == nil && !os.SameFile(oldInfo, newInfo) {
		return fmt.Errorf("rename %s: destination already exists", name)
	}
	if newErr != nil && !os.IsNotExist(newErr) {
		return fmt.Errorf("check %s: %w", name, newErr)
	}
	if err := os.Rename(oldPath, newPath); err != nil {
		return fmt.Errorf("rename %s: %w", filepath.Base(oldPath), err)
	}
	if err := a.updateRenamedBuffers(oldPath, newPath); err != nil {
		_ = os.Rename(newPath, oldPath)
		return err
	}
	newRelative := relativeToRoot(a.root, newPath)
	a.refreshFileBrowser(newRelative)
	a.message = "renamed " + oldRelative + " to " + newRelative
	return nil
}

func (a *App) updateRenamedBuffers(oldPath, newPath string) error {
	renamed := make([]*editorBuffer, 0, 1)
	for _, editorBuffer := range a.buffers {
		current := editorBuffer.text
		if current.Path() != oldPath {
			continue
		}
		a.notifyLSPDidClose(current)
		if err := current.SetPath(newPath); err != nil {
			return err
		}
		editorBuffer.mode = a.modeForPath(newPath)
		editorBuffer.highlighter = a.highlighterForBuffer(current)
		renamed = append(renamed, editorBuffer)
	}
	if len(renamed) > 0 && renamed[0] == a.currentEditorBuffer() {
		a.activateCurrentMode()
	}
	for _, current := range renamed {
		a.notifyLSPDidOpen(current.text)
	}
	return nil
}

func (a *App) confirmFileDelete() error {
	entry, ok := a.browser.currentEntry()
	if !ok || entry.node.parent == nil {
		return fmt.Errorf("select a file to delete")
	}
	if entry.node.directory {
		return fmt.Errorf("directory deletion is not supported")
	}
	relative := entry.node.path
	items := []paletteItem{
		{label: "Delete file", detail: relative + " · cannot be undone", value: "delete"},
		{label: "Cancel", value: "cancel"},
	}
	a.choose("Delete "+entry.node.name+"?", items, func(item paletteItem) {
		if item.value != "delete" {
			return
		}
		if err := a.deleteBrowserFile(relative); err != nil {
			a.message = err.Error()
		}
	})
	return nil
}

func (a *App) deleteBrowserFile(relative string) error {
	path := filepath.Join(a.root, relative)
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("delete %s: %w", relative, err)
	}
	if info.IsDir() {
		return fmt.Errorf("directory deletion is not supported")
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("delete %s: %w", relative, err)
	}
	for _, current := range a.buffers {
		if current.text.Path() == path {
			a.notifyLSPDidClose(current.text)
			current.diagnostics = nil
			clear(current.breakpoints)
		}
	}
	parent := relativeToRoot(a.root, filepath.Dir(path))
	a.refreshFileBrowser(parent)
	a.message = "deleted " + relative
	return nil
}

func (a *App) browserDirectory() (string, string, error) {
	entry, ok := a.browser.currentEntry()
	if !ok {
		return "", "", fmt.Errorf("select a file or directory")
	}
	node := entry.node
	if !node.directory {
		node = node.parent
	}
	relative := node.path
	if relative == "" {
		relative = "."
	}
	return filepath.Join(a.root, node.path), relative, nil
}

func (a *App) refreshFileBrowser(selectedPath string) {
	a.replaceFileBrowser(scanWorkspace(a.root), selectedPath)
}

func (a *App) syncFileBrowser() {
	if !a.hasFileBrowserChanges() {
		return
	}
	contents := scanWorkspace(a.root)
	if equalPaths(contents.files, a.files) && equalPaths(contents.directories, a.directories) {
		a.captureFileBrowserDirectories()
		return
	}
	selectedPath := ""
	if entry, ok := a.browser.currentEntry(); ok {
		selectedPath = entry.node.path
	}
	a.replaceFileBrowser(contents, selectedPath)
}

func (a *App) replaceFileBrowser(contents workspaceContents, selectedPath string) {
	expanded := a.browser.expanded
	query := append([]rune(nil), a.browser.query...)
	top := a.browser.top
	focused := a.browser.focused
	a.files = contents.files
	a.directories = contents.directories
	browser := newFileBrowser(a.root, a.files, a.directories)
	for path, isExpanded := range expanded {
		if isExpanded {
			browser.expanded[path] = true
		}
	}
	browser.focused = focused
	browser.rebuild()
	if len(query) > 0 {
		browser.setQuery(query)
	}
	for index, entry := range browser.entries {
		if filepath.Clean(entry.node.path) == filepath.Clean(selectedPath) {
			browser.selected = index
			break
		}
	}
	browser.top = min(top, max(0, len(browser.entries)-1))
	a.browser = browser
	a.captureFileBrowserDirectories()
}

func (a *App) hasFileBrowserChanges() bool {
	if len(a.browser.directoryVersions) == 0 {
		return true
	}
	for path, version := range a.browser.directoryVersions {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() || !info.ModTime().Equal(version.modified) || info.Size() != version.size {
			return true
		}
	}
	return false
}

func (a *App) captureFileBrowserDirectories() {
	versions := make(map[string]directoryVersion, len(a.directories)+1)
	paths := make([]string, 0, len(a.directories)+1)
	paths = append(paths, a.root)
	for _, directory := range a.directories {
		paths = append(paths, filepath.Join(a.root, directory))
	}
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			continue
		}
		versions[path] = directoryVersion{modified: info.ModTime(), size: info.Size()}
	}
	a.browser.directoryVersions = versions
}

func equalPaths(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func validFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || name == "." || name == ".." || filepath.Base(name) != name {
		return "", fmt.Errorf("enter a file name without a directory")
	}
	return name, nil
}

func relativeToRoot(root, path string) string {
	relative, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return filepath.Clean(relative)
}

func (a *App) closeFileBrowser() {
	a.showFiles = false
	if a.browser == nil {
		return
	}
	a.browser.focused = false
	a.browser.setQuery(nil)
}
