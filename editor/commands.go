package editor

import (
	"bytes"
	"fmt"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
)

func (a *App) registerCommands() {
	a.commands = map[string]plugin.Command{
		"buffer.close":            a.closeBuffer,
		"buffer.next":             a.nextBuffer,
		"buffer.previous":         a.previousBuffer,
		"buffer.select":           a.selectBuffer,
		"buffer.switch":           a.switchBuffer,
		"command.palette":         a.commandPalette,
		"completion.show":         a.showCompletion,
		"cursor.add-below":        a.addCursorBelow,
		"cursor.add-next-match":   a.addCursorAtNextMatch,
		"cursor.file-end":         a.moveToFileEnd,
		"cursor.file-start":       a.moveToFileStart,
		"cursor.line-end":         a.moveToLineEnd,
		"cursor.line-start":       a.moveToLineStart,
		"cursor.page-down":        a.pageDown,
		"cursor.page-up":          a.pageUp,
		"debug.continue":          a.debugContinue,
		"debug.disconnect":        a.debugDisconnect,
		"debug.launch":            a.debugLaunch,
		"debug.next":              a.debugNext,
		"debug.step-in":           a.debugStepIn,
		"debug.step-out":          a.debugStepOut,
		"debug.start":             a.debugStart,
		"debug.toggle-breakpoint": a.debugToggleBreakpoint,
		"editor.quit":             a.quit,
		"edit.cut":                a.cut,
		"edit.copy":               a.copy,
		"edit.paste":              a.paste,
		"edit.redo":               a.redo,
		"edit.undo":               a.undo,
		"file.new":                a.newFile,
		"file.open":               a.openFilePalette,
		"file.rename":             a.renameFile,
		"file.save":               a.save,
		"lsp.definition":          a.requestLSPDefinition,
		"lsp.start":               a.lspStart,
		"search.buffer":           a.searchBuffer,
		"search.project":          a.searchProject,
		"shell.exec":              a.shellExec,
		"shell.run":               a.shellRun,
		"switch.mode":             a.switchMode,
		"terminal.open":           a.openTerminal,
		"theme.select":            a.selectTheme,
		"view.files":              a.toggleFiles,
	}
}

func (a *App) selectTheme(arguments string) error {
	themeID := strings.TrimSpace(arguments)
	if themeID != "" {
		return a.activateTheme(themeID)
	}
	items := make([]paletteItem, 0, len(a.themeIDs))
	for _, id := range a.themeIDs {
		current := a.themes[id]
		borders := current.Borders
		detail := fmt.Sprintf("%c%c%c · separators %c %c",
			borders.Corners.TopLeft,
			borders.Lines.Horizontal,
			borders.Corners.TopRight,
			borders.Separator.Horizontal,
			borders.Separator.Vertical,
		)
		if current.ID == a.theme.ID {
			detail = "current · " + detail
		}
		items = append(items, paletteItem{
			label: current.Name, detail: detail, value: current.ID, kind: "theme",
		})
	}
	a.choose("Select Theme", items, func(item paletteItem) {
		if err := a.activateTheme(item.value); err != nil {
			a.message = err.Error()
		}
	})
	return nil
}

func (a *App) activateTheme(id string) error {
	selected, exists := a.themes[id]
	if !exists {
		return fmt.Errorf("unknown theme %q", id)
	}
	a.theme = selected
	a.message = "theme: " + selected.Name
	return nil
}

func (a *App) execute(specification string) error {
	return a.executeDepth(specification, 0)
}

func (a *App) executeDepth(specification string, depth int) error {
	if depth > 16 {
		return fmt.Errorf("extension recursion limit reached")
	}
	name, arguments, _ := strings.Cut(strings.TrimSpace(specification), " ")
	arguments = strings.TrimSpace(arguments)
	if command := a.commands[name]; command != nil {
		return command(arguments)
	}
	commands, ok := a.extensions.functions[name]
	if !ok {
		return fmt.Errorf("unknown command %q", name)
	}
	for _, command := range commands {
		command = strings.ReplaceAll(command, "{file}", a.current().Path())
		command = strings.ReplaceAll(command, "{root}", a.root)
		if err := a.executeDepth(command, depth+1); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func (a *App) runHooks(name string) {
	for _, function := range a.extensions.hooks[name] {
		if err := a.execute(function); err != nil {
			a.message = fmt.Sprintf("%s hook: %v", name, err)
			return
		}
	}
}

func (a *App) commandPalette(arguments string) error {
	a.message = ""
	names := make([]string, 0, len(a.commands)+len(a.extensions.functions))
	for name := range a.commands {
		names = append(names, name)
	}
	for name := range a.extensions.functions {
		names = append(names, name)
	}
	sort.Strings(names)
	commands := paletteItemsByRecent(names, a.recentCommands, "command")
	files := paletteItemsByRecent(a.files, a.recentFiles, "file")
	a.palette = &palette{
		title:           "Command Palette",
		hideQueryMarker: true,
		source: func(query string) ([]paletteItem, string) {
			if strings.HasPrefix(query, ">") {
				return commands, strings.TrimSpace(strings.TrimPrefix(query, ">"))
			}
			return files, query
		},
		onChoose: func(item paletteItem) {
			if item.kind == "command" {
				a.recentCommands = promoteRecent(a.recentCommands, item.value)
				if err := a.execute(item.value); err != nil {
					a.message = err.Error()
				}
				return
			}
			if err := a.open(item.value); err != nil {
				a.message = err.Error()
			}
		},
	}
	a.palette.update()
	return nil
}

func (a *App) openFilePalette(arguments string) error {
	if arguments != "" {
		return a.open(arguments)
	}
	items := paletteItemsByRecent(a.files, a.recentFiles, "file")
	a.choose("Open file", items, func(item paletteItem) {
		if err := a.open(item.value); err != nil {
			a.message = err.Error()
		}
	})
	return nil
}

func (a *App) recordRecentFile(path string) {
	relative, err := filepath.Rel(a.root, path)
	if err != nil || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
		return
	}
	a.recentFiles = promoteRecent(a.recentFiles, filepath.Clean(relative))
}

func promoteRecent(values []string, value string) []string {
	result := make([]string, 0, min(32, len(values)+1))
	result = append(result, value)
	for _, current := range values {
		if current == value {
			continue
		}
		result = append(result, current)
		if len(result) == 32 {
			break
		}
	}
	return result
}

func paletteItemsByRecent(values, recent []string, kind string) []paletteItem {
	available := make(map[string]bool, len(values))
	for _, value := range values {
		available[value] = true
	}
	items := make([]paletteItem, 0, len(values))
	added := make(map[string]bool, len(values))
	for _, value := range recent {
		if !available[value] || added[value] {
			continue
		}
		items = append(items, paletteItem{label: value, value: value, kind: kind})
		added[value] = true
	}
	for _, value := range values {
		if added[value] {
			continue
		}
		items = append(items, paletteItem{label: value, value: value, kind: kind})
	}
	return items
}

func (a *App) save(arguments string) error {
	current := a.current()
	if current.IsReadOnly() {
		return fmt.Errorf("%s is read-only", current.Name())
	}
	if a.currentEditorBuffer().terminal != nil {
		return fmt.Errorf("terminal buffers cannot be saved")
	}
	if current.Path() == "" && arguments == "" {
		a.promptMinibuffer("Save as", func(path string) {
			if err := a.save(path); err != nil {
				a.message = err.Error()
			}
		})
		return nil
	}
	if arguments != "" && !filepath.IsAbs(arguments) {
		arguments = filepath.Join(a.root, arguments)
	}
	previousPath := current.Path()
	if err := current.Save(arguments); err != nil {
		return err
	}
	if current.Path() != previousPath {
		a.currentEditorBuffer().mode = a.modeForPath(current.Path())
	}
	a.currentEditorBuffer().highlighter = a.highlighterForBuffer(current)
	a.activateCurrentMode()
	a.notifyLSPDidOpen(current)
	a.message = "saved " + current.Name()
	a.runHooks("save")
	return nil
}

func (a *App) quit(arguments string) error {
	dirty := make([]*buffer.Buffer, 0)
	for _, editorBuffer := range a.buffers {
		current := editorBuffer.text
		if editorBuffer.terminal != nil {
			continue
		}
		if current.IsDirty() {
			dirty = append(dirty, current)
		}
	}
	if len(dirty) > 0 && arguments != "force" {
		a.confirmQuit(dirty)
		return nil
	}
	a.finishQuit()
	return nil
}

func (a *App) confirmQuit(dirty []*buffer.Buffer) {
	detail := fmt.Sprintf("%d modified buffers", len(dirty))
	if len(dirty) == 1 {
		detail = dirty[0].Name()
	}
	items := []paletteItem{
		{label: "Save all and quit", detail: detail, value: "save"},
		{label: "Discard all and quit", detail: "cannot be undone", value: "discard"},
		{label: "Cancel", value: "cancel"},
	}
	a.choose("Save changes before quitting?", items, func(item paletteItem) {
		switch item.value {
		case "save":
			a.saveDirtyBuffersAndQuit(dirty, 0)
		case "discard":
			a.finishQuit()
		}
	})
}

func (a *App) saveDirtyBuffersAndQuit(dirty []*buffer.Buffer, index int) {
	if index >= len(dirty) {
		a.finishQuit()
		return
	}
	current := dirty[index]
	if !current.IsDirty() {
		a.saveDirtyBuffersAndQuit(dirty, index+1)
		return
	}
	if current.Path() == "" {
		a.promptMinibuffer("Save "+current.Name()+" as", func(path string) {
			if !filepath.IsAbs(path) {
				path = filepath.Join(a.root, path)
			}
			if err := a.saveBufferForQuit(current, path); err != nil {
				a.message = err.Error()
				return
			}
			a.saveDirtyBuffersAndQuit(dirty, index+1)
		})
		return
	}
	if err := a.saveBufferForQuit(current, ""); err != nil {
		a.message = err.Error()
		return
	}
	a.saveDirtyBuffersAndQuit(dirty, index+1)
}

func (a *App) saveBufferForQuit(current *buffer.Buffer, path string) error {
	previousPath := current.Path()
	if err := current.Save(path); err != nil {
		return err
	}
	if current.Path() != previousPath {
		a.editorBufferFor(current).mode = a.modeForPath(current.Path())
	}
	a.editorBufferFor(current).highlighter = a.highlighterForBuffer(current)
	if current == a.current() {
		a.runHooks("save")
	}
	return nil
}

func (a *App) finishQuit() {
	a.closeWorkspaceSearch()
	for _, current := range a.buffers {
		if current.terminal != nil && current.terminal.cancel != nil {
			current.terminal.cancel()
		}
	}
	a.running = false
	if a.lspCancel != nil {
		a.lspCancel()
	}
	if a.dapCancel != nil {
		a.dapCancel()
	}
}

func (a *App) closeBuffer(arguments string) error {
	current := a.current()
	if a.currentEditorBuffer().terminal == nil && current.IsDirty() && arguments != "force" {
		a.confirmBufferClose(current)
		return nil
	}
	a.closeBufferNow(current)
	return nil
}

func (a *App) confirmBufferClose(current *buffer.Buffer) {
	items := []paletteItem{
		{label: "Save and close", detail: current.Name(), value: "save"},
		{label: "Discard changes", detail: "cannot be undone", value: "discard"},
		{label: "Cancel", value: "cancel"},
	}
	a.choose("Save changes before closing?", items, func(item paletteItem) {
		switch item.value {
		case "save":
			a.saveAndCloseBuffer(current)
		case "discard":
			a.closeBufferNow(current)
		}
	})
}

func (a *App) saveAndCloseBuffer(current *buffer.Buffer) {
	if current.Path() == "" {
		a.promptMinibuffer("Save as", func(path string) {
			if !filepath.IsAbs(path) {
				path = filepath.Join(a.root, path)
			}
			if err := a.saveBufferBeforeClose(current, path); err != nil {
				a.message = err.Error()
			}
		})
		return
	}
	if err := a.saveBufferBeforeClose(current, ""); err != nil {
		a.message = err.Error()
	}
}

func (a *App) saveBufferBeforeClose(current *buffer.Buffer, path string) error {
	previousPath := current.Path()
	if err := current.Save(path); err != nil {
		return err
	}
	if current.Path() != previousPath {
		a.editorBufferFor(current).mode = a.modeForPath(current.Path())
	}
	a.editorBufferFor(current).highlighter = a.highlighterForBuffer(current)
	a.runHooks("save")
	a.closeBufferNow(current)
	return nil
}

func (a *App) closeBufferNow(removed *buffer.Buffer) {
	index := -1
	for currentIndex, current := range a.buffers {
		if current.text == removed {
			index = currentIndex
			break
		}
	}
	if index < 0 {
		return
	}
	a.notifyLSPDidClose(removed)
	if terminalBuffer := a.editorBufferFor(removed).terminal; terminalBuffer != nil && terminalBuffer.cancel != nil {
		terminalBuffer.cancel()
	}
	a.buffers = append(a.buffers[:index], a.buffers[index+1:]...)
	if index < a.active {
		a.active--
	}
	if len(a.buffers) == 0 {
		a.addBuffer(buffer.New())
	}
	a.active = min(a.active, len(a.buffers)-1)
	a.topLine = 0
	a.leftColumn = 0
	a.message = "closed " + removed.Name()
	a.activateCurrentMode()
}

func (a *App) nextBuffer(arguments string) error {
	a.active = (a.active + 1) % len(a.buffers)
	a.activateCurrentMode()
	a.ensureCursorVisible()
	return nil
}

func (a *App) previousBuffer(arguments string) error {
	a.active = (a.active + len(a.buffers) - 1) % len(a.buffers)
	a.activateCurrentMode()
	a.ensureCursorVisible()
	return nil
}

func (a *App) selectBuffer(arguments string) error {
	number, err := strconv.Atoi(strings.TrimSpace(arguments))
	if err != nil || number < 1 || number > 9 {
		return fmt.Errorf("buffer.select expects a number from 1 to 9")
	}
	if number > len(a.buffers) {
		return fmt.Errorf("buffer %d is not open", number)
	}
	a.active = number - 1
	a.activateCurrentMode()
	a.ensureCursorVisible()
	return nil
}

func (a *App) undo(arguments string) error {
	current := a.current()
	if current.IsReadOnly() {
		return fmt.Errorf("%s is read-only", current.Name())
	}
	if a.currentEditorBuffer().terminal != nil {
		return fmt.Errorf("terminal buffers do not have undo history")
	}
	if !current.Undo() {
		a.message = "nothing to undo"
		return nil
	}
	a.finishHistoryChange(current, "undo")
	return nil
}

func (a *App) redo(arguments string) error {
	current := a.current()
	if current.IsReadOnly() {
		return fmt.Errorf("%s is read-only", current.Name())
	}
	if a.currentEditorBuffer().terminal != nil {
		return fmt.Errorf("terminal buffers do not have undo history")
	}
	if !current.Redo() {
		a.message = "nothing to redo"
		return nil
	}
	a.finishHistoryChange(current, "redo")
	return nil
}

func (a *App) finishHistoryChange(current *buffer.Buffer, operation string) {
	cursors := current.Cursors()
	for index, cursor := range cursors {
		point := current.Point(current.Offset(cursor.Point))
		cursors[index] = buffer.Cursor{Anchor: point, Point: point}
	}
	current.SetCursors(cursors)
	a.editorBufferFor(current).highlighter.Invalidate(0)
	a.notifyLSPFullChange(current)
	a.message = operation
	a.ensureCursorVisible()
}

func (a *App) switchBuffer(arguments string) error {
	items := make([]paletteItem, 0, len(a.buffers))
	for index, current := range a.buffers {
		items = append(items, paletteItem{label: current.text.Name(), detail: current.text.Path(), value: fmt.Sprint(index)})
	}
	a.choose("Switch buffer", items, func(item paletteItem) {
		for index := range a.buffers {
			if item.value == fmt.Sprint(index) {
				a.active = index
				a.activateCurrentMode()
				return
			}
		}
	})
	return nil
}

func (a *App) toggleFiles(arguments string) error {
	if !a.showFiles {
		a.CloseSidebar()
		a.closeWorkspaceSearch()
		a.syncFileBrowser()
		a.showFiles = true
		a.browser.focused = true
		return nil
	}
	if !a.browser.focused {
		a.browser.focused = true
		return nil
	}
	a.closeFileBrowser()
	return nil
}

func (a *App) addCursorBelow(arguments string) error {
	current := a.current()
	cursors := current.Cursors()
	last := cursors[len(cursors)-1].Point
	if last.Line+1 >= current.LineCount() {
		return nil
	}
	point := current.Point(current.Offset(buffer.Point{Line: last.Line + 1, Column: last.Column}))
	cursors = append(cursors, buffer.Cursor{Anchor: point, Point: point})
	current.SetCursors(cursors)
	a.message = fmt.Sprintf("%d cursors", len(cursors))
	return nil
}

func (a *App) addCursorAtNextMatch(arguments string) error {
	current := a.current()
	cursors := current.Cursors()
	if len(cursors) == 0 {
		return nil
	}
	firstStart := min(current.Offset(cursors[0].Anchor), current.Offset(cursors[0].Point))
	firstEnd := max(current.Offset(cursors[0].Anchor), current.Offset(cursors[0].Point))
	if firstStart == firstEnd {
		a.message = "select text before adding its next match"
		return nil
	}
	needle := current.Slice(firstStart, firstEnd)
	searchStart := firstEnd
	selectedOffsets := make(map[int]bool, len(cursors))
	selectedOffsets[firstStart] = true
	for _, cursor := range cursors[1:] {
		start := min(current.Offset(cursor.Anchor), current.Offset(cursor.Point))
		end := max(current.Offset(cursor.Anchor), current.Offset(cursor.Point))
		searchStart = max(searchStart, end)
		if bytes.Equal(current.Slice(start, end), needle) {
			selectedOffsets[start] = true
		}
	}
	content := current.Bytes()
	match := nextUnselectedMatch(content, needle, searchStart, len(content), selectedOffsets)
	if match < 0 {
		match = nextUnselectedMatch(content, needle, 0, searchStart, selectedOffsets)
	}
	if match < 0 {
		a.message = "no other match"
		return nil
	}
	start := current.Point(match)
	end := current.Point(match + len(needle))
	cursors = append(cursors, buffer.Cursor{Anchor: start, Point: end})
	current.SetCursors(cursors)
	a.message = fmt.Sprintf("%d cursors", len(cursors))
	a.ensureCursorVisible()
	return nil
}

func nextUnselectedMatch(content, needle []byte, start, end int, selected map[int]bool) int {
	start = min(max(0, start), len(content))
	end = min(max(start, end), len(content))
	for start < end {
		relative := bytes.Index(content[start:end], needle)
		if relative < 0 {
			return -1
		}
		match := start + relative
		if !selected[match] {
			return match
		}
		start = match + max(1, len(needle))
	}
	return -1
}

func (a *App) moveToLineStart(arguments string) error {
	a.moveLineEdge(false, arguments == "select")
	a.ensureCursorVisible()
	return nil
}

func (a *App) moveToLineEnd(arguments string) error {
	a.moveLineEdge(true, arguments == "select")
	a.ensureCursorVisible()
	return nil
}

func (a *App) moveToFileStart(arguments string) error {
	a.moveDocumentEdge(false, arguments == "select")
	a.ensureCursorVisible()
	return nil
}

func (a *App) moveToFileEnd(arguments string) error {
	a.moveDocumentEdge(true, arguments == "select")
	a.ensureCursorVisible()
	return nil
}

func (a *App) pageUp(arguments string) error {
	a.movePage(false, arguments == "select")
	a.ensureCursorVisible()
	return nil
}

func (a *App) pageDown(arguments string) error {
	a.movePage(true, arguments == "select")
	a.ensureCursorVisible()
	return nil
}

func (a *App) movePage(down, extend bool) {
	_, height := a.screen.Size()
	delta := max(1, height-3)
	if !down {
		delta = -delta
	}
	a.moveCursors(0, delta, extend)
}

func (a *App) searchBuffer(arguments string) error {
	current := a.current()
	items := make([]paletteItem, 0, current.LineCount())
	for line := range current.LineCount() {
		text := string(current.Line(line))
		if strings.TrimSpace(text) == "" {
			continue
		}
		items = append(items, paletteItem{
			label:  text,
			detail: fmt.Sprintf("line %d", line+1),
			value:  fmt.Sprint(line),
		})
	}
	a.choose("Search buffer", items, func(item paletteItem) {
		line, _ := strconv.Atoi(item.value)
		point := buffer.Point{Line: line}
		current.SetCursors([]buffer.Cursor{{Anchor: point, Point: point}})
		a.ensureCursorVisible()
	})
	return nil
}

func (a *App) searchProject(arguments string) error {
	a.openWorkspaceSearch(arguments)
	return nil
}

func (a *App) showCompletion(arguments string) error {
	if a.hasLanguageServerForCurrentMode() {
		a.requestLSPCompletion()
		return nil
	}
	return a.showLocalCompletion()
}

func (a *App) showLocalCompletion() error {
	current := a.current()
	point := current.Cursors()[0].Point
	line := []rune(string(current.Line(point.Line)))
	column := min(point.Column, len(line))
	start := column
	for start > 0 && (unicode.IsLetter(line[start-1]) || unicode.IsDigit(line[start-1]) || line[start-1] == '_') {
		start--
	}
	prefix := string(line[start:column])
	words := make(map[string]struct{})
	for _, field := range strings.FieldsFunc(string(current.Bytes()), func(value rune) bool {
		return !unicode.IsLetter(value) && !unicode.IsDigit(value) && value != '_'
	}) {
		if strings.HasPrefix(field, prefix) && field != prefix {
			words[field] = struct{}{}
		}
	}
	items := make([]paletteItem, 0, len(words))
	for word := range words {
		items = append(items, paletteItem{label: word, value: word})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].label < items[j].label })
	a.chooseCompletion("Complete", items, true, func(item paletteItem) {
		a.completeWord(item.value)
	})
	return nil
}

func (a *App) completeWord(value string) {
	current := a.current()
	point := current.Cursors()[0].Point
	line := []rune(string(current.Line(point.Line)))
	column := min(point.Column, len(line))
	start := column
	for start > 0 && (unicode.IsLetter(line[start-1]) || unicode.IsDigit(line[start-1]) || line[start-1] == '_') {
		start--
	}
	end := current.Offset(point)
	begin := current.Offset(buffer.Point{Line: point.Line, Column: start})
	change := newTextChange(current, current.Point(begin), current.Point(end), value)
	current.BeginTransaction()
	current.Delete(begin, end)
	current.Insert(begin, []byte(value))
	current.EndTransaction()
	updated := current.Point(begin + len(value))
	current.SetCursors([]buffer.Cursor{{Anchor: updated, Point: updated}})
	a.editorBufferFor(current).highlighter.Invalidate(point.Line)
	a.notifyLSPChanges(current, []textChange{change})
}

func (a *App) shellRun(arguments string) error {
	if arguments == "" {
		a.prompt("Run shell command", func(command string) {
			if err := a.shellRun(command); err != nil {
				a.message = err.Error()
			}
		})
		return nil
	}
	output, err := runCommand(a.root, "/bin/sh", "-c", arguments)
	a.showOutput("Shell", output)
	return err
}

func (a *App) shellExec(arguments string) error {
	if arguments == "" {
		return fmt.Errorf("shell.exec requires a command")
	}
	output, err := runCommand(a.root, "/bin/sh", "-c", arguments)
	if output != "" {
		a.message = truncate(output, 120)
	}
	return err
}

func (a *App) showOutput(title, output string) {
	items := make([]paletteItem, 0)
	if output == "" {
		output = "(no output)"
	}
	for _, line := range strings.Split(output, "\n") {
		items = append(items, paletteItem{label: line})
	}
	a.choose(title, items, func(item paletteItem) {})
}
