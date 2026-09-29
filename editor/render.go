package editor

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/syntax"
	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

func (a *App) render() error {
	a.updateMessageLifetime(time.Now())
	width, height := a.screen.Size()
	base := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Background}
	a.screen.Clear(base)
	if width < 20 || height < 4 {
		a.screen.SetCursorVisible(true)
		a.screen.Text(0, 0, "terminal is too small", base)
		return a.screen.Flush(0, 0)
	}

	a.renderTabs(width)
	statusRow := height - 1
	if a.minibuffer != nil {
		statusRow--
	}
	sidebarWidth := 0
	activeView := a.currentView()
	if activeView != nil {
		a.renderView(activeView, statusRow)
	} else {
		sidebarWidth = a.renderSidebar(statusRow)
		if a.tileRoot != nil {
			a.syncFocusedTile()
			a.renderEditorTiles(editorRegion{
				x: sidebarWidth, y: 1, width: width - sidebarWidth, height: statusRow - 1,
			})
		} else {
			a.renderBuffer(sidebarWidth, width, statusRow)
		}
	}
	a.renderStatus(width, statusRow)
	if a.palette != nil {
		if a.palette.completion {
			cursorX, cursorY := a.cursorPosition(sidebarWidth, statusRow)
			a.renderCompletion(width, statusRow, cursorX, cursorY)
		} else {
			a.renderPalette(width, statusRow)
		}
	} else if a.minibuffer == nil && activeView == nil {
		if item, ok := a.visibleDiagnosticAtCursor(); ok {
			cursorX, cursorY := a.cursorPosition(sidebarWidth, statusRow)
			a.renderDiagnostic(width, statusRow, cursorX, cursorY, item)
		}
	}
	if a.minibuffer != nil {
		a.renderMinibuffer(width, height-1)
	}

	cursorX, cursorY := a.cursorPosition(sidebarWidth, statusRow)
	if a.palette != nil && !a.palette.completion {
		left, top, panelWidth, _ := paletteBounds(width, statusRow)
		count := fmt.Sprintf("%d / %d", min(a.palette.selected+1, len(a.palette.filtered)), len(a.palette.filtered))
		prefixWidth := 2
		if a.palette.hideQueryMarker {
			prefixWidth = 0
		}
		inputX := left + 2 + prefixWidth
		countX := left + panelWidth - displayWidth(count) - 2
		queryWidth := max(0, countX-inputX-1)
		cursorX = inputX + min(queryWidth, displayWidth(string(a.palette.query)))
		cursorY = top + 1
	}
	if a.minibuffer != nil {
		promptWidth := displayWidth(a.minibuffer.title + ": ")
		cursorX = min(width-1, promptWidth+displayWidth(string(a.minibuffer.query)))
		cursorY = height - 1
	}
	if a.palette == nil && a.minibuffer == nil && a.showFiles && a.browser.focused {
		cursorX = min(sidebarWidth-2, 3+displayWidth(string(a.browser.query)))
		cursorY = 2
	}
	if a.palette == nil && a.minibuffer == nil && a.sidebar != nil {
		cursorX = 2
		cursorY = 2 + a.sidebar.selected - a.sidebar.top
	}
	if a.palette == nil && a.minibuffer == nil && a.workspaceSearch != nil {
		visibleQuery := trailingDisplayText(a.workspaceSearch.query, max(0, sidebarWidth-5))
		cursorX = min(sidebarWidth-2, 3+displayWidth(visibleQuery))
		cursorY = 2
	}
	softwareCursors := a.usesSoftwareCursors()
	hideCursor := softwareCursors || (activeView != nil && a.palette == nil && a.minibuffer == nil)
	a.screen.SetCursorVisible(!hideCursor)
	if softwareCursors && a.cursorBlinkOn {
		a.renderSoftwareCursors(sidebarWidth, statusRow)
	}
	return a.screen.Flush(cursorX, cursorY)
}

func (a *App) renderTabs(width int) {
	style := terminal.Style{Foreground: a.theme.TabText, Background: a.theme.TabBackground}
	active := terminal.Style{
		Foreground: a.theme.TabActiveText,
		Background: a.theme.TabActiveBackground,
		Bold:       true,
	}
	x := 0
	for index, editorBuffer := range a.buffers {
		current := editorBuffer.text
		name := current.Name()
		if editorBuffer.terminal == nil && current.IsDirty() {
			name += " ●"
		}
		label := " " + name + " "
		if index < 9 {
			label = fmt.Sprintf(" %d:%s ", index+1, name)
		}
		currentStyle := style
		if index == a.active {
			currentStyle = active
		}
		a.screen.Text(x, 0, label, currentStyle)
		x += displayWidth(label)
		if x >= width {
			break
		}
	}
}

func (a *App) renderSidebar(statusRow int) int {
	if a.workspaceSearch != nil {
		return a.renderWorkspaceSearch(statusRow)
	}
	if a.sidebar != nil {
		return a.renderPluginSidebar(statusRow)
	}
	return a.renderFiles(statusRow)
}

func (a *App) renderPluginSidebar(statusRow int) int {
	width, _ := a.screen.Size()
	sidebarWidth := fileSidebarWidth(width)
	panelHeight := statusRow - 1
	if panelHeight < 2 {
		return sidebarWidth
	}
	panelStyle := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Panel}
	border := terminal.Style{Foreground: a.theme.PanelBorder, Background: a.theme.Panel}
	accent := terminal.Style{Foreground: a.theme.Accent, Background: a.theme.Panel, Bold: true}
	helpText := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
	selected := terminal.Style{Foreground: a.theme.SelectionText, Background: a.theme.Selection, Bold: true}
	a.drawPanel(0, 1, sidebarWidth, panelHeight, a.sidebar.title, a.theme.Accent, panelStyle, border)
	if panelHeight < 5 {
		return sidebarWidth
	}

	rows := max(0, panelHeight-4)
	a.sidebar.ensureVisible(rows)
	for row := 0; row < rows && a.sidebar.top+row < len(a.sidebar.rows); row++ {
		index := a.sidebar.top + row
		entry := a.sidebar.rows[index]
		y := 2 + row
		if !entry.set {
			style := border
			if len(entry.title) > 0 && entry.title[0] != ' ' {
				style = accent
			}
			a.screen.Text(2, y, truncate(entry.title, sidebarWidth-4), style)
			continue
		}
		style := panelStyle
		if index == a.sidebar.selected {
			style = selected
			fillRow(a.screen, 1, y, sidebarWidth-2, style)
		}
		detail := entry.item.Detail
		detailWidth := displayWidth(detail)
		labelWidth := sidebarWidth - 4
		if detail != "" {
			labelWidth -= detailWidth + 1
		}
		a.screen.Text(2, y, truncate(entry.item.Label, max(0, labelWidth)), style)
		if detail != "" {
			detailStyle := style
			switch entry.item.DetailTone {
			case ui.ToneSuccess:
				detailStyle.Foreground = a.theme.Success
			case ui.ToneWarning:
				detailStyle.Foreground = a.theme.Warning
			case ui.ToneDanger:
				detailStyle.Foreground = a.theme.Danger
			}
			a.screen.Text(sidebarWidth-detailWidth-2, y, detail, detailStyle)
		}
	}

	footerSeparator := panelHeight - 2
	a.drawPanelSeparator(0, footerSeparator, sidebarWidth, border)
	help := make([]styledText, 0, len(a.sidebar.help)*2+2)
	for _, item := range a.sidebar.help {
		help = append(help,
			styledText{text: item.Key, style: accent},
			styledText{text: " " + item.Label + "  ", style: helpText},
		)
	}
	help = append(help, styledText{text: "Esc", style: accent}, styledText{text: " close", style: helpText})
	drawStyledText(a.screen, 2, footerSeparator+1, sidebarWidth-4, help)
	return sidebarWidth
}

func (a *App) renderFiles(statusRow int) int {
	if !a.showFiles {
		return 0
	}
	width, _ := a.screen.Size()
	sidebarWidth := fileSidebarWidth(width)
	panelHeight := statusRow - 1
	if panelHeight < 2 {
		return sidebarWidth
	}
	panel := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Panel}
	border := terminal.Style{Foreground: a.theme.PanelBorder, Background: a.theme.Panel}
	accent := terminal.Style{Foreground: a.theme.Accent, Background: a.theme.Panel, Bold: true}
	helpText := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
	a.drawPanel(
		0, 1, sidebarWidth, panelHeight,
		"Files · "+filepath.Base(a.root), a.theme.Accent, panel, border,
	)
	if panelHeight < 7 {
		return sidebarWidth
	}

	a.screen.Text(1, 2, "› ", accent)
	if len(a.browser.query) == 0 {
		placeholder := border
		a.screen.Text(3, 2, truncate("type to filter…", sidebarWidth-5), placeholder)
	} else {
		a.screen.Text(3, 2, truncate(string(a.browser.query), sidebarWidth-5), panel)
	}
	a.drawPanelSeparator(0, 3, sidebarWidth, border)

	rows := max(0, panelHeight-7)
	a.browser.ensureVisible(rows)
	activePath := ""
	if a.current().Path() != "" {
		if relative, err := filepath.Rel(a.root, a.current().Path()); err == nil {
			activePath = filepath.Clean(relative)
		}
	}
	for row := 0; row < rows && a.browser.top+row < len(a.browser.entries); row++ {
		index := a.browser.top + row
		entry := a.browser.entries[index]
		style := panel
		if entry.node.path != "" && filepath.Clean(entry.node.path) == activePath {
			style.Foreground = a.theme.Accent
			style.Bold = true
		}
		if a.browser.focused && index == a.browser.selected {
			style = terminal.Style{
				Foreground: a.theme.SelectionText,
				Background: a.theme.Selection,
				Bold:       true,
			}
			fillRow(a.screen, 1, 4+row, sidebarWidth-2, style)
		}
		indent := strings.Repeat("  ", entry.depth)
		marker := "• "
		if entry.node.directory {
			marker = "▸ "
			if a.browser.expanded[entry.node.path] || len(a.browser.query) > 0 {
				marker = "▾ "
			}
		}
		label := indent + marker + entry.node.name
		a.screen.Text(2, 4+row, truncate(label, sidebarWidth-4), style)
	}

	footerSeparator := panelHeight - 3
	a.drawPanelSeparator(0, footerSeparator, sidebarWidth, border)
	help := []styledText{{text: "M-f", style: accent}, {text: " focus", style: helpText}}
	if a.browser.focused {
		navigation := []styledText{
			{text: "↑↓", style: accent}, {text: " move  ", style: helpText},
			{text: "←→", style: accent}, {text: " fold  ", style: helpText},
			{text: "Enter", style: accent}, {text: " open", style: helpText},
		}
		drawStyledText(a.screen, 2, footerSeparator+1, sidebarWidth-4, navigation)
		help = []styledText{
			{text: "C-S-N", style: accent}, {text: " new  ", style: helpText},
			{text: "C-r", style: accent}, {text: " rename  ", style: helpText},
			{text: "Del", style: accent}, {text: " delete", style: helpText},
		}
	}
	drawStyledText(a.screen, 2, footerSeparator+2, sidebarWidth-4, help)
	return sidebarWidth
}

func fileSidebarWidth(width int) int {
	sidebarWidth := min(42, max(24, width/3))
	return min(sidebarWidth, max(12, width-20))
}

func searchSidebarWidth(width int) int {
	sidebarWidth := min(72, max(36, width/2))
	return min(sidebarWidth, max(12, width-20))
}

func (a *App) renderBuffer(sidebarWidth, width, statusRow int) {
	a.renderBufferRegion(a.currentEditorBuffer(), editorRegion{
		x: sidebarWidth, y: 1, width: width - sidebarWidth, height: statusRow - 1,
	}, a.topLine, a.leftColumn)
}

type editorRegion struct {
	x      int
	y      int
	width  int
	height int
}

func (a *App) renderBufferRegion(editorBuffer *editorBuffer, region editorRegion, topLine, leftColumn int) {
	if editorBuffer == nil || region.width <= 0 || region.height <= 0 {
		return
	}
	current := editorBuffer.text
	if editorBuffer.terminal != nil {
		a.renderTerminalBufferRegion(editorBuffer, region, topLine, leftColumn)
		return
	}
	lineNumberWidth := len(strconv.Itoa(max(1, current.LineCount()))) + 2
	lineNumberWidth = min(lineNumberWidth, region.width)
	textX := region.x + lineNumberWidth
	available := max(0, region.width-lineNumberWidth)
	muted := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Background}
	highlighter := editorBuffer.highlighter

	for row := 0; row < region.height; row++ {
		lineNumber := topLine + row
		y := region.y + row
		if lineNumber >= current.LineCount() {
			if region.width > 1 {
				a.screen.Set(region.x+1, y, '~', muted)
			}
			continue
		}
		gutter := fmt.Sprintf("%*d ", lineNumberWidth-1, lineNumber+1)
		gutterStyle := muted
		if editorBuffer.breakpoints[lineNumber] {
			gutter = fmt.Sprintf("%*d●", lineNumberWidth-1, lineNumber+1)
			gutterStyle.Foreground = a.theme.Error
		}
		a.screen.Text(region.x, y, truncate(gutter, lineNumberWidth), gutterStyle)
		line := string(current.Line(lineNumber))
		runes := []rune(line)
		spans := highlighter.Highlight(lineNumber, line)
		errorRanges := a.errorRanges(current, lineNumber, len(runes))
		if len(errorRanges) > 0 {
			diagnosticStyle := terminal.Style{
				Foreground: a.theme.Foreground,
				Background: a.theme.DiagnosticBackground,
			}
			for x := textX; x < textX+available; x++ {
				a.screen.Set(x, y, ' ', diagnosticStyle)
			}
		}
		displayColumn := 0
		for column, value := range runes {
			style := a.theme.syntaxStyle(kindAt(spans, column))
			if len(errorRanges) > 0 {
				style.Background = a.theme.DiagnosticBackground
			}
			if a.isBufferSelected(current, lineNumber, column) {
				style.Background = a.theme.Selection
			}
			if columnInRanges(column, errorRanges) {
				style.CurlyUnderline = true
				style.UnderlineColor = a.theme.Error
				style.HasUnderlineColor = true
			}
			cellWidth := terminal.RuneWidth(value)
			if value == '\t' {
				cellWidth = 4 - displayColumn%4
			}
			if displayColumn+cellWidth > leftColumn && displayColumn-leftColumn < available {
				x := textX + max(0, displayColumn-leftColumn)
				if value == '\t' || displayColumn < leftColumn {
					for offset := max(leftColumn-displayColumn, 0); offset < cellWidth && x < textX+available; offset++ {
						a.screen.Set(x, y, ' ', style)
						x++
					}
				} else if x+cellWidth <= textX+available {
					a.screen.Set(x, y, value, style)
				}
			}
			displayColumn += cellWidth
		}
		for _, currentRange := range errorRanges {
			if currentRange.start < len(runes) || currentRange.end <= len(runes) {
				continue
			}
			x := textX + sourceDisplayWidth(runes) - leftColumn
			if x < textX || x >= textX+available {
				continue
			}
			style := terminal.Style{
				Foreground:        a.theme.Foreground,
				Background:        a.theme.DiagnosticBackground,
				UnderlineColor:    a.theme.Error,
				CurlyUnderline:    true,
				HasUnderlineColor: true,
			}
			a.screen.Set(x, y, ' ', style)
		}
	}
}

func (a *App) renderTerminalBufferRegion(editorBuffer *editorBuffer, region editorRegion, topLine, leftColumn int) {
	if editorBuffer == nil || region.width <= 0 || region.height <= 0 {
		return
	}
	current := editorBuffer.text
	style := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Background}
	for row := 0; row < region.height; row++ {
		lineNumber := topLine + row
		if lineNumber >= current.LineCount() {
			continue
		}
		line := []rune(string(current.Line(lineNumber)))
		displayColumn := 0
		for _, value := range line {
			cellWidth := terminal.RuneWidth(value)
			if displayColumn+cellWidth > leftColumn && displayColumn-leftColumn < region.width {
				x := region.x + max(0, displayColumn-leftColumn)
				if displayColumn >= leftColumn && x+cellWidth <= region.x+region.width {
					a.screen.Set(x, region.y+row, value, style)
				}
			}
			displayColumn += cellWidth
		}
	}
}

func (a *App) renderStatus(width, row int) {
	style := terminal.Style{Foreground: a.theme.StatusText, Background: a.theme.Status}
	for x := range width {
		a.screen.Set(x, row, ' ', style)
	}
	if view := a.currentView(); view != nil {
		left := " " + a.message
		right := fmt.Sprintf("%s  %s  View ", view.title, a.theme.Name)
		rightWidth := displayWidth(right)
		a.screen.Text(0, row, truncate(left, max(0, width-rightWidth-1)), style)
		if rightWidth < width {
			a.screen.Text(width-rightWidth, row, right, style)
		}
		return
	}
	point := a.current().Cursors()[0].Point
	left := " " + a.message
	if item, ok := a.visibleDiagnosticAtCursor(); ok {
		left = " Error: " + item.message
	}
	sections := a.editorStatusSections(point)
	for len(sections) > 0 && statusSectionsWidth(sections, a.theme.Borders.StatusSeparator) >= width {
		if !sections[0].optional {
			break
		}
		sections = sections[1:]
	}
	rightWidth := statusSectionsWidth(sections, a.theme.Borders.StatusSeparator)
	rightStart := max(0, width-rightWidth)
	left = truncate(left, max(0, rightStart-1))
	a.screen.Text(0, row, left, style)
	a.drawStatusSections(width-rightWidth, row, sections, style)
}

type statusSection struct {
	text     string
	style    terminal.Style
	optional bool
}

func (a *App) editorStatusSections(point buffer.Point) []statusSection {
	sections := make([]statusSection, 0, len(a.statusItems)+4)
	for index, item := range a.statusItems {
		text := strings.TrimSpace(item.text)
		if text == "" {
			continue
		}
		sections = append(sections, statusSection{
			text:     truncate(text, 32),
			style:    a.statusStyle(a.theme.Accent, 14+(index%2)*8, false),
			optional: true,
		})
	}
	if text, errors := a.diagnosticStatus(); text != "" {
		tone := a.theme.Warning
		if errors > 0 {
			tone = a.theme.Danger
		}
		sections = append(sections, statusSection{
			text:     text,
			style:    a.statusStyle(tone, 28, true),
			optional: true,
		})
	}
	mode := a.modeForBuffer(a.current())
	sections = append(sections,
		statusSection{text: mode.Name, style: a.statusStyle(a.theme.Accent, 34, true)},
		statusSection{
			text:  fmt.Sprintf("%d:%d", point.Line+1, point.Column+1),
			style: a.statusStyle(a.theme.Accent, 52, true),
		},
		statusSection{
			text:  fmt.Sprintf("%d%%", scrollPercentage(point.Line, a.current().LineCount())),
			style: a.statusStyle(a.theme.Accent, 76, true),
		},
	)
	return sections
}

func (a *App) diagnosticStatus() (string, int) {
	errors := 0
	warnings := 0
	for _, item := range a.currentEditorBuffer().diagnostics {
		switch item.severity {
		case 1:
			errors++
		case 2:
			warnings++
		}
	}
	parts := make([]string, 0, 2)
	if errors > 0 {
		parts = append(parts, fmt.Sprintf("%d✖", errors))
	}
	if warnings > 0 {
		parts = append(parts, fmt.Sprintf("%d⚠", warnings))
	}
	return strings.Join(parts, " "), errors
}

func (a *App) statusStyle(tone terminal.Color, intensity int, bold bool) terminal.Style {
	return terminal.Style{
		Foreground: a.theme.Foreground,
		Background: blendColor(a.theme.Status, tone, intensity),
		Bold:       bold,
	}
}

func scrollPercentage(line, lineCount int) int {
	if lineCount <= 0 {
		return 100
	}
	return min(100, max(0, (line+1)*100/lineCount))
}

func statusSectionsWidth(sections []statusSection, separator rune) int {
	width := 0
	for _, section := range sections {
		width += terminal.RuneWidth(separator) + displayWidth(section.text) + 2
	}
	return width
}

func (a *App) drawStatusSections(x, row int, sections []statusSection, base terminal.Style) {
	previousBackground := base.Background
	for _, section := range sections {
		separatorStyle := terminal.Style{
			Foreground: section.style.Background,
			Background: previousBackground,
		}
		a.screen.Set(x, row, a.theme.Borders.StatusSeparator, separatorStyle)
		x += terminal.RuneWidth(a.theme.Borders.StatusSeparator)
		text := " " + section.text + " "
		a.screen.Text(x, row, text, section.style)
		x += displayWidth(text)
		previousBackground = section.style.Background
	}
}

func (a *App) renderPalette(width, statusRow int) {
	p := a.palette
	left, top, panelWidth, boxHeight := paletteBounds(width, statusRow)
	panel := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Panel}
	border := terminal.Style{Foreground: a.theme.PanelBorder, Background: a.theme.Panel}
	accent := terminal.Style{Foreground: a.theme.Accent, Background: a.theme.Panel, Bold: true}
	helpText := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
	selected := terminal.Style{Foreground: a.theme.SelectionText, Background: a.theme.Selection, Bold: true}
	a.drawPanel(left, top, panelWidth, boxHeight, p.title, a.theme.Accent, panel, border)

	count := fmt.Sprintf("%d / %d", min(p.selected+1, len(p.filtered)), len(p.filtered))
	prefix := "› "
	if p.hideQueryMarker {
		prefix = ""
	}
	inputX := left + 2 + displayWidth(prefix)
	countX := left + panelWidth - displayWidth(count) - 2
	queryWidth := max(0, countX-inputX-1)
	a.screen.Text(left+2, top+1, prefix, panel)
	a.screen.Text(inputX, top+1, truncate(string(p.query), queryWidth), panel)
	a.screen.Text(countX, top+1, count, helpText)
	a.drawPanelSeparator(left, top+2, panelWidth, border)

	rows := max(0, boxHeight-6)
	start := visibleStart(p.selected, len(p.filtered), rows)
	for row := 0; row < rows && start+row < len(p.filtered); row++ {
		index := start + row
		item := p.filtered[index]
		style := panel
		prefix := "  "
		if index == p.selected {
			style = selected
			prefix = "› "
			fillRow(a.screen, left+1, top+3+row, panelWidth-2, style)
		}
		label := prefix + item.label
		if item.detail != "" {
			label += "  " + item.detail
		}
		a.screen.Text(left+1, top+3+row, truncate(label, panelWidth-2), style)
	}

	footerSeparator := top + boxHeight - 3
	a.drawPanelSeparator(left, footerSeparator, panelWidth, border)
	help := []styledText{
		{text: "↑↓", style: accent}, {text: " select   ", style: helpText},
		{text: "Enter", style: accent}, {text: " choose   ", style: helpText},
		{text: "Esc", style: accent}, {text: " cancel", style: helpText},
	}
	if p.source != nil {
		help = []styledText{
			{text: "Files · ", style: helpText}, {text: ">", style: accent},
			{text: " commands   ", style: helpText}, {text: "↑↓", style: accent},
			{text: " select   ", style: helpText}, {text: "Enter", style: accent},
			{text: " open/run   ", style: helpText}, {text: "Esc", style: accent},
			{text: " cancel", style: helpText},
		}
	}
	drawStyledText(a.screen, left+2, footerSeparator+1, panelWidth-4, help)
}

func (a *App) renderCompletion(width, statusRow, cursorX, cursorY int) {
	p := a.palette
	if len(p.filtered) == 0 {
		return
	}
	selectedItem := p.filtered[p.selected]
	suggestionWidth := min(44, max(28, width/3))
	suggestionWidth = min(suggestionWidth, width-2)
	rows := min(7, len(p.filtered))
	boxHeight := min(statusRow-1, rows+4)
	rows = max(1, boxHeight-4)
	documentationWidth := min(52, max(32, width-suggestionWidth-5))
	showDocumentation := width >= suggestionWidth+documentationWidth+3 &&
		(selectedItem.detail != "" || selectedItem.documentation != "")
	totalWidth := suggestionWidth
	if showDocumentation {
		totalWidth += documentationWidth + 1
	}
	left := min(cursorX, max(0, width-totalWidth))
	top := cursorY + 1
	if top+boxHeight > statusRow {
		top = max(1, cursorY-boxHeight)
	}

	panel := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Panel}
	border := terminal.Style{Foreground: a.theme.PanelBorder, Background: a.theme.Panel}
	accent := terminal.Style{Foreground: a.theme.Accent, Background: a.theme.Panel, Bold: true}
	helpText := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
	selected := terminal.Style{Foreground: a.theme.SelectionText, Background: a.theme.Selection, Bold: true}
	a.drawPanel(left, top, suggestionWidth, boxHeight, p.title, a.theme.Accent, panel, border)
	start := visibleStart(p.selected, len(p.filtered), rows)
	for row := 0; row < rows && start+row < len(p.filtered); row++ {
		index := start + row
		item := p.filtered[index]
		style := panel
		if index == p.selected {
			style = selected
			fillRow(a.screen, left+1, top+1+row, suggestionWidth-2, style)
		}
		kind := ""
		if item.kind != "" {
			kind = "  " + item.kind
		}
		labelWidth := max(1, suggestionWidth-displayWidth(kind)-4)
		a.screen.Text(left+2, top+1+row, truncate(item.label, labelWidth), style)
		if kind != "" {
			a.screen.Text(left+suggestionWidth-displayWidth(kind)-2, top+1+row, kind, style)
		}
	}
	separator := top + boxHeight - 3
	a.drawPanelSeparator(left, separator, suggestionWidth, border)
	position := fmt.Sprintf("%d/%d", p.selected+1, len(p.filtered))
	help := []styledText{
		{text: position + "  ", style: helpText}, {text: "↑↓", style: accent},
		{text: " select  ", style: helpText}, {text: "↵/Tab", style: accent},
		{text: " accept  ", style: helpText}, {text: "Esc", style: accent},
	}
	drawStyledText(a.screen, left+2, separator+1, suggestionWidth-4, help)

	if showDocumentation {
		a.renderCompletionDocumentation(
			left+suggestionWidth+1, top, documentationWidth, boxHeight, selectedItem, panel, border,
		)
	}
}

func (a *App) renderCompletionDocumentation(
	left, top, width, height int,
	item paletteItem,
	panel, border terminal.Style,
) {
	a.drawPanel(left, top, width, height, "Documentation", a.theme.Accent, panel, border)
	content := cleanDocumentation(item.detail)
	if item.documentation != "" {
		if content != "" {
			content += "\n\n"
		}
		content += cleanDocumentation(item.documentation)
	}
	lines := wrapText(content, width-4)
	for row := 0; row < height-2 && row < len(lines); row++ {
		style := panel
		if row == 0 && item.detail != "" {
			style.Bold = true
		}
		a.screen.Text(left+2, top+1+row, truncate(lines[row], width-4), style)
	}
}

func (a *App) renderDiagnostic(width, statusRow, cursorX, cursorY int, item diagnostic) {
	panelWidth := min(72, max(30, width-4))
	lines := wrapText(item.message, panelWidth-4)
	boxHeight := min(statusRow-1, max(5, len(lines)+4))
	lineLimit := max(1, boxHeight-4)
	left := min(cursorX, max(0, width-panelWidth))
	top := cursorY + 1
	if top+boxHeight > statusRow {
		top = max(1, cursorY-boxHeight)
	}
	panel := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.DiagnosticBackground}
	border := terminal.Style{Foreground: a.theme.Diagnostic, Background: a.theme.DiagnosticBackground}
	helpText := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.DiagnosticBackground}
	title := "Error"
	if item.source != "" {
		title += " · " + item.source
	}
	if item.code != "" {
		title += " [" + item.code + "]"
	}
	a.drawPanel(left, top, panelWidth, boxHeight, title, a.theme.Diagnostic, panel, border)
	for row := 0; row < lineLimit && row < len(lines); row++ {
		a.screen.Text(left+2, top+1+row, truncate(lines[row], panelWidth-4), panel)
	}
	separator := top + boxHeight - 3
	a.drawPanelSeparator(left, separator, panelWidth, border)
	a.screen.Text(left+2, separator+1, "Move the cursor away to dismiss", helpText)
}

func (a *App) renderMinibuffer(width, row int) {
	style := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Background}
	for x := range width {
		a.screen.Set(x, row, ' ', style)
	}
	heading := style
	heading.Bold = true
	a.screen.Text(0, row, a.minibuffer.title+": ", heading)
	x := displayWidth(a.minibuffer.title + ": ")
	a.screen.Text(x, row, string(a.minibuffer.query), style)
}

func paletteBounds(width, statusRow int) (int, int, int, int) {
	panelWidth := min(max(16, width-4), max(40, width*3/4))
	panelWidth = min(panelWidth, width)
	maxHeight := max(1, statusRow-1)
	boxHeight := min(maxHeight, min(16, max(7, statusRow*2/3)))
	left := max(0, (width-panelWidth)/2)
	top := max(1, (statusRow-boxHeight)/2)
	return left, top, panelWidth, boxHeight
}

func (a *App) drawPanel(
	left, top, width, height int,
	title string,
	titleColor terminal.Color,
	panel, border terminal.Style,
) {
	if width < 2 || height < 2 {
		return
	}
	characters := a.theme.Borders
	for y := top; y < top+height; y++ {
		fillRow(a.screen, left, y, width, panel)
	}
	for x := left + 1; x < left+width-1; x++ {
		a.screen.Set(x, top, characters.Lines.Horizontal, border)
		a.screen.Set(x, top+height-1, characters.Lines.Horizontal, border)
	}
	for y := top + 1; y < top+height-1; y++ {
		a.screen.Set(left, y, characters.Lines.Vertical, border)
		a.screen.Set(left+width-1, y, characters.Lines.Vertical, border)
	}
	a.screen.Set(left, top, characters.Corners.TopLeft, border)
	a.screen.Set(left+width-1, top, characters.Corners.TopRight, border)
	a.screen.Set(left, top+height-1, characters.Corners.BottomLeft, border)
	a.screen.Set(left+width-1, top+height-1, characters.Corners.BottomRight, border)
	if title != "" && width > 6 {
		heading := panel
		heading.Foreground = titleColor
		heading.Bold = true
		a.screen.Text(left+2, top, " "+truncate(title, width-6)+" ", heading)
	}
}

func (a *App) drawPanelSeparator(left, row, width int, style terminal.Style) {
	if width < 2 {
		return
	}
	characters := a.theme.Borders
	a.screen.Set(left, row, characters.Lines.Vertical, style)
	for x := left + 1; x < left+width-1; x++ {
		a.screen.Set(x, row, characters.Separator.Horizontal, style)
	}
	a.screen.Set(left+width-1, row, characters.Lines.Vertical, style)
}

func fillRow(screen *terminal.Screen, left, row, width int, style terminal.Style) {
	for x := left; x < left+width; x++ {
		screen.Set(x, row, ' ', style)
	}
}

type styledText struct {
	text  string
	style terminal.Style
}

func drawStyledText(screen *terminal.Screen, left, row, width int, segments []styledText) {
	used := 0
	for _, segment := range segments {
		for _, value := range segment.text {
			cellWidth := terminal.RuneWidth(value)
			if used+cellWidth > width {
				return
			}
			screen.Set(left+used, row, value, segment.style)
			used += cellWidth
		}
	}
}

func visibleStart(selected, total, rows int) int {
	if rows <= 0 || total <= rows {
		return 0
	}
	start := selected - rows/2
	return min(max(0, start), total-rows)
}

func cleanDocumentation(value string) string {
	replacer := strings.NewReplacer(
		"\r", "", "```go", "", "```", "", "`", "", "**", "", "__", "",
	)
	return strings.TrimSpace(replacer.Replace(value))
}

func wrapText(value string, width int) []string {
	if width <= 0 || value == "" {
		return nil
	}
	lines := make([]string, 0)
	for _, paragraph := range strings.Split(value, "\n") {
		paragraph = strings.TrimSpace(paragraph)
		if paragraph == "" {
			if len(lines) > 0 && lines[len(lines)-1] != "" {
				lines = append(lines, "")
			}
			continue
		}
		line := ""
		for _, word := range strings.Fields(paragraph) {
			if line == "" {
				line = word
				continue
			}
			if displayWidth(line)+1+displayWidth(word) <= width {
				line += " " + word
				continue
			}
			lines = append(lines, truncate(line, width))
			line = word
		}
		if line != "" {
			lines = append(lines, truncate(line, width))
		}
	}
	return lines
}

func (a *App) cursorPosition(sidebarWidth, statusRow int) (int, int) {
	region := editorRegion{x: sidebarWidth, y: 1}
	width, _ := a.screen.Size()
	region.width = width - sidebarWidth
	region.height = statusRow - 1
	if a.tileRoot != nil {
		if focused, ok := a.editorTileRegion(a.focusedTile, region); ok {
			region = focused
		}
	}
	x, y := a.bufferPointPositionInRegion(
		a.currentEditorBuffer(), a.current().Cursors()[0].Point, region, a.topLine, a.leftColumn,
	)
	return max(0, min(width-1, x)), max(0, min(statusRow-1, y))
}

func (a *App) bufferPointPosition(point buffer.Point, sidebarWidth int) (int, int) {
	return a.bufferPointPositionInRegion(
		a.currentEditorBuffer(), point,
		editorRegion{x: sidebarWidth, y: 1}, a.topLine, a.leftColumn,
	)
}

func (a *App) bufferPointPositionInRegion(
	editorBuffer *editorBuffer,
	point buffer.Point,
	region editorRegion,
	topLine, leftColumn int,
) (int, int) {
	current := editorBuffer.text
	lineNumberWidth := len(strconv.Itoa(max(1, current.LineCount()))) + 2
	if editorBuffer.terminal != nil {
		lineNumberWidth = 0
	}
	line := []rune(string(current.Line(point.Line)))
	column := min(point.Column, len(line))
	x := region.x + lineNumberWidth + sourceDisplayWidth(line[:column]) - leftColumn
	y := region.y + point.Line - topLine
	return x, y
}

func (a *App) usesSoftwareCursors() bool {
	if len(a.current().Cursors()) < 2 || a.minibuffer != nil || a.sidebar != nil || a.workspaceSearch != nil {
		return false
	}
	if a.palette != nil && !a.palette.completion {
		return false
	}
	return !a.showFiles || !a.browser.focused
}

func (a *App) renderSoftwareCursors(sidebarWidth, statusRow int) {
	width, _ := a.screen.Size()
	region := editorRegion{
		x: sidebarWidth, y: 1, width: width - sidebarWidth, height: statusRow - 1,
	}
	if a.tileRoot != nil {
		if focused, ok := a.editorTileRegion(a.focusedTile, region); ok {
			region = focused
		}
	}
	lineNumberWidth := len(strconv.Itoa(max(1, a.current().LineCount()))) + 2
	textX := region.x + lineNumberWidth
	style := terminal.Style{
		Foreground: a.theme.Background,
		Background: a.theme.Cursor,
	}
	for _, cursor := range a.current().Cursors() {
		x, y := a.bufferPointPositionInRegion(
			a.currentEditorBuffer(), cursor.Point, region, a.topLine, a.leftColumn,
		)
		if x < textX || x >= region.x+region.width || y < region.y || y >= region.y+region.height {
			continue
		}
		a.screen.Restyle(x, y, style)
	}
}

func (a *App) isBufferSelected(current *buffer.Buffer, line, column int) bool {
	position := current.Offset(buffer.Point{Line: line, Column: column})
	for _, cursor := range current.Cursors() {
		start := current.Offset(cursor.Anchor)
		end := current.Offset(cursor.Point)
		if start > end {
			start, end = end, start
		}
		if position >= start && position < end {
			return true
		}
	}
	return false
}

type columnRange struct {
	start int
	end   int
}

func (a *App) errorRanges(current *buffer.Buffer, line, lineLength int) []columnRange {
	ranges := make([]columnRange, 0)
	editorBuffer := a.editorBufferFor(current)
	if editorBuffer == nil {
		return ranges
	}
	for _, item := range editorBuffer.diagnostics {
		if !isErrorDiagnostic(item) || line < item.line || line > item.endLine {
			continue
		}
		start := 0
		if line == item.line {
			start = pointFromLSP(current, line, item.column).Column
		}
		end := lineLength
		if line == item.endLine {
			end = pointFromLSP(current, line, item.endColumn).Column
		}
		if item.line == item.endLine && end <= start {
			end = start + 1
		}
		if start < end {
			ranges = append(ranges, columnRange{start: start, end: end})
		}
	}
	return ranges
}

func columnInRanges(column int, ranges []columnRange) bool {
	for _, current := range ranges {
		if column >= current.start && column < current.end {
			return true
		}
	}
	return false
}

func isErrorDiagnostic(item diagnostic) bool {
	return item.severity == 1
}

func (a *App) diagnosticAtCursor() (diagnostic, bool) {
	current := a.current()
	point := current.Cursors()[0].Point
	for _, item := range a.currentEditorBuffer().diagnostics {
		if !isErrorDiagnostic(item) || point.Line < item.line || point.Line > item.endLine {
			continue
		}
		start := 0
		if point.Line == item.line {
			start = pointFromLSP(current, point.Line, item.column).Column
		}
		end := len([]rune(string(current.Line(point.Line))))
		if point.Line == item.endLine {
			end = pointFromLSP(current, point.Line, item.endColumn).Column
		}
		if item.line == item.endLine && end <= start {
			end = start + 1
		}
		if point.Column >= start && point.Column < end {
			return item, true
		}
	}
	return diagnostic{}, false
}

func (a *App) visibleDiagnosticAtCursor() (diagnostic, bool) {
	if !a.showDiagnostic || a.palette != nil || a.minibuffer != nil {
		return diagnostic{}, false
	}
	return a.diagnosticAtCursor()
}

func kindAt(spans []syntax.Span, column int) syntax.Kind {
	for _, span := range spans {
		if column >= span.Start && column < span.End {
			return span.Kind
		}
	}
	return syntax.Plain
}

func truncate(value string, width int) string {
	value = strings.ReplaceAll(value, "\t", "    ")
	if displayWidth(value) <= width {
		return value
	}
	if width <= 1 {
		return ""
	}
	result := make([]rune, 0, width)
	used := 0
	for _, current := range value {
		cellWidth := terminal.RuneWidth(current)
		if used+cellWidth > width-1 {
			break
		}
		result = append(result, current)
		used += cellWidth
	}
	return string(result) + "…"
}

func displayWidth(value string) int {
	return sourceDisplayWidth([]rune(value))
}

func sourceDisplayWidth(values []rune) int {
	width := 0
	for _, value := range values {
		if value == '\t' {
			width += 4 - width%4
			continue
		}
		width += terminal.RuneWidth(value)
	}
	return width
}
