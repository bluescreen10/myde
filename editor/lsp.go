package editor

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"
	"unicode/utf16"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/protocol"
)

type lspCompletion struct {
	label         string
	detail        string
	documentation string
	kind          string
	resolve       json.RawMessage
	edits         []lspEdit
}

type completionDetails struct {
	buffer        *buffer.Buffer
	revision      uint64
	epoch         uint64
	value         string
	detail        string
	documentation string
}

type lspEdit struct {
	start buffer.Point
	end   buffer.Point
	text  string
}

type definitionTarget struct {
	path      string
	line      int
	character int
}

func (a *App) lspStart(arguments string) error {
	mode := a.currentEditorBuffer().mode
	if arguments == "" {
		if registered := a.modes[mode]; registered != nil && registered.languageServerProgram().Command != "" {
			return a.StartLanguageServer(mode)
		}
		a.prompt("Language server command", func(command string) {
			if err := a.runLanguageServerCommand(mode, command); err != nil {
				a.message = err.Error()
			}
		})
		return nil
	}
	return a.runLanguageServerCommand(mode, arguments)
}

func (a *App) runLanguageServerCommand(mode, arguments string) error {
	command, commandArguments, err := splitCommand(arguments)
	if err != nil {
		return err
	}
	return a.RunLanguageServer(mode, plugin.Program{Command: command, Arguments: commandArguments})
}

func (a *App) lspRestart(arguments string) error {
	if arguments != "" {
		return a.runLanguageServerCommand(a.currentEditorBuffer().mode, arguments)
	}
	return a.RestartLanguageServer(a.currentEditorBuffer().mode)
}

func (a *App) lspStop(arguments string) error {
	if arguments == "" {
		arguments = a.currentEditorBuffer().mode
	}
	return a.StopLanguageServer(arguments)
}

func (a *App) lspLog(arguments string) error {
	if arguments == "" {
		arguments = a.currentEditorBuffer().mode
	}
	mode, err := a.registeredMode(arguments)
	if err != nil {
		return err
	}
	if mode.languageServer == nil {
		return fmt.Errorf("no language server log for %s", mode.Name)
	}
	content := "Workspace: " + a.root + "\nServer: " + mode.serverProgram.Command + "\n\n" + strings.Join(mode.languageServer.messages, "\n\n")
	a.OpenReadOnlyBuffer(mode.Name+" language server log", []byte(content))
	return nil
}

func languageServerRequest(root string, method string, params json.RawMessage) (any, error) {
	switch method {
	case "workspace/configuration":
		var request struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(params, &request); err != nil {
			return nil, err
		}
		return make([]any, len(request.Items)), nil
	case "workspace/workspaceFolders":
		return []map[string]string{{"uri": fileURI(root), "name": filepath.Base(root)}}, nil
	case "client/registerCapability", "client/unregisterCapability", "window/workDoneProgress/create", "window/showMessageRequest":
		return nil, nil
	default:
		return nil, fmt.Errorf("unsupported client request: %s", method)
	}
}

func (a *App) startLanguageServer(program plugin.Program, mode *editingMode) error {
	command, err := exec.LookPath(program.Command)
	if err != nil {
		return fmt.Errorf("find %s: %w", program.Command, err)
	}
	a.clearLanguageServer(mode)
	ctx, cancel := context.WithCancel(context.Background())
	server, err := protocol.StartWithOptions(ctx, command, protocol.ProcessOptions{
		Directory: a.root,
		OnRequest: func(method string, params json.RawMessage) (any, error) {
			return languageServerRequest(a.root, method, params)
		},
	}, program.Arguments...)
	if err != nil {
		cancel()
		return err
	}
	session := &languageServerSession{mode: mode, process: server, cancel: cancel}
	mode.languageServer = session
	mode.serverStopped = false
	mode.serverProgram = plugin.Program{Command: program.Command, Arguments: append([]string(nil), program.Arguments...)}

	requestContext, requestCancel := context.WithTimeout(ctx, 5*time.Second)
	defer requestCancel()
	var initialized struct {
		Capabilities struct {
			TextDocumentSync   json.RawMessage `json:"textDocumentSync"`
			CompletionProvider struct {
				ResolveProvider bool `json:"resolveProvider"`
			} `json:"completionProvider"`
		} `json:"capabilities"`
	}
	err = server.Request(requestContext, "initialize", map[string]any{
		"processId":        nil,
		"rootUri":          fileURI(a.root),
		"workspaceFolders": []map[string]string{{"uri": fileURI(a.root), "name": filepath.Base(a.root)}},
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"publishDiagnostics": map[string]any{},
				"completion": map[string]any{
					"completionItem": map[string]any{
						"snippetSupport":      false,
						"documentationFormat": []string{"markdown", "plaintext"},
						"resolveSupport": map[string]any{
							"properties": []string{"documentation", "detail"},
						},
					},
				},
				"definition": map[string]any{},
			},
		},
	}, &initialized)
	if err != nil {
		a.clearLanguageServer(mode)
		return err
	}
	session.syncKind = synchronizationKind(initialized.Capabilities.TextDocumentSync)
	session.completionResolve = initialized.Capabilities.CompletionProvider.ResolveProvider
	if err := server.Notify("initialized", map[string]any{}); err != nil {
		a.clearLanguageServer(mode)
		return err
	}
	for _, current := range a.buffers {
		if current.mode == modeKey(mode.Name) {
			a.notifyLSPDidOpen(current.text)
		}
	}
	go a.readLSPEvents(session)
	a.message = mode.Name + " language server started: " + command
	return nil
}

func (a *App) notifyLSPDidOpen(current *buffer.Buffer) {
	editorBuffer := a.editorBufferFor(current)
	session := a.languageServerForBuffer(current)
	if editorBuffer == nil || session == nil || current.Path() == "" || editorBuffer.lsp == session && editorBuffer.lspURI == fileURI(current.Path()) {
		return
	}
	if editorBuffer.lsp != nil {
		a.notifyLSPDidClose(current)
	}
	mode := a.modeForBuffer(current)
	languageID := mode.LanguageID
	if languageID == "" {
		languageID = mode.Syntax
	}
	err := session.process.Notify("textDocument/didOpen", map[string]any{
		"textDocument": map[string]any{"uri": fileURI(current.Path()), "languageId": languageID, "version": current.Revision(), "text": string(current.Bytes())},
	})
	if err != nil {
		a.message = "open language server document: " + err.Error()
		return
	}
	editorBuffer.lsp = session
	editorBuffer.lspURI = fileURI(current.Path())
}

func (a *App) notifyLSPDidClose(current *buffer.Buffer) {
	editorBuffer := a.editorBufferFor(current)
	if editorBuffer == nil {
		return
	}
	editorBuffer.diagnostics = nil
	session := editorBuffer.lsp
	uri := editorBuffer.lspURI
	editorBuffer.lsp = nil
	editorBuffer.lspURI = ""
	if session == nil || uri == "" {
		return
	}
	_ = session.process.Notify("textDocument/didClose", map[string]any{"textDocument": map[string]any{"uri": uri}})
}

type textChange struct {
	start textPosition
	end   textPosition
	text  string
}

type textPosition struct {
	line      int
	character int
}

func newTextChange(current *buffer.Buffer, start, end buffer.Point, text string) textChange {
	return textChange{
		start: protocolPosition(current, start),
		end:   protocolPosition(current, end),
		text:  text,
	}
}

func (a *App) notifyLSPChanges(current *buffer.Buffer, changes []textChange) {
	session := a.languageServerForBuffer(current)
	editorBuffer := a.editorBufferFor(current)
	if editorBuffer == nil || session == nil || current.Path() == "" || editorBuffer.lsp != session {
		return
	}
	contentChanges := []map[string]any{{"text": string(current.Bytes())}}
	if session.syncKind == 2 {
		contentChanges = make([]map[string]any, 0, len(changes))
		for _, change := range changes {
			contentChanges = append(contentChanges, map[string]any{
				"range": map[string]any{
					"start": map[string]int{"line": change.start.line, "character": change.start.character},
					"end":   map[string]int{"line": change.end.line, "character": change.end.character},
				},
				"text": change.text,
			})
		}
	}
	_ = session.process.Notify("textDocument/didChange", map[string]any{
		"textDocument": map[string]any{
			"uri":     fileURI(current.Path()),
			"version": current.Revision(),
		},
		"contentChanges": contentChanges,
	})
}

func (a *App) notifyLSPFullChange(current *buffer.Buffer) {
	session := a.languageServerForBuffer(current)
	editorBuffer := a.editorBufferFor(current)
	if editorBuffer == nil || session == nil || current.Path() == "" || editorBuffer.lsp != session {
		return
	}
	_ = session.process.Notify("textDocument/didChange", map[string]any{
		"textDocument": map[string]any{
			"uri":     fileURI(current.Path()),
			"version": current.Revision(),
		},
		"contentChanges": []map[string]any{{"text": string(current.Bytes())}},
	})
}

func synchronizationKind(value json.RawMessage) int {
	var number int
	if json.Unmarshal(value, &number) == nil && number != 0 {
		return number
	}
	var options struct {
		Change int `json:"change"`
	}
	if json.Unmarshal(value, &options) == nil && options.Change != 0 {
		return options.Change
	}
	return 1
}

func protocolPosition(current *buffer.Buffer, point buffer.Point) textPosition {
	line := []rune(string(current.Line(point.Line)))
	column := min(point.Column, len(line))
	return textPosition{
		line:      point.Line,
		character: len(utf16.Encode(line[:column])),
	}
}

func (a *App) requestLSPCompletion() {
	session := a.languageServerForBuffer(a.current())
	if session == nil || a.current().Path() == "" {
		return
	}
	a.completionEpoch++
	server := session.process
	current := a.current()
	revision := current.Revision()
	epoch := a.completionEpoch
	point := current.Cursors()[0].Point
	position := protocolPosition(current, point)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var result json.RawMessage
		err := server.Request(ctx, "textDocument/completion", map[string]any{
			"textDocument": map[string]any{"uri": fileURI(current.Path())},
			"position":     map[string]any{"line": position.line, "character": position.character},
		}, &result)
		if err != nil {
			a.servers <- serverEvent{
				lspSource:          session,
				message:            "completion: " + err.Error(),
				completionReady:    true,
				completionBuffer:   current,
				completionRevision: revision,
				completionPoint:    point,
				completionEpoch:    epoch,
			}
			return
		}
		a.servers <- serverEvent{
			lspSource:          session,
			completions:        parseCompletions(result, current, point),
			completionReady:    true,
			completionBuffer:   current,
			completionRevision: revision,
			completionPoint:    point,
			completionEpoch:    epoch,
		}
	}()
}

func (a *App) resolveSelectedCompletion() {
	session := a.languageServerForBuffer(a.current())
	if session == nil || !session.completionResolve || a.palette == nil || !a.palette.completion ||
		a.palette.completionPending || len(a.palette.filtered) == 0 {
		return
	}
	item := a.palette.filtered[a.palette.selected]
	if item.resolved || len(item.resolve) == 0 {
		return
	}
	markCompletionResolved(a.palette.items, item.value)
	markCompletionResolved(a.palette.filtered, item.value)

	server := session.process
	current := a.current()
	details := completionDetails{
		buffer:   current,
		revision: current.Revision(),
		epoch:    a.completionEpoch,
		value:    item.value,
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		var resolved completionWire
		if err := server.Request(ctx, "completionItem/resolve", item.resolve, &resolved); err != nil {
			return
		}
		details.detail = resolved.Detail
		details.documentation = completionDocumentation(resolved.Documentation)
		a.servers <- serverEvent{lspSource: session, completionDetails: &details}
	}()
}

func markCompletionResolved(items []paletteItem, value string) {
	for index := range items {
		if items[index].value == value {
			items[index].resolved = true
		}
	}
}

func (a *App) applyCompletionDetails(details completionDetails) {
	if a.palette == nil || !a.palette.completion || details.buffer != a.current() ||
		details.revision != details.buffer.Revision() || details.epoch != a.completionEpoch {
		return
	}
	updateCompletionDetails(a.palette.items, details)
	updateCompletionDetails(a.palette.filtered, details)
}

func updateCompletionDetails(items []paletteItem, details completionDetails) {
	for index := range items {
		if items[index].value != details.value {
			continue
		}
		if details.detail != "" {
			items[index].detail = details.detail
		}
		if details.documentation != "" {
			items[index].documentation = details.documentation
		}
	}
}

func (a *App) requestLSPDefinition(arguments string) error {
	a.activateCurrentMode()
	session := a.languageServerForBuffer(a.current())
	if session == nil {
		return fmt.Errorf("no language server is running; use lsp.restart")
	}
	current := a.current()
	if current.Path() == "" {
		return fmt.Errorf("save the buffer before requesting a definition")
	}
	server := session.process
	point := current.Cursors()[0].Point
	position := protocolPosition(current, point)
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var result json.RawMessage
		err := server.Request(ctx, "textDocument/definition", map[string]any{
			"textDocument": map[string]any{"uri": fileURI(current.Path())},
			"position":     map[string]any{"line": position.line, "character": position.character},
		}, &result)
		if err != nil {
			a.servers <- serverEvent{lspSource: session, message: "definition: " + err.Error()}
			return
		}
		target, ok := parseDefinition(result)
		if !ok {
			a.servers <- serverEvent{lspSource: session, message: "definition not found"}
			return
		}
		a.servers <- serverEvent{lspSource: session, definition: &target}
	}()
	return nil
}

func (a *App) readLSPEvents(session *languageServerSession) {
	server := session.process
	defer func() { a.servers <- serverEvent{lspStopped: session} }()
	for event := range server.Events() {
		if event.Method == "window/showMessage" || event.Method == "window/logMessage" {
			var message languageServerMessage
			if json.Unmarshal(event.Params, &message) == nil && message.Message != "" {
				a.servers <- serverEvent{lspSource: session, lspMessage: &message}
			}
			continue
		}
		if event.Method != "textDocument/publishDiagnostics" {
			continue
		}
		var params struct {
			URI         string `json:"uri"`
			Diagnostics []struct {
				Range struct {
					Start struct {
						Line      int `json:"line"`
						Character int `json:"character"`
					} `json:"start"`
					End struct {
						Line      int `json:"line"`
						Character int `json:"character"`
					} `json:"end"`
				} `json:"range"`
				Severity int             `json:"severity"`
				Code     json.RawMessage `json:"code"`
				Source   string          `json:"source"`
				Message  string          `json:"message"`
			} `json:"diagnostics"`
		}
		if json.Unmarshal(event.Params, &params) != nil {
			continue
		}
		path := pathFromURI(params.URI)
		diagnostics := make([]diagnostic, 0, len(params.Diagnostics))
		for _, item := range params.Diagnostics {
			diagnostics = append(diagnostics, diagnostic{
				line:      item.Range.Start.Line,
				column:    item.Range.Start.Character,
				endLine:   item.Range.End.Line,
				endColumn: item.Range.End.Character,
				severity:  item.Severity,
				code:      diagnosticCode(item.Code),
				source:    item.Source,
				message:   item.Message,
			})
		}
		a.servers <- serverEvent{lspSource: session, path: path, diagnostics: diagnostics}
	}
}

type lspRange struct {
	Start struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	} `json:"start"`
	End struct {
		Line      int `json:"line"`
		Character int `json:"character"`
	} `json:"end"`
}

type completionWire struct {
	Label         string          `json:"label"`
	Detail        string          `json:"detail"`
	Documentation json.RawMessage `json:"documentation"`
	Kind          int             `json:"kind"`
	InsertText    string          `json:"insertText"`
	TextEdit      *struct {
		Range   lspRange `json:"range"`
		NewText string   `json:"newText"`
	} `json:"textEdit"`
	AdditionalTextEdits []struct {
		Range   lspRange `json:"range"`
		NewText string   `json:"newText"`
	} `json:"additionalTextEdits"`
}

func parseCompletions(result json.RawMessage, current *buffer.Buffer, point buffer.Point) []lspCompletion {
	var rawItems []json.RawMessage
	if json.Unmarshal(result, &rawItems) != nil {
		var wrapped struct {
			Items []json.RawMessage `json:"items"`
		}
		if json.Unmarshal(result, &wrapped) != nil {
			return nil
		}
		rawItems = wrapped.Items
	}
	completions := make([]lspCompletion, 0, len(rawItems))
	for _, rawItem := range rawItems {
		var item completionWire
		if json.Unmarshal(rawItem, &item) != nil {
			continue
		}
		completion := lspCompletion{
			label:         item.Label,
			detail:        item.Detail,
			documentation: completionDocumentation(item.Documentation),
			kind:          completionKind(item.Kind),
			resolve:       rawItem,
		}
		if item.TextEdit != nil {
			completion.edits = append(completion.edits, editFromLSP(current, item.TextEdit.Range, item.TextEdit.NewText))
		} else {
			text := item.InsertText
			if text == "" {
				text = item.Label
			}
			completion.edits = append(completion.edits, wordCompletionEdit(current, point, text))
		}
		for _, edit := range item.AdditionalTextEdits {
			completion.edits = append(completion.edits, editFromLSP(current, edit.Range, edit.NewText))
		}
		completions = append(completions, completion)
	}
	return completions
}

func completionDocumentation(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return text
	}
	var markup struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(value, &markup) == nil {
		return markup.Value
	}
	return ""
}

func completionKind(kind int) string {
	names := map[int]string{
		1: "text", 2: "method", 3: "function", 4: "constructor", 5: "field",
		6: "variable", 7: "class", 8: "interface", 9: "module", 10: "property",
		11: "unit", 12: "value", 13: "enum", 14: "keyword", 15: "snippet",
		16: "color", 17: "file", 18: "reference", 19: "folder", 20: "enum member",
		21: "constant", 22: "struct", 23: "event", 24: "operator", 25: "type parameter",
	}
	return names[kind]
}

func diagnosticCode(value json.RawMessage) string {
	if len(value) == 0 || string(value) == "null" {
		return ""
	}
	var text string
	if json.Unmarshal(value, &text) == nil {
		return text
	}
	var number json.Number
	if json.Unmarshal(value, &number) == nil {
		return number.String()
	}
	return ""
}

func editFromLSP(current *buffer.Buffer, source lspRange, text string) lspEdit {
	return lspEdit{
		start: pointFromLSP(current, source.Start.Line, source.Start.Character),
		end:   pointFromLSP(current, source.End.Line, source.End.Character),
		text:  text,
	}
}

func wordCompletionEdit(current *buffer.Buffer, point buffer.Point, text string) lspEdit {
	line := []rune(string(current.Line(point.Line)))
	column := min(point.Column, len(line))
	start := column
	for start > 0 && (unicode.IsLetter(line[start-1]) || unicode.IsDigit(line[start-1]) || line[start-1] == '_') {
		start--
	}
	return lspEdit{start: buffer.Point{Line: point.Line, Column: start}, end: point, text: text}
}

func pointFromLSP(current *buffer.Buffer, line, character int) buffer.Point {
	runes := []rune(string(current.Line(line)))
	units := 0
	column := 0
	for column < len(runes) && units < character {
		units += len(utf16.Encode([]rune{runes[column]}))
		column++
	}
	return buffer.Point{Line: line, Column: column}
}

type completionOffsetEdit struct {
	start   int
	end     int
	text    []byte
	primary bool
}

func (a *App) applyLSPCompletion(current *buffer.Buffer, completion lspCompletion) {
	if current != a.current() || len(completion.edits) == 0 {
		return
	}
	a.showDiagnostic = false
	edits := make([]completionOffsetEdit, 0, len(completion.edits))
	changedLine := current.LineCount()
	for index, source := range completion.edits {
		start := current.Offset(source.start)
		end := current.Offset(source.end)
		edits = append(edits, completionOffsetEdit{
			start: start, end: end, text: []byte(source.text), primary: index == 0,
		})
		changedLine = min(changedLine, source.start.Line)
	}
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].start > edits[j].start
	})

	current.BeginTransaction()
	for _, change := range edits {
		current.Delete(change.start, change.end)
		current.Insert(change.start, change.text)
	}
	current.EndTransaction()

	for _, change := range edits {
		if !change.primary {
			continue
		}
		final := change.start + len(change.text)
		for _, other := range edits {
			if other.start < change.start {
				final += len(other.text) - (other.end - other.start)
			}
		}
		point := current.Point(final)
		current.SetCursors([]buffer.Cursor{{Anchor: point, Point: point}})
		break
	}
	a.editorBufferFor(current).highlighter.Invalidate(changedLine)
	a.notifyLSPFullChange(current)
	a.ensureCursorVisible()
}

type definitionWire struct {
	URI                  string   `json:"uri"`
	Range                lspRange `json:"range"`
	TargetURI            string   `json:"targetUri"`
	TargetSelectionRange lspRange `json:"targetSelectionRange"`
}

func parseDefinition(result json.RawMessage) (definitionTarget, bool) {
	var locations []definitionWire
	if json.Unmarshal(result, &locations) != nil {
		var location definitionWire
		if json.Unmarshal(result, &location) != nil {
			return definitionTarget{}, false
		}
		locations = []definitionWire{location}
	}
	if len(locations) == 0 {
		return definitionTarget{}, false
	}
	location := locations[0]
	uri := location.URI
	position := location.Range.Start
	if location.TargetURI != "" {
		uri = location.TargetURI
		position = location.TargetSelectionRange.Start
	}
	if uri == "" {
		return definitionTarget{}, false
	}
	return definitionTarget{
		path:      pathFromURI(uri),
		line:      position.Line,
		character: position.Character,
	}, true
}

func (a *App) openDefinition(target definitionTarget) {
	if err := a.open(target.path); err != nil {
		a.message = "open definition: " + err.Error()
		return
	}
	point := pointFromLSP(a.current(), target.line, target.character)
	a.current().SetCursors([]buffer.Cursor{{Anchor: point, Point: point}})
	a.ensureCursorVisible()
}

func fileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}

func pathFromURI(value string) string {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "file" {
		return value
	}
	return filepath.FromSlash(parsed.Path)
}
