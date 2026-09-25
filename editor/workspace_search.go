package editor

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/terminal"
)

const workspaceSearchResultLimit = 1000

type workspaceSearchResult struct {
	path       string
	line       int
	byteColumn int
	text       string
}

type workspaceSearchPanel struct {
	query      []rune
	results    []workspaceSearchResult
	selected   int
	top        int
	searching  bool
	err        error
	generation uint64
	cancel     context.CancelFunc
}

type workspaceSearchRow struct {
	path        string
	resultIndex int
	match       bool
}

func (a *App) openWorkspaceSearch(query string) {
	if a.workspaceSearch != nil && query == "" {
		a.message = ""
		return
	}
	a.CloseSidebar()
	a.closeFileBrowser()
	if a.workspaceSearch == nil {
		a.workspaceSearch = &workspaceSearchPanel{}
	}
	a.workspaceSearch.query = []rune(query)
	a.startWorkspaceSearch()
	a.message = ""
}

func (a *App) closeWorkspaceSearch() {
	if a.workspaceSearch == nil {
		return
	}
	if a.workspaceSearch.cancel != nil {
		a.workspaceSearch.cancel()
	}
	a.workspaceSearch = nil
}

func (a *App) handleWorkspaceSearchEvent(event terminal.Event) error {
	panel := a.workspaceSearch
	switch event.Key {
	case terminal.KeyUp:
		panel.move(-1)
	case terminal.KeyDown:
		panel.move(1)
	case terminal.KeyPageUp:
		_, height := a.screen.Size()
		panel.move(-max(1, (height-7)/2))
	case terminal.KeyPageDown:
		_, height := a.screen.Size()
		panel.move(max(1, (height-7)/2))
	case terminal.KeyHome:
		panel.selected = 0
	case terminal.KeyEnd:
		panel.selected = max(0, len(panel.results)-1)
	case terminal.KeyEnter:
		return a.openWorkspaceSearchResult()
	case terminal.KeyTab:
		a.closeWorkspaceSearch()
	case terminal.KeyBackspace:
		if len(panel.query) > 0 {
			panel.query = panel.query[:len(panel.query)-1]
			a.startWorkspaceSearch()
		}
	case terminal.KeyRune:
		if !event.Control && !event.Alt && !event.Super && event.Rune != 0 {
			panel.query = append(panel.query, event.Rune)
			a.startWorkspaceSearch()
		}
	}
	return nil
}

func (a *App) startWorkspaceSearch() {
	panel := a.workspaceSearch
	if panel == nil {
		return
	}
	if panel.cancel != nil {
		panel.cancel()
		panel.cancel = nil
	}
	panel.generation++
	panel.results = nil
	panel.selected = 0
	panel.top = 0
	panel.err = nil
	query := string(panel.query)
	if strings.TrimSpace(query) == "" {
		panel.searching = false
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	panel.cancel = cancel
	panel.searching = true
	generation := panel.generation
	root := a.root
	files := append([]string(nil), a.files...)
	go func() {
		results, err := searchWorkspace(ctx, root, files, query)
		a.servers <- serverEvent{workspaceSearch: &workspaceSearchEvent{
			panel: panel, generation: generation, results: results, err: err,
		}}
	}()
}

func (a *App) applyWorkspaceSearchEvent(event *workspaceSearchEvent) {
	panel := a.workspaceSearch
	if panel == nil || panel != event.panel || panel.generation != event.generation {
		return
	}
	panel.cancel = nil
	panel.searching = false
	if event.err != nil && !errors.Is(event.err, context.Canceled) {
		panel.err = event.err
		return
	}
	panel.results = event.results
	panel.selected = min(panel.selected, max(0, len(panel.results)-1))
}

func (a *App) openWorkspaceSearchResult() error {
	panel := a.workspaceSearch
	if panel == nil || panel.selected < 0 || panel.selected >= len(panel.results) {
		return nil
	}
	result := panel.results[panel.selected]
	if err := a.open(result.path); err != nil {
		return err
	}
	line := []byte(a.current().Line(result.line))
	byteColumn := min(result.byteColumn, len(line))
	column := utf8.RuneCount(line[:byteColumn])
	point := buffer.Point{Line: result.line, Column: column}
	a.current().SetCursors([]buffer.Cursor{{Anchor: point, Point: point}})
	a.ensureCursorVisible()
	return nil
}

func (p *workspaceSearchPanel) move(delta int) {
	p.selected = min(max(0, p.selected+delta), max(0, len(p.results)-1))
}

func (p *workspaceSearchPanel) rows() []workspaceSearchRow {
	rows := make([]workspaceSearchRow, 0, len(p.results)*2)
	previous := ""
	for index, result := range p.results {
		if result.path != previous {
			rows = append(rows, workspaceSearchRow{path: result.path})
			previous = result.path
		}
		rows = append(rows, workspaceSearchRow{resultIndex: index, match: true})
	}
	return rows
}

func (p *workspaceSearchPanel) ensureVisible(rows []workspaceSearchRow, height int) {
	if height <= 0 {
		p.top = 0
		return
	}
	selectedRow := 0
	for index, row := range rows {
		if row.match && row.resultIndex == p.selected {
			selectedRow = index
			break
		}
	}
	if selectedRow < p.top {
		p.top = selectedRow
	}
	if selectedRow >= p.top+height {
		p.top = selectedRow - height + 1
	}
	p.top = min(max(0, p.top), max(0, len(rows)-height))
}

func (a *App) renderWorkspaceSearch(statusRow int) int {
	width, _ := a.screen.Size()
	sidebarWidth := searchSidebarWidth(width)
	panelHeight := statusRow - 1
	if panelHeight < 2 {
		return sidebarWidth
	}
	panel := a.workspaceSearch
	panelStyle := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Panel}
	border := terminal.Style{Foreground: a.theme.PanelBorder, Background: a.theme.Panel}
	accent := terminal.Style{Foreground: a.theme.Accent, Background: a.theme.Panel, Bold: true}
	selected := terminal.Style{Foreground: a.theme.StatusText, Background: a.theme.Selection}
	title := "Search · " + filepath.Base(a.root)
	if len(panel.results) > 0 {
		title += fmt.Sprintf(" · %d", len(panel.results))
	}
	a.drawPanel(0, 1, sidebarWidth, panelHeight, title, a.theme.Accent, panelStyle, border)
	if panelHeight < 7 {
		return sidebarWidth
	}

	a.screen.Text(1, 2, "› ", accent)
	queryWidth := max(0, sidebarWidth-5)
	if len(panel.query) == 0 {
		a.screen.Text(3, 2, truncate("type to search…", queryWidth), border)
	} else {
		a.screen.Text(3, 2, trailingDisplayText(panel.query, queryWidth), panelStyle)
	}
	a.drawPanelSeparator(0, 3, sidebarWidth, border)

	footerSeparator := panelHeight - 2
	resultHeight := max(0, footerSeparator-4)
	rows := panel.rows()
	panel.ensureVisible(rows, resultHeight)
	if panel.searching {
		a.screen.Text(2, 4, "Searching…", border)
	} else if panel.err != nil {
		a.screen.Text(2, 4, truncate(panel.err.Error(), sidebarWidth-4), terminal.Style{
			Foreground: a.theme.Danger, Background: a.theme.Panel,
		})
	} else if len(panel.query) > 0 && len(panel.results) == 0 {
		a.screen.Text(2, 4, "No matches", border)
	} else {
		for row := 0; row < resultHeight && panel.top+row < len(rows); row++ {
			entry := rows[panel.top+row]
			y := 4 + row
			if !entry.match {
				a.screen.Text(2, y, truncate(entry.path, sidebarWidth-4), accent)
				continue
			}
			result := panel.results[entry.resultIndex]
			style := panelStyle
			if entry.resultIndex == panel.selected {
				style = selected
				fillRow(a.screen, 1, y, sidebarWidth-2, style)
			}
			lineNumber := fmt.Sprintf("%d ", result.line+1)
			lineStyle := style
			lineStyle.Foreground = a.theme.Muted
			if entry.resultIndex == panel.selected {
				lineStyle.Foreground = a.theme.StatusText
			}
			a.screen.Text(2, y, lineNumber, lineStyle)
			text := strings.TrimSpace(result.text)
			a.screen.Text(2+displayWidth(lineNumber), y,
				truncate(text, sidebarWidth-4-displayWidth(lineNumber)), style)
		}
	}

	a.drawPanelSeparator(0, footerSeparator, sidebarWidth, border)
	help := []styledText{
		{text: "↑↓", style: accent}, {text: " move  ", style: border},
		{text: "Enter", style: accent}, {text: " open  ", style: border},
		{text: "Esc", style: accent}, {text: " close", style: border},
	}
	drawStyledText(a.screen, 2, footerSeparator+1, sidebarWidth-4, help)
	return sidebarWidth
}

func trailingDisplayText(value []rune, width int) string {
	if width <= 0 {
		return ""
	}
	start := len(value)
	used := 0
	for start > 0 {
		cellWidth := terminal.RuneWidth(value[start-1])
		if used+cellWidth > width {
			break
		}
		start--
		used += cellWidth
	}
	return string(value[start:])
}

func searchWorkspace(ctx context.Context, root string, files []string, query string) ([]workspaceSearchResult, error) {
	command := exec.CommandContext(ctx, "rg", "--json", "--fixed-strings", "--smart-case", "--color", "never", "--", query, ".")
	command.Dir = root
	output, err := command.Output()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err == nil {
		return parseRipgrepSearchResults(output), nil
	}
	var commandError *exec.Error
	if errors.As(err, &commandError) && errors.Is(commandError.Err, exec.ErrNotFound) {
		return searchWorkspaceFiles(ctx, root, files, query)
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) && exitError.ExitCode() == 1 {
		return nil, nil
	}
	return nil, fmt.Errorf("search workspace: %w", err)
}

type ripgrepJSONEvent struct {
	Type string `json:"type"`
	Data struct {
		Path struct {
			Text string `json:"text"`
		} `json:"path"`
		Lines struct {
			Text string `json:"text"`
		} `json:"lines"`
		LineNumber int `json:"line_number"`
		Submatches []struct {
			Start int `json:"start"`
		} `json:"submatches"`
	} `json:"data"`
}

func parseRipgrepSearchResults(output []byte) []workspaceSearchResult {
	results := make([]workspaceSearchResult, 0)
	scanner := bufio.NewScanner(bytes.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() && len(results) < workspaceSearchResultLimit {
		var event ripgrepJSONEvent
		if json.Unmarshal(scanner.Bytes(), &event) != nil || event.Type != "match" {
			continue
		}
		column := 0
		if len(event.Data.Submatches) > 0 {
			column = event.Data.Submatches[0].Start
		}
		path := strings.TrimPrefix(filepath.ToSlash(event.Data.Path.Text), "./")
		results = append(results, workspaceSearchResult{
			path: filepath.FromSlash(path), line: max(0, event.Data.LineNumber-1),
			byteColumn: column, text: strings.TrimRight(event.Data.Lines.Text, "\r\n"),
		})
	}
	return results
}

func searchWorkspaceFiles(ctx context.Context, root string, files []string, query string) ([]workspaceSearchResult, error) {
	results := make([]workspaceSearchResult, 0)
	caseSensitive := strings.ToLower(query) != query
	needle := query
	if !caseSensitive {
		needle = strings.ToLower(needle)
	}
	for _, relative := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		contents, err := os.ReadFile(filepath.Join(root, relative))
		if err != nil || bytes.IndexByte(contents, 0) >= 0 {
			continue
		}
		scanner := bufio.NewScanner(bytes.NewReader(contents))
		scanner.Buffer(make([]byte, 64*1024), 1024*1024)
		line := 0
		for scanner.Scan() {
			text := scanner.Text()
			haystack := text
			if !caseSensitive {
				haystack = strings.ToLower(haystack)
			}
			if column := strings.Index(haystack, needle); column >= 0 {
				results = append(results, workspaceSearchResult{
					path: relative, line: line, byteColumn: column, text: text,
				})
				if len(results) == workspaceSearchResultLimit {
					return results, nil
				}
			}
			line++
		}
	}
	return results, nil
}
