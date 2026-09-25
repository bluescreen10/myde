//go:build linux

package editor

import (
	"fmt"
	"os/exec"
	"strings"
)

func writeSystemClipboard(text string) error {
	commands := [][]string{
		{"wl-copy"},
		{"xclip", "-selection", "clipboard"},
		{"xsel", "--clipboard", "--input"},
	}
	for _, specification := range commands {
		path, err := exec.LookPath(specification[0])
		if err != nil {
			continue
		}
		command := exec.Command(path, specification[1:]...)
		command.Stdin = strings.NewReader(text)
		if err := command.Run(); err != nil {
			return fmt.Errorf("write clipboard with %s: %w", specification[0], err)
		}
		return nil
	}
	return fmt.Errorf("clipboard unavailable: install wl-clipboard, xclip, or xsel")
}

func readSystemClipboard() (string, error) {
	commands := [][]string{
		{"wl-paste", "--no-newline"},
		{"xclip", "-selection", "clipboard", "-out"},
		{"xsel", "--clipboard", "--output"},
	}
	for _, specification := range commands {
		path, err := exec.LookPath(specification[0])
		if err != nil {
			continue
		}
		output, err := exec.Command(path, specification[1:]...).Output()
		if err != nil {
			return "", fmt.Errorf("read clipboard with %s: %w", specification[0], err)
		}
		return string(output), nil
	}
	return "", fmt.Errorf("clipboard unavailable: install wl-clipboard, xclip, or xsel")
}
