//go:build !darwin && !linux && !windows

package editor

import "fmt"

func writeSystemClipboard(text string) error {
	return fmt.Errorf("system clipboard is unsupported on this platform")
}

func readSystemClipboard() (string, error) {
	return "", fmt.Errorf("system clipboard is unsupported on this platform")
}
