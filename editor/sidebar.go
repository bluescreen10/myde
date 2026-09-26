package editor

import (
	"time"

	"github.com/bluescreen10/myde/plugin"
	"github.com/bluescreen10/myde/terminal"
)

type sidebarRow struct {
	title string
	item  plugin.SidebarItem
	set   bool
}

type sidebarPanel struct {
	title             string
	rows              []sidebarRow
	selected          int
	top               int
	fullScreen        bool
	previewFocused    bool
	preview           plugin.SidebarPreview
	previewLines      []sidebarPreviewLine
	previewTop        int
	previewLeft       int
	previewValue      string
	previewKind       string
	previewLoading    bool
	previewError      error
	previewGeneration uint64
	help              []plugin.KeyHelp
	onAction          func(plugin.Action, plugin.SidebarItem) error
	onPreview         func(plugin.SidebarItem) (plugin.SidebarPreview, error)
	onRefresh         func() (plugin.Sidebar, error)
	refreshInterval   time.Duration
	nextRefresh       time.Time
	refreshing        bool
}

func newSidebarPanel(sidebar plugin.Sidebar) *sidebarPanel {
	panel := &sidebarPanel{
		title:           sidebar.Title,
		fullScreen:      sidebar.FullScreen,
		help:            append([]plugin.KeyHelp(nil), sidebar.Help...),
		onAction:        sidebar.OnAction,
		onPreview:       sidebar.OnPreview,
		onRefresh:       sidebar.OnRefresh,
		refreshInterval: sidebar.RefreshInterval,
	}
	if panel.onRefresh != nil && panel.refreshInterval > 0 {
		panel.nextRefresh = time.Now().Add(panel.refreshInterval)
	}
	for _, section := range sidebar.Sections {
		panel.rows = append(panel.rows, sidebarRow{title: section.Title})
		if len(section.Items) == 0 {
			panel.rows = append(panel.rows, sidebarRow{title: "  (none)"})
			continue
		}
		for _, item := range section.Items {
			panel.rows = append(panel.rows, sidebarRow{item: item, set: true})
		}
	}
	panel.selected = panel.firstSelectable()
	if sidebar.SelectedValue != "" {
		for index, row := range panel.rows {
			if row.set && row.item.Value == sidebar.SelectedValue &&
				(sidebar.SelectedKind == "" || row.item.Kind == sidebar.SelectedKind) {
				panel.selected = index
				break
			}
		}
	}
	return panel
}

func (p *sidebarPanel) selectedItem() (plugin.SidebarItem, bool) {
	if p.selected < 0 || p.selected >= len(p.rows) || !p.rows[p.selected].set {
		return plugin.SidebarItem{}, false
	}
	return p.rows[p.selected].item, true
}

func (p *sidebarPanel) firstSelectable() int {
	for index, row := range p.rows {
		if row.set {
			return index
		}
	}
	return 0
}

func (p *sidebarPanel) move(delta int) {
	if len(p.rows) == 0 || delta == 0 {
		return
	}
	direction := 1
	if delta < 0 {
		direction = -1
	}
	remaining := max(delta, -delta)
	for remaining > 0 {
		candidate := p.selected + direction
		for candidate >= 0 && candidate < len(p.rows) && !p.rows[candidate].set {
			candidate += direction
		}
		if candidate < 0 || candidate >= len(p.rows) {
			return
		}
		p.selected = candidate
		remaining--
	}
}

func (p *sidebarPanel) moveToEnd(end bool) {
	if end {
		for index := len(p.rows) - 1; index >= 0; index-- {
			if p.rows[index].set {
				p.selected = index
				return
			}
		}
		return
	}
	p.selected = p.firstSelectable()
}

func (p *sidebarPanel) ensureVisible(rows int) {
	if rows <= 0 {
		p.top = 0
		return
	}
	if p.selected < p.top {
		p.top = p.selected
	}
	if p.selected >= p.top+rows {
		p.top = p.selected - rows + 1
	}
	p.top = min(max(0, p.top), max(0, len(p.rows)-rows))
}

func (a *App) handleSidebarEvent(event terminal.Event) error {
	panel := a.sidebar
	if panel.fullScreen && panel.previewFocused {
		return a.handleSidebarPreviewEvent(event)
	}
	before := panel.selected
	switch event.Key {
	case terminal.KeyUp:
		panel.move(-1)
	case terminal.KeyDown:
		panel.move(1)
	case terminal.KeyPageUp:
		_, height := a.screen.Size()
		panel.move(-max(1, height-6))
	case terminal.KeyPageDown:
		_, height := a.screen.Size()
		panel.move(max(1, height-6))
	case terminal.KeyHome:
		panel.moveToEnd(false)
	case terminal.KeyEnd:
		panel.moveToEnd(true)
	case terminal.KeyEnter:
		if panel.fullScreen && panel.onPreview != nil {
			panel.previewFocused = true
			return nil
		}
		return panel.perform(plugin.Activate)
	case terminal.KeyTab:
		if panel.fullScreen && panel.onPreview != nil {
			panel.previewFocused = true
			return nil
		}
		a.CloseSidebar()
	case terminal.KeyRune:
		if event.Control || event.Alt || event.Super {
			return nil
		}
		switch event.Rune {
		case '+':
			return panel.perform(plugin.Add)
		case '-':
			return panel.perform(plugin.Remove)
		}
	}
	if panel.selected != before {
		a.requestSidebarPreview()
	}
	return nil
}

func (a *App) handleSidebarPreviewEvent(event terminal.Event) error {
	panel := a.sidebar
	visibleRows := a.sidebarPreviewHeight()
	switch event.Key {
	case terminal.KeyTab:
		panel.previewFocused = false
	case terminal.KeyUp:
		panel.previewTop--
	case terminal.KeyDown:
		panel.previewTop++
	case terminal.KeyPageUp:
		panel.previewTop -= max(1, visibleRows-1)
	case terminal.KeyPageDown:
		panel.previewTop += max(1, visibleRows-1)
	case terminal.KeyHome:
		panel.previewTop = 0
	case terminal.KeyEnd:
		panel.previewTop = len(panel.previewLines) - visibleRows
	case terminal.KeyLeft:
		panel.previewLeft--
	case terminal.KeyRight:
		panel.previewLeft++
	}
	panel.previewTop = min(max(0, panel.previewTop), max(0, len(panel.previewLines)-visibleRows))
	panel.previewLeft = min(max(0, panel.previewLeft), max(0, panel.previewWidth()-1))
	return nil
}

func (a *App) sidebarPreviewHeight() int {
	_, height := a.screen.Size()
	return max(1, height-6)
}

func (p *sidebarPanel) previewWidth() int {
	width := 0
	for _, line := range p.previewLines {
		width = max(width, displayWidth(line.text))
	}
	return width
}

func (a *App) requestSidebarPreview() {
	panel := a.sidebar
	if panel == nil || !panel.fullScreen || panel.onPreview == nil {
		return
	}
	item, ok := panel.selectedItem()
	if !ok {
		panel.preview = plugin.SidebarPreview{}
		panel.previewLines = nil
		panel.previewLoading = false
		panel.previewError = nil
		return
	}
	if item.Value != panel.previewValue || item.Kind != panel.previewKind {
		panel.preview = plugin.SidebarPreview{}
		panel.previewLines = nil
		panel.previewTop = 0
		panel.previewLeft = 0
	}
	panel.previewValue = item.Value
	panel.previewKind = item.Kind
	panel.previewGeneration++
	panel.previewLoading = true
	panel.previewError = nil
	generation := panel.previewGeneration
	load := panel.onPreview
	if a.servers == nil {
		preview, err := load(item)
		a.applySidebarPreviewEvent(&sidebarPreviewEvent{
			panel: panel, generation: generation, preview: preview, err: err,
		})
		return
	}
	go func() {
		preview, err := load(item)
		a.servers <- serverEvent{sidebarPreview: &sidebarPreviewEvent{
			panel: panel, generation: generation, preview: preview, err: err,
		}}
	}()
}

func (a *App) applySidebarPreviewEvent(event *sidebarPreviewEvent) {
	panel := a.sidebar
	if panel == nil || panel != event.panel || panel.previewGeneration != event.generation {
		return
	}
	panel.previewLoading = false
	panel.previewError = event.err
	if event.err != nil {
		return
	}
	panel.preview = event.preview
	panel.previewLines = buildSidebarPreviewLines(event.preview)
	panel.previewTop = min(panel.previewTop, max(0, len(panel.previewLines)-a.sidebarPreviewHeight()))
}

func (p *sidebarPanel) perform(action plugin.Action) error {
	if p.onAction == nil || p.selected < 0 || p.selected >= len(p.rows) {
		return nil
	}
	row := p.rows[p.selected]
	if !row.set {
		return nil
	}
	return p.onAction(action, row.item)
}
