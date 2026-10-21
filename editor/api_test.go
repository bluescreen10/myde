package editor_test

import (
	"bytes"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/bluescreen10/myde/editor"
	"github.com/bluescreen10/myde/plugin"
	searchplugin "github.com/bluescreen10/myde/plugins/search"
	"github.com/bluescreen10/myde/settings"
	"github.com/bluescreen10/myde/terminal"
)

type testSession struct{ width, height int }

func (s *testSession) Size() (int, int, error) { return s.width, s.height, nil }

// Decode terminal output as a client would; no editor state is exposed here.
type testScreen struct {
	rows map[int][]rune
	x, y int
	raw  bytes.Buffer
}

func (s *testScreen) Write(data []byte) (int, error) {
	s.raw.Write(data)
	for i := 0; i < len(data); {
		if data[i] == 0x1b && i+1 < len(data) && data[i+1] == '[' {
			end := i + 2
			for end < len(data) && (data[end] < 0x40 || data[end] > 0x7e) {
				end++
			}
			if end >= len(data) {
				break
			}
			if data[end] == 'H' {
				fields := strings.Split(string(data[i+2:end]), ";")
				if len(fields) == 2 {
					s.y, _ = strconv.Atoi(fields[0])
					s.x, _ = strconv.Atoi(fields[1])
					s.y--
					s.x--
				}
			}
			i = end + 1
			continue
		}
		value, size := utf8.DecodeRune(data[i:])
		i += size
		if value == '\n' {
			s.y++
			s.x = 0
			continue
		}
		width := terminal.RuneWidth(value)
		if s.x >= 0 && s.y >= 0 {
			row := s.rows[s.y]
			for len(row) < s.x+max(1, width) {
				row = append(row, ' ')
			}
			row[s.x] = value
			if width == 2 {
				row[s.x+1] = 0
			}
			s.rows[s.y] = row
		}
		s.x += width
	}
	return len(data), nil
}
func (s *testScreen) row(y int) string {
	var text strings.Builder
	for _, r := range s.rows[y] {
		if r != 0 {
			text.WriteRune(r)
		}
	}
	return text.String()
}
func (s *testScreen) text() string {
	var text strings.Builder
	for y := 0; y < 60; y++ {
		text.WriteString(s.row(y))
		text.WriteByte('\n')
	}
	return text.String()
}

type harness struct {
	t       *testing.T
	app     *editor.App
	screen  *testScreen
	session *testSession
	root    string
}

func newHarness(t *testing.T, root string, installed ...plugin.Plugin) *harness {
	return harnessWithSettings(t, root, nil, installed...)
}
func harnessWithSettings(t *testing.T, root string, store *settings.Store, installed ...plugin.Plugin) *harness {
	t.Helper()
	if root == "" {
		root = t.TempDir()
	}
	t.Setenv("MYDE_CONFIG", filepath.Join(root, ".myde"))
	h := &harness{t: t, root: root, session: &testSession{100, 24}, screen: &testScreen{rows: make(map[int][]rune)}}
	var err error
	installed = append(installed, searchplugin.New())
	h.app, err = editor.NewWithSettings(root, nil, h.session, strings.NewReader(""), h.screen, store, installed...)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := h.app.Close(); err != nil {
			t.Error(err)
		}
	})
	h.draw()
	return h
}
func (h *harness) draw() {
	h.t.Helper()
	if err := h.app.Render(); err != nil {
		h.t.Fatal(err)
	}
}
func (h *harness) exec(command string) {
	h.t.Helper()
	if err := h.app.Execute(command); err != nil {
		h.t.Fatalf("%s: %v", command, err)
	}
	h.draw()
}
func (h *harness) key(e terminal.Event) {
	h.t.Helper()
	if err := h.app.HandleEvent(e); err != nil {
		h.t.Fatal(err)
	}
	h.draw()
}
func (h *harness) text(value string) {
	h.t.Helper()
	for _, r := range value {
		event := terminal.Event{Key: terminal.KeyRune, Rune: r}
		if r == '\n' {
			event.Key = terminal.KeyEnter
		}
		h.key(event)
	}
}
func (h *harness) replace(value string) {
	h.t.Helper()
	if err := h.app.ReplaceCurrentDocument([]byte(value)); err != nil {
		h.t.Fatal(err)
	}
	h.draw()
}
func (h *harness) wait(wanted string, condition func() bool) {
	h.t.Helper()
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		h.app.Update()
		h.draw()
		if condition() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	h.t.Fatalf("timed out waiting for %s; screen:\n%s", wanted, h.screen.text())
}
func (h *harness) mouse(x, y, button int, motion, release bool) {
	h.t.Helper()
	h.key(terminal.Event{Key: terminal.KeyMouse, MouseX: x, MouseY: y, MouseButton: button, MouseMotion: motion, MouseRelease: release})
}
func (h *harness) content() string           { return string(h.app.CurrentDocument().Content) }
func (h *harness) selection() string         { return h.app.CurrentDocument().Selection }
func (h *harness) contains(text string) bool { return strings.Contains(h.screen.text(), text) }

var _ io.Writer = (*testScreen)(nil)
