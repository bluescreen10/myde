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
go build -o myde ./cmd/myde
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
| `Cmd-S` (macOS) / `C-s` (other OSes) | save |
| `Cmd-X/V/Z/Y` (macOS) / `C-x/v/z/y` (other OSes) | cut / paste / undo / redo |
| `Cmd-F` (macOS) / `C-f` (other OSes) | search in the current buffer |
| `C-x C-f`, `C-x C-s`, `C-x b`, `C-x k` | Emacs open/save/switch/close chords on macOS |
| `C-q` | quit; modified buffers offer Save All, Discard All, or Cancel |
| `C-1` … `C-9` | switch directly to buffer 1 … 9 |
| `Shift-Arrow` | extend the text selection |
| `Cmd-Left` / `Cmd-Right` | beginning / end of line on terminals that forward the Super modifier |
| `M-j` | add a cursor on the next line |
| `M-f` | open, focus, or hide the file explorer |
| `M-.` | go to definition through the active language server |
| `C-Space` | completion (LSP plus local words) |

Arrow, Home, End, Page Up, Page Down, Backspace, Delete, Enter, and Tab work in
the editing area. Shift with navigation extends a selection; typing replaces
selected text. Typing filters every command palette with fuzzy matching. `Escape`
cancels the current panel, prompt, key sequence, or action. Saving an unnamed
buffer opens its filename prompt in the minibuffer below the modeline.
Closing a modified file opens a Save, Discard, or Cancel confirmation; quitting
with modified buffers opens a Save All, Discard All, or Cancel confirmation.
Clean new buffers close immediately without being treated as modified.
Control-number switching uses the CSI-u keyboard protocol requested by myde;
`M-1` through `M-9` provide the same bindings on older terminals.

The file explorer is a navigable tree on the left. Type while it is focused to
filter paths, use Up/Down to select entries, Left/Right to collapse or expand
folders, and Enter to open a file and close the explorer. Escape clears the
filter, closes the explorer, and returns focus to the editor.

## Commands

Open the centered panel with `C-p`. It searches workspace files by default; put
`>` at the start of the query to search commands. The panel keeps the query,
results, and keyboard help in separate bordered regions. Important commands include:

- `search.project` — fast recursive search with ripgrep.
- `git.status`, `git.diff`, `git.diff staged`, `git.diff file`,
  `git.history`, `git.stage`, and `git.commit`.
- `lsp.start` — prompt for a language server command. Diagnostics appear inline,
  completion uses `textDocument/completion`, and `lsp.definition` navigates to
  definitions. In Go workspaces, `gopls` starts automatically when it is on
  `PATH`; completion opens with `C-Space` and updates while identifiers are
  typed. Completion suggestions do not capture text input: use Up/Down and
  Enter or Tab to accept one. Completion opens beside the text cursor and shows
  signature and documentation details supplied by the language server; servers
  such as `gopls` that resolve completion items are supported. LSP errors use
  red curly underlines and a tinted line background. Moving the cursor onto an
  error opens a diagnostic card with its source, code, and full message.
- `debug.start` — prompt for a DAP adapter command. Then use `debug.launch`,
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
hook open = git.status

# Keys use ctrl-/alt- names.
bind ctrl-g = git.status
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

# File buffers keep 1,000 undo entries by default. Zero disables history.
set history-limit = 2000
```

The placeholders `{file}` and `{root}` expand to the active file and workspace.
Unknown or invalid declarations leave the previous configuration active and show
an error in the status line.

## Architecture

- `buffer` — piece-table text storage, transactional undo/redo, rune-aware
  positions, atomic saving, and external-change detection.
- `terminal` — raw input, ANSI true-color rendering, and row-level frame diffing.
- `syntax` — incremental stateful highlighting for Go, Rust, JavaScript,
  TypeScript, Python, shell, and C/C++.
- `protocol` — Content-Length framed JSON transport, JSON-RPC/LSP process
  management, and Debug Adapter Protocol process management.
- `editor` — commands, palettes, multi-cursor edits, panes, extensions, LSP/DAP,
  Git, search, hooks, and the VS Dark 2026 theme.

The built-in highlighter is intentionally lightweight. The `syntax.Highlighter`
boundary is where a Tree-sitter-backed parser can be installed without coupling
the buffer or UI to a C dependency.
