package settings_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bluescreen10/myde/settings"
)

func TestOpenAtCreatesLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "myde")
	store, err := settings.OpenAt(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, directory := range []string{store.Directory, store.ThemesDirectory, store.PluginsDirectory} {
		info, err := os.Stat(directory)
		if err != nil {
			t.Fatalf("stat %s: %v", directory, err)
		}
		if !info.IsDir() {
			t.Fatalf("%s is not a directory", directory)
		}
	}
	if got := store.Value("theme"); got != settings.DefaultTheme {
		t.Fatalf("theme = %q, want %q", got, settings.DefaultTheme)
	}
	contents, err := os.ReadFile(store.SettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	if got, want := string(contents), "theme = "+settings.DefaultTheme+
		"\ngo-format-on-save = true\ngo-imports-on-save = true\n"; got != want {
		t.Fatalf("settings.conf = %q, want %q", got, want)
	}
}

func TestOpenUsesConfigDirectoryEnvironment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "portable")
	t.Setenv("MYDE_CONFIG_DIR", root)
	store, err := settings.Open()
	if err != nil {
		t.Fatal(err)
	}
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	if store.Directory != absolute {
		t.Fatalf("directory = %q, want %q", store.Directory, absolute)
	}
}

func TestOpenAtPreservesAndLoadsSettings(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.conf")
	contents := "# personal settings\nplugin.auto-update = false\ntheme = midnight\n"
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := settings.OpenAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := store.Value("theme"); got != "midnight" {
		t.Fatalf("theme = %q", got)
	}
	if got := store.Value("plugin.auto-update"); got != "false" {
		t.Fatalf("plugin.auto-update = %q", got)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != contents {
		t.Fatal("opening the store rewrote settings.conf")
	}
}

func TestSetPersistsSettings(t *testing.T) {
	root := t.TempDir()
	store, err := settings.OpenAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Set("theme", "midnight"); err != nil {
		t.Fatal(err)
	}
	if err := store.Set("plugin.auto-update", "false"); err != nil {
		t.Fatal(err)
	}

	reopened, err := settings.OpenAt(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Value("theme"); got != "midnight" {
		t.Fatalf("theme = %q", got)
	}
	contents, err := os.ReadFile(store.SettingsFile)
	if err != nil {
		t.Fatal(err)
	}
	want := "go-format-on-save = true\ngo-imports-on-save = true\n" +
		"plugin.auto-update = false\ntheme = midnight\n"
	if string(contents) != want {
		t.Fatalf("settings.conf = %q, want %q", contents, want)
	}
}

func TestOpenAtRejectsMalformedSettings(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.conf")
	if err := os.WriteFile(path, []byte("theme midnight\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := settings.OpenAt(root)
	if err == nil || !strings.Contains(err.Error(), "expected key = value") {
		t.Fatalf("error = %v", err)
	}
}
