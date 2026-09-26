package editor

import (
	"strings"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/terminal"
)

type viewDocumentLineKind uint8

const (
	viewLinePlain viewDocumentLineKind = iota
	viewLineMeta
	viewLineHunk
	viewLineAdded
	viewLineRemoved
)

type viewDocumentLine struct {
	text        string
	kind        viewDocumentLineKind
	changeStart int
	changeEnd   int
}

type viewPaneBounds struct {
	left   int
	top    int
	width  int
	height int
}

func buildViewDocumentLines(document plugin.ViewDocument) []viewDocumentLine {
	text := strings.ReplaceAll(string(document.Content), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	raw := strings.Split(text, "\n")
	lines := make([]viewDocumentLine, len(raw))
	for index, line := range raw {
		lines[index] = viewDocumentLine{text: line}
		if document.Syntax != "diff" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "),
			strings.HasPrefix(line, "new file "), strings.HasPrefix(line, "deleted file "),
			strings.HasPrefix(line, "rename from "), strings.HasPrefix(line, "rename to "):
			lines[index].kind = viewLineMeta
		case strings.HasPrefix(line, "@@"):
			lines[index].kind = viewLineHunk
		case strings.HasPrefix(line, "+"):
			lines[index].kind = viewLineAdded
		case strings.HasPrefix(line, "-"):
			lines[index].kind = viewLineRemoved
		}
	}
	if document.Syntax == "diff" {
		markChangedViewCharacters(lines)
	}
	return lines
}

func markChangedViewCharacters(lines []viewDocumentLine) {
	for index := 0; index < len(lines); {
		if lines[index].kind != viewLineRemoved {
			index++
			continue
		}
		removedStart := index
		for index < len(lines) && lines[index].kind == viewLineRemoved {
			index++
		}
		addedStart := index
		for index < len(lines) && lines[index].kind == viewLineAdded {
			index++
		}
		pairs := min(addedStart-removedStart, index-addedStart)
		for offset := 0; offset < pairs; offset++ {
			removed := &lines[removedStart+offset]
			added := &lines[addedStart+offset]
			removedStartColumn, removedEndColumn, addedStartColumn, addedEndColumn :=
				changedViewRuneRanges(strings.TrimPrefix(removed.text, "-"), strings.TrimPrefix(added.text, "+"))
			removed.changeStart = removedStartColumn + 1
			removed.changeEnd = removedEndColumn + 1
			added.changeStart = addedStartColumn + 1
			added.changeEnd = addedEndColumn + 1
		}
	}
}

func changedViewRuneRanges(before, after string) (int, int, int, int) {
	left := []rune(before)
	right := []rune(after)
	prefix := 0
	for prefix < len(left) && prefix < len(right) && left[prefix] == right[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(left)-prefix && suffix < len(right)-prefix &&
		left[len(left)-1-suffix] == right[len(right)-1-suffix] {
		suffix++
	}
	return prefix, len(left) - suffix, prefix, len(right) - suffix
}

func (a *App) renderView(view *viewPanel, statusRow int) {
	width, _ := a.screen.Size()
	bounds := layoutViewPanes(view, viewPaneBounds{left: 0, top: 1, width: width, height: statusRow - 1})
	view.focus = min(max(0, view.focus), max(0, len(view.panes)-1))
	for index, pane := range view.panes {
		if index >= len(bounds) {
			break
		}
		a.renderViewPane(pane, bounds[index], index == view.focus)
	}
}

func layoutViewPanes(view *viewPanel, available viewPaneBounds) []viewPaneBounds {
	count := len(view.panes)
	if count == 0 {
		return nil
	}
	totalWeight := 0
	for _, pane := range view.panes {
		totalWeight += max(1, pane.weight)
	}
	result := make([]viewPaneBounds, 0, count)
	position := available.left
	remaining := available.width + count - 1
	if view.direction == plugin.LayoutVertical {
		position = available.top
		remaining = available.height + count - 1
	}
	remainingWeight := totalWeight
	for index, pane := range view.panes {
		size := remaining
		if index < count-1 {
			size = remaining * max(1, pane.weight) / remainingWeight
			minimumRemaining := (count - index - 1) * 3
			size = min(max(3, size), max(3, remaining-minimumRemaining))
		}
		bounds := available
		if view.direction == plugin.LayoutVertical {
			bounds.top = position
			bounds.height = size
		} else {
			bounds.left = position
			bounds.width = size
		}
		result = append(result, bounds)
		position += size - 1
		remaining -= size
		remainingWeight -= max(1, pane.weight)
	}
	return result
}

func (a *App) renderViewPane(pane *viewPane, bounds viewPaneBounds, focused bool) {
	if bounds.width < 2 || bounds.height < 2 {
		return
	}
	panelStyle := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Panel}
	border := terminal.Style{Foreground: a.theme.PanelBorder, Background: a.theme.Panel}
	if focused {
		border.Foreground = a.theme.Accent
	}
	title := pane.title
	if pane.document.Title != "" {
		title = pane.document.Title
	}
	a.drawPanel(bounds.left, bounds.top, bounds.width, bounds.height, title, border.Foreground, panelStyle, border)
	if bounds.height < 5 {
		return
	}
	if pane.list != nil {
		a.renderViewList(pane, bounds, panelStyle, border)
		return
	}
	a.renderViewDocument(pane, bounds, panelStyle, border)
}

func (a *App) renderViewList(pane *viewPane, bounds viewPaneBounds, panelStyle, border terminal.Style) {
	list := pane.list
	accent := terminal.Style{Foreground: a.theme.Accent, Background: a.theme.Panel, Bold: true}
	selected := terminal.Style{Foreground: a.theme.StatusText, Background: a.theme.Selection, Bold: true}
	contentHeight := max(0, bounds.height-4)
	list.ensureVisible(contentHeight)
	for row := 0; row < contentHeight && list.top+row < len(list.rows); row++ {
		index := list.top + row
		entry := list.rows[index]
		y := bounds.top + 1 + row
		if !entry.set {
			style := border
			if len(entry.title) > 0 && entry.title[0] != ' ' {
				style = accent
			}
			a.screen.Text(bounds.left+2, y, truncate(entry.title, bounds.width-4), style)
			continue
		}
		style := panelStyle
		if index == list.selected {
			style = selected
			fillRow(a.screen, bounds.left+1, y, bounds.width-2, style)
		}
		detail := entry.item.Detail
		detailWidth := displayWidth(detail)
		labelWidth := bounds.width - 4
		if detail != "" {
			labelWidth -= detailWidth + 1
		}
		a.screen.Text(bounds.left+2, y, truncate(entry.item.Label, max(0, labelWidth)), style)
		if detail != "" {
			detailStyle := style
			switch entry.item.DetailTone {
			case plugin.ToneSuccess:
				detailStyle.Foreground = a.theme.Success
			case plugin.ToneWarning:
				detailStyle.Foreground = a.theme.Warning
			case plugin.ToneDanger:
				detailStyle.Foreground = a.theme.Danger
			}
			a.screen.Text(bounds.left+bounds.width-detailWidth-2, y, detail, detailStyle)
		}
	}

	footer := bounds.top + bounds.height - 3
	a.drawPanelSeparator(bounds.left, footer, bounds.width, border)
	help := []styledText{
		{text: "Enter", style: accent}, {text: " preview  ", style: border},
		{text: "Tab", style: accent}, {text: " pane  ", style: border},
	}
	for _, item := range list.help {
		help = append(help,
			styledText{text: item.Key, style: accent},
			styledText{text: " " + item.Label + "  ", style: border},
		)
	}
	drawStyledText(a.screen, bounds.left+2, footer+1, bounds.width-4, help)
}

func (a *App) renderViewDocument(pane *viewPane, bounds viewPaneBounds, panelStyle, border terminal.Style) {
	contentWidth := max(0, bounds.width-2)
	contentHeight := max(0, bounds.height-4)
	maxLeft := max(0, pane.documentWidth()-contentWidth)
	pane.left = min(pane.left, maxLeft)
	pane.top = min(pane.top, max(0, len(pane.lines)-contentHeight))
	contentTop := bounds.top + 1
	if pane.err != nil {
		style := terminal.Style{Foreground: a.theme.Danger, Background: a.theme.Panel}
		a.screen.Text(bounds.left+2, contentTop, truncate(pane.err.Error(), bounds.width-4), style)
	} else if pane.loading && len(pane.lines) == 0 {
		style := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
		a.screen.Text(bounds.left+2, contentTop, "Loading…", style)
	} else if len(pane.lines) == 0 {
		style := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
		a.screen.Text(bounds.left+2, contentTop, "(empty)", style)
	} else {
		for row := 0; row < contentHeight && pane.top+row < len(pane.lines); row++ {
			line := pane.lines[pane.top+row]
			style, changed := a.viewDocumentStyles(line.kind)
			fillRow(a.screen, bounds.left+1, contentTop+row, contentWidth, style)
			a.drawViewDocumentLine(bounds.left+1, contentTop+row, contentWidth, pane.left, line, style, changed)
		}
	}
	footer := bounds.top + bounds.height - 3
	a.drawPanelSeparator(bounds.left, footer, bounds.width, border)
	accent := terminal.Style{Foreground: a.theme.Accent, Background: a.theme.Panel, Bold: true}
	help := []styledText{
		{text: "↑↓", style: accent}, {text: " scroll  ", style: border},
		{text: "←→", style: accent}, {text: " pan  ", style: border},
		{text: "Tab", style: accent}, {text: " pane", style: border},
	}
	drawStyledText(a.screen, bounds.left+2, footer+1, bounds.width-4, help)
}

func (a *App) viewDocumentStyles(kind viewDocumentLineKind) (terminal.Style, terminal.Style) {
	style := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Panel}
	changed := style
	switch kind {
	case viewLineMeta:
		style.Foreground = a.theme.Muted
		changed = style
	case viewLineHunk:
		style.Foreground = a.theme.Accent
		style.Background = blendColor(a.theme.Panel, a.theme.Accent, 14)
		changed = style
	case viewLineAdded:
		style.Background = blendColor(a.theme.Panel, a.theme.Success, 22)
		changed = style
		changed.Background = blendColor(a.theme.Panel, a.theme.Success, 44)
	case viewLineRemoved:
		style.Background = blendColor(a.theme.Panel, a.theme.Danger, 22)
		changed = style
		changed.Background = blendColor(a.theme.Panel, a.theme.Danger, 44)
	}
	return style, changed
}

func blendColor(base, tint terminal.Color, percent int) terminal.Color {
	percent = min(max(0, percent), 100)
	blend := func(left, right uint8) uint8 {
		return uint8((int(left)*(100-percent) + int(right)*percent) / 100)
	}
	return terminal.Color{
		R: blend(base.R, tint.R),
		G: blend(base.G, tint.G),
		B: blend(base.B, tint.B),
	}
}

func (a *App) drawViewDocumentLine(
	left, row, width, scroll int,
	line viewDocumentLine,
	style, changed terminal.Style,
) {
	displayColumn := 0
	for column, value := range []rune(line.text) {
		currentStyle := style
		if column >= line.changeStart && column < line.changeEnd {
			currentStyle = changed
		}
		cellWidth := terminal.RuneWidth(value)
		if value == '\t' {
			cellWidth = 4 - displayColumn%4
		}
		if displayColumn+cellWidth > scroll && displayColumn-scroll < width {
			x := left + max(0, displayColumn-scroll)
			if value == '\t' || displayColumn < scroll {
				for offset := max(scroll-displayColumn, 0); offset < cellWidth && x < left+width; offset++ {
					a.screen.Set(x, row, ' ', currentStyle)
					x++
				}
			} else if x+cellWidth <= left+width {
				a.screen.Set(x, row, value, currentStyle)
			}
		}
		displayColumn += cellWidth
	}
}
