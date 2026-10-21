package pathutil_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/bluescreen10/myde/internal/pathutil"
)

func TestAbsPreservesSymlinkAndMissingNames(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "target"), 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink("target", link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	path := filepath.Join(link, "NewDir", "NewFile.go")
	got, err := pathutil.Abs(path)
	if err != nil || got != path {
		t.Fatalf("Abs(%q) = %q, %v", path, got, err)
	}
}

func TestAbsCorrectsExistingComponents(t *testing.T) {
	if runtime.GOOS != "darwin" {
		t.Skip("macOS path spelling")
	}
	root := t.TempDir()
	directory := filepath.Join(root, "gamekit", "ui", "win32")
	if err := os.MkdirAll(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(directory, "Main.go"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "GameKit")); err != nil {
		t.Skip("filesystem is case sensitive")
	}
	for _, test := range []struct{ input, want string }{
		{"GameKit", "gamekit"},
		{"GameKit/UI/Win32", "gamekit/ui/win32"},
		{"GameKit/UI/Win32/main.GO", "gamekit/ui/win32/Main.go"},
		{"GameKit/UI/Win32/New.go", "gamekit/ui/win32/New.go"},
		{"GameKit/UI/NewDir/New.go", "gamekit/ui/NewDir/New.go"},
	} {
		t.Run(test.input, func(t *testing.T) {
			got, err := pathutil.Abs(filepath.Join(root, test.input))
			if want := filepath.Join(root, test.want); err != nil || got != want {
				t.Fatalf("Abs = %q, %v; want %q", got, err, want)
			}
		})
	}
}
