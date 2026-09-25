// Package terminal provides a small ANSI terminal backend.
package terminal

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

// Color is a 24-bit terminal color.
type Color struct {
	R uint8
	G uint8
	B uint8
}

// Style controls a cell's appearance.
type Style struct {
	Foreground        Color
	Background        Color
	UnderlineColor    Color
	Bold              bool
	Underline         bool
	CurlyUnderline    bool
	HasUnderlineColor bool
}

// Cell is one terminal character and its style.
type Cell struct {
	Rune         rune
	Style        Style
	continuation bool
}

// Screen holds a frame before it is flushed to the terminal.
type Screen struct {
	width  int
	height int
	cells  []Cell

	output io.Writer
	last   []Cell
}

// NewScreen creates a screen for output.
func NewScreen(output io.Writer, width, height int) *Screen {
	return &Screen{
		width:  width,
		height: height,
		cells:  make([]Cell, width*height),
		output: output,
	}
}

// Size returns the current screen dimensions.
func (s *Screen) Size() (int, int) {
	return s.width, s.height
}

// Resize changes the frame dimensions.
func (s *Screen) Resize(width, height int) {
	if width == s.width && height == s.height {
		return
	}
	s.width = width
	s.height = height
	s.cells = make([]Cell, width*height)
	s.last = nil
}

// Clear fills the frame with spaces in style.
func (s *Screen) Clear(style Style) {
	for i := range s.cells {
		s.cells[i] = Cell{Rune: ' ', Style: style}
	}
}

// Set places a rune if its coordinates are on-screen.
func (s *Screen) Set(x, y int, value rune, style Style) {
	if x < 0 || x >= s.width || y < 0 || y >= s.height {
		return
	}
	index := y*s.width + x
	if s.cells[index].continuation && x > 0 {
		s.cells[index-1] = Cell{Rune: ' ', Style: style}
	}
	if RuneWidth(s.cells[index].Rune) == 2 && x+1 < s.width {
		s.cells[index+1] = Cell{Rune: ' ', Style: style}
	}
	if value < ' ' || value == 0x7f {
		value = ' '
	}
	width := RuneWidth(value)
	if width == 0 {
		return
	}
	if width == 2 && x+1 >= s.width {
		value = ' '
		width = 1
	}
	s.cells[index] = Cell{Rune: value, Style: style}
	if width == 2 && x+1 < s.width {
		s.cells[index+1] = Cell{Style: style, continuation: true}
	}
}

// Text draws runes until text or the row ends.
func (s *Screen) Text(x, y int, text string, style Style) {
	for _, value := range text {
		if x >= s.width {
			return
		}
		if value == '\t' {
			spaces := 4 - x%4
			for range spaces {
				s.Set(x, y, ' ', style)
				x++
			}
			continue
		}
		s.Set(x, y, value, style)
		x += RuneWidth(value)
	}
}

// Flush redraws changed rows and places the hardware cursor.
func (s *Screen) Flush(cursorX, cursorY int) error {
	var output bytes.Buffer
	for y := range s.height {
		if s.rowUnchanged(y) {
			continue
		}
		fmt.Fprintf(&output, "\x1b[%d;1H", y+1)
		current := Style{}
		styleSet := false
		for x := range s.width {
			cell := s.cells[y*s.width+x]
			if cell.continuation {
				continue
			}
			if !styleSet || cell.Style != current {
				writeStyle(&output, cell.Style)
				current = cell.Style
				styleSet = true
			}
			value := cell.Rune
			if value == 0 {
				value = ' '
			}
			output.WriteRune(value)
		}
	}
	output.WriteString("\x1b[0m")
	fmt.Fprintf(&output, "\x1b[%d;%dH", cursorY+1, cursorX+1)
	if _, err := s.output.Write(output.Bytes()); err != nil {
		return fmt.Errorf("draw terminal: %w", err)
	}
	s.last = append(s.last[:0], s.cells...)
	return nil
}

// RuneWidth returns the terminal cell width of a rune.
func RuneWidth(value rune) int {
	if unicode.Is(unicode.Mn, value) || unicode.Is(unicode.Me, value) {
		return 0
	}
	if unicode.In(value, unicode.Han, unicode.Hangul, unicode.Hiragana, unicode.Katakana) ||
		(value >= 0x1F300 && value <= 0x1FAFF) ||
		(value >= 0xFF01 && value <= 0xFF60) {
		return 2
	}
	return 1
}

func (s *Screen) rowUnchanged(row int) bool {
	if len(s.last) != len(s.cells) {
		return false
	}
	start := row * s.width
	for i := start; i < start+s.width; i++ {
		if s.last[i] != s.cells[i] {
			return false
		}
	}
	return true
}

func writeStyle(output *bytes.Buffer, style Style) {
	output.WriteString("\x1b[0")
	if style.Bold {
		output.WriteString(";1")
	}
	if style.Underline {
		output.WriteString(";4")
	}
	if style.CurlyUnderline {
		output.WriteString(";4:3")
	}
	if style.HasUnderlineColor {
		fmt.Fprintf(output, ";58;2;%d;%d;%d",
			style.UnderlineColor.R, style.UnderlineColor.G, style.UnderlineColor.B)
	}
	fmt.Fprintf(output, ";38;2;%d;%d;%d;48;2;%d;%d;%dm",
		style.Foreground.R, style.Foreground.G, style.Foreground.B,
		style.Background.R, style.Background.G, style.Background.B)
}

// Key identifies non-text keyboard input.
type Key uint16

const (
	KeyRune Key = iota
	KeyEscape
	KeyEnter
	KeyBackspace
	KeyDelete
	KeyTab
	KeyUp
	KeyDown
	KeyLeft
	KeyRight
	KeyHome
	KeyEnd
	KeyPageUp
	KeyPageDown
)

// Event is one decoded keyboard event.
type Event struct {
	Key     Key
	Rune    rune
	Shift   bool
	Control bool
	Alt     bool
	Super   bool
}

// Reader decodes terminal input.
type Reader struct {
	input *bufio.Reader
	file  *os.File
}

// NewReader returns a keyboard reader.
func NewReader(input io.Reader) *Reader {
	reader := &Reader{input: bufio.NewReader(input)}
	reader.file, _ = input.(*os.File)
	return reader
}

// ReadEvent waits for one keyboard event.
func (r *Reader) ReadEvent() (Event, error) {
	first, err := r.input.ReadByte()
	if err != nil {
		return Event{}, err
	}
	if first == 0x1b {
		return r.readEscape()
	}
	if first == '\r' || first == '\n' {
		return Event{Key: KeyEnter}, nil
	}
	if first == 0x7f || first == 0x08 {
		return Event{Key: KeyBackspace}, nil
	}
	if first == '\t' {
		return Event{Key: KeyTab}, nil
	}
	if first == 0 {
		return Event{Key: KeyRune, Rune: ' ', Control: true}, nil
	}
	if first > 0 && first < 0x20 {
		return Event{Key: KeyRune, Rune: rune(first + 0x60), Control: true}, nil
	}
	if first < utf8.RuneSelf {
		return Event{Key: KeyRune, Rune: rune(first)}, nil
	}
	if err := r.input.UnreadByte(); err != nil {
		return Event{}, fmt.Errorf("decode input: %w", err)
	}
	value, _, err := r.input.ReadRune()
	return Event{Key: KeyRune, Rune: value}, err
}

func (r *Reader) readEscape() (Event, error) {
	next, err := readByteAfterEscape(r.input, r.file)
	if err != nil {
		return Event{Key: KeyEscape}, nil
	}
	if next != '[' && next != 'O' {
		if next < utf8.RuneSelf {
			return Event{Key: KeyRune, Rune: rune(next), Alt: true}, nil
		}
		return Event{Key: KeyEscape}, nil
	}

	var sequence strings.Builder
	for sequence.Len() < 8 {
		value, readErr := r.input.ReadByte()
		if readErr != nil {
			return Event{}, readErr
		}
		sequence.WriteByte(value)
		if (value >= 'A' && value <= 'Z') || value == '~' || value == 'u' {
			break
		}
	}
	return escapeEvent(sequence.String()), nil
}

func escapeEvent(sequence string) Event {
	switch sequence {
	case "A":
		return Event{Key: KeyUp}
	case "B":
		return Event{Key: KeyDown}
	case "C":
		return Event{Key: KeyRight}
	case "D":
		return Event{Key: KeyLeft}
	case "H", "1~", "7~":
		return Event{Key: KeyHome}
	case "F", "4~", "8~":
		return Event{Key: KeyEnd}
	case "3~":
		return Event{Key: KeyDelete}
	case "5~":
		return Event{Key: KeyPageUp}
	case "6~":
		return Event{Key: KeyPageDown}
	default:
		return modifiedKeyEvent(sequence)
	}
}

func modifiedKeyEvent(sequence string) Event {
	terminator := byte(0)
	if len(sequence) > 0 {
		terminator = sequence[len(sequence)-1]
	}
	trimmed := strings.TrimSuffix(strings.TrimSuffix(sequence, "u"), "~")
	if strings.ContainsRune("ABCDHF", rune(terminator)) {
		trimmed = strings.TrimSuffix(trimmed, string(terminator))
	}
	fields := strings.Split(trimmed, ";")
	codepoint := 0
	modifier := 1
	if terminator == 'u' && len(fields) >= 1 {
		codepoint, _ = strconv.Atoi(fields[0])
		if len(fields) >= 2 {
			modifier, _ = strconv.Atoi(fields[1])
		}
	} else if terminator == '~' && len(fields) == 3 && fields[0] == "27" {
		modifier, _ = strconv.Atoi(fields[1])
		codepoint, _ = strconv.Atoi(fields[2])
	} else if strings.ContainsRune("ABCDHF", rune(terminator)) {
		if len(fields) >= 2 {
			modifier, _ = strconv.Atoi(fields[len(fields)-1])
		}
		key := map[byte]Key{'A': KeyUp, 'B': KeyDown, 'C': KeyRight, 'D': KeyLeft, 'H': KeyHome, 'F': KeyEnd}[terminator]
		bits := modifier - 1
		return eventWithModifiers(key, 0, bits)
	}
	if codepoint == 0 || modifier == 0 {
		return Event{Key: KeyEscape}
	}
	bits := modifier - 1
	if bits == 0 {
		switch codepoint {
		case 9:
			return Event{Key: KeyTab}
		case 13:
			return Event{Key: KeyEnter}
		case 27:
			return Event{Key: KeyEscape}
		case 127:
			return Event{Key: KeyBackspace}
		}
	}
	return eventWithModifiers(KeyRune, rune(codepoint), bits)
}

func eventWithModifiers(key Key, value rune, bits int) Event {
	return Event{
		Key:     key,
		Rune:    value,
		Shift:   bits&1 != 0,
		Alt:     bits&2 != 0,
		Control: bits&4 != 0,
		Super:   bits&8 != 0,
	}
}

// ParseColor parses #RRGGBB.
func ParseColor(value string) (Color, error) {
	if len(value) != 7 || value[0] != '#' {
		return Color{}, fmt.Errorf("parse color %q: want #RRGGBB", value)
	}
	number, err := strconv.ParseUint(value[1:], 16, 24)
	if err != nil {
		return Color{}, fmt.Errorf("parse color %q: %w", value, err)
	}
	return Color{R: uint8(number >> 16), G: uint8(number >> 8), B: uint8(number)}, nil
}

// Session owns terminal raw mode and alternate-screen state.
type Session struct {
	input  *os.File
	output *os.File
	mu     sync.Mutex
	state  any
	active bool
}

// Open puts the controlling terminal into raw mode.
func Open(input, output *os.File) (*Session, error) {
	session := &Session{input: input, output: output}
	if err := session.Resume(); err != nil {
		return nil, err
	}
	return session, nil
}

// Size returns terminal columns and rows.
func (s *Session) Size() (int, int, error) {
	return terminalSize(s.output.Fd())
}

// Close restores the terminal.
func (s *Session) Close() error {
	return s.Suspend()
}

// Suspend restores cooked input and leaves the alternate screen.
func (s *Session) Suspend() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.active {
		return nil
	}
	_, writeErr := s.output.WriteString("\x1b[<u\x1b[0m\x1b[?1049l\x1b[?25h")
	restoreErr := restore(s.input.Fd(), s.state)
	if restoreErr == nil {
		s.active = false
	}
	if writeErr != nil {
		return fmt.Errorf("leave terminal screen: %w", writeErr)
	}
	return restoreErr
}

// Resume enters raw input mode and the alternate screen.
func (s *Session) Resume() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active {
		return nil
	}
	state, err := enableRaw(s.input.Fd())
	if err != nil {
		return err
	}
	s.state = state
	if _, err := s.output.WriteString("\x1b[?1049h\x1b[>1u\x1b[?25h\x1b[2J"); err != nil {
		_ = restore(s.input.Fd(), state)
		return fmt.Errorf("enter terminal screen: %w", err)
	}
	s.active = true
	return nil
}

// Invalidate forces the next flush to redraw every row.
func (s *Screen) Invalidate() {
	s.last = nil
}
