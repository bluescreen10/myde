//go:build !linux && !darwin

package terminal

import "fmt"

func enableRaw(fd uintptr) (any, error) {
	return nil, fmt.Errorf("raw terminal is unsupported on this platform")
}

func restore(fd uintptr, previous any) error {
	return nil
}

func terminalSize(fd uintptr) (int, int, error) {
	return 0, 0, fmt.Errorf("terminal size is unsupported on this platform")
}
