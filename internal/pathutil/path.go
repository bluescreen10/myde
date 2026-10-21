// Package pathutil resolves editor paths using their spelling on disk.
package pathutil

import (
	"os"
	"path/filepath"
	"runtime"
)

// Abs returns an absolute path. On macOS it also corrects the spelling of
// existing components, since language servers may reject differently cased
// paths even when the filesystem accepts them. Symlinks and missing components
// retain their names.
func Abs(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		absolute = diskSpelling(absolute)
	}
	return absolute, nil
}

func diskSpelling(path string) string {
	parent := filepath.Dir(path)
	if parent == path {
		return path
	}
	parent = diskSpelling(parent)
	name := filepath.Base(path)
	path = filepath.Join(parent, name)
	entries, err := os.ReadDir(parent)
	if err != nil {
		return path
	}
	for _, entry := range entries {
		if entry.Name() == name {
			return path
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return path
	}
	for _, entry := range entries {
		candidate, err := entry.Info()
		if err == nil && os.SameFile(info, candidate) {
			return filepath.Join(parent, entry.Name())
		}
	}
	return path
}
