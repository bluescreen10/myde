//go:build darwin

package editor

import (
	"fmt"
	"os/exec"
	"strings"
)

func writeSystemClipboard(text string) error {
	command := exec.Command("/usr/bin/pbcopy")
	command.Stdin = strings.NewReader(text)
	if err := command.Run(); err != nil {
		return fmt.Errorf("write macOS clipboard: %w", err)
	}
	return nil
}

func readSystemClipboard() (string, error) {
	output, err := exec.Command("/usr/bin/pbpaste").Output()
	if err != nil {
		return "", fmt.Errorf("read macOS clipboard: %w", err)
	}
	return string(output), nil
}
