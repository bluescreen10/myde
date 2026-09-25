//go:build darwin

package terminal

import (
	"fmt"
	"syscall"
	"unsafe"
)

func enableRaw(fd uintptr) (any, error) {
	state, err := readTermios(fd, syscall.TIOCGETA)
	if err != nil {
		return nil, err
	}
	raw := *state
	raw.Iflag &^= syscall.BRKINT | syscall.ICRNL | syscall.INPCK | syscall.ISTRIP | syscall.IXON
	raw.Oflag &^= syscall.OPOST
	raw.Cflag |= syscall.CS8
	raw.Lflag &^= syscall.ECHO | syscall.ICANON | syscall.IEXTEN | syscall.ISIG
	raw.Cc[syscall.VMIN] = 1
	raw.Cc[syscall.VTIME] = 0
	if err := writeTermios(fd, syscall.TIOCSETA, &raw); err != nil {
		return nil, err
	}
	return state, nil
}

func restore(fd uintptr, previous any) error {
	state, ok := previous.(*syscall.Termios)
	if !ok {
		return fmt.Errorf("restore terminal: invalid state")
	}
	return writeTermios(fd, syscall.TIOCSETA, state)
}

func terminalSize(fd uintptr) (int, int, error) {
	var size struct {
		rows uint16
		cols uint16
		x    uint16
		y    uint16
	}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return 0, 0, fmt.Errorf("read terminal size: %w", errno)
	}
	return int(size.cols), int(size.rows), nil
}

func readTermios(fd uintptr, request uintptr) (*syscall.Termios, error) {
	var state syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(unsafe.Pointer(&state)))
	if errno != 0 {
		return nil, fmt.Errorf("read terminal state: %w", errno)
	}
	return &state, nil
}

func writeTermios(fd uintptr, request uintptr, state *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, request, uintptr(unsafe.Pointer(state)))
	if errno != 0 {
		return fmt.Errorf("write terminal state: %w", errno)
	}
	return nil
}
