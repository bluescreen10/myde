 # myde

`myde` is a small, fast terminal editor written in Go. Its interaction model is
inspired by Emacs: commands have names, keys invoke commands, and a saved
extension file can define new functions, bindings, hooks, and theme colors
without restarting the editor.

This repository is an early but usable implementation. It deliberately has no
runtime Go dependencies. Text is stored in a piece table, rendering updates only
changed terminal rows, and syntax state is reparsed from the edited line rather
than rebuilding an entire document.

## Build and run

```sh
go install github.com/bluescreen10/myde@latest

# Or build the current checkout:
go build -o myde .
./myde .
./myde path/to/file.go
./myde -root path/to/project file.go
```

`myde` supports macOS and Linux terminals. Project search uses `rg` when it is
installed. Git, language servers, and debug adapters are optional external
processes.

## Default keys

| Key | Action |
|---|---|
| `C-p` | file/command panel; prefix the query with `>` for commands |
| `M-s` (macOS) / `C-s` (other OSes) | save |
| `C-c/v/x` | copy / paste / cut through the system clipboard |
| `C-z/y` | undo / redo |
| `M-f` (macOS) / `C-f` (other OSes) | search in the current buffer |
| `C-Shift-F` | search across all files under the workspace root |
| `M-q` | quit; modified buffers offer Save All, Discard All, or Cancel |
| `C-1` … `C-9` | switch directly to buffer 1 … 9 |
| `C-Shift-N` | create a file in the focused explorer directory, or a new buffer in the editor |
| `C-r` | rename the selected file while the file explorer is focused |
| `Shift-Arrow` | extend the text selection |
| `M-Left` / `M-Right` (macOS) | beginning / end of line |
| `M-Up` / `M-Down` (macOS) | beginning / end of file |
| `M-Shift-Up` / `M-Shift-Down` | page up / page down |
| `M-d` (macOS) | add the next occurrence of the selected text as another cursor |
| `M-j` | add a cursor on the next line |
| `M-b` | open, focus, or hide the file explorer |
| `M-.` | go to definition through the active language server |
| `C-Space` | completion (LSP plus local words) |

Arrow, Home, End, Page Up, Page Down, Backspace, Delete, Enter, and Tab work in
the editing area. Shift with navigation extends a selection; typing replaces
selected text. Undo groups consecutively typed words instead of removing one
character at a time. Typing filters every command palette with fuzzy matching.
`Escape` first removes additional cursors and a second press clears the remaining
selection. It also cancels the current panel, prompt, key sequence, or action. Saving an unnamed
buffer opens its filename prompt in the minibuffer below the modeline.
Closing a modified file opens a Save, Discard, or Cancel confirmation; quitting
with modified buffers opens a Save All, Discard All, or Cancel confirmation.
Clean new buffers close immediately without being treated as modified.
Control-number switching uses the CSI-u keyboard protocol requested by myde;
`M-1` through `M-9` provide the same bindings on older terminals.

Myde negotiates enhanced keyboard reporting for modified arrows, including
Kitty protocol event-type and alternate-key fields.

Some macOS terminal defaults translate Option-Left/Right into `Esc b`/`Esc f`
before applications can identify the arrow key. In Ghostty, preserve the arrow
and modifier with explicit CSI bindings:

```ini
keybind = alt+arrow_left=csi:1;3D
keybind = alt+arrow_right=csi:1;3C
keybind = alt+arrow_up=csi:1;3A
keybind = alt+arrow_down=csi:1;3B
keybind = alt+shift+arrow_up=csi:1;4A
keybind = alt+shift+arrow_down=csi:1;4B
```

The file explorer is a navigable tree on the left. Type while it is focused to
filter paths, use Up/Down to select entries, Left/Right to collapse or expand
folders, and Enter to open a file and close the explorer. `C-Shift-N` creates a
file in the selected directory, or beside the selected file. `C-r` renames the
selected file, and Delete removes it after confirmation. Escape cancels a
prompt or closes the explorer and returns focus to the editor. While visible,
the explorer automatically reflects files created, renamed, or removed by
external tools without losing its current filter or expanded folders.

## Commands

Open the centered panel with `C-p`. It searches workspace files by default; put
`>` at the start of the query to search commands. The panel keeps the query,
results, and keyboard help in separate bordered regions. Recently opened files
and recently chosen commands appear first. Important commands include:

- `search.project` — open the live workspace-search sidebar. Results are grouped
  by filename as the query changes; Up/Down selects matches and Enter opens one.
  Ripgrep is used when available, with a built-in recursive fallback.
- `git.stage` — open (or raise) the persistent Git Stage tab. Its horizontal
  layout separates unstaged and staged files on the left and automatically
  previews the selected diff on the
  right. `+` stages, `-` unstages, and Enter or Tab focuses the diff; Tab returns
  to the changes list. Added and removed lines use green and red backgrounds,
  with stronger highlighting on changed characters. The list refreshes in the
  background and advances to the next file after an action.
- `git.diff` and `git.diff staged` — open workspace changes in a read-only
  buffer.
- `git.commit` — open a centered commit-message prompt and create the commit.
- `switch.mode` — select the active buffer's mode from the modes registered by
  core and plugins. File extensions choose the initial mode automatically.
- `theme.select` — choose one of the JSON themes shipped with myde. Passing a
  theme ID, such as `theme.select midnight`, selects it directly.
- `file.new` — create a file in the focused explorer directory, or create a
  clean untitled buffer when invoked from the editor.
- `file.rename` — rename the selected explorer file without losing an open
  buffer's edits or undo history.
- `go.fmt` — format the active buffer in memory as one undoable edit.
- `go.vet`, `go.build`, and `go.test` — run the corresponding Go tool for the
  active file's package. Save modified buffers before running these commands.
- `lsp.start` — prompt for a language server command. Diagnostics appear inline,
  completion uses `textDocument/completion`, and `lsp.definition` navigates to
  definitions. When a Go buffer becomes active, the Go plugin starts `gopls`
  automatically when it is on `PATH`; completion opens with `C-Space` and
  updates while identifiers are typed. Completion suggestions do not capture
  text input: use Up/Down and Enter or Tab to accept one. Completion opens
  beside the text cursor and shows signature and documentation details supplied
  by the language server; servers such as `gopls` that resolve completion items
  are supported. LSP errors use
  red curly underlines and a tinted line background. Moving the cursor onto an
  error opens a diagnostic card with its source, code, and full message.
- `debug.start` — start the adapter registered for the active mode, or prompt
  when that mode has no adapter. Then use `debug.launch`,
  `debug.toggle-breakpoint`, `debug.continue`, `debug.next`, `debug.step-in`,
  `debug.step-out`, and `debug.disconnect`.
- `shell.run` — run a non-interactive command at the workspace root and inspect
  its output.
- `shell.exec` — run a command without opening an output panel, useful in hooks.
- `terminal.open` — open or switch to the `*terminal*` buffer. Enter runs the
  current prompt with the shell from `$SHELL` (falling back to `/bin/sh`);
  output and the next prompt remain in that buffer, `cd` updates its working
  directory, and `exit` closes the terminal buffer.
- `editor.quit force` and `buffer.close force` — explicitly discard edits.

Commands with arguments can be invoked from extensions. The palette lists the
argument-free form; commands that need input open a prompt.

## Extensions and hot reload

Create `.myde` at the workspace root, or set `MYDE_CONFIG` to another file. The
file is checked twice per second and successfully saved changes take effect
without restarting.

```text
# A function is a semicolon-separated sequence of commands.
def format = shell.exec gofmt -w {file}

# Hooks invoke a function or built-in command.
hook save = format
hook open = shell.exec printf opened

# Keys use ctrl-/alt- names.
bind ctrl-g = git.stage
bind alt-n = buffer.next

# Start with the VS Code Dark 2026 theme and override individual colors.
color background = #1F1F1F
color accent = #4FA8FF
color keyword = #C586C0
color string = #CE9178
color panel = #252526
color panelborder = #5A5A5A
color diagnostic = #F14C4C
color diagnosticbackground = #3A2427
color success = #4EC9B0
color warning = #D7BA7D
color danger = #F14C4C

# File buffers keep 1,000 undo entries by default. Zero disables history.
set history-limit = 2000

# Select the startup theme by ID.
set theme = midnight
```

The placeholders `{file}` and `{root}` expand to the active file and workspace.
Unknown or invalid declarations leave the previous configuration active and show
an error in the status line.

## Themes

Built-in themes are JSON files in `themes`. A theme defines its colors and the
literal characters used for panel separators, corners, and border lines:

```json
"borders": {
  "separator": { "vertical": "│", "horizontal": "─" },
  "corners": {
    "top_left": "╭", "top_right": "╮",
    "bottom_left": "╰", "bottom_right": "╯"
  },
  "lines": { "horizontal": "─", "vertical": "│" }
}
```

Each value must be exactly one character; a space makes that part invisible.
The files are embedded in the binary so they remain available outside the
source tree. VS Dark 2026, the editor's original theme, remains the default.
The built-ins also include Paper, Midnight, Retro Green, and Retro Orange; the
retro themes use monochrome phosphor palettes and ASCII terminal borders.
Run `theme.select` from the command palette to switch themes for the current
session, or use `set theme = <id>` in `.myde` to select one at startup.

## Plugins

Plugins register commands and editing modes through the public `plugin.Host`
interface and describe UI through the shared `ui` package. A mode can associate
file extensions with syntax, an LSP server, and a DAP adapter. `Host.NewView`
creates a persistent tab from generic layouts, list widgets, and rich-text
widgets; its handle can raise, update, or destroy the tab. Custom widgets return
rich-text lines and spans with semantic theme tones and optional display-column
placement; they never emit terminal escape codes.
Plugins can also inspect or replace the active document and open transient
sidebars, text prompts, and read-only buffers without importing editor internals.
The Git plugin in `plugins/git` owns Git subprocess and repository logic. The Go
plugin in `plugins/golang` owns Go mode, `gopls`, Delve, and the `go.*` commands.

## Architecture

- `buffer` — piece-table text storage, transactional undo/redo, rune-aware
  positions, atomic saving, and external-change detection.
- `terminal` — raw input, ANSI true-color rendering, and row-level frame diffing.
- `syntax` — incremental stateful highlighting for Go, Rust, JavaScript,
  TypeScript, Python, shell, and C/C++.
- `protocol` — Content-Length framed JSON transport, JSON-RPC/LSP process
  management, and Debug Adapter Protocol process management.
- `ui` — shared view, layout, widget, rich-text, sidebar, and semantic-style
  definitions used by both the editor and plugins.
- `plugin` — the command registration, editing-mode, and host-service boundary
  used by plugins; host UI methods accept types from `ui`.
- `plugins/git` — Git staging, diffs, and commits implemented outside the editor
  core.
- `plugins/golang` — Go mode, formatting, package commands, gopls, and Delve.
- `editor` — commands, palettes, multi-cursor edits, generic views and sidebars,
  extensions, LSP/DAP, search, hooks, and the VS Dark 2026 theme. Each editor
  buffer owns its syntax cache, diagnostics, breakpoints, mode, and terminal
  state rather than storing those in path-keyed application maps.

The built-in highlighter is intentionally lightweight. The `syntax.Highlighter`
boundary is where a Tree-sitter-backed parser can be installed without coupling
the buffer or UI to a C dependency.
