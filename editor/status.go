package editor

import (
	"fmt"
	"strings"
	"time"

	"github.com/bluescreen10/myde/plugin"
)

const defaultStatusRefreshInterval = 2 * time.Second

type statusItem struct {
	name            string
	text            string
	refreshInterval time.Duration
	onRefresh       func() (string, error)
	nextRefresh     time.Time
	refreshing      bool
}

// RegisterStatus adds a plugin-owned section to the status bar.
func (a *App) RegisterStatus(name string, item plugin.StatusItem) error {
	name = strings.TrimSpace(name)
	if name == "" || strings.ContainsAny(name, " \t\r\n") {
		return fmt.Errorf("invalid status item name %q", name)
	}
	if a.statusNames == nil {
		a.statusNames = make(map[string]bool)
	}
	if a.statusNames[name] {
		return fmt.Errorf("status item %s is already registered", name)
	}
	if item.OnRefresh == nil && strings.TrimSpace(item.Text) == "" {
		return fmt.Errorf("status item %s has no text or refresh callback", name)
	}
	if item.RefreshInterval < 0 {
		return fmt.Errorf("status item %s has a negative refresh interval", name)
	}
	interval := item.RefreshInterval
	if item.OnRefresh != nil && interval == 0 {
		interval = defaultStatusRefreshInterval
	}
	a.statusItems = append(a.statusItems, &statusItem{
		name:            name,
		text:            strings.TrimSpace(item.Text),
		refreshInterval: interval,
		onRefresh:       item.OnRefresh,
	})
	a.statusNames[name] = true
	return nil
}

func (a *App) pollStatusItems() {
	now := time.Now()
	for _, item := range a.statusItems {
		if item.onRefresh == nil || item.refreshing || now.Before(item.nextRefresh) {
			continue
		}
		item.refreshing = true
		item.nextRefresh = now.Add(item.refreshInterval)
		refresh := item.onRefresh
		go func() {
			text, err := refresh()
			a.servers <- serverEvent{statusRefresh: &statusRefreshEvent{
				item: item,
				text: text,
				err:  err,
			}}
		}()
	}
}
