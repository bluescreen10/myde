// Package settings manages myde's user-wide configuration directory.
package settings

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	// DefaultTheme is the theme written to a new settings file.
	DefaultTheme = "vs-dark-2026"
	application  = "myde"
	configDirEnv = "MYDE_CONFIG_DIR"
)

// Store contains the paths and values in myde's user configuration directory.
type Store struct {
	Directory        string
	SettingsFile     string
	ThemesDirectory  string
	PluginsDirectory string

	mu     sync.RWMutex
	values map[string]string
}

// Open creates and loads myde's settings directory. MYDE_CONFIG_DIR takes
// precedence over the platform's standard user configuration directory.
func Open() (*Store, error) {
	if configured := strings.TrimSpace(os.Getenv(configDirEnv)); configured != "" {
		return OpenAt(configured)
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("find user configuration directory: %w", err)
	}
	return OpenAt(filepath.Join(base, application))
}

// OpenAt creates and loads a settings directory at root. It is useful to
// callers that provide an explicit portable configuration location.
func OpenAt(root string) (*Store, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("settings directory is empty")
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve settings directory: %w", err)
	}
	store := &Store{
		Directory:        absolute,
		SettingsFile:     filepath.Join(absolute, "settings.conf"),
		ThemesDirectory:  filepath.Join(absolute, "themes"),
		PluginsDirectory: filepath.Join(absolute, "plugins"),
	}
	for _, directory := range []string{store.Directory, store.ThemesDirectory, store.PluginsDirectory} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, fmt.Errorf("create settings directory %s: %w", directory, err)
		}
	}
	if err := createSettingsFile(store.SettingsFile); err != nil {
		return nil, err
	}
	values, err := readSettings(store.SettingsFile)
	if err != nil {
		return nil, err
	}
	store.values = values
	return store, nil
}

// Value returns a setting, or an empty string when the key is not present.
func (s *Store) Value(key string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values[key]
}

// Set changes a setting and writes the complete settings file.
func (s *Store) Set(key, value string) error {
	key = strings.TrimSpace(key)
	value = strings.TrimSpace(value)
	if err := validateKey(key); err != nil {
		return err
	}
	if strings.ContainsAny(value, "\r\n") {
		return fmt.Errorf("setting %q contains a newline", key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	updated := make(map[string]string, len(s.values)+1)
	for current, currentValue := range s.values {
		updated[current] = currentValue
	}
	updated[key] = value
	if err := writeSettings(s.SettingsFile, updated); err != nil {
		return err
	}
	s.values = updated
	return nil
}

func createSettingsFile(path string) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if os.IsExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("create settings file %s: %w", path, err)
	}
	if _, err := fmt.Fprintf(file, "theme = %s\n", DefaultTheme); err != nil {
		_ = file.Close()
		_ = os.Remove(path)
		return fmt.Errorf("initialize settings file %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return fmt.Errorf("close settings file %s: %w", path, err)
	}
	return nil
}

func readSettings(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open settings file %s: %w", path, err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for lineNumber := 1; scanner.Scan(); lineNumber++ {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		left, right, found := strings.Cut(line, "=")
		if !found {
			return nil, fmt.Errorf("%s:%d: expected key = value", path, lineNumber)
		}
		key := strings.TrimSpace(left)
		if err := validateKey(key); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, lineNumber, err)
		}
		values[key] = strings.TrimSpace(right)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read settings file %s: %w", path, err)
	}
	return values, nil
}

func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("setting key is empty")
	}
	for _, current := range key {
		if current >= 'a' && current <= 'z' || current >= 'A' && current <= 'Z' ||
			current >= '0' && current <= '9' || current == '-' || current == '_' || current == '.' {
			continue
		}
		return fmt.Errorf("invalid setting key %q", key)
	}
	return nil
}

func writeSettings(path string, values map[string]string) error {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	temporary, err := os.CreateTemp(filepath.Dir(path), ".settings.conf-*")
	if err != nil {
		return fmt.Errorf("create temporary settings file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	for _, key := range keys {
		if _, err := fmt.Fprintf(temporary, "%s = %s\n", key, values[key]); err != nil {
			_ = temporary.Close()
			return fmt.Errorf("write settings file: %w", err)
		}
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync settings file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close settings file: %w", err)
	}
	if err := replaceFile(temporaryPath, path); err != nil {
		return fmt.Errorf("replace settings file %s: %w", path, err)
	}
	return nil
}
