package editor

import (
	"fmt"
	"time"

	"github.com/bluescreen10/myde/buffer"
	"github.com/bluescreen10/myde/terminal"
	"github.com/bluescreen10/myde/ui"
)

type viewRow struct {
	title string
	item  ui.ListItem
	set   bool
}

type viewList struct {
	rows         []viewRow
	selected     int
	top          int
	selectionSet bool
	help         []ui.KeyHelp
	previewPane  string
	onAction     func(ui.Action, ui.ListItem) error
	onSelect     func(ui.ListItem) (ui.Widget, error)
}

type viewPane struct {
	id            string
	title         string
	weight        int
	list          *viewList
	widget        ui.Widget
	content       ui.WidgetContent
	top           int
	left          int
	loading       bool
	err           error
	generation    uint64
	selectedKind  string
	selectedValue string
}

type viewPanel struct {
	id              uint64
	title           string
	direction       ui.LayoutDirection
	panes           []*viewPane
	focus           int
	onRefresh       func() (ui.View, error)
	refreshInterval time.Duration
	nextRefresh     time.Time
	refreshing      bool
}

type viewPreviewEvent struct {
	viewID     uint64
	paneID     string
	pane       *viewPane
	generation uint64
	widget     ui.Widget
	content    ui.WidgetContent
	err        error
}

type viewRefreshEvent struct {
	viewID  uint64
	view    *viewPanel
	updated ui.View
	err     error
}

type viewHandle struct {
	app *App
	id  uint64
}

func newViewPanel(id uint64, view ui.View) *viewPanel {
	panel := &viewPanel{
		id:              id,
		title:           view.Title,
		direction:       view.Layout.Direction,
		onRefresh:       view.OnRefresh,
		refreshInterval: view.RefreshInterval,
	}
	if panel.direction == "" {
		panel.direction = ui.LayoutHorizontal
	}
	if panel.onRefresh != nil && panel.refreshInterval > 0 {
		panel.nextRefresh = time.Now().Add(panel.refreshInterval)
	}
	for index, definition := range view.Layout.Panes {
		pane := &viewPane{
			id: definition.ID, title: definition.Title, weight: max(1, definition.Weight),
		}
		if pane.id == "" {
			pane.id = fmt.Sprintf("pane-%d", index)
		}
		if definition.List != nil {
			pane.list = newViewList(*definition.List)
		}
		if definition.Widget != nil {
			pane.widget = definition.Widget
			pane.content = renderWidget(definition.Widget)
		}
		panel.panes = append(panel.panes, pane)
	}
	return panel
}

func newViewList(definition ui.List) *viewList {
	list := &viewList{
		help:         append([]ui.KeyHelp(nil), definition.Help...),
		previewPane:  definition.PreviewPane,
		onAction:     definition.OnAction,
		onSelect:     definition.OnSelect,
		selectionSet: definition.SelectedValue != "",
	}
	for _, section := range definition.Sections {
		list.rows = append(list.rows, viewRow{title: section.Title})
		if len(section.Items) == 0 {
			list.rows = append(list.rows, viewRow{title: "  (none)"})
			continue
		}
		for _, item := range section.Items {
			list.rows = append(list.rows, viewRow{item: item, set: true})
		}
	}
	list.selected = list.firstSelectable()
	if definition.SelectedValue != "" {
		list.selectItem(definition.SelectedKind, definition.SelectedValue)
	}
	return list
}

func renderWidget(widget ui.Widget) ui.WidgetContent {
	if widget == nil {
		return ui.WidgetContent{}
	}
	return cloneWidgetContent(widget.RenderWidget())
}

func cloneWidgetContent(content ui.WidgetContent) ui.WidgetContent {
	cloned := ui.WidgetContent{Title: content.Title, Lines: make([]ui.RichTextLine, len(content.Lines))}
	for index, line := range content.Lines {
		cloned.Lines[index] = line
		cloned.Lines[index].Spans = append([]ui.RichTextSpan(nil), line.Spans...)
	}
	return cloned
}

func (l *viewList) selectedItem() (ui.ListItem, bool) {
	if l == nil || l.selected < 0 || l.selected >= len(l.rows) || !l.rows[l.selected].set {
		return ui.ListItem{}, false
	}
	return l.rows[l.selected].item, true
}

func (l *viewList) firstSelectable() int {
	for index, row := range l.rows {
		if row.set {
			return index
		}
	}
	return 0
}

func (l *viewList) selectItem(kind, value string) bool {
	for index, row := range l.rows {
		if row.set && row.item.Value == value && (kind == "" || row.item.Kind == kind) {
			l.selected = index
			return true
		}
	}
	return false
}

func (l *viewList) move(delta int) {
	if len(l.rows) == 0 || delta == 0 {
		return
	}
	direction := 1
	if delta < 0 {
		direction = -1
	}
	for remaining := max(delta, -delta); remaining > 0; remaining-- {
		candidate := l.selected + direction
		for candidate >= 0 && candidate < len(l.rows) && !l.rows[candidate].set {
			candidate += direction
		}
		if candidate < 0 || candidate >= len(l.rows) {
			return
		}
		l.selected = candidate
	}
}

func (l *viewList) moveToEnd(end bool) {
	if !end {
		l.selected = l.firstSelectable()
		return
	}
	for index := len(l.rows) - 1; index >= 0; index-- {
		if l.rows[index].set {
			l.selected = index
			return
		}
	}
}

func (l *viewList) ensureVisible(height int) {
	if height <= 0 {
		l.top = 0
		return
	}
	if l.selected < l.top {
		l.top = l.selected
	}
	if l.selected >= l.top+height {
		l.top = l.selected - height + 1
	}
	l.top = min(max(0, l.top), max(0, len(l.rows)-height))
}

func (v *viewPanel) paneByID(id string) *viewPane {
	for _, pane := range v.panes {
		if pane.id == id {
			return pane
		}
	}
	return nil
}

func (a *App) NewView(view ui.View) ui.ViewHandle {
	a.nextViewID++
	id := a.nextViewID
	title := view.Title
	if title == "" {
		title = "View"
		view.Title = title
	}
	placeholder := buffer.NewReadOnly(title, nil)
	editorBuffer := a.newEditorBuffer(placeholder)
	editorBuffer.view = newViewPanel(id, view)
	a.buffers = append(a.buffers, editorBuffer)
	a.active = len(a.buffers) - 1
	a.topLine = 0
	a.leftColumn = 0
	a.CloseSidebar()
	a.closeFileBrowser()
	a.closeWorkspaceSearch()
	a.requestViewSelections(editorBuffer.view)
	return &viewHandle{app: a, id: id}
}

func (h *viewHandle) Show() bool {
	index, _ := h.app.viewByID(h.id)
	if index < 0 {
		return false
	}
	h.app.active = index
	h.app.CloseSidebar()
	h.app.closeFileBrowser()
	h.app.closeWorkspaceSearch()
	return true
}

func (h *viewHandle) Update(view ui.View) bool {
	index, current := h.app.viewByID(h.id)
	if index < 0 {
		return false
	}
	replacement := newViewPanel(h.id, view)
	preserveViewState(current, replacement)
	h.app.buffers[index].view = replacement
	if view.Title != "" && view.Title != h.app.buffers[index].text.Name() {
		h.app.buffers[index].text = buffer.NewReadOnly(view.Title, nil)
	}
	h.app.requestViewSelections(replacement)
	return true
}

func (h *viewHandle) Destroy() {
	index, _ := h.app.viewByID(h.id)
	if index < 0 {
		return
	}
	h.app.closeBufferNow(h.app.buffers[index].text)
}

func (a *App) viewByID(id uint64) (int, *viewPanel) {
	for index, editorBuffer := range a.buffers {
		if editorBuffer.view != nil && editorBuffer.view.id == id {
			return index, editorBuffer.view
		}
	}
	return -1, nil
}

func preserveViewState(previous, replacement *viewPanel) {
	if previous == nil || replacement == nil {
		return
	}
	if previous.focus >= 0 && previous.focus < len(previous.panes) {
		focusedID := previous.panes[previous.focus].id
		for index, pane := range replacement.panes {
			if pane.id == focusedID {
				replacement.focus = index
				break
			}
		}
	}
	for _, pane := range replacement.panes {
		old := previous.paneByID(pane.id)
		if old == nil {
			continue
		}
		if pane.list != nil && old.list != nil {
			if selected, ok := old.list.selectedItem(); ok {
				if !pane.list.selectionSet {
					pane.list.selectItem(selected.Kind, selected.Value)
				}
			}
			pane.list.top = old.list.top
		}
		if pane.widget == nil && old.widget != nil {
			pane.widget = old.widget
			pane.content = cloneWidgetContent(old.content)
		}
		pane.top = old.top
		pane.left = old.left
		pane.selectedKind = old.selectedKind
		pane.selectedValue = old.selectedValue
	}
}

func (a *App) currentView() *viewPanel {
	if current := a.currentEditorBuffer(); current != nil {
		return current.view
	}
	return nil
}

func (a *App) handleViewEvent(event terminal.Event) error {
	view := a.currentView()
	if view == nil || len(view.panes) == 0 {
		return nil
	}
	if event.Key == terminal.KeyTab {
		view.focus = (view.focus + 1) % len(view.panes)
		return nil
	}
	pane := view.panes[view.focus]
	if pane.list != nil {
		return a.handleViewListEvent(view, pane, event)
	}
	return a.handleViewWidgetEvent(pane, event)
}

func (a *App) handleViewListEvent(view *viewPanel, pane *viewPane, event terminal.Event) error {
	list := pane.list
	before := list.selected
	switch event.Key {
	case terminal.KeyUp:
		list.move(-1)
	case terminal.KeyDown:
		list.move(1)
	case terminal.KeyPageUp:
		_, height := a.screen.Size()
		list.move(-max(1, height-6))
	case terminal.KeyPageDown:
		_, height := a.screen.Size()
		list.move(max(1, height-6))
	case terminal.KeyHome:
		list.moveToEnd(false)
	case terminal.KeyEnd:
		list.moveToEnd(true)
	case terminal.KeyEnter:
		if target := view.paneByID(list.previewPane); target != nil {
			for index, candidate := range view.panes {
				if candidate == target {
					view.focus = index
					return nil
				}
			}
		}
		return list.perform(ui.Activate)
	case terminal.KeyRune:
		if event.Control || event.Alt || event.Super {
			return nil
		}
		switch event.Rune {
		case '+':
			return list.perform(ui.Add)
		case '-':
			return list.perform(ui.Remove)
		}
	}
	if list.selected != before {
		a.requestViewSelection(view, pane)
	}
	return nil
}

func (a *App) handleViewWidgetEvent(pane *viewPane, event terminal.Event) error {
	visibleRows := a.viewWidgetHeight()
	switch event.Key {
	case terminal.KeyUp:
		pane.top--
	case terminal.KeyDown:
		pane.top++
	case terminal.KeyPageUp:
		pane.top -= max(1, visibleRows-1)
	case terminal.KeyPageDown:
		pane.top += max(1, visibleRows-1)
	case terminal.KeyHome:
		pane.top = 0
	case terminal.KeyEnd:
		pane.top = len(pane.content.Lines) - visibleRows
	case terminal.KeyLeft:
		pane.left--
	case terminal.KeyRight:
		pane.left++
	}
	pane.top = min(max(0, pane.top), max(0, len(pane.content.Lines)-visibleRows))
	pane.left = min(max(0, pane.left), max(0, pane.widgetWidth()-1))
	return nil
}

func (l *viewList) perform(action ui.Action) error {
	item, ok := l.selectedItem()
	if !ok || l.onAction == nil {
		return nil
	}
	return l.onAction(action, item)
}

func (p *viewPane) widgetWidth() int {
	width := 0
	for _, line := range p.content.Lines {
		column := 0
		for _, span := range line.Spans {
			if span.Positioned {
				column = max(0, span.Column)
			}
			for _, value := range []rune(span.Text) {
				value = safeWidgetRune(value)
				if value == '\t' {
					column += 4 - column%4
				} else {
					column += terminal.RuneWidth(value)
				}
			}
			width = max(width, column)
		}
	}
	return width
}

func (a *App) viewWidgetHeight() int {
	_, height := a.screen.Size()
	return max(1, height-6)
}

func (a *App) requestViewSelections(view *viewPanel) {
	for _, pane := range view.panes {
		if pane.list != nil {
			a.requestViewSelection(view, pane)
		}
	}
}

func (a *App) requestViewSelection(view *viewPanel, pane *viewPane) {
	list := pane.list
	item, ok := list.selectedItem()
	target := view.paneByID(list.previewPane)
	if target == nil || list.onSelect == nil {
		return
	}
	if !ok {
		target.generation++
		target.selectedKind = ""
		target.selectedValue = ""
		target.widget = nil
		target.content = ui.WidgetContent{}
		target.top = 0
		target.left = 0
		target.loading = false
		target.err = nil
		return
	}
	target.generation++
	selectionChanged := target.selectedKind != item.Kind || target.selectedValue != item.Value
	target.selectedKind = item.Kind
	target.selectedValue = item.Value
	target.loading = true
	target.err = nil
	if selectionChanged {
		target.widget = nil
		target.content = ui.WidgetContent{}
		target.top = 0
		target.left = 0
	}
	generation := target.generation
	load := list.onSelect
	if a.servers == nil {
		widget, err := load(item)
		content := renderWidget(widget)
		a.applyViewPreviewEvent(&viewPreviewEvent{
			viewID: view.id, paneID: target.id, pane: target,
			generation: generation, widget: widget, content: content, err: err,
		})
		return
	}
	go func() {
		widget, err := load(item)
		content := renderWidget(widget)
		a.servers <- serverEvent{viewPreview: &viewPreviewEvent{
			viewID: view.id, paneID: target.id, pane: target,
			generation: generation, widget: widget, content: content, err: err,
		}}
	}()
}

func (a *App) applyViewPreviewEvent(event *viewPreviewEvent) {
	_, view := a.viewByID(event.viewID)
	if view == nil {
		return
	}
	pane := view.paneByID(event.paneID)
	if pane == nil || pane != event.pane || pane.generation != event.generation {
		return
	}
	pane.loading = false
	pane.err = event.err
	if event.err != nil {
		return
	}
	pane.widget = event.widget
	pane.content = cloneWidgetContent(event.content)
}

func (a *App) pollViewRefreshes() {
	for _, editorBuffer := range a.buffers {
		view := editorBuffer.view
		if view == nil || view.onRefresh == nil || view.refreshInterval <= 0 || view.refreshing ||
			time.Now().Before(view.nextRefresh) {
			continue
		}
		view.refreshing = true
		view.nextRefresh = time.Now().Add(view.refreshInterval)
		refresh := view.onRefresh
		if a.servers == nil {
			updated, err := refresh()
			a.applyViewRefreshEvent(&viewRefreshEvent{
				viewID: view.id, view: view, updated: updated, err: err,
			})
			continue
		}
		go func(source *viewPanel) {
			updated, err := refresh()
			a.servers <- serverEvent{viewRefresh: &viewRefreshEvent{
				viewID: source.id, view: source, updated: updated, err: err,
			}}
		}(view)
	}
}

func (a *App) applyViewRefreshEvent(event *viewRefreshEvent) {
	index, current := a.viewByID(event.viewID)
	if current == nil {
		return
	}
	if current != event.view {
		return
	}
	current.refreshing = false
	if event.err != nil {
		a.message = event.err.Error()
		return
	}
	replacement := newViewPanel(event.viewID, event.updated)
	preserveViewState(current, replacement)
	a.buffers[index].view = replacement
	a.requestViewSelections(replacement)
}
