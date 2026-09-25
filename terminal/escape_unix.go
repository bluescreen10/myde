//go:build linux || darwin

package terminal

import (
	"bufio"
	"errors"
	"io"
	"os"
	"syscall"
	"time"
)

func readByteAfterEscape(reader *bufio.Reader, file *os.File) (byte, error) {
	if reader.Buffered() > 0 || file == nil {
		return reader.ReadByte()
	}
	if err := syscall.SetNonblock(int(file.Fd()), true); err != nil {
		return reader.ReadByte()
	}
	defer syscall.SetNonblock(int(file.Fd()), false)

	deadline := time.Now().Add(35 * time.Millisecond)
	for time.Now().Before(deadline) {
		value, err := reader.ReadByte()
		if err == nil {
			return value, nil
		}
		if !errors.Is(err, syscall.EAGAIN) && !errors.Is(err, io.EOF) {
			return 0, err
		}
		time.Sleep(time.Millisecond)
	}
	return 0, io.EOF
}
