package editor

import (
	"bytes"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/bluescreen10/myde/buffer"
)

type terminalParseMode uint8

const (
	terminalText terminalParseMode = iota
	terminalEscape
	terminalCSI
	terminalOSC
	terminalOSCEscape
)

type terminalState struct {
	width     int
	height    int
	lines     [][]rune
	screenTop int
	row       int
	column    int
	savedRow  int
	savedCol  int
	mode      terminalParseMode
	sequence  []byte
	pending   []byte
}

func newTerminalState(width, height int) *terminalState {
	state := &terminalState{width: max(1, width), height: max(1, height)}
	state.ensureLine(0)
	return state
}

func (state *terminalState) Resize(width, height int) {
	state.width = max(1, width)
	state.height = max(1, height)
	if state.row < state.screenTop {
		state.screenTop = state.row
	}
	if state.row >= state.screenTop+state.height {
		state.screenTop = state.row - state.height + 1
	}
}

func (state *terminalState) Write(content []byte) {
	state.pending = append(state.pending, content...)
	for len(state.pending) > 0 {
		switch state.mode {
		case terminalText:
			if !state.consumeText() {
				return
			}
		case terminalEscape:
			state.consumeEscape()
		case terminalCSI:
			state.consumeCSI()
		case terminalOSC:
			state.consumeOSC()
		case terminalOSCEscape:
			state.consumeOSCEscape()
		}
	}
}

func (state *terminalState) consumeText() bool {
	value := state.pending[0]
	if value == 0x1b {
		state.pending = state.pending[1:]
		state.mode = terminalEscape
		return true
	}
	if value < utf8.RuneSelf {
		state.pending = state.pending[1:]
		switch value {
		case '\r':
			state.column = 0
		case '\n', 0x0b, 0x0c:
			state.lineFeed(false)
		case '\b':
			state.column = max(0, state.column-1)
		case '\t':
			state.column = min(state.width-1, (state.column/8+1)*8)
		case 0x07, 0x00:
		default:
			if value >= ' ' && value != 0x7f {
				state.putRune(rune(value))
			}
		}
		return true
	}
	if !utf8.FullRune(state.pending) {
		return false
	}
	decoded, size := utf8.DecodeRune(state.pending)
	state.pending = state.pending[size:]
	state.putRune(decoded)
	return true
}

func (state *terminalState) consumeEscape() {
	value := state.pending[0]
	state.pending = state.pending[1:]
	switch value {
	case '[':
		state.sequence = state.sequence[:0]
		state.mode = terminalCSI
	case ']':
		state.mode = terminalOSC
	case '7':
		state.savedRow, state.savedCol = state.row, state.column
		state.mode = terminalText
	case '8':
		state.row, state.column = state.savedRow, state.savedCol
		state.ensureLine(state.row)
		state.mode = terminalText
	case 'D':
		state.lineFeed(false)
		state.mode = terminalText
	case 'E':
		state.lineFeed(true)
		state.mode = terminalText
	case 'M':
		state.row = max(state.screenTop, state.row-1)
		state.mode = terminalText
	case 'c':
		width, height := state.width, state.height
		*state = *newTerminalState(width, height)
	default:
		state.mode = terminalText
	}
}

func (state *terminalState) consumeCSI() {
	value := state.pending[0]
	state.pending = state.pending[1:]
	if value >= 0x40 && value <= 0x7e {
		state.applyCSI(value, parseTerminalParameters(state.sequence))
		state.sequence = state.sequence[:0]
		state.mode = terminalText
		return
	}
	if len(state.sequence) < 128 {
		state.sequence = append(state.sequence, value)
	}
}

func (state *terminalState) consumeOSC() {
	value := state.pending[0]
	state.pending = state.pending[1:]
	switch value {
	case 0x07:
		state.mode = terminalText
	case 0x1b:
		state.mode = terminalOSCEscape
	}
}

func (state *terminalState) consumeOSCEscape() {
	value := state.pending[0]
	state.pending = state.pending[1:]
	if value == '\\' {
		state.mode = terminalText
	} else {
		state.mode = terminalOSC
	}
}

func parseTerminalParameters(sequence []byte) []int {
	text := strings.TrimLeft(string(sequence), "?<>=!")
	if text == "" {
		return nil
	}
	fields := strings.Split(text, ";")
	parameters := make([]int, len(fields))
	for index, field := range fields {
		field, _, _ = strings.Cut(field, ":")
		parameters[index], _ = strconv.Atoi(field)
	}
	return parameters
}

func terminalParameter(parameters []int, index, fallback int) int {
	if index >= len(parameters) || parameters[index] == 0 {
		return fallback
	}
	return parameters[index]
}

func (state *terminalState) applyCSI(command byte, parameters []int) {
	count := terminalParameter(parameters, 0, 1)
	switch command {
	case 'A':
		state.row = max(state.screenTop, state.row-count)
	case 'B', 'e':
		state.row = min(state.screenTop+state.height-1, state.row+count)
		state.ensureLine(state.row)
	case 'C', 'a':
		state.column = min(state.width-1, state.column+count)
	case 'D':
		state.column = max(0, state.column-count)
	case 'E':
		state.row = min(state.screenTop+state.height-1, state.row+count)
		state.column = 0
		state.ensureLine(state.row)
	case 'F':
		state.row = max(state.screenTop, state.row-count)
		state.column = 0
	case 'G', '`':
		state.column = min(state.width-1, max(0, count-1))
	case 'H', 'f':
		row := terminalParameter(parameters, 0, 1)
		column := terminalParameter(parameters, 1, 1)
		state.row = state.screenTop + min(state.height-1, max(0, row-1))
		state.column = min(state.width-1, max(0, column-1))
		state.ensureLine(state.row)
	case 'd':
		state.row = state.screenTop + min(state.height-1, max(0, count-1))
		state.ensureLine(state.row)
	case 'J':
		state.eraseDisplay(terminalParameter(parameters, 0, 0))
	case 'K':
		state.eraseLine(terminalParameter(parameters, 0, 0))
	case 'P':
		state.deleteCharacters(count)
	case '@':
		state.insertCharacters(count)
	case 'X':
		state.eraseCharacters(count)
	case 'S':
		for range count {
			state.scrollUp()
		}
	case 's':
		state.savedRow, state.savedCol = state.row, state.column
	case 'u':
		state.row, state.column = state.savedRow, state.savedCol
		state.ensureLine(state.row)
	}
}

func (state *terminalState) putRune(value rune) {
	if state.column >= state.width {
		state.lineFeed(true)
	}
	state.ensureLine(state.row)
	line := state.lines[state.row]
	for len(line) <= state.column {
		line = append(line, ' ')
	}
	line[state.column] = value
	state.lines[state.row] = line
	state.column++
}

func (state *terminalState) lineFeed(resetColumn bool) {
	if state.row >= state.screenTop+state.height-1 {
		state.scrollUp()
	} else {
		state.row++
		state.ensureLine(state.row)
	}
	if resetColumn {
		state.column = 0
	}
}

func (state *terminalState) scrollUp() {
	state.row++
	state.screenTop++
	state.ensureLine(state.row)
}

func (state *terminalState) ensureLine(row int) {
	for len(state.lines) <= row {
		state.lines = append(state.lines, nil)
	}
}

func (state *terminalState) eraseDisplay(mode int) {
	last := state.screenTop + state.height - 1
	state.ensureLine(last)
	switch mode {
	case 1:
		for row := state.screenTop; row < state.row; row++ {
			state.lines[row] = nil
		}
		state.eraseLine(1)
	case 2:
		for row := state.screenTop; row <= last; row++ {
			state.lines[row] = nil
		}
	case 3:
		if state.screenTop > 0 {
			state.lines = append([][]rune(nil), state.lines[state.screenTop:]...)
			state.row -= state.screenTop
			state.screenTop = 0
		}
	default:
		state.eraseLine(0)
		for row := state.row + 1; row <= last; row++ {
			state.lines[row] = nil
		}
	}
}

func (state *terminalState) eraseLine(mode int) {
	state.ensureLine(state.row)
	line := state.lines[state.row]
	switch mode {
	case 1:
		limit := min(state.column+1, len(line))
		for index := 0; index < limit; index++ {
			line[index] = ' '
		}
	case 2:
		line = nil
	default:
		if state.column < len(line) {
			line = line[:state.column]
		}
	}
	state.lines[state.row] = line
}

func (state *terminalState) deleteCharacters(count int) {
	state.ensureLine(state.row)
	line := state.lines[state.row]
	if state.column >= len(line) {
		return
	}
	end := min(len(line), state.column+count)
	line = append(line[:state.column], line[end:]...)
	state.lines[state.row] = line
}

func (state *terminalState) insertCharacters(count int) {
	state.ensureLine(state.row)
	line := state.lines[state.row]
	for len(line) < state.column {
		line = append(line, ' ')
	}
	spaces := make([]rune, count)
	for index := range spaces {
		spaces[index] = ' '
	}
	line = append(line[:state.column], append(spaces, line[state.column:]...)...)
	if len(line) > state.width {
		line = line[:state.width]
	}
	state.lines[state.row] = line
}

func (state *terminalState) eraseCharacters(count int) {
	state.ensureLine(state.row)
	line := state.lines[state.row]
	for len(line) < min(state.width, state.column+count) {
		line = append(line, ' ')
	}
	for index := state.column; index < min(len(line), state.column+count); index++ {
		line[index] = ' '
	}
	state.lines[state.row] = line
}

func (state *terminalState) Snapshot() ([]byte, buffer.Point) {
	state.ensureLine(state.row)
	lastRow := state.row
	for row := len(state.lines) - 1; row > lastRow; row-- {
		if terminalLineLength(state.lines[row]) > 0 {
			lastRow = row
			break
		}
	}
	var output bytes.Buffer
	for row := 0; row <= lastRow; row++ {
		line := state.lines[row]
		length := terminalLineLength(line)
		if row == state.row {
			length = max(length, state.column)
		}
		for len(line) < length {
			line = append(line, ' ')
		}
		output.WriteString(string(line[:length]))
		if row < lastRow {
			output.WriteByte('\n')
		}
	}
	return output.Bytes(), buffer.Point{Line: state.row, Column: state.column}
}

func terminalLineLength(line []rune) int {
	length := len(line)
	for length > 0 && line[length-1] == ' ' {
		length--
	}
	return length
}
