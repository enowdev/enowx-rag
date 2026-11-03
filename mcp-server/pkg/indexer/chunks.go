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
	// attached are the lines that belong to the declaration below them:
	// comments, and decorators / annotations (@Override, @property, ...).
	attached = regexp.MustCompile(`^\s*(?://|#|/\*|\*|--|"""|///|@)`)
)

// members find, by language, the declarations nested in a type: methods,
// nested classes. They match indented lines only, so never a top-level
// declaration. A declaration too long for one chunk (a big class) is split at
// its members before falling back to blank lines. Languages not listed (Go:
// its methods are top-level already) fall back to blank lines.
var members = map[string]*regexp.Regexp{
	".py":    regexp.MustCompile(`^\s+(?:async\s+)?(?:def|class)\s+([A-Za-z_]\w*)`),
	".rs":    regexp.MustCompile(`^\s+(?:pub(?:\([\w:]+\))?\s+)?(?:(?:async|const|unsafe|extern\s+"\w+")\s+)*fn\s+([A-Za-z_]\w*)`),
	".rb":    regexp.MustCompile(`^\s+def\s+([\w.?!=]+)`),
	".java":  javaMember,
	".cs":    javaMember,
	".kt":    regexp.MustCompile(`^\s+(?:(?:public|private|protected|internal|override|open|abstract|final|suspend|inline|operator|infix|tailrec)\s+)*fun\s+(?:<[^>]*>\s*)?(?:[\w.]+\.)?([A-Za-z_]\w*)`),
	".swift": regexp.MustCompile(`^\s+(?:(?:public|private|fileprivate|internal|open|static|class|override|final|mutating|nonmutating|@\w+)\s+)*func\s+([A-Za-z_]\w*)`),
	".php":   regexp.MustCompile(`^\s+(?:(?:public|private|protected|static|final|abstract)\s+)*function\s+&?([A-Za-z_]\w*)`),
	".js":    jsMember,
	".jsx":   jsMember,
	".ts":    jsMember,
	".tsx":   jsMember,
}

var (
	// javaMember: a method or constructor with at least one modifier, so a
	// call inside a method body ("foo(x);", "return bar(y)") never matches.
	javaMember = regexp.MustCompile(`^\s+(?:@\w+\s+)*(?:(?:public|private|protected|internal|static|final|abstract|override|virtual|async|synchronized|sealed|partial|extern|unsafe|default)\s+)+(?:<[^>]+>\s+)?(?:[\w<>\[\],.?]+\s+)?([A-Za-z_]\w*)\s*\(`)
	// jsMember: a class method whose signature fits on its line and opens its
	// body: "  async load(id: string): Promise<X> {". Control-flow keywords
	// that look the same ("  if (x) {") are filtered out by notMember.
	jsMember  = regexp.MustCompile(`^\s+(?:(?:public|private|protected|static|async|readonly|override|abstract|get|set|declare)\s+)*\*?\s*(#?[A-Za-z_$][\w$]*)\s*(?:<[^>]*>)?\s*\([^)]*\)\s*(?::\s*[^={;]+)?\{\s*$`)
	notMember = map[string]bool{"if": true, "for": true, "while": true, "switch": true, "catch": true,
		"function": true, "return": true, "with": true, "do": true, "else": true, "try": true}
)

// span is a range of lines (0-based, end excluded) and the symbol it holds.
type span struct {
	start, end int
	symbol     string
}

// chunkCode cuts file path's text into chunks of at most maxChars, at its
// top-level declarations (with the comments right above them). Small
// declarations are packed together with the one that follows; a declaration
// too long is split at its members (methods), then at blank lines, then at
// lines. A language enowX knows no declarations of is cut at blank lines.
func chunkCode(text, path string, maxChars int) []Chunk {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	ext := strings.ToLower(filepath.Ext(path))
	member := members[ext]
	long := func(sg span) []span { return splitLong(lines, sg, maxChars, member) }

	var out []Chunk
	for _, sp := range pack(lines, segments(lines, declarations[ext]), maxChars, long) {
		body := strings.Join(lines[sp.start:sp.end], "\n")
		if strings.TrimSpace(body) == "" {
			continue
		}
		out = append(out, Chunk{Text: body, Start: sp.start + 1, End: sp.end, Symbol: sp.symbol})
	}
	return out
}

// segments are the line spans starting at a top-level declaration (or at a
// blank line when the language has no declarations known), the comments
// above a declaration pulled in.
func segments(lines []string, decl *regexp.Regexp) []span {
	var segs []span
	cur := span{}
	begin := func(i int, symbol string) {
		if i > cur.start {
			cur.end = i
			segs = append(segs, cur)
		}
		cur = span{start: i, symbol: symbol}
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
	return append(segs, cur)
}

// pack groups contiguous segments into chunk spans of at most maxChars: a
// small segment (under a quarter of the size: an import block, a const, a
// one-line type) joins the small ones next to it and the segment that
// follows; a segment of fair size stands alone; one too long goes to long.
// Packing only small segments keeps boundaries local: adding a function
// changes the chunks around it, not every chunk after it, so the others keep
// their content (and their IDs: nothing to re-embed).
func pack(lines []string, segs []span, maxChars int, long func(span) []span) []span {
	small := maxChars / 4
	var out []span
	cur := span{start: -1}
	flush := func() {
		if cur.start >= 0 {
			out = append(out, cur)
			cur = span{start: -1}
		}
	}
	for _, sg := range segs {
		n := size(lines[sg.start:sg.end])
		switch {
		case n > maxChars:
			flush()
			out = append(out, long(sg)...)
		case n < small:
			if cur.start >= 0 && size(lines[cur.start:sg.end]) > maxChars {
				flush()
			}
			if cur.start < 0 {
				cur = sg
			} else {
				cur.end = sg.end
				if cur.symbol == "" {
					cur.symbol = sg.symbol
				}
			}
		default:
			if cur.start >= 0 && size(lines[cur.start:sg.end]) <= maxChars {
				sym := sg.symbol // the small lead-in joins this declaration
				if sym == "" {
					sym = cur.symbol
				}
				out = append(out, span{start: cur.start, end: sg.end, symbol: sym})
				cur = span{start: -1}
				continue
			}
			flush()
			out = append(out, sg)
		}
	}
	flush()
	return out
}

// splitLong splits a segment too long for one chunk at its members (the
// shallowest ones: a closure inside a method stays in the method), each with
// the comments and decorators above it, symbol "Outer.member"; pieces are
// packed like top-level segments, and a piece still too long is cut at blank
// lines, then at lines.
func splitLong(lines []string, sg span, maxChars int, member *regexp.Regexp) []span {
	byBlank := func(p span) []span {
		var out []span
		for _, c := range cutLong(lines, p.start, p.end, maxChars) {
			out = append(out, span{start: c[0], end: c[1], symbol: p.symbol})
		}
		return out
	}
	if member == nil {
		return byBlank(sg)
	}
	type hit struct {
		line, indent int
		name         string
	}
	var hits []hit
	shallowest := -1
	for i := sg.start + 1; i < sg.end; i++ {
		m := member.FindStringSubmatch(lines[i])
		if m == nil || notMember[m[len(m)-1]] {
			continue
		}
		ind := len(lines[i]) - len(strings.TrimLeft(lines[i], " \t"))
		hits = append(hits, hit{line: i, indent: ind, name: m[len(m)-1]})
		if shallowest < 0 || ind < shallowest {
			shallowest = ind
		}
	}
	pieces := []span{{start: sg.start, symbol: sg.symbol}}
	for _, h := range hits {
		if h.indent != shallowest {
			continue
		}
		last := &pieces[len(pieces)-1]
		at := h.line
		for at > last.start+1 && attached.MatchString(lines[at-1]) {
			at--
		}
		if at <= last.start {
			continue
		}
		last.end = at
		name := h.name
		if sg.symbol != "" {
			name = sg.symbol + "." + name
		}
		pieces = append(pieces, span{start: at, symbol: name})
	}
	pieces[len(pieces)-1].end = sg.end
	if len(pieces) == 1 {
		return byBlank(sg)
	}
	return pack(lines, pieces, maxChars, byBlank)
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
