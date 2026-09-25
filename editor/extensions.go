package editor

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type extensions struct {
	path      string
	modified  time.Time
	functions map[string][]string
	hooks     map[string][]string
	bindings  map[string]string
	colors    map[string]string
	settings  map[string]string
}

func newExtensions(root string) *extensions {
	path := filepath.Join(root, ".myde")
	if configured := os.Getenv("MYDE_CONFIG"); configured != "" {
		path = configured
	}
	return &extensions{path: path}
}

func (e *extensions) reloadIfChanged() (bool, error) {
	info, err := os.Stat(e.path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat extension file: %w", err)
	}
	if !info.ModTime().After(e.modified) {
		return false, nil
	}
	if err := e.load(); err != nil {
		return false, err
	}
	e.modified = info.ModTime()
	return true, nil
}

func (e *extensions) load() error {
	file, err := os.Open(e.path)
	if err != nil {
		return fmt.Errorf("open extension file: %w", err)
	}
	defer file.Close()

	functions := make(map[string][]string)
	hooks := make(map[string][]string)
	bindings := make(map[string]string)
	colors := make(map[string]string)
	settings := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		kind, rest, found := strings.Cut(line, " ")
		if !found {
			return fmt.Errorf("%s:%d: incomplete declaration", e.path, lineNumber)
		}
		left, right, found := strings.Cut(strings.TrimSpace(rest), "=")
		if !found {
			return fmt.Errorf("%s:%d: expected =", e.path, lineNumber)
		}
		name := strings.TrimSpace(left)
		value := strings.TrimSpace(right)
		switch kind {
		case "def":
			for _, command := range strings.Split(value, ";") {
				functions[name] = append(functions[name], strings.TrimSpace(command))
			}
		case "hook":
			hooks[name] = append(hooks[name], value)
		case "bind":
			bindings[normalizeKey(name)] = value
		case "color":
			colors[name] = value
		case "set":
			settings[name] = value
		default:
			return fmt.Errorf("%s:%d: unknown declaration %q", e.path, lineNumber, kind)
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("read extension file: %w", err)
	}
	e.functions = functions
	e.hooks = hooks
	e.bindings = bindings
	e.colors = colors
	e.settings = settings
	return nil
}

func normalizeKey(key string) string {
	return strings.ToLower(strings.ReplaceAll(strings.TrimSpace(key), "control-", "ctrl-"))
}
