// Package buffer provides editable, file-backed text buffers.
package buffer

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
	"unicode/utf8"
)

// Point identifies a zero-based line and display column.
type Point struct {
	Line   int
	Column int
}

// Cursor is a selection. Anchor and Point are equal when there is no selection.
type Cursor struct {
	Anchor Point
	Point  Point
}

type source uint8

const (
	originalSource source = iota
	addSource
)

type piece struct {
	source source
	start  int
	length int
}

type historyChange struct {
	start  int
	before []byte
	after  []byte
}

type historyEntry struct {
	changes     []historyChange
	beforeState uint64
	afterState  uint64
}

// Buffer stores edits as a piece table. Insertions do not copy the unchanged file.
type Buffer struct {
	mu sync.RWMutex

	path     string
	name     string
	original []byte
	added    []byte
	pieces   []piece
	length   int

	cursors    []Cursor
	lineStarts []int
	revision   uint64
	state      uint64
	savedState uint64
	nextState  uint64
	modified   time.Time

	history          []historyEntry
	historyIndex     int
	historyLimit     int
	transactionDepth int
	pendingChanges   []historyChange
	mergeNextGroup   bool
}

// New returns an empty, unnamed buffer.
func New() *Buffer {
	return &Buffer{
		cursors:      []Cursor{{}},
		historyLimit: 1000,
	}
}

// NewNamed returns an empty buffer with a display name and no backing file.
func NewNamed(name string) *Buffer {
	b := New()
	b.name = name
	return b
}

// Open loads path into a buffer.
func Open(path string) (*Buffer, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", path, err)
	}

	data, err := os.ReadFile(absolute)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", absolute, err)
	}

	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", absolute, err)
	}

	b := New()
	b.path = absolute
	b.original = data
	b.length = len(data)
	b.modified = info.ModTime()
	if len(data) > 0 {
		b.pieces = []piece{{source: originalSource, length: len(data)}}
	}
	return b, nil
}

// Name returns the file name or "untitled" for an unnamed buffer.
func (b *Buffer) Name() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.path == "" {
		if b.name != "" {
			return b.name
		}
		return "untitled"
	}
	return filepath.Base(b.path)
}

// Path returns the absolute backing file path, if any.
func (b *Buffer) Path() string {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.path
}

// Len returns the buffer length in bytes.
func (b *Buffer) Len() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.length
}

// Revision changes after each edit or reload.
func (b *Buffer) Revision() uint64 {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.revision
}

// IsDirty reports whether the buffer differs from its last saved state.
func (b *Buffer) IsDirty() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.state != b.savedState
}

// Bytes returns an independent snapshot of the buffer.
func (b *Buffer) Bytes() []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return b.bytesLocked()
}

// Slice returns an independent byte range. Invalid bounds are clamped.
func (b *Buffer) Slice(start, end int) []byte {
	b.mu.RLock()
	defer b.mu.RUnlock()

	start = clamp(start, 0, b.length)
	end = clamp(end, start, b.length)
	result := make([]byte, 0, end-start)
	position := 0
	for _, current := range b.pieces {
		pieceEnd := position + current.length
		if pieceEnd <= start {
			position = pieceEnd
			continue
		}
		if position >= end {
			break
		}
		from := max(start-position, 0)
		to := min(end-position, current.length)
		data := b.sourceBytesLocked(current.source)
		result = append(result, data[current.start+from:current.start+to]...)
		position = pieceEnd
	}
	return result
}

// Insert adds text at offset.
func (b *Buffer) Insert(offset int, text []byte) {
	if len(text) == 0 {
		return
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	offset = clamp(offset, 0, b.length)
	b.insertLocked(offset, text)
	b.revision++
	b.recordChangeLocked(historyChange{start: offset, after: append([]byte(nil), text...)})
}

func (b *Buffer) insertLocked(offset int, text []byte) {
	if len(text) == 0 {
		return
	}
	added := piece{source: addSource, start: len(b.added), length: len(text)}
	b.added = append(b.added, text...)
	b.splitAndInsertLocked(offset, added)
	b.length += len(text)
	b.updateLinesAfterInsertLocked(offset, text)
}

// Delete removes the half-open byte range [start, end).
func (b *Buffer) Delete(start, end int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	start = clamp(start, 0, b.length)
	end = clamp(end, start, b.length)
	if start == end {
		return
	}
	removed := b.rangeLocked(start, end)
	b.deleteLocked(start, end)
	b.revision++
	b.recordChangeLocked(historyChange{start: start, before: removed})
}

func (b *Buffer) deleteLocked(start, end int) {
	if start == end {
		return
	}
	pieces := make([]piece, 0, len(b.pieces))
	position := 0
	for _, current := range b.pieces {
		pieceEnd := position + current.length
		if pieceEnd <= start || position >= end {
			pieces = append(pieces, current)
			position = pieceEnd
			continue
		}
		if start > position {
			pieces = append(pieces, piece{
				source: current.source,
				start:  current.start,
				length: start - position,
			})
		}
		if end < pieceEnd {
			skip := end - position
			pieces = append(pieces, piece{
				source: current.source,
				start:  current.start + skip,
				length: pieceEnd - end,
			})
		}
		position = pieceEnd
	}

	b.pieces = pieces
	b.length -= end - start
	b.updateLinesAfterDeleteLocked(start, end)
}

// BeginTransaction groups subsequent edits into one undo entry.
func (b *Buffer) BeginTransaction() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.transactionDepth++
}

// EndTransaction closes an edit group started by BeginTransaction.
func (b *Buffer) EndTransaction() {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.transactionDepth == 0 {
		return
	}
	b.transactionDepth--
	if b.transactionDepth == 0 && len(b.pendingChanges) > 0 {
		b.commitHistoryLocked(b.pendingChanges)
		b.pendingChanges = nil
	}
}

// MergeNextEditGroup folds the next completed edit group into the current undo entry.
func (b *Buffer) MergeNextEditGroup() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.mergeNextGroup = true
}

// SetHistoryLimit sets the maximum undo entries. Zero disables history.
func (b *Buffer) SetHistoryLimit(entries int) {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.historyLimit = max(0, entries)
	if len(b.history) <= b.historyLimit {
		return
	}
	if b.historyLimit == 0 {
		b.history = nil
		b.historyIndex = 0
		return
	}
	start := max(0, b.historyIndex-b.historyLimit)
	end := min(len(b.history), start+b.historyLimit)
	if end-start < b.historyLimit {
		start = max(0, end-b.historyLimit)
	}
	b.history = append([]historyEntry(nil), b.history[start:end]...)
	b.historyIndex -= start
}

// Undo restores the state before the latest edit group.
func (b *Buffer) Undo() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.mergeNextGroup = false
	if b.transactionDepth != 0 || b.historyIndex == 0 {
		return false
	}
	entry := b.history[b.historyIndex-1]
	for index := len(entry.changes) - 1; index >= 0; index-- {
		change := entry.changes[index]
		b.deleteLocked(change.start, change.start+len(change.after))
		b.insertLocked(change.start, change.before)
	}
	b.historyIndex--
	b.state = entry.beforeState
	b.revision++
	return true
}

// Redo reapplies the next undone edit group.
func (b *Buffer) Redo() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.mergeNextGroup = false
	if b.transactionDepth != 0 || b.historyIndex >= len(b.history) {
		return false
	}
	entry := b.history[b.historyIndex]
	for _, change := range entry.changes {
		b.deleteLocked(change.start, change.start+len(change.before))
		b.insertLocked(change.start, change.after)
	}
	b.historyIndex++
	b.state = entry.afterState
	b.revision++
	return true
}

// Offset converts a line and rune column to a byte offset.
func (b *Buffer) Offset(point Point) int {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureLineStartsLocked()
	line := clamp(point.Line, 0, len(b.lineStarts)-1)
	start := b.lineStarts[line]
	end := b.length
	if line+1 < len(b.lineStarts) {
		end = b.lineStarts[line+1]
	}
	data := b.rangeLocked(start, end)
	offset := 0
	for column := 0; column < point.Column && offset < len(data); column++ {
		if data[offset] == '\n' {
			break
		}
		_, size := utf8.DecodeRune(data[offset:])
		if size == 0 {
			break
		}
		offset += size
	}
	return start + offset
}

// Point converts a byte offset to a line and rune column.
func (b *Buffer) Point(offset int) Point {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureLineStartsLocked()
	offset = clamp(offset, 0, b.length)
	line := sort.Search(len(b.lineStarts), func(index int) bool {
		return b.lineStarts[index] > offset
	}) - 1
	line = max(0, line)
	start := b.lineStarts[line]
	return Point{Line: line, Column: utf8.RuneCount(b.rangeLocked(start, offset))}
}

// Line returns one line without its trailing newline.
func (b *Buffer) Line(line int) []byte {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureLineStartsLocked()
	if line < 0 || line >= len(b.lineStarts) {
		return nil
	}
	start := b.lineStarts[line]
	end := b.length
	if line+1 < len(b.lineStarts) {
		end = b.lineStarts[line+1]
	}
	data := b.rangeLocked(start, end)
	return bytes.TrimSuffix(data, []byte{'\n'})
}

// LineCount returns the number of logical lines.
func (b *Buffer) LineCount() int {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.ensureLineStartsLocked()
	return len(b.lineStarts)
}

// Cursors returns an independent copy of the cursors.
func (b *Buffer) Cursors() []Cursor {
	b.mu.RLock()
	defer b.mu.RUnlock()

	return append([]Cursor(nil), b.cursors...)
}

// SetCursors replaces the cursor set. An empty set becomes one cursor at origin.
func (b *Buffer) SetCursors(cursors []Cursor) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if len(cursors) == 0 {
		b.cursors = []Cursor{{}}
		return
	}
	b.cursors = append(b.cursors[:0], cursors...)
}

// Save writes the buffer atomically. A non-empty path changes its backing file.
func (b *Buffer) Save(path string) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if path != "" {
		absolute, err := filepath.Abs(path)
		if err != nil {
			return fmt.Errorf("resolve %s: %w", path, err)
		}
		b.path = absolute
	}
	if b.path == "" {
		return errors.New("save buffer: no path")
	}

	data := b.bytesLocked()
	mode := os.FileMode(0o644)
	if info, err := os.Stat(b.path); err == nil {
		mode = info.Mode().Perm()
	}
	temporary := b.path + ".myde-tmp"
	if err := os.WriteFile(temporary, data, mode); err != nil {
		return fmt.Errorf("write %s: %w", temporary, err)
	}
	if err := os.Rename(temporary, b.path); err != nil {
		_ = os.Remove(temporary)
		return fmt.Errorf("replace %s: %w", b.path, err)
	}
	info, err := os.Stat(b.path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", b.path, err)
	}
	b.modified = info.ModTime()
	b.savedState = b.state
	return nil
}

// HasExternalChange reports whether the backing file changed after it was read.
func (b *Buffer) HasExternalChange() bool {
	b.mu.RLock()
	defer b.mu.RUnlock()

	if b.path == "" {
		return false
	}
	info, err := os.Stat(b.path)
	return err == nil && info.ModTime().After(b.modified)
}

// Reload replaces an unmodified buffer with its backing file.
func (b *Buffer) Reload() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.path == "" {
		return errors.New("reload buffer: no path")
	}
	if b.state != b.savedState {
		return errors.New("reload buffer: unsaved changes")
	}
	data, err := os.ReadFile(b.path)
	if err != nil {
		return fmt.Errorf("read %s: %w", b.path, err)
	}
	info, err := os.Stat(b.path)
	if err != nil {
		return fmt.Errorf("stat %s: %w", b.path, err)
	}
	b.original = data
	b.added = nil
	b.pieces = nil
	if len(data) > 0 {
		b.pieces = []piece{{source: originalSource, length: len(data)}}
	}
	b.length = len(data)
	b.modified = info.ModTime()
	b.revision++
	b.nextState++
	b.state = b.nextState
	b.savedState = b.state
	b.history = nil
	b.historyIndex = 0
	b.pendingChanges = nil
	b.transactionDepth = 0
	b.mergeNextGroup = false
	b.lineStarts = nil
	return nil
}

func (b *Buffer) splitAndInsertLocked(offset int, added piece) {
	if offset == b.length {
		b.pieces = append(b.pieces, added)
		return
	}

	position := 0
	for i, current := range b.pieces {
		pieceEnd := position + current.length
		if offset > pieceEnd {
			position = pieceEnd
			continue
		}

		within := offset - position
		replacement := make([]piece, 0, 3)
		if within > 0 {
			replacement = append(replacement, piece{source: current.source, start: current.start, length: within})
		}
		replacement = append(replacement, added)
		if within < current.length {
			replacement = append(replacement, piece{
				source: current.source,
				start:  current.start + within,
				length: current.length - within,
			})
		}
		pieces := make([]piece, 0, len(b.pieces)+2)
		pieces = append(pieces, b.pieces[:i]...)
		pieces = append(pieces, replacement...)
		pieces = append(pieces, b.pieces[i+1:]...)
		b.pieces = pieces
		return
	}
	b.pieces = append(b.pieces, added)
}

func (b *Buffer) recordChangeLocked(change historyChange) {
	if b.transactionDepth > 0 {
		b.pendingChanges = append(b.pendingChanges, change)
		return
	}
	b.commitHistoryLocked([]historyChange{change})
}

func (b *Buffer) commitHistoryLocked(changes []historyChange) {
	merge := b.mergeNextGroup
	b.mergeNextGroup = false
	if len(changes) == 0 {
		return
	}
	if merge && b.historyLimit > 0 && b.historyIndex == len(b.history) && b.historyIndex > 0 {
		b.nextState++
		entry := &b.history[b.historyIndex-1]
		entry.changes = append(entry.changes, changes...)
		entry.afterState = b.nextState
		b.state = entry.afterState
		return
	}
	if b.historyIndex < len(b.history) {
		b.history = b.history[:b.historyIndex]
	}
	b.nextState++
	entry := historyEntry{
		changes:     append([]historyChange(nil), changes...),
		beforeState: b.state,
		afterState:  b.nextState,
	}
	b.state = entry.afterState
	if b.historyLimit == 0 {
		b.history = nil
		b.historyIndex = 0
		return
	}
	b.history = append(b.history, entry)
	b.historyIndex++
	if len(b.history) > b.historyLimit {
		removed := len(b.history) - b.historyLimit
		b.history = append([]historyEntry(nil), b.history[removed:]...)
		b.historyIndex -= removed
	}
}

func (b *Buffer) ensureLineStartsLocked() {
	if b.lineStarts != nil {
		return
	}
	data := b.bytesLocked()
	b.lineStarts = make([]int, 1, bytes.Count(data, []byte{'\n'})+1)
	for i, value := range data {
		if value == '\n' {
			b.lineStarts = append(b.lineStarts, i+1)
		}
	}
}

func (b *Buffer) updateLinesAfterInsertLocked(offset int, text []byte) {
	if b.lineStarts == nil {
		return
	}
	addedStarts := make([]int, 0, bytes.Count(text, []byte{'\n'}))
	for index, value := range text {
		if value == '\n' {
			addedStarts = append(addedStarts, offset+index+1)
		}
	}
	for index, start := range b.lineStarts {
		if start > offset {
			b.lineStarts[index] += len(text)
		}
	}
	if len(addedStarts) == 0 {
		return
	}
	b.lineStarts = append(b.lineStarts, addedStarts...)
	sort.Ints(b.lineStarts)
}

func (b *Buffer) updateLinesAfterDeleteLocked(start, end int) {
	if b.lineStarts == nil {
		return
	}
	removed := end - start
	starts := b.lineStarts[:0]
	for _, lineStart := range b.lineStarts {
		if lineStart > start && lineStart <= end {
			continue
		}
		if lineStart > end {
			lineStart -= removed
		}
		starts = append(starts, lineStart)
	}
	b.lineStarts = starts
}

func (b *Buffer) bytesLocked() []byte {
	return b.rangeLocked(0, b.length)
}

func (b *Buffer) rangeLocked(start, end int) []byte {
	result := make([]byte, 0, end-start)
	position := 0
	for _, current := range b.pieces {
		pieceEnd := position + current.length
		if pieceEnd <= start {
			position = pieceEnd
			continue
		}
		if position >= end {
			break
		}
		from := max(start-position, 0)
		to := min(end-position, current.length)
		data := b.sourceBytesLocked(current.source)
		result = append(result, data[current.start+from:current.start+to]...)
		position = pieceEnd
	}
	return result
}

func (b *Buffer) sourceBytesLocked(current source) []byte {
	if current == originalSource {
		return b.original
	}
	return b.added
}

func clamp(value, low, high int) int {
	return min(max(value, low), high)
}
