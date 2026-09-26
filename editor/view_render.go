package editor

import (
	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

type viewPaneBounds struct {
	left   int
	top    int
	width  int
	height int
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
	if view.direction == ui.LayoutVertical {
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
		if view.direction == ui.LayoutVertical {
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
	if pane.content.Title != "" {
		title = pane.content.Title
	}
	a.drawPanel(bounds.left, bounds.top, bounds.width, bounds.height, title, border.Foreground, panelStyle, border)
	if bounds.height < 5 {
		return
	}
	if pane.list != nil {
		a.renderViewList(pane, bounds, panelStyle, border)
		return
	}
	a.renderViewWidget(pane, bounds, panelStyle, border)
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
			case ui.ToneSuccess:
				detailStyle.Foreground = a.theme.Success
			case ui.ToneWarning:
				detailStyle.Foreground = a.theme.Warning
			case ui.ToneDanger:
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

func (a *App) renderViewWidget(pane *viewPane, bounds viewPaneBounds, panelStyle, border terminal.Style) {
	contentWidth := max(0, bounds.width-2)
	contentHeight := max(0, bounds.height-4)
	maxLeft := max(0, pane.widgetWidth()-contentWidth)
	pane.left = min(pane.left, maxLeft)
	pane.top = min(pane.top, max(0, len(pane.content.Lines)-contentHeight))
	contentTop := bounds.top + 1
	if pane.err != nil {
		style := terminal.Style{Foreground: a.theme.Danger, Background: a.theme.Panel}
		a.screen.Text(bounds.left+2, contentTop, truncate(pane.err.Error(), bounds.width-4), style)
	} else if pane.loading && len(pane.content.Lines) == 0 {
		style := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
		a.screen.Text(bounds.left+2, contentTop, "Loading…", style)
	} else if len(pane.content.Lines) == 0 {
		style := terminal.Style{Foreground: a.theme.Muted, Background: a.theme.Panel}
		a.screen.Text(bounds.left+2, contentTop, "(empty)", style)
	} else {
		for row := 0; row < contentHeight && pane.top+row < len(pane.content.Lines); row++ {
			line := pane.content.Lines[pane.top+row]
			style := a.resolveWidgetStyle(panelStyle, line.Style)
			fillRow(a.screen, bounds.left+1, contentTop+row, contentWidth, style)
			a.drawRichTextLine(bounds.left+1, contentTop+row, contentWidth, pane.left, line, style)
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

func (a *App) resolveWidgetStyle(base terminal.Style, style ui.WidgetStyle) terminal.Style {
	if style.Foreground != ui.ToneDefault {
		base.Foreground = a.widgetTone(style.Foreground)
	}
	if style.Background != ui.ToneDefault {
		intensity := int(style.BackgroundIntensity)
		if intensity == 0 {
			intensity = 100
		}
		base.Background = blendColor(a.theme.Panel, a.widgetTone(style.Background), intensity)
	}
	if style.Bold {
		base.Bold = true
	}
	return base
}

func (a *App) widgetTone(tone ui.Tone) terminal.Color {
	switch tone {
	case ui.ToneForeground:
		return a.theme.Foreground
	case ui.TonePanel:
		return a.theme.Panel
	case ui.ToneAccent:
		return a.theme.Accent
	case ui.ToneMuted:
		return a.theme.Muted
	case ui.ToneSuccess:
		return a.theme.Success
	case ui.ToneWarning:
		return a.theme.Warning
	case ui.ToneDanger:
		return a.theme.Danger
	default:
		return a.theme.Foreground
	}
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

func (a *App) drawRichTextLine(
	left, row, width, scroll int,
	line ui.RichTextLine,
	lineStyle terminal.Style,
) {
	displayColumn := 0
	for _, span := range line.Spans {
		style := a.resolveWidgetStyle(lineStyle, span.Style)
		if span.Positioned {
			displayColumn = max(0, span.Column)
		}
		for _, value := range []rune(span.Text) {
			value = safeWidgetRune(value)
			cellWidth := terminal.RuneWidth(value)
			if value == '\t' {
				cellWidth = 4 - displayColumn%4
			}
			if displayColumn+cellWidth > scroll && displayColumn-scroll < width {
				x := left + max(0, displayColumn-scroll)
				if value == '\t' || displayColumn < scroll {
					for offset := max(scroll-displayColumn, 0); offset < cellWidth && x < left+width; offset++ {
						a.screen.Set(x, row, ' ', style)
						x++
					}
				} else if x+cellWidth <= left+width {
					a.screen.Set(x, row, value, style)
				}
			}
			displayColumn += cellWidth
		}
	}
}

func safeWidgetRune(value rune) rune {
	if value != '\t' && (value < ' ' || value == 0x7f) {
		return '�'
	}
	return value
}
