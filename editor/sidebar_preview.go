package editor

import (
	"strings"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/terminal"
)

type sidebarPreviewLineKind uint8

const (
	previewPlain sidebarPreviewLineKind = iota
	previewMeta
	previewHunk
	previewAdded
	previewRemoved
)

type sidebarPreviewLine struct {
	text        string
	kind        sidebarPreviewLineKind
	changeStart int
	changeEnd   int
}

func buildSidebarPreviewLines(preview plugin.SidebarPreview) []sidebarPreviewLine {
	text := strings.ReplaceAll(string(preview.Content), "\r\n", "\n")
	text = strings.TrimSuffix(text, "\n")
	if text == "" {
		return nil
	}
	raw := strings.Split(text, "\n")
	lines := make([]sidebarPreviewLine, len(raw))
	for index, line := range raw {
		lines[index] = sidebarPreviewLine{text: line}
		if preview.Syntax != "diff" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "),
			strings.HasPrefix(line, "--- "), strings.HasPrefix(line, "+++ "):
			lines[index].kind = previewMeta
		case strings.HasPrefix(line, "@@"):
			lines[index].kind = previewHunk
		case strings.HasPrefix(line, "+"):
			lines[index].kind = previewAdded
		case strings.HasPrefix(line, "-"):
			lines[index].kind = previewRemoved
		}
	}
	if preview.Syntax == "diff" {
		markChangedDiffCharacters(lines)
	}
	return lines
}

func markChangedDiffCharacters(lines []sidebarPreviewLine) {
	for index := 0; index < len(lines); {
		if lines[index].kind != previewRemoved {
			index++
			continue
		}
		removedStart := index
		for index < len(lines) && lines[index].kind == previewRemoved {
			index++
		}
		addedStart := index
		for index < len(lines) && lines[index].kind == previewAdded {
			index++
		}
		pairs := min(addedStart-removedStart, index-addedStart)
		for offset := 0; offset < pairs; offset++ {
			removed := &lines[removedStart+offset]
			added := &lines[addedStart+offset]
			removedStartColumn, removedEndColumn, addedStartColumn, addedEndColumn :=
				changedRuneRanges(strings.TrimPrefix(removed.text, "-"), strings.TrimPrefix(added.text, "+"))
			removed.changeStart = removedStartColumn + 1
			removed.changeEnd = removedEndColumn + 1
			added.changeStart = addedStartColumn + 1
			added.changeEnd = addedEndColumn + 1
		}
	}
}

func changedRuneRanges(before, after string) (int, int, int, int) {
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

func (a *App) renderFullScreenSidebar(statusRow int) int {
	width, _ := a.screen.Size()
	panelHeight := statusRow - 1
	if panelHeight < 2 {
		return 0
	}
	leftWidth := min(46, max(28, width/3))
	leftWidth = min(leftWidth, max(12, width-20))
	rightLeft := leftWidth - 1
	rightWidth := width - rightLeft
	panelStyle := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Panel}
	baseBorder := terminal.Style{Foreground: a.theme.PanelBorder, Background: a.theme.Panel}
	leftBorder := baseBorder
	rightBorder := baseBorder
	if a.sidebar.previewFocused {
		rightBorder.Foreground = a.theme.Accent
	} else {
		leftBorder.Foreground = a.theme.Accent
	}

	a.drawPanel(0, 1, leftWidth, panelHeight, a.sidebar.title, leftBorder.Foreground, panelStyle, leftBorder)
	previewTitle := a.sidebar.preview.Title
	if previewTitle == "" {
		if item, ok := a.sidebar.selectedItem(); ok {
			previewTitle = item.Value
		} else {
			previewTitle = "Diff"
		}
	}
	a.drawPanel(rightLeft, 1, rightWidth, panelHeight, previewTitle, rightBorder.Foreground, panelStyle, rightBorder)
	if panelHeight < 5 {
		return 0
	}

	a.renderFullScreenSidebarList(leftWidth, panelHeight, panelStyle, leftBorder)
	a.renderSidebarPreview(rightLeft, rightWidth, panelHeight, panelStyle, rightBorder)
	return 0
}

func (a *App) renderFullScreenSidebarList(width, panelHeight int, panelStyle, border terminal.Style) {
	accent := terminal.Style{Foreground: a.theme.Accent, Background: a.theme.Panel, Bold: true}
	selected := terminal.Style{Foreground: a.theme.StatusText, Background: a.theme.Selection, Bold: true}
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
			a.screen.Text(2, y, truncate(entry.title, width-4), style)
			continue
		}
		style := panelStyle
		if index == a.sidebar.selected {
			style = selected
			fillRow(a.screen, 1, y, width-2, style)
		}
		detail := entry.item.Detail
		detailWidth := displayWidth(detail)
		labelWidth := width - 4
		if detail != "" {
			labelWidth -= detailWidth + 1
		}
		a.screen.Text(2, y, truncate(entry.item.Label, max(0, labelWidth)), style)
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
			a.screen.Text(width-detailWidth-2, y, detail, detailStyle)
		}
	}

	footer := panelHeight - 2
	a.drawPanelSeparator(0, footer, width, border)
	help := []styledText{
		{text: "Enter/Tab", style: accent}, {text: " diff  ", style: border},
	}
	for _, item := range a.sidebar.help {
		help = append(help,
			styledText{text: item.Key, style: accent},
			styledText{text: " " + item.Label + "  ", style: border},
		)
	}
	help = append(help,
		styledText{text: "Esc", style: accent}, styledText{text: " close", style: border},
	)
	drawStyledText(a.screen, 2, footer+1, width-4, help)
}

func (a *App) renderSidebarPreview(left, width, panelHeight int, panelStyle, border terminal.Style) {
	footer := panelHeight - 2
	contentWidth := max(0, width-2)
	contentHeight := max(0, panelHeight-4)
	maxLeft := max(0, a.sidebar.previewWidth()-contentWidth)
	a.sidebar.previewLeft = min(a.sidebar.previewLeft, maxLeft)
	a.sidebar.previewTop = min(a.sidebar.previewTop, max(0, len(a.sidebar.previewLines)-contentHeight))

	if a.sidebar.previewError != nil {
		style := terminal.Style{Foreground: a.theme.Danger, Background: a.theme.Panel}
		a.screen.Text(left+2, 2, truncate(a.sidebar.previewError.Error(), width-4), style)
	} else if a.sidebar.previewLoading && len(a.sidebar.previewLines) == 0 {
		style := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
		a.screen.Text(left+2, 2, "Loading diff…", style)
	} else if len(a.sidebar.previewLines) == 0 {
		style := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
		a.screen.Text(left+2, 2, "(no changes)", style)
	} else {
		for row := 0; row < contentHeight && a.sidebar.previewTop+row < len(a.sidebar.previewLines); row++ {
			line := a.sidebar.previewLines[a.sidebar.previewTop+row]
			style, changed := a.sidebarPreviewStyles(line.kind)
			fillRow(a.screen, left+1, 2+row, contentWidth, style)
			a.drawSidebarPreviewLine(left+1, 2+row, contentWidth, a.sidebar.previewLeft, line, style, changed)
		}
	}

	a.drawPanelSeparator(left, footer, width, border)
	accent := terminal.Style{Foreground: a.theme.Accent, Background: a.theme.Panel, Bold: true}
	help := []styledText{
		{text: "↑↓", style: accent}, {text: " scroll  ", style: border},
		{text: "←→", style: accent}, {text: " pan  ", style: border},
		{text: "Tab", style: accent}, {text: " changes", style: border},
	}
	drawStyledText(a.screen, left+2, footer+1, width-4, help)
}

func (a *App) sidebarPreviewStyles(kind sidebarPreviewLineKind) (terminal.Style, terminal.Style) {
	style := terminal.Style{Foreground: a.theme.Foreground, Background: a.theme.Panel}
	changed := style
	switch kind {
	case previewMeta:
		style.Foreground = a.theme.Muted
		changed = style
	case previewHunk:
		style.Foreground = a.theme.Accent
		style.Background = blendColor(a.theme.Panel, a.theme.Accent, 14)
		changed = style
	case previewAdded:
		style.Background = blendColor(a.theme.Panel, a.theme.Success, 22)
		changed = style
		changed.Background = blendColor(a.theme.Panel, a.theme.Success, 44)
	case previewRemoved:
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

func (a *App) drawSidebarPreviewLine(
	left, row, width, scroll int,
	line sidebarPreviewLine,
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
