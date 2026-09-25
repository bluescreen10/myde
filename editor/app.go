package editor

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/protocol"
	"github.com/bluescreen10/myde/syntax"
	"github.com/bluescreen10/myde/terminal"
)

type diagnostic struct {
	line      int
	column    int
	endLine   int
	endColumn int
	severity  int
	code      string
	source    string
	message   string
}

type serverEvent struct {
	path               string
	diagnostics        []diagnostic
	completions        []lspCompletion
	completionBuffer   *buffer.Buffer
	completionRevision uint64
	completionPoint    buffer.Point
	completionEpoch    uint64
	completionDetails  *completionDetails
	definition         *definitionTarget
	message            string
	debugReady         *protocol.DebugProcess
	terminal           *shellBuffer
	terminalOutput     string
}

// App is an interactive editor session.
type App struct {
	root    string
	session *terminal.Session
	screen  *terminal.Screen
	reader  *terminal.Reader

	buffers      []*buffer.Buffer
	highlights   map[*buffer.Buffer]*syntax.Highlighter
	active       int
	topLine      int
	leftColumn   int
	historyLimit int
	files        []string
	showFiles    bool
	browser      *fileBrowser
	sidebar      *sidebarPanel
	diagnostics  map[string][]diagnostic
	breakpoints  map[string]map[int]bool
	terminals    map[*buffer.Buffer]*shellBuffer

	theme           Theme
	extensions      *extensions
	bindings        map[string]string
	commands        map[string]plugin.Command
	palette         *palette
	minibuffer      *minibuffer
	message         string
	prefix          bool
	running         bool
	completionEpoch uint64
	typingBuffer    *buffer.Buffer
	typingKind      typingGroup

	lsp                  *protocol.Process
	lspCancel            context.CancelFunc
	lspSync              int
	lspCompletionResolve bool
	dap                  *protocol.DebugProcess
	dapCancel            context.CancelFunc
	servers              chan serverEvent
}

// New creates an editor rooted at root and opens paths.
func New(
	root string,
	paths []string,
	session *terminal.Session,
	input io.Reader,
	output io.Writer,
	installed ...plugin.Plugin,
) (*App, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	width, height, err := session.Size()
	if err != nil {
		return nil, err
	}
	app := &App{
		root:         absolute,
		session:      session,
		screen:       terminal.NewScreen(output, width, height),
		reader:       terminal.NewReader(input),
		highlights:   make(map[*buffer.Buffer]*syntax.Highlighter),
		diagnostics:  make(map[string][]diagnostic),
		breakpoints:  make(map[string]map[int]bool),
		terminals:    make(map[*buffer.Buffer]*shellBuffer),
		historyLimit: 1000,
		theme:        VSDark2026(),
		extensions:   newExtensions(absolute),
		bindings:     defaultBindings(),
		servers:      make(chan serverEvent, 64),
	}
	app.registerCommands()
	for _, current := range installed {
		if err := current.Load(app); err != nil {
			return nil, fmt.Errorf("load plugin %s: %w", current.Name(), err)
		}
	}
	app.files = workspaceFiles(absolute)
	app.browser = newFileBrowser(absolute, app.files)
	for _, path := range paths {
		if err := app.open(path); err != nil {
			return nil, err
		}
	}
	if len(app.buffers) == 0 {
		app.addBuffer(buffer.New())
	}
	app.reloadExtensions()
	app.autoStartGoLSP()
	return app, nil
}

// Run processes input until the quit command is invoked.
func (a *App) Run() error {
	events := make(chan terminal.Event)
	acknowledged := make(chan struct{})
	errors := make(chan error, 1)
	go func() {
		for {
			event, err := a.reader.ReadEvent()
			if err != nil {
				errors <- err
				return
			}
			events <- event
			<-acknowledged
		}
	}()

	a.running = true
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for a.running {
		if err := a.render(); err != nil {
			return err
		}
		select {
		case event := <-events:
			if err := a.handleEvent(event); err != nil {
				a.message = err.Error()
			}
			acknowledged <- struct{}{}
		case event := <-a.servers:
			a.handleServerEvent(event)
		case <-ticker.C:
			a.pollChanges()
		case err := <-errors:
			return err
		}
	}
	return nil
}

func (a *App) current() *buffer.Buffer {
	return a.buffers[a.active]
}

func (a *App) addBuffer(current *buffer.Buffer) {
	current.SetHistoryLimit(a.historyLimit)
	a.buffers = append(a.buffers, current)
	a.active = len(a.buffers) - 1
	a.highlights[current] = syntax.New(current.Path())
}

func (a *App) open(path string) error {
	if !filepath.IsAbs(path) {
		path = filepath.Join(a.root, path)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	for index, current := range a.buffers {
		if current.Path() == absolute {
			a.active = index
			a.CloseSidebar()
			return nil
		}
	}
	opened, err := buffer.Open(absolute)
	if err != nil {
		return err
	}
	a.addBuffer(opened)
	a.CloseSidebar()
	a.topLine = 0
	a.leftColumn = 0
	a.runHooks("open")
	a.notifyLSPDidOpen(opened)
	return nil
}

func (a *App) pollChanges() {
	if changed, err := a.extensions.reloadIfChanged(); err != nil {
		a.message = err.Error()
	} else if changed {
		a.applyExtensions()
		a.message = "extensions reloaded"
	}
	for _, current := range a.buffers {
		if !current.HasExternalChange() {
			continue
		}
		if current.IsDirty() {
			a.message = current.Name() + " changed on disk; buffer kept because it has edits"
			continue
		}
		if err := current.Reload(); err != nil {
			a.message = err.Error()
			continue
		}
		a.highlights[current] = syntax.New(current.Path())
		if current == a.current() {
			a.topLine = 0
		}
		a.message = current.Name() + " reloaded"
	}
	if width, height, err := a.session.Size(); err == nil {
		a.screen.Resize(width, height)
	}
}

func (a *App) reloadExtensions() {
	changed, err := a.extensions.reloadIfChanged()
	if err != nil {
		a.message = err.Error()
		return
	}
	if changed {
		a.applyExtensions()
	}
}

func (a *App) applyExtensions() {
	a.bindings = defaultBindings()
	a.historyLimit = 1000
	for key, command := range a.extensions.bindings {
		a.bindings[key] = command
	}
	a.theme = VSDark2026()
	for name, value := range a.extensions.colors {
		if err := a.theme.setColor(name, value); err != nil {
			a.message = err.Error()
		}
	}
	if configured := a.extensions.settings["history-limit"]; configured != "" {
		limit, err := strconv.Atoi(configured)
		if err != nil || limit < 0 {
			a.message = "history-limit must be a non-negative integer"
		} else {
			a.historyLimit = limit
		}
	}
	for _, current := range a.buffers {
		if a.terminals[current] == nil {
			current.SetHistoryLimit(a.historyLimit)
		}
	}
}

func (a *App) handleServerEvent(event serverEvent) {
	if event.terminal != nil {
		a.finishTerminalCommand(event.terminal, event.terminalOutput)
	}
	if event.debugReady != nil && event.debugReady == a.dap {
		a.startDebugConfiguration(event.debugReady)
	}
	if event.path != "" {
		a.diagnostics[event.path] = event.diagnostics
	}
	if event.message != "" {
		a.message = event.message
	}
	if event.completionDetails != nil {
		a.applyCompletionDetails(*event.completionDetails)
	}
	if len(event.completions) > 0 && event.completionBuffer == a.current() &&
		event.completionRevision == event.completionBuffer.Revision() &&
		event.completionPoint == event.completionBuffer.Cursors()[0].Point &&
		event.completionEpoch == a.completionEpoch && a.minibuffer == nil &&
		(a.palette == nil || a.palette.completion) {
		items := make([]paletteItem, 0, len(event.completions))
		for index, completion := range event.completions {
			items = append(items, paletteItem{
				label:         completion.label,
				detail:        completion.detail,
				documentation: completion.documentation,
				kind:          completion.kind,
				resolve:       completion.resolve,
				value:         strconv.Itoa(index),
			})
		}
		a.chooseCompletion("LSP Completion", items, false, func(item paletteItem) {
			index, err := strconv.Atoi(item.value)
			if err != nil || index < 0 || index >= len(event.completions) {
				return
			}
			a.applyLSPCompletion(event.completionBuffer, event.completions[index])
		})
		a.resolveSelectedCompletion()
	}
	if event.definition != nil {
		a.openDefinition(*event.definition)
	}
}

func workspaceFiles(root string) []string {
	files := make([]string, 0, 256)
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() && (entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == "node_modules") {
			return filepath.SkipDir
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err == nil {
			files = append(files, relative)
		}
		if len(files) >= 10000 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Strings(files)
	return files
}

func defaultBindings() map[string]string {
	bindings := map[string]string{
		"ctrl-p":     "command.palette",
		"ctrl-q":     "editor.quit",
		"ctrl-space": "completion.show",
		"alt-.":      "lsp.definition",
		"alt-j":      "cursor.add-below",
		"alt-f":      "view.files",
	}
	for key, command := range platformBindings() {
		bindings[key] = command
	}
	for number := 1; number <= 9; number++ {
		bindings[fmt.Sprintf("ctrl-%d", number)] = fmt.Sprintf("buffer.select %d", number)
		bindings[fmt.Sprintf("alt-%d", number)] = fmt.Sprintf("buffer.select %d", number)
	}
	return bindings
}

func (a *App) handleEvent(event terminal.Event) error {
	if !a.isTypingEvent(event) {
		a.finishTypingGroup()
	}
	if event.Key == terminal.KeyEscape {
		if a.palette != nil || a.minibuffer != nil || a.prefix {
			a.cancelAction()
			return nil
		}
		if a.sidebar != nil {
			a.CloseSidebar()
			a.message = ""
			return nil
		}
		if a.browser.focused {
			a.closeFileBrowser()
			a.message = ""
			return nil
		}
		a.cancelAction()
		return nil
	}
	if a.minibuffer != nil {
		return a.handleMinibufferEvent(event)
	}
	if a.palette != nil {
		if a.palette.completion {
			return a.handleCompletionEvent(event)
		}
		return a.handlePaletteEvent(event)
	}
	key := keyName(event)
	if a.prefix {
		a.prefix = false
		switch key {
		case "ctrl-s":
			return a.execute("file.save")
		case "ctrl-f":
			return a.execute("file.open")
		case "b":
			return a.execute("buffer.switch")
		case "k":
			return a.execute("buffer.close")
		case "ctrl-c":
			return a.execute("editor.quit")
		default:
			return fmt.Errorf("unknown ctrl-x sequence: %s", key)
		}
	}
	if command := a.bindings[key]; command != "" {
		return a.execute(command)
	}
	if key == "ctrl-x" {
		a.prefix = true
		a.message = "C-x"
		return nil
	}
	if a.showFiles && a.browser.focused {
		return a.handleFileBrowserEvent(event)
	}
	if a.sidebar != nil {
		return a.handleSidebarEvent(event)
	}
	if event.Super {
		switch event.Key {
		case terminal.KeyUp:
			a.moveDocumentEdge(false, event.Shift)
		case terminal.KeyDown:
			a.moveDocumentEdge(true, event.Shift)
		case terminal.KeyLeft:
			a.moveLineEdge(false, event.Shift)
		case terminal.KeyRight:
			a.moveLineEdge(true, event.Shift)
		default:
			break
		}
		if event.Key == terminal.KeyUp || event.Key == terminal.KeyDown ||
			event.Key == terminal.KeyLeft || event.Key == terminal.KeyRight {
			a.ensureCursorVisible()
			return nil
		}
	}
	if terminalBuffer := a.terminals[a.current()]; terminalBuffer != nil {
		return a.handleTerminalEvent(event, terminalBuffer)
	}
	switch event.Key {
	case terminal.KeyRune:
		if event.Control || event.Alt || event.Super || event.Rune == 0 {
			return nil
		}
		a.insertTyped(event.Rune)
		if isCompletionRune(event.Rune) && strings.EqualFold(filepath.Ext(a.current().Path()), ".go") {
			a.requestLSPCompletion()
		}
	case terminal.KeyEnter:
		a.insert([]byte{'\n'})
	case terminal.KeyTab:
		a.insert([]byte("    "))
	case terminal.KeyBackspace:
		a.backspace()
		if strings.EqualFold(filepath.Ext(a.current().Path()), ".go") {
			a.requestLSPCompletion()
		}
	case terminal.KeyDelete:
		a.deleteForward()
		if strings.EqualFold(filepath.Ext(a.current().Path()), ".go") {
			a.requestLSPCompletion()
		}
	case terminal.KeyUp:
		a.moveCursors(0, -1, event.Shift)
	case terminal.KeyDown:
		a.moveCursors(0, 1, event.Shift)
	case terminal.KeyLeft:
		a.moveCursors(-1, 0, event.Shift)
	case terminal.KeyRight:
		a.moveCursors(1, 0, event.Shift)
	case terminal.KeyHome:
		a.moveLineEdge(false, event.Shift)
	case terminal.KeyEnd:
		a.moveLineEdge(true, event.Shift)
	case terminal.KeyPageUp:
		_, height := a.screen.Size()
		a.moveCursors(0, -max(1, height-3), event.Shift)
	case terminal.KeyPageDown:
		_, height := a.screen.Size()
		a.moveCursors(0, max(1, height-3), event.Shift)
	}
	a.ensureCursorVisible()
	return nil
}

func (a *App) cancelAction() {
	a.palette = nil
	a.minibuffer = nil
	a.prefix = false
	a.message = ""
	a.completionEpoch++
	current := a.current()
	cursors := current.Cursors()
	for index, cursor := range cursors {
		cursors[index].Anchor = cursor.Point
	}
	current.SetCursors(cursors)
}

func isCompletionRune(value rune) bool {
	return value == '.' || value == '_' || unicode.IsLetter(value) || unicode.IsDigit(value)
}

type edit struct {
	cursor int
	start  int
	end    int
	text   []byte
}

type typingGroup uint8

const (
	typingWord typingGroup = iota + 1
	typingWhitespace
	typingPunctuation
)

func (a *App) isTypingEvent(event terminal.Event) bool {
	if event.Key != terminal.KeyRune || event.Control || event.Alt || event.Super || event.Rune == 0 {
		return false
	}
	if a.prefix || a.minibuffer != nil || (a.palette != nil && !a.palette.completion) {
		return false
	}
	if a.showFiles && a.browser.focused || a.sidebar != nil || a.terminals[a.current()] != nil {
		return false
	}
	return a.bindings[keyName(event)] == ""
}

func (a *App) insertTyped(value rune) {
	current := a.current()
	kind := typingGroupForRune(value)
	if a.typingBuffer == current && a.typingKind == kind {
		current.MergeNextEditGroup()
	} else {
		a.typingBuffer = current
		a.typingKind = kind
	}
	a.insert([]byte(string(value)))
}

func (a *App) finishTypingGroup() {
	a.typingBuffer = nil
	a.typingKind = 0
}

func typingGroupForRune(value rune) typingGroup {
	if unicode.IsLetter(value) || unicode.IsNumber(value) || unicode.IsMark(value) || value == '_' {
		return typingWord
	}
	if unicode.IsSpace(value) {
		return typingWhitespace
	}
	return typingPunctuation
}

func (a *App) insert(text []byte) {
	current := a.current()
	edits := cursorEdits(current, text, false, false)
	a.applyEdits(edits)
}

func (a *App) backspace() {
	current := a.current()
	edits := cursorEdits(current, nil, true, false)
	a.applyEdits(edits)
}

func (a *App) deleteForward() {
	current := a.current()
	edits := cursorEdits(current, nil, false, true)
	a.applyEdits(edits)
}

func cursorEdits(current *buffer.Buffer, text []byte, backward, forward bool) []edit {
	cursors := current.Cursors()
	edits := make([]edit, 0, len(cursors))
	for index, cursor := range cursors {
		start := current.Offset(cursor.Anchor)
		end := current.Offset(cursor.Point)
		if start > end {
			start, end = end, start
		}
		if start == end && backward && start > 0 {
			near := current.Slice(max(0, start-4), start)
			_, size := utf8.DecodeLastRune(near)
			start -= size
		}
		if start == end && forward && end < current.Len() {
			near := current.Slice(end, min(current.Len(), end+4))
			_, size := utf8.DecodeRune(near)
			end += size
		}
		if start == end && len(text) == 0 {
			continue
		}
		edits = append(edits, edit{cursor: index, start: start, end: end, text: text})
	}
	return edits
}

func (a *App) applyEdits(edits []edit) {
	if len(edits) == 0 {
		return
	}
	current := a.current()
	if current.IsReadOnly() {
		a.message = current.Name() + " is read-only"
		return
	}
	cursors := current.Cursors()
	changedLine := current.LineCount()
	for _, change := range edits {
		changedLine = min(changedLine, current.Point(change.start).Line)
	}
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].start > edits[j].start
	})
	changes := make([]textChange, 0, len(edits))
	for _, change := range edits {
		changes = append(changes, newTextChange(
			current,
			current.Point(change.start),
			current.Point(change.end),
			string(change.text),
		))
	}
	current.BeginTransaction()
	for _, change := range edits {
		current.Delete(change.start, change.end)
		current.Insert(change.start, change.text)
	}
	current.EndTransaction()
	for _, change := range edits {
		final := change.start + len(change.text)
		for _, other := range edits {
			if other.start < change.start {
				final += len(other.text) - (other.end - other.start)
			}
		}
		point := current.Point(final)
		cursors[change.cursor] = buffer.Cursor{Anchor: point, Point: point}
	}
	current.SetCursors(cursors)
	a.highlights[current].Invalidate(changedLine)
	a.notifyLSPChanges(current, changes)
	a.ensureCursorVisible()
}

func (a *App) moveCursors(horizontal, vertical int, extend bool) {
	current := a.current()
	cursors := current.Cursors()
	for index, cursor := range cursors {
		point := cursor.Point
		if !extend && vertical == 0 && cursor.Anchor != cursor.Point {
			anchorOffset := current.Offset(cursor.Anchor)
			pointOffset := current.Offset(cursor.Point)
			if horizontal < 0 {
				point = current.Point(min(anchorOffset, pointOffset))
			} else if horizontal > 0 {
				point = current.Point(max(anchorOffset, pointOffset))
			}
			cursors[index] = buffer.Cursor{Anchor: point, Point: point}
			continue
		}
		if vertical != 0 {
			point.Line = max(0, min(current.LineCount()-1, point.Line+vertical))
			point = current.Point(current.Offset(point))
		}
		if horizontal < 0 {
			if point.Column > 0 {
				point.Column--
			} else if point.Line > 0 {
				point.Line--
				point = current.Point(current.Offset(buffer.Point{Line: point.Line, Column: 1 << 30}))
			}
		}
		if horizontal > 0 {
			lineEnd := current.Point(current.Offset(buffer.Point{Line: point.Line, Column: 1 << 30}))
			if point.Column < lineEnd.Column {
				point.Column++
			} else if point.Line+1 < current.LineCount() {
				point = buffer.Point{Line: point.Line + 1}
			}
		}
		anchor := point
		if extend {
			anchor = cursor.Anchor
		}
		cursors[index] = buffer.Cursor{Anchor: anchor, Point: point}
	}
	current.SetCursors(cursors)
	a.ensureCursorVisible()
}

func (a *App) moveLineEdge(end, extend bool) {
	current := a.current()
	cursors := current.Cursors()
	for index, cursor := range cursors {
		point := buffer.Point{Line: cursor.Point.Line}
		if end {
			point = current.Point(current.Offset(buffer.Point{Line: point.Line, Column: 1 << 30}))
		}
		anchor := point
		if extend {
			anchor = cursor.Anchor
		}
		cursors[index] = buffer.Cursor{Anchor: anchor, Point: point}
	}
	current.SetCursors(cursors)
}

func (a *App) moveDocumentEdge(end, extend bool) {
	current := a.current()
	cursors := current.Cursors()
	for index, cursor := range cursors {
		point := buffer.Point{}
		if end {
			point = current.Point(current.Len())
		}
		anchor := point
		if extend {
			anchor = cursor.Anchor
		}
		cursors[index] = buffer.Cursor{Anchor: anchor, Point: point}
	}
	current.SetCursors(cursors)
}

func (a *App) ensureCursorVisible() {
	width, height := a.screen.Size()
	statusRow := height - 1
	if a.minibuffer != nil {
		statusRow--
	}
	bodyHeight := max(1, statusRow-1)
	point := a.current().Cursors()[0].Point
	if point.Line < a.topLine {
		a.topLine = point.Line
	}
	if point.Line >= a.topLine+bodyHeight {
		a.topLine = point.Line - bodyHeight + 1
	}
	sidebarWidth := 0
	if a.showFiles || a.sidebar != nil {
		sidebarWidth = fileSidebarWidth(width)
	}
	lineNumberWidth := len(strconv.Itoa(max(1, a.current().LineCount()))) + 2
	available := max(1, width-sidebarWidth-lineNumberWidth)
	line := []rune(string(a.current().Line(point.Line)))
	column := min(point.Column, len(line))
	displayColumn := sourceDisplayWidth(line[:column])
	if displayColumn < a.leftColumn {
		a.leftColumn = displayColumn
	}
	if displayColumn >= a.leftColumn+available {
		a.leftColumn = displayColumn - available + 1
	}
}

func keyName(event terminal.Event) string {
	var name string
	switch event.Key {
	case terminal.KeyRune:
		if event.Control && event.Rune == ' ' {
			name = "space"
		} else {
			name = string(event.Rune)
		}
	case terminal.KeyEnter:
		name = "enter"
	case terminal.KeyEscape:
		name = "escape"
	case terminal.KeyTab:
		name = "tab"
	case terminal.KeyBackspace:
		name = "backspace"
	case terminal.KeyDelete:
		name = "delete"
	case terminal.KeyUp:
		name = "up"
	case terminal.KeyDown:
		name = "down"
	case terminal.KeyLeft:
		name = "left"
	case terminal.KeyRight:
		name = "right"
	}
	if event.Shift && (event.Control || event.Alt || event.Super) {
		name = "shift-" + name
	}
	if event.Control {
		name = "ctrl-" + name
	}
	if event.Alt {
		name = "alt-" + name
	}
	if event.Super {
		name = "super-" + name
	}
	return normalizeKey(name)
}

func (a *App) handlePaletteEvent(event terminal.Event) error {
	p := a.palette
	switch event.Key {
	case terminal.KeyEscape:
		a.palette = nil
	case terminal.KeyBackspace:
		if len(p.query) > 0 {
			p.query = p.query[:len(p.query)-1]
			p.selected = 0
			p.update()
		}
	case terminal.KeyUp:
		p.selected = max(0, p.selected-1)
	case terminal.KeyDown:
		p.selected = min(max(0, len(p.filtered)-1), p.selected+1)
	case terminal.KeyEnter:
		query := string(p.query)
		if p.onChoose != nil && len(p.filtered) > 0 {
			item := p.filtered[p.selected]
			a.palette = nil
			p.onChoose(item)
		} else if p.onSubmit != nil {
			a.palette = nil
			p.onSubmit(query)
		}
	case terminal.KeyRune:
		if !event.Control && !event.Alt && !event.Super && event.Rune != 0 {
			p.query = append(p.query, event.Rune)
			p.selected = 0
			p.update()
		}
	}
	return nil
}

func (a *App) handleCompletionEvent(event terminal.Event) error {
	completion := a.palette
	switch event.Key {
	case terminal.KeyUp:
		completion.selected = max(0, completion.selected-1)
		a.resolveSelectedCompletion()
		return nil
	case terminal.KeyDown:
		completion.selected = min(max(0, len(completion.filtered)-1), completion.selected+1)
		a.resolveSelectedCompletion()
		return nil
	case terminal.KeyEnter, terminal.KeyTab:
		if a.current().Revision() != completion.completionRevision || len(completion.filtered) == 0 {
			a.palette = nil
			return nil
		}
		item := completion.filtered[completion.selected]
		a.palette = nil
		completion.onChoose(item)
		return nil
	}

	local := completion.localCompletion
	a.palette = nil
	if err := a.handleEvent(event); err != nil {
		return err
	}
	if local && isCompletionEdit(event) {
		return a.showLocalCompletion()
	}
	return nil
}

func isCompletionEdit(event terminal.Event) bool {
	if event.Key == terminal.KeyBackspace || event.Key == terminal.KeyDelete {
		return true
	}
	return event.Key == terminal.KeyRune && !event.Control && !event.Alt && !event.Super && event.Rune != 0
}

func (a *App) handleMinibufferEvent(event terminal.Event) error {
	prompt := a.minibuffer
	switch event.Key {
	case terminal.KeyBackspace:
		if len(prompt.query) > 0 {
			prompt.query = prompt.query[:len(prompt.query)-1]
		}
	case terminal.KeyEnter:
		query := string(prompt.query)
		if query == "" {
			return nil
		}
		a.minibuffer = nil
		prompt.onSubmit(query)
	case terminal.KeyRune:
		if !event.Control && !event.Alt && !event.Super && event.Rune != 0 {
			prompt.query = append(prompt.query, event.Rune)
		}
	}
	return nil
}

func (a *App) prompt(title string, submit func(string)) {
	a.minibuffer = nil
	a.message = ""
	a.palette = &palette{title: title, onSubmit: submit}
	a.palette.update()
}

func (a *App) promptMinibuffer(title string, submit func(string)) {
	a.palette = nil
	a.message = ""
	a.minibuffer = &minibuffer{title: title, onSubmit: submit}
}

func (a *App) choose(title string, items []paletteItem, choose func(paletteItem)) {
	a.minibuffer = nil
	a.message = ""
	a.palette = &palette{title: title, items: items, onChoose: choose}
	a.palette.update()
}

func (a *App) chooseCompletion(title string, items []paletteItem, local bool, choose func(paletteItem)) {
	if len(items) == 0 {
		a.palette = nil
		return
	}
	a.minibuffer = nil
	a.palette = &palette{
		title:              title,
		items:              items,
		onChoose:           choose,
		completion:         true,
		localCompletion:    local,
		completionRevision: a.current().Revision(),
	}
	a.palette.update()
}

func parseLineNumber(value string) int {
	number, _ := strconv.Atoi(value)
	return max(0, number-1)
}

func splitCommand(value string) (string, []string, error) {
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return "", nil, errors.New("command is empty")
	}
	return fields[0], fields[1:], nil
}

func runCommand(root, name string, arguments ...string) (string, error) {
	command := exec.Command(name, arguments...)
	command.Dir = root
	output, err := command.CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("%s: %w", name, err)
	}
	return strings.TrimSpace(string(output)), nil
}
