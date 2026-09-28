// Package syntax provides incremental, line-oriented source highlighting.
package syntax

import (
	"path/filepath"
	"strings"
	"unicode"
)

// Kind classifies highlighted source text.
type Kind uint8

const (
	Plain Kind = iota
	Comment
	Keyword
	String
	Number
	Type
	Import
	Declaration
	Function
	Delimiter
	Delimiter2
	Delimiter3
	Constant
	Added
	Removed
)

// Span assigns a syntax kind to a half-open rune range.
type Span struct {
	Start int
	End   int
	Kind  Kind
}

type lineState struct {
	inBlockComment bool
	inImportBlock  bool
	delimiterDepth int
}

type lineResult struct {
	text          string
	start         lineState
	end           lineState
	spans         []Span
	declaredTypes []string
}

// Highlighter caches parser state per line and reparses only invalidated tails.
type Highlighter struct {
	language   string
	lines      []lineResult
	knownTypes map[string]bool
}

// New returns a highlighter selected from a file name.
func New(path string) *Highlighter {
	return &Highlighter{language: languageForPath(path)}
}

// NewLanguage returns a highlighter for an explicitly selected language.
func NewLanguage(language string) *Highlighter {
	language = strings.TrimSpace(strings.ToLower(language))
	if language == "" {
		language = "plain"
	}
	return &Highlighter{language: language}
}

// Invalidate discards parser state at and after line.
func (h *Highlighter) Invalidate(line int) {
	if line < 0 {
		line = 0
	}
	if line < len(h.lines) {
		h.truncate(line)
	}
}

// Highlight returns syntax spans for a line. Calls should normally be in line order.
func (h *Highlighter) Highlight(line int, text string) []Span {
	if line < len(h.lines) && h.lines[line].text == text {
		return h.lines[line].spans
	}
	if line < len(h.lines) {
		h.truncate(line)
	}
	for len(h.lines) <= line {
		start := lineState{}
		if len(h.lines) > 0 {
			start = h.lines[len(h.lines)-1].end
		}
		current := ""
		if len(h.lines) == line {
			current = text
		}
		result := h.parseLine(current, start)
		h.lines = append(h.lines, result)
	}
	return h.lines[line].spans
}

func (h *Highlighter) parseLine(text string, start lineState) lineResult {
	runes := []rune(text)
	result := lineResult{text: text, start: start, end: start}
	importLine := start.inImportBlock
	previousWord := ""
	if h.language == "diff" {
		result.spans = diffSpans(runes)
		return result
	}
	for i := 0; i < len(runes); {
		if result.end.inBlockComment {
			end := findPair(runes, i, '*', '/')
			if end < 0 {
				result.spans = append(result.spans, Span{Start: i, End: len(runes), Kind: Comment})
				break
			}
			result.spans = append(result.spans, Span{Start: i, End: end + 2, Kind: Comment})
			result.end.inBlockComment = false
			i = end + 2
			continue
		}
		if startsPair(runes, i, '/', '/') || (h.language == "shell" && runes[i] == '#') {
			result.spans = append(result.spans, Span{Start: i, End: len(runes), Kind: Comment})
			break
		}
		if startsPair(runes, i, '/', '*') {
			end := findPair(runes, i+2, '*', '/')
			if end < 0 {
				result.spans = append(result.spans, Span{Start: i, End: len(runes), Kind: Comment})
				result.end.inBlockComment = true
				break
			}
			result.spans = append(result.spans, Span{Start: i, End: end + 2, Kind: Comment})
			i = end + 2
			continue
		}
		if runes[i] == '"' || runes[i] == '\'' || runes[i] == '`' {
			end := stringEnd(runes, i, runes[i])
			kind := String
			if importLine {
				kind = Import
			}
			result.spans = append(result.spans, Span{Start: i, End: end, Kind: kind})
			i = end
			continue
		}
		if strings.ContainsRune("()[]{}", runes[i]) {
			closing := strings.ContainsRune(")]}", runes[i])
			if closing && result.end.delimiterDepth > 0 {
				result.end.delimiterDepth--
			}
			kind := delimiterKind(result.end.delimiterDepth)
			result.spans = append(result.spans, Span{Start: i, End: i + 1, Kind: kind})
			if !closing {
				result.end.delimiterDepth++
			}
			if h.language == "go" && runes[i] == ')' && result.end.inImportBlock {
				result.end.inImportBlock = false
				importLine = false
			}
			i++
			continue
		}
		if unicode.IsDigit(runes[i]) {
			end := i + 1
			for end < len(runes) && (unicode.IsDigit(runes[end]) || strings.ContainsRune("._xabcdefABCDEF", runes[end])) {
				end++
			}
			result.spans = append(result.spans, Span{Start: i, End: end, Kind: Number})
			i = end
			continue
		}
		if unicode.IsLetter(runes[i]) || runes[i] == '_' {
			end := i + 1
			for end < len(runes) && (unicode.IsLetter(runes[end]) || unicode.IsDigit(runes[end]) || runes[end] == '_') {
				end++
			}
			word := string(runes[i:end])
			next := nextNonSpace(runes, end)
			importKeyword := isImportKeyword(h.language, word)
			kind := h.classifyWord(word, previousWord, next, runes, i)
			if kind != Plain {
				result.spans = append(result.spans, Span{Start: i, End: end, Kind: kind})
			}
			if importKeyword {
				importLine = true
				if h.language == "go" && next == '(' {
					result.end.inImportBlock = true
				}
			}
			if isDeclaredTypeName(h.language, previousWord) {
				result.declaredTypes = append(result.declaredTypes, word)
				if h.knownTypes == nil {
					h.knownTypes = make(map[string]bool)
				}
				h.knownTypes[word] = true
			}
			previousWord = word
			i = end
			continue
		}
		i++
	}
	return result
}

func (h *Highlighter) truncate(line int) {
	h.lines = h.lines[:line]
	h.knownTypes = make(map[string]bool)
	for _, result := range h.lines {
		for _, name := range result.declaredTypes {
			h.knownTypes[name] = true
		}
	}
}

func (h *Highlighter) classifyWord(word, previousWord string, next rune, line []rune, start int) Kind {
	if isImportKeyword(h.language, word) {
		return Declaration
	}
	if isDeclarationKeyword(h.language, word) {
		return Declaration
	}
	if isConstant(h.language, word) {
		return Constant
	}
	if isKeyword(h.language, word) {
		return Keyword
	}
	if isBuiltinType(h.language, word) || isDeclaredTypeName(h.language, previousWord) || h.knownTypes[word] {
		return Type
	}
	if next == '(' {
		return Function
	}
	if start > 0 && line[start-1] == '*' {
		return Type
	}
	first := firstRune(word)
	if first != 0 && unicode.IsUpper(first) {
		return Type
	}
	return Plain
}

func delimiterKind(depth int) Kind {
	switch depth % 3 {
	case 1:
		return Delimiter2
	case 2:
		return Delimiter3
	default:
		return Delimiter
	}
}

func nextNonSpace(line []rune, start int) rune {
	for _, current := range line[start:] {
		if !unicode.IsSpace(current) {
			return current
		}
	}
	return 0
}

func firstRune(value string) rune {
	for _, current := range value {
		return current
	}
	return 0
}

func languageForPath(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".js", ".jsx", ".ts", ".tsx":
		return "javascript"
	case ".py":
		return "python"
	case ".sh", ".bash", ".zsh":
		return "shell"
	case ".c", ".h", ".cc", ".cpp", ".hpp":
		return "c"
	case ".diff", ".patch":
		return "diff"
	default:
		return "plain"
	}
}

func diffSpans(line []rune) []Span {
	if len(line) == 0 {
		return nil
	}
	kind := Plain
	text := string(line)
	switch {
	case strings.HasPrefix(text, "+++") || strings.HasPrefix(text, "---") || strings.HasPrefix(text, "diff "):
		kind = Type
	case line[0] == '+':
		kind = Added
	case line[0] == '-':
		kind = Removed
	case strings.HasPrefix(text, "@@"):
		kind = Keyword
	}
	if kind == Plain {
		return nil
	}
	return []Span{{Start: 0, End: len(line), Kind: kind}}
}

func isKeyword(language, word string) bool {
	keywords := map[string]string{
		"go":         "break case chan const continue default defer else fallthrough for func go goto if import interface map package range return select struct switch type var",
		"rust":       "as async await break const continue crate dyn else enum extern false fn for if impl in let loop match mod move mut pub ref return self Self static struct super trait true type unsafe use where while",
		"javascript": "async await break case catch class const continue debugger default delete do else export extends false finally for from function get if import in instanceof let new null of return set static super switch this throw true try typeof undefined var void while with yield",
		"python":     "and as assert async await break class continue def del elif else except False finally for from global if import in is lambda None nonlocal not or pass raise return True try while with yield",
		"shell":      "case do done elif else esac export fi for function if in local readonly then while",
		"c":          "auto break case char const continue default do double else enum extern float for goto if inline int long register restrict return short signed sizeof static struct switch typedef union unsigned void volatile while",
	}
	return strings.Contains(" "+keywords[language]+" ", " "+word+" ")
}

func isImportKeyword(language, word string) bool {
	keywords := map[string]string{
		"go":         "import",
		"rust":       "crate extern mod use",
		"javascript": "export from import",
		"python":     "from import",
	}
	return strings.Contains(" "+keywords[language]+" ", " "+word+" ")
}

func isDeclarationKeyword(language, word string) bool {
	keywords := map[string]string{
		"go":         "const func interface package struct type var",
		"rust":       "const enum fn let static struct trait type",
		"javascript": "class const function interface let type var",
		"python":     "class def",
		"c":          "enum struct typedef union",
	}
	return strings.Contains(" "+keywords[language]+" ", " "+word+" ")
}

func isConstant(language, word string) bool {
	constants := map[string]string{
		"go":         "false iota nil true",
		"rust":       "false true",
		"javascript": "false null true undefined",
		"python":     "False None True",
		"c":          "NULL false true",
	}
	return strings.Contains(" "+constants[language]+" ", " "+word+" ")
}

func isDeclaredTypeName(language, previousWord string) bool {
	keywords := map[string]string{
		"go":         "type",
		"rust":       "enum struct trait type",
		"javascript": "class interface type",
		"python":     "class",
		"c":          "enum struct typedef union",
	}
	return strings.Contains(" "+keywords[language]+" ", " "+previousWord+" ")
}

func isBuiltinType(language, word string) bool {
	types := map[string]string{
		"go":         "any bool byte comparable complex64 complex128 error float32 float64 int int8 int16 int32 int64 rune string uint uint8 uint16 uint32 uint64 uintptr",
		"rust":       "bool char f32 f64 i8 i16 i32 i64 i128 isize str u8 u16 u32 u64 u128 usize",
		"javascript": "Array BigInt Boolean Function Number Object Promise String Symbol",
		"python":     "bool bytes dict float int list object set str tuple",
		"c":          "bool char double float int long short signed size_t unsigned void",
	}
	return strings.Contains(" "+types[language]+" ", " "+word+" ")
}

func startsPair(runes []rune, index int, first, second rune) bool {
	return index+1 < len(runes) && runes[index] == first && runes[index+1] == second
}

func findPair(runes []rune, start int, first, second rune) int {
	for i := start; i+1 < len(runes); i++ {
		if runes[i] == first && runes[i+1] == second {
			return i
		}
	}
	return -1
}

func stringEnd(runes []rune, start int, quote rune) int {
	escaped := false
	for i := start + 1; i < len(runes); i++ {
		if runes[i] == quote && !escaped {
			return i + 1
		}
		if quote != '`' && runes[i] == '\\' && !escaped {
			escaped = true
		} else {
			escaped = false
		}
	}
	return len(runes)
}
