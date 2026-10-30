package indexer

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Chunk is a piece of a file: its text, the lines it spans (1-based, End
// included) and the symbol it starts with ("" when none).
type Chunk struct {
	Text       string
	Start, End int
	Symbol     string
}

// declarations find where a top-level declaration starts, by language,
// the name it declares in the last group.
var declarations = map[string]*regexp.Regexp{
	".go":     regexp.MustCompile(`^(?:func|type|var|const)\s+(?:\([^)]*\)\s*)?([A-Za-z_][\w]*)`),
	".py":     regexp.MustCompile(`^(?:async\s+)?(?:def|class)\s+([A-Za-z_]\w*)`),
	".js":     jsDecl,
	".jsx":    jsDecl,
	".ts":     jsDecl,
	".tsx":    jsDecl,
	".vue":    jsDecl,
	".svelte": jsDecl,
	".rs":     regexp.MustCompile(`^(?:pub(?:\([\w:]+\))?\s+)?(?:async\s+)?(?:fn|struct|enum|trait|impl|mod|type|const|static)\s+(?:<[^>]*>\s*)?([A-Za-z_]\w*)`),
	".java":   classDecl,
	".kt":     classDecl,
	".swift":  classDecl,
	".cs":     classDecl,
	".php":    classDecl,
	".rb":     regexp.MustCompile(`^(?:def|class|module)\s+([\w.:?!]+)`),
	".md":     regexp.MustCompile(`^#{1,3}\s+(.+)`),
	".proto":  regexp.MustCompile(`^(?:message|service|enum)\s+(\w+)`),
	".sql":    regexp.MustCompile(`(?i)^(?:create|alter)\s+(?:table|view|index|function)\s+(?:if\s+not\s+exists\s+)?([\w.]+)`),
}

var (
	jsDecl    = regexp.MustCompile(`^(?:export\s+)?(?:default\s+)?(?:async\s+)?(?:function\*?|class|interface|type|enum|const|let)\s+([A-Za-z_$][\w$]*)`)
	classDecl = regexp.MustCompile(`^(?:(?:public|private|protected|internal|static|final|abstract|open|override|data|sealed|export)\s+)*(?:class|interface|enum|struct|record|fun|func|function|object|protocol|extension)\s+([A-Za-z_]\w*)`)
	comment   = regexp.MustCompile(`^\s*(?://|#|/\*|\*|--|"""|///)`)
)

// chunkCode cuts file path's text into chunks of at most maxChars, at its
// top-level declarations (with the comments right above them), several
// small ones together; a declaration too long is cut at blank lines, then
// at lines. A language enowX knows no declarations of is cut at blank
// lines.
func chunkCode(text, path string, maxChars int) []Chunk {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	decl := declarations[strings.ToLower(filepath.Ext(path))]
	// Segments: line spans starting at a declaration (or a blank line when
	// the language has none), comments above it pulled in.
	type seg struct {
		start, end int // 0-based, end excluded
		symbol     string
	}
	var segs []seg
	cur := seg{}
	begin := func(i int, symbol string) {
		if i > cur.start {
			cur.end = i
			segs = append(segs, cur)
		}
		cur = seg{start: i, symbol: symbol}
	}
	for i, line := range lines {
		if decl != nil {
			if m := decl.FindStringSubmatch(line); m != nil {
				at := i
				for at > cur.start && comment.MatchString(lines[at-1]) {
					at--
				}
				begin(at, strings.TrimSpace(m[len(m)-1]))
			}
		} else if strings.TrimSpace(line) == "" && i > cur.start {
			begin(i, "")
		}
	}
	cur.end = len(lines)
	segs = append(segs, cur)

	size := func(s, e int) int {
		n := 0
		for _, l := range lines[s:e] {
			n += len(l) + 1
		}
		return n
	}
	var out []Chunk
	emit := func(s, e int, symbol string) {
		body := strings.Join(lines[s:e], "\n")
		if strings.TrimSpace(body) == "" {
			return
		}
		out = append(out, Chunk{Text: body, Start: s + 1, End: e, Symbol: symbol})
	}
	// Pack small segments together; cut long ones.
	packS, packE, packSym := -1, -1, ""
	flush := func() {
		if packS >= 0 {
			emit(packS, packE, packSym)
			packS, packE, packSym = -1, -1, ""
		}
	}
	for _, sg := range segs {
		n := size(sg.start, sg.end)
		if n > maxChars {
			flush()
			for _, c := range cutLong(lines, sg.start, sg.end, maxChars) {
				emit(c[0], c[1], sg.symbol)
			}
			continue
		}
		if packS >= 0 && size(packS, sg.end) > maxChars {
			flush()
		}
		if packS < 0 {
			packS, packSym = sg.start, sg.symbol
		} else if packSym == "" {
			packSym = sg.symbol
		}
		packE = sg.end
	}
	flush()
	return out
}

// cutLong cuts lines[s:e] into spans of at most maxChars, at blank lines
// when it can, else at a line; a single line longer is cut alone.
func cutLong(lines []string, s, e, maxChars int) [][2]int {
	var out [][2]int
	start, n, lastBlank := s, 0, -1
	for i := s; i < e; i++ {
		l := len(lines[i]) + 1
		if n+l > maxChars && i > start {
			cut := i
			if lastBlank > start {
				cut = lastBlank
			}
			out = append(out, [2]int{start, cut})
			start, n, lastBlank = cut, size(lines[cut:i]), -1
		}
		n += l
		if strings.TrimSpace(lines[i]) == "" {
			lastBlank = i
		}
	}
	if start < e {
		out = append(out, [2]int{start, e})
	}
	return out
}

func size(lines []string) int {
	n := 0
	for _, l := range lines {
		n += len(l) + 1
	}
	return n
}

// secretName finds files that hold secrets, never indexed.
var secretName = regexp.MustCompile(`(?i)(^|/)(\.env(\..*)?|\.envrc|\.npmrc|\.netrc|\.pypirc|\.git-credentials|id_(rsa|dsa|ecdsa|ed25519)(\.pub)?|.*\.(pem|key|p12|pfx|jks|keystore|crt|cer|der|kdbx|ovpn)|credentials(\.\w+)?|.*secrets?\.(json|ya?ml|toml|env)|service[-_]account.*\.json)$`)

// secretText finds text that holds a secret: a private key, a cloud key.
var secretText = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----|\bAKIA[0-9A-Z]{16}\b|\bgh[pousr]_[A-Za-z0-9]{30,}\b|\bsk-[A-Za-z0-9_-]{20,}\b|\bxox[abpr]-[A-Za-z0-9-]{10,}\b`)

// IsSecret reports whether the file at path (relative, with slashes)
// holds secrets by its name.
func IsSecret(path string) bool { return secretName.MatchString(path) }

// gitFiles are the files git keeps or would keep in dir, its ignores
// honored, relative with slashes; false when dir is no repository.
func gitFiles(dir string) ([]string, bool) {
	out, err := exec.Command("git", "-C", dir, "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, false
	}
	var files []string
	for _, f := range bytes.Split(out, []byte{0}) {
		if len(f) > 0 {
			files = append(files, string(f))
		}
	}
	return files, true
}
