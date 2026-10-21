// Package search provides persistent workspace search through the plugin API.
package search

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

type searchPlugin struct {
	host              plugin.Host
	root              string
	ctx               context.Context
	cancel            context.CancelFunc
	mu                sync.Mutex
	query             string
	results           []result
	handle            ui.ViewHandle
	versions          map[string]fileVersion
	documents         map[string]string
	searchedDocuments map[string]string
	generation        uint64
	scanCancel        context.CancelFunc
	err               error
	closed            bool
}

type fileVersion struct {
	modified time.Time
	size     int64
}

// New returns a workspace search plugin. History belongs to this instance.
func New() plugin.Plugin             { return &searchPlugin{} }
func (g *searchPlugin) Name() string { return "search" }
func (g *searchPlugin) Load(host plugin.Host) error {
	g.host = host
	g.root = host.Root()
	g.ctx, g.cancel = context.WithCancel(context.Background())
	if err := host.RegisterGlobalCommand("search.project", g.open); err != nil {
		return err
	}
	return host.RegisterBinding("ctrl-shift-f", "search.project")
}

func (g *searchPlugin) open(query string) error {
	g.captureDocuments()
	g.mu.Lock()
	if query != "" && query != g.query {
		g.changeQuery(query)
	}
	view := g.view()
	g.mu.Unlock()
	view.FocusPane = "results"
	if g.handle != nil && g.handle.Update(view) {
		g.handle.Show()
		return nil
	}
	g.handle = g.host.NewView(view)
	return nil
}

func (g *searchPlugin) changeQuery(query string) {
	g.query = query
	g.results = nil
	g.versions = nil
	g.err = nil
	g.generation++
	if g.scanCancel != nil {
		g.scanCancel()
		g.scanCancel = nil
	}
}

func (g *searchPlugin) input(query string) (ui.View, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.changeQuery(query)
	return g.view(), nil
}

// captureDocuments runs on the host's event loop; background scans use only
// independent document snapshots and never call into the host.
func (g *searchPlugin) captureDocuments() {
	documents := make(map[string]string)
	for _, document := range g.host.OpenDocuments() {
		if !document.Dirty || document.Path == "" {
			continue
		}
		path, err := filepath.Rel(g.root, document.Path)
		if err != nil || path == ".." || strings.HasPrefix(path, ".."+string(filepath.Separator)) {
			continue
		}
		documents[path] = string(document.Content)
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !maps.Equal(documents, g.documents) {
		g.documents = documents
		g.generation++
		if g.scanCancel != nil {
			g.scanCancel()
			g.scanCancel = nil
		}
	}
}

func (g *searchPlugin) refresh() (ui.View, error) {
	g.mu.Lock()
	query, revision := g.query, g.generation
	documents := maps.Clone(g.documents)
	if g.closed || strings.TrimSpace(query) == "" {
		view := g.view()
		g.mu.Unlock()
		return view, nil
	}
	g.mu.Unlock()
	files, versions, err := workspaceFiles(g.ctx, g.root)
	g.mu.Lock()
	if g.closed || revision != g.generation {
		view := g.view()
		g.mu.Unlock()
		return view, nil
	}
	if err != nil {
		g.err = err
		view := g.view()
		g.mu.Unlock()
		return view, nil
	}
	if g.versions != nil && maps.Equal(versions, g.versions) && maps.Equal(documents, g.searchedDocuments) {
		view := g.view()
		g.mu.Unlock()
		return view, nil
	}
	if g.scanCancel != nil {
		g.scanCancel()
	}
	ctx, cancel := context.WithCancel(g.ctx)
	g.scanCancel = cancel
	g.generation++
	revision = g.generation
	g.mu.Unlock()
	defer cancel()
	results, err := searchWorkspace(ctx, g.root, files, query)
	if err == nil && len(documents) > 0 {
		filtered := results[:0]
		for _, entry := range results {
			if _, overridden := documents[entry.path]; !overridden {
				filtered = append(filtered, entry)
			}
		}
		results = filtered
		for path, content := range documents {
			results = append(results, searchWorkspaceDocument(path, []byte(content), query)...)
		}
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].path != results[j].path {
			return results[i].path < results[j].path
		}
		return results[i].line < results[j].line
	})
	if len(results) > resultLimit {
		results = results[:resultLimit]
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closed && revision == g.generation && !errors.Is(err, context.Canceled) {
		g.results = results
		g.versions = versions
		g.searchedDocuments = documents
		g.err = err
		g.scanCancel = nil
	}
	return g.view(), nil
}

// view is called with the plugin mutex held.
func (g *searchPlugin) view() ui.View {
	sections := make([]ui.ListSection, 0)
	previous := ""
	for _, entry := range g.results {
		if len(sections) == 0 || entry.path != previous {
			sections = append(sections, ui.ListSection{Title: entry.path})
			previous = entry.path
		}
		sections[len(sections)-1].Items = append(sections[len(sections)-1].Items, ui.ListItem{
			Label: fmt.Sprintf("%d %s", entry.line+1, strings.TrimSpace(entry.text)), Value: fmt.Sprintf("%s:%d:%d", entry.path, entry.line, entry.byteColumn), Data: entry.text,
		})
	}
	message := ""
	if len(sections) == 0 {
		if strings.TrimSpace(g.query) != "" {
			message = "Searching…"
			if g.versions != nil {
				message = "No matches"
			}
		}
		if g.err != nil {
			message = g.err.Error()
		}
	}
	title := "Search · " + filepath.Base(g.root)
	if len(g.results) > 0 {
		title += fmt.Sprintf(" · %d", len(g.results))
	}
	return ui.View{
		Title: "Search",
		Layout: ui.Layout{Direction: ui.LayoutHorizontal, Panes: []ui.Pane{
			{ID: "results", Title: title, Weight: 1, List: &ui.List{
				Input: &ui.TextInput{Value: g.query, Placeholder: "type to search…"}, OnInput: g.input,
				Sections: sections, Message: message,
				Help: []ui.KeyHelp{{Key: "↑↓", Label: "move"}}, OnAction: g.activate,
				PreviewPane: "preview", OnSelect: g.preview,
			}},
			{ID: "preview", Title: "Preview", Weight: 2},
		}},
		BeforeRefresh: g.captureDocuments, RefreshInterval: 500 * time.Millisecond, OnRefresh: g.refresh,
	}
}

func (g *searchPlugin) preview(item ui.ListItem) (ui.Widget, error) {
	g.mu.Lock()
	target, found := g.findResult(item.Value)
	query := g.query
	content, unsaved := g.documents[target.path]
	closed := g.closed
	g.mu.Unlock()
	if !found || closed {
		return ui.WidgetContent{}, nil
	}
	if !unsaved {
		data, err := os.ReadFile(filepath.Join(g.root, target.path))
		if err != nil {
			return nil, fmt.Errorf("preview %s: %w", target.path, err)
		}
		content = string(data)
	}
	lines := strings.Split(strings.TrimSuffix(strings.ReplaceAll(content, "\r\n", "\n"), "\n"), "\n")
	line := min(target.line, len(lines)-1)
	column := min(target.byteColumn, len(lines[line]))
	preview := ui.WidgetContent{
		Title: fmt.Sprintf("%s:%d", target.path, line+1), FocusLine: line,
		Lines: make([]ui.RichTextLine, len(lines)),
	}
	for _, value := range lines[line][:column] {
		if value == '\t' {
			preview.FocusColumn += 4 - preview.FocusColumn%4
		} else {
			preview.FocusColumn += terminal.RuneWidth(value)
		}
	}
	for index, text := range lines {
		preview.Lines[index].Spans = []ui.RichTextSpan{{Text: text}}
		if index == line {
			end := min(column+len(query), len(text))
			preview.Lines[index].Style = ui.WidgetStyle{Background: ui.ToneAccent, BackgroundIntensity: 15}
			preview.Lines[index].Spans = []ui.RichTextSpan{
				{Text: text[:column]},
				{Text: text[column:end], Style: ui.WidgetStyle{Foreground: ui.TonePanel, Background: ui.ToneAccent, Bold: true}},
				{Text: text[end:]},
			}
		}
	}
	return preview, nil
}

// findResult is called with the plugin mutex held.
func (g *searchPlugin) findResult(value string) (result, bool) {
	for _, entry := range g.results {
		if fmt.Sprintf("%s:%d:%d", entry.path, entry.line, entry.byteColumn) == value {
			return entry, true
		}
	}
	return result{}, false
}

func (g *searchPlugin) activate(action ui.Action, item ui.ListItem) error {
	if action != ui.Activate {
		return nil
	}
	g.mu.Lock()
	target, found := g.findResult(item.Value)
	g.mu.Unlock()
	if !found {
		return nil
	}
	column := utf8.RuneCountInString(target.text[:min(target.byteColumn, len(target.text))])
	return g.host.OpenLocation(target.path, plugin.Position{Line: target.line, Column: column})
}

// Close cancels this plugin's searches when the host releases its plugins.
func (g *searchPlugin) Close() error {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.closed = true
	g.generation++
	if g.cancel != nil {
		g.cancel()
	}
	if g.scanCancel != nil {
		g.scanCancel()
		g.scanCancel = nil
	}
	return nil
}

func workspaceFiles(ctx context.Context, root string) ([]string, map[string]fileVersion, error) {
	files := make([]string, 0)
	versions := make(map[string]fileVersion)
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil {
			if path == root {
				return err
			}
			return nil
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" || entry.Name() == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return nil
		}
		if info, err := entry.Info(); err == nil {
			files = append(files, relative)
			versions[relative] = fileVersion{modified: info.ModTime(), size: info.Size()}
		}
		if len(files) >= 10000 {
			return filepath.SkipAll
		}
		return nil
	})
	sort.Strings(files)
	return files, versions, err
}
