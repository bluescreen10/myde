package editor

import (
	"fmt"
	"strings"

	"github.com/bluescreen10/myde/plugin"
)

// Setting returns a workspace override or user-wide plugin setting.
func (a *App) Setting(name string) string {
	name = strings.TrimSpace(name)
	if a.extensions != nil {
		if value, ok := a.extensions.settings[name]; ok {
			return value
		}
	}
	if a.settings != nil {
		return a.settings.Value(name)
	}
	return ""
}

// Subscribe registers a synchronous handler for a namespaced lifecycle event.
func (a *App) Subscribe(name string, handler plugin.EventHandler) error {
	name = strings.TrimSpace(name)
	if !isEventName(name) {
		return fmt.Errorf("invalid event name %q; expected namespace.event-name", name)
	}
	if handler == nil {
		return fmt.Errorf("event %s has no handler", name)
	}
	if a.subscribers == nil {
		a.subscribers = make(map[string][]plugin.EventHandler)
	}
	a.subscribers[name] = append(a.subscribers[name], handler)
	return nil
}

func isEventName(name string) bool {
	namespace, event, found := strings.Cut(name, ".")
	return found && !strings.Contains(event, ".") && isEventNamePart(namespace) && isEventNamePart(event)
}

func isEventNamePart(part string) bool {
	if part == "" || part[0] == '-' || part[len(part)-1] == '-' {
		return false
	}
	for _, value := range part {
		if value >= 'a' && value <= 'z' || value >= '0' && value <= '9' || value == '-' {
			continue
		}
		return false
	}
	return true
}

func (a *App) publishEvent(name, value string, editorBuffer *editorBuffer) error {
	handlers := append([]plugin.EventHandler(nil), a.subscribers[name]...)
	if len(handlers) == 0 {
		return nil
	}
	previous := a.eventBuffer
	a.eventBuffer = editorBuffer
	defer func() { a.eventBuffer = previous }()
	event := plugin.Event{Name: name, Value: value}
	for _, handler := range handlers {
		if err := handler(event); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}
	return nil
}

func fileEventValue(editorBuffer *editorBuffer) string {
	if editorBuffer == nil || editorBuffer.text == nil {
		return ""
	}
	if path := editorBuffer.text.Path(); path != "" {
		return path
	}
	return editorBuffer.text.Name()
}

func (a *App) pluginEditorBuffer() *editorBuffer {
	if a.eventBuffer != nil {
		return a.eventBuffer
	}
	return a.currentEditorBuffer()
}
