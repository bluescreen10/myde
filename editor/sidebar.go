package editor

import (
	"time"

	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

type sidebarRow struct {
	title string
	item  ui.SidebarItem
	set   bool
}

type sidebarPanel struct {
	title           string
	width           int
	message         string
	input           *ui.TextInput
	query           []rune
	cursor          int
	onInput         func(string) (ui.Sidebar, error)
	onClose         func(ui.SidebarState)
	beforeRefresh   func()
	rows            []sidebarRow
	selected        int
	top             int
	help            []ui.KeyHelp
	onAction        func(ui.Action, ui.SidebarItem) error
	onRefresh       func() (ui.Sidebar, error)
	refreshInterval time.Duration
	nextRefresh     time.Time
	refreshing      bool
}

func newSidebarPanel(sidebar ui.Sidebar) *sidebarPanel {
	panel := &sidebarPanel{
		title:           sidebar.Title,
		width:           sidebar.Width,
		message:         sidebar.Message,
		onInput:         sidebar.OnInput,
		onClose:         sidebar.OnClose,
		beforeRefresh:   sidebar.BeforeRefresh,
		top:             sidebar.Top,
		help:            append([]ui.KeyHelp(nil), sidebar.Help...),
		onAction:        sidebar.OnAction,
		onRefresh:       sidebar.OnRefresh,
		refreshInterval: sidebar.RefreshInterval,
	}
	if sidebar.Input != nil {
		input := *sidebar.Input
		panel.input = &input
		panel.query = []rune(input.Value)
		panel.cursor = len(panel.query)
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

func (p *sidebarPanel) selectedItem() (ui.SidebarItem, bool) {
	if p.selected < 0 || p.selected >= len(p.rows) || !p.rows[p.selected].set {
		return ui.SidebarItem{}, false
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
		return panel.perform(ui.Activate)
	case terminal.KeyTab:
		a.CloseSidebar()
	case terminal.KeyRune:
		if event.Control || event.Alt || event.Super {
			return nil
		}
		switch event.Rune {
		case '+':
			return panel.perform(ui.Add)
		case '-':
			return panel.perform(ui.Remove)
		}
	}
	return nil
}

func (p *sidebarPanel) perform(action ui.Action) error {
	if p.onAction == nil || p.selected < 0 || p.selected >= len(p.rows) {
		return nil
	}
	row := p.rows[p.selected]
	if !row.set {
		return nil
	}
	return p.onAction(action, row.item)
}

func (a *App) sidebarWidth(width int) int {
	if a.sidebar != nil {
		if a.sidebar.width > 0 {
			return min(a.sidebar.width, max(12, width/2))
		}
		return fileSidebarWidth(width)
	}
	if a.showFiles {
		return fileSidebarWidth(width)
	}
	return 0
}

func (p *sidebarPanel) contentTop() int {
	if p.input != nil {
		return 4
	}
	return 2
}

func (a *App) handleSidebarInput(event terminal.Event) (bool, error) {
	panel := a.sidebar
	handled, changed := a.editTextInput(&panel.query, &panel.cursor, event)
	if !handled || !changed {
		return handled, nil
	}
	if panel.onInput == nil {
		return true, nil
	}
	updated, err := panel.onInput(string(panel.query))
	if err != nil {
		return true, err
	}
	replacement := newSidebarPanel(updated)
	replacement.cursor = min(panel.cursor, len(replacement.query))
	a.sidebar = replacement
	return true, nil
}
