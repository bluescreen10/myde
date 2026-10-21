 # myde

`myde` is a small, fast terminal editor written in Go. Its interaction model is
inspired by Emacs: commands have names, keys invoke commands, and a saved
extension file can define new functions, bindings, hooks, and theme colors
without restarting the editor.

This repository is an early but usable implementation. Its only runtime Go
dependency provides PTY support for persistent terminal shells. Text is stored
in a piece table, rendering updates only changed terminal rows, and syntax state
is reparsed from the edited line rather than rebuilding an entire document.

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
| `C-Shift-F` | open or restore workspace search from any view |
| `C-g` | go to a line number |
| `M-q` | quit; modified buffers offer Save All, Discard All, or Cancel |
| `C-1` … `C-9` | switch directly to buffer 1 … 9 |
| `C-Shift-N` | create a file in the focused explorer directory, or a new buffer in the editor |
| `C-r` | rename the selected file while the file explorer is focused |
| `Shift-Arrow` | extend the text selection |
| `C-Left` / `C-Right` | beginning / end of line or input |
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
Prompts, palettes, search, and explorer filters support Left/Right, Home/End,
and the configured beginning/end shortcuts; insert and delete at the cursor.
With an empty explorer filter, Left/Right still fold directories and Delete
removes the selected file. Drag with the left mouse button to select source
text, then use `C-c` to copy it without line numbers. The mouse wheel scrolls
the editor, and clicking a split pane focuses it.
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
prompt or closes the explorer and returns focus to the editor. The workspace
index and explorer automatically reflect files created, renamed, or removed by
external tools, even while the explorer is hidden, without losing its current
filter or expanded folders.

## Commands

Open the centered panel with `C-p`. It searches workspace files by default; put
`>` at the start of the query to search commands. The panel keeps the query,
results, and keyboard help in separate bordered regions. Recently opened files
and recently chosen commands appear first. Important commands include:

- `search.project` — open or raise the bundled Search plugin's single **Search**
  tab. The left pane contains the query and results grouped by filename. The
  right pane previews the selected file, scrolls to the match, and highlights it.
  Up/Down or a mouse click selects matches; Enter opens the location for editing.
  Tab switches between results and preview; the preview supports scrolling and
  mouse selection for copying. Switching tabs retains the query, results,
  selection, and scroll position. Disk changes and unsaved edits refresh the
  search even while its tab is hidden. `C-Shift-F` raises the existing tab and
  focuses the query, including from Git Stage and prompts.
  Ripgrep is used when available, with a built-in recursive fallback.
- `cursor.goto-line` — prompt for a 1-based line number (`C-g`), or pass one
  directly, such as `cursor.goto-line 42`. Values past EOF go to the last line.
- `git.stage` — open (or raise) the persistent Git Stage tab. Its horizontal
  layout separates unstaged and staged files on the left and automatically
  previews the selected diff on the
  right. `+` stages, `-` unstages, and Enter opens the selected file for editing.
  Tab switches between the changes list and diff. Click a file to update its
  preview, click either pane to focus it, and use the wheel to scroll the pane
  under the pointer. Drag across diff text and use `C-c` to copy the selection;
  unchanged background refreshes preserve it. Added and removed lines use green and red backgrounds,
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
- `go.lsp.start`, `go.lsp.stop`, and `go.lsp.restart` — control the Go mode's
  server from any buffer or plugin view. `go.lsp.run` starts `gopls`, or replaces
  it with the command and arguments supplied to that command.
- `lsp.start`, `lsp.stop`, and `lsp.restart` — control the current mode's server.
  Start reuses a running server; stop keeps it stopped until explicitly started.
  `lsp.run <command>` starts a custom server for the current mode. Servers are
  shared by buffers in the same mode and keep running when switching modes, so
  Go, TypeScript, Rust, and other plugin modes can run concurrently. Each server
  owns its synchronization and completion capabilities and receives only its
  mode's documents. Diagnostics, completion, and definitions use that server.
  The Go plugin starts `gopls` automatically when a Go buffer becomes active.
- `lsp.definition` — go to a definition with the current mode's server.
  `C-Space` opens completion; Up/Down and Enter or Tab select a suggestion.
  Completion details can include signatures and documentation. Errors appear
  with curly underlines and a diagnostic card when the cursor moves onto them.
- `lsp.log` — open the current mode's recent language server messages, including
  workspace-loading errors. Use `lsp.log Go` to inspect Go's server from any
  buffer. If `gopls` reports "no views", check this log for the underlying Go
  environment or workspace error, then run `go.lsp.restart` after correcting it.
- `go.debug.start`, `go.debug.stop`, and `go.debug.restart` — control Go's Delve
  adapter from any buffer or view. `go.debug.run <program>` starts Go's adapter
  if needed and launches that program; without an argument it prompts.
- `debug.start`, `debug.stop`, and `debug.restart` — control the current mode's
  adapter. Debuggers are shared within a mode and run independently of other
  modes' adapters. `debug.launch`, `debug.toggle-breakpoint`, `debug.continue`,
  `debug.next`, `debug.step-in`, and `debug.step-out` use the current mode's
  debugger. Configuration sends only that mode's breakpoints.
  `debug.disconnect` is another name for stopping the current mode's debugger.
- `shell.run` — run a non-interactive command at the workspace root and inspect
  its output.
- `shell.exec` — run a command without opening an output panel, useful in hooks.
- `terminal.open` — open or switch to a persistent interactive shell from
  `$SHELL` (falling back to `/bin/sh`) in the `*terminal*` buffer. The shell
  owns its prompt, working directory, completion, and native history, so
  Up/Down and `C-R` behave as they do in a regular terminal. `C-C` interrupts
  the foreground command, output streams into the buffer as it arrives, and
  `exit` closes the terminal buffer. Keys without an editor binding are sent
  directly to the shell.
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
  "status_separator": "",
  "separator": { "vertical": "│", "horizontal": "─" },
  "corners": {
    "top_left": "╭", "top_right": "╮",
    "bottom_left": "╰", "bottom_right": "╯"
  },
  "lines": { "horizontal": "─", "vertical": "│" }
}
```

Each value must be exactly one character; a space makes that part invisible.
`status_separator` draws the chevron between status-bar sections and defaults
to `` when omitted so existing themes remain compatible.
Themes can also define `plain`, `import`, `declaration`, `function`, `constant`,
and three delimiter colors (`delimiter`, `delimiter2`, and `delimiter3`) for
richer source highlighting. Older themes fall back to their existing string,
keyword, type, number, and foreground colors for those roles.
Tab colors are independently configurable with `tab_background`, `tab_text`,
`tab_active_background`, and `tab_active_text`. `selection_text` controls text
on highlighted navigation rows; omitted tab and selection-text colors inherit
the theme's existing background, muted, selection, accent, and status colors.
The files are embedded in the binary so they remain available outside the
source tree. VS Dark 2026, the editor's original theme, remains the default.
The built-ins also include Paper, Midnight, Retro Green, and Retro Orange; the
retro themes use monochrome phosphor palettes and ASCII terminal borders.
Run `theme.select` from the command palette to switch themes and save the
selection as the user-wide default. A workspace can override it with
`set theme = <id>` in `.myde`.

## User settings

On startup myde creates its directory beneath the platform's standard user
configuration location (`os.UserConfigDir`):

```text
myde/
  settings.conf
  themes/
  plugins/
```

This is normally `~/.config/myde` on Linux,
`~/Library/Application Support/myde` on macOS, and `%AppData%\\myde` on
Windows. Set `MYDE_CONFIG_DIR` to use a different directory. The override points
directly at the directory containing `settings.conf`, rather than its parent.
It is separate from `MYDE_CONFIG`, which points to a workspace extension file.
`settings.conf` uses one `key = value` setting per line:

```text
theme = midnight
go-format-on-save = true
go-imports-on-save = true
```

The Go plugin runs `goimports` and then `gofmt` before saving `.go` files.
Both behaviors default to `true`; set either value to `false` to disable it.
Because `go-imports-on-save` invokes the external `goimports` executable, it
must be available on `PATH` while that setting is enabled. Workspace `set`
declarations with the same names override these user-wide values.

The `themes` and `plugins` directories reserve stable locations for
user-installed JSON themes and compiled Go plugins. Myde does not load their
contents yet.

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
`Host.OpenFile` opens an editable workspace file. Plugins control services by
mode name through `StartLanguageServer`, `RunLanguageServer`,
`StopLanguageServer`, and `RestartLanguageServer`, with equivalent
`StartDebugAdapter`, `RunDebugAdapter`, `StopDebugAdapter`, and
`RestartDebugAdapter` methods. `LaunchDebugProgram` launches a program in the
named mode's debugger. Start reuses an existing session; Run replaces that
mode's session with explicit configuration; Restart preserves its configuration.
Replacing, stopping, or disconnecting a session leaves other modes alone.
`CurrentDocument` includes the mode, zero-based cursor position, and selection.
`OpenDocuments` provides independent snapshots of open files; `OpenLocation`
opens a file at a zero-based line and rune column. `RegisterGlobalCommand`
registers commands available from inputs and plugin views, and `RegisterBinding`
provides shortcuts that workspace bindings can override. Sidebars can include
a `ui.TextInput`, an `OnInput` callback, and an `OnClose` callback that returns
selection and scroll state to the owning plugin. `BeforeRefresh` captures host
data on the UI loop; `OnRefresh` processes snapshots in the background. Plugins
that implement `io.Closer` are closed when the editor releases them.
View lists can also include a `ui.TextInput` and `OnInput` callback, returning an
updated view as the query changes. A view's `BeforeRefresh` captures document
snapshots on the UI loop, and `FocusPane` can focus its query when raised.
Preview widgets can specify `FocusLine` and `FocusColumn` to show a location;
subsequent refreshes preserve scrolling when that location stays the same.
Plugins can replace built-in syntax-only fallback modes and claim their file
extensions; modes and extensions already owned by another plugin are protected.
`Host.Subscribe` registers synchronous lifecycle handlers for `file.open`,
`file.before-save`, `file.after-save`, and `file.close`. Handlers receive a
`plugin.Event` containing the event name and file path. A before-save handler
may replace the event's document through the host; returning an error cancels
the write. Event names use the `namespace.event-name` form.
`Host.OpenTextEditor` opens an ephemeral, fully editable buffer whose save
callback can validate and consume multi-line content. A successful submission
closes the buffer; an error leaves it open for correction.
`Host.RegisterStatus` adds a right-aligned status-bar section. It accepts static
text or a refresh callback and interval; callbacks run in the background. Empty
text hides the section, which lets a provider disappear when it has nothing to
report. The Git plugin uses this API to publish `⎇ branch`. Running `git.commit`
without arguments opens `COMMIT_EDITMSG`; saving that buffer commits its full
multi-line contents.
The Git plugin in `plugins/git` owns Git subprocess and repository logic. The Go
plugin in `plugins/golang` owns Go mode, `gopls`, Delve, and the `go.*` commands.
The Search plugin in `plugins/search` owns workspace scanning, query history,
retained results, document overlays, and refresh cancellation. Each plugin
instance keeps its own history and uses the same persistent view API as Git Stage.

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
- `plugins/search` — persistent workspace search, scanning, and result navigation.
- `editor` — commands, palettes, multi-cursor edits, generic views and sidebars,
  extensions, mode-scoped LSP/DAP sessions, hooks, and the VS Dark 2026 theme. Each editor
  buffer owns its syntax cache, diagnostics, breakpoints, mode, and terminal
  state rather than storing those in path-keyed application maps.

The built-in highlighter is intentionally lightweight. The `syntax.Highlighter`
boundary is where a Tree-sitter-backed parser can be installed without coupling
the buffer or UI to a C dependency.

## Embedding and API tests

`editor.New` and `NewWithSettings` accept a `TerminalSession` size provider;
`terminal.Session` supplies it for a real terminal. An embedding host can call
`Execute`, `HandleEvent`, `Update`, and `Render` from its own event loop, then
`Close` to release all mode servers, debug adapters, and terminal shells. Calls
must be serialized. `Run` provides the built-in loop and closes services on exit.

All test files use external `_test` packages. Editor tests drive the public
host and event APIs, inspect document snapshots and terminal output, and use
subprocesses speaking LSP and DAP to check service isolation and wire messages.
