//go:build !linux && !darwin

package terminal

import (
	"bufio"
	"os"
)

func readByteAfterEscape(reader *bufio.Reader, file *os.File) (byte, error) {
	return reader.ReadByte()
}
