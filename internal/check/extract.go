// Package check finds the code that a piece of text talks about, such as a
// bug report, a pull request description or an AI's answer, and checks that
// it exists in a repository: functions, files, line numbers, quoted code,
// command-line flags and commits.
package check

import (
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// Kind is the kind of thing a reference points at.
type Kind string

const (
	Symbol  Kind = "symbol"  // a function, method, type, constant or variable
	File    Kind = "file"    // a file or folder, perhaps with a line number
	Snippet Kind = "snippet" // a block of quoted code
	Flag    Kind = "flag"    // a command-line option such as --insecure
	Commit  Kind = "commit"  // a git commit
)

// Ref is one piece of code mentioned in the text.
type Ref struct {
	Kind Kind     `json:"kind"`
	Text string   `json:"text"`           // as written
	Name string   `json:"name,omitempty"` // the symbol, flag or commit
	Path string   `json:"path,omitempty"` // the file
	Site string   `json:"site,omitempty"` // for a link to a file on GitHub: the repository, as owner/name
	Line int      `json:"line,omitempty"` // a line number in the file, if given
	End  int      `json:"end,omitempty"`  // the last line of a range
	Lang string   `json:"lang,omitempty"` // a code block's language
	At   int      `json:"at"`             // the line of the text that mentions it
	Near []string `json:"-"`              // for a file and line: the functions named in the same sentence
	Code string   `json:"-"`              // a code block's code
	Diff bool     `json:"-"`              // the code block is a patch
	Aim  string   `json:"-"`              // for a code block: "quote" of the repository, "other" code, or "" if unclear
	Cmd  string   `json:"-"`              // for a flag: the command it was given to, such as curl
	Idea bool     `json:"-"`              // named in a suggestion ("replace X with Y"), not claimed to exist
	// For a symbol written as a member, such as check.Parse or Repo::get:
	// what it is a member of.
	Qual []string `json:"-"`
	// For a symbol: the files named in the same sentence, which it is said
	// to be in.
	Files []string `json:"-"`
}

// Text is what Extract finds in a piece of text.
type Text struct {
	Refs   []Ref
	Blocks []Block
}

// Block is a fenced code block, kept so that names the writer made up in
// their own code are not taken for claims about the repository.
type Block struct {
	Lang    string
	Aim     string
	Shell   bool            // commands, such as gcc poc.c -o poc
	Words   map[string]bool // its identifiers and file names
	Defines map[string]bool // the functions, classes and variables it defines
}

// definitions finds names that a line of code defines: functions, classes,
// variables, macros and environment variables, in C, Python, JavaScript or shell.
var definitions = []*regexp.Regexp{
	regexp.MustCompile(`^\s*(?:static\s+|inline\s+|extern\s+|const\s+)*[A-Za-z_][\w\s\*]*?\b([A-Za-z_]\w*)\s*\([^;]*\)\s*\{?\s*$`),
	regexp.MustCompile(`^\s*(?:async\s+)?def\s+([A-Za-z_]\w*)\s*\(`),
	regexp.MustCompile(`^\s*class\s+([A-Za-z_]\w*)`),
	regexp.MustCompile(`\bfunction\s+([A-Za-z_$][\w$]*)`),
	regexp.MustCompile(`^\s*(?:const|let|var)\s+([A-Za-z_$][\w$]*)\s*=`),
	regexp.MustCompile(`^\s*(?:[A-Za-z_][\w]*[\s\*]+)+([A-Za-z_]\w*)\s*(?:\[[^\]]*\])?\s*(?:=|;)`),
	regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*=[^=]`),
	regexp.MustCompile(`^\s*#\s*define\s+([A-Za-z_]\w*)`),
	regexp.MustCompile(`^\s*export\s+([A-Za-z_]\w*)=`),
	regexp.MustCompile(`(?:^|\s)-o\s+([\w.-]+)`), // gcc poc.c -o poc
}

var (
	assignment  = regexp.MustCompile(`^[\w.\[\]]+\s*(?:=|\+=|-=)\s*\S`)
	declaration = regexp.MustCompile(`^(?:[A-Za-z_]\w*[\s\*]+)+[A-Za-z_]\w*\s*(?:\[[^\]]*\])?\s*=`)
	bareCall    = regexp.MustCompile(`^[\w.]+\(.*\)\s*;?\s*$`)
	pyImport    = regexp.MustCompile(`^from\s+\S+\s+import\b`)
	pyControl   = regexp.MustCompile(`^(?:if|elif|else|for|while|try|except|with|finally)\b.*:$`)
)

// codeLine reports whether a line outside a code block is code or a command
// all the same, as in reports that paste code without fences.
func codeLine(line string) bool {
	t := strings.TrimSpace(line)
	if i := strings.Index(t, " //"); i > 0 {
		t = strings.TrimSpace(t[:i])
	}
	if t == "" || strings.HasPrefix(t, "- ") || strings.HasPrefix(t, "* ") {
		return false
	}
	for _, p := range []string{"#include", "#define", "import ", "def ", "async def ", "class ", "return ", "int ", "char ",
		"char*", "void ", "struct ", "static ", "const ", "unsigned ", "size_t ", "bool ", "long ", "double ", "float ",
		"let ", "var ", "func ", "package ", "if (", "if(", "for (", "for(", "while (", "while(", "curl ", "echo ",
		"sudo ", "gcc ", "clang ", "./", "python3 ", "python ", "make ", "export ", "await ", "}"} {
		if strings.HasPrefix(t, p) {
			return true
		}
	}
	return strings.HasSuffix(t, ";") || strings.HasSuffix(t, "{") || pyImport.MatchString(t) || pyControl.MatchString(t) ||
		assignment.MatchString(t) || declaration.MatchString(t) || bareCall.MatchString(t)
}

var (
	extensions = `c|h|cc|cpp|cxx|hpp|hh|m|mm|go|py|pyi|js|jsx|mjs|cjs|ts|tsx|rs|java|kt|kts|scala|rb|php|cs|fs|swift|` +
		`sh|bash|zsh|ps1|pl|pm|lua|r|jl|ex|exs|erl|hs|ml|sql|yaml|yml|json|toml|ini|cfg|conf|xml|html|htm|css|scss|` +
		`md|rst|txt|cmake|mk|am|in|ac|proto|gradle|tf|hcl|vue|svelte|dart|zig|nim|v|sv|vhd|asm|s`
	// A file path, then perhaps :12, :12-30 or #L12-L30.
	filePath = regexp.MustCompile(`(?:^|[\s(\["'<,])((?:\.{1,2}/)?(?:[\w.@+-]+/)*(?:[\w@+-][\w.@+-]*\.(?:` + extensions +
		`)|Makefile|Dockerfile|CMakeLists\.txt))(?::(\d+)(?:-(\d+))?|#L(\d+)(?:-L(\d+))?)?\b`)
	lineAfter    = regexp.MustCompile(`^[\s,(]*(?:on |at )?(?:[Ll]ines?|L)\s*(\d+)(?:\s*(?:-|–|to)\s*(\d+))?`)
	lineBefore   = regexp.MustCompile(`(?:[Ll]ines?|L)\s*(\d+)(?:\s*(?:-|–|to)\s*(\d+))?\s+(?:of|in)\s+$`)
	githubBlob   = regexp.MustCompile(`https?://github\.com/([^/\s]+/[^/\s]+)/blob/[^/\s]+/([^\s#)>\]"'` + "`" + `]+)(?:#L(\d+)(?:-L(\d+))?)?`)
	codeSpan     = regexp.MustCompile("`([^`\n]+)`")
	callInProse  = regexp.MustCompile(`\b([A-Za-z_]\w*(?:(?:\.|::|->)[A-Za-z_]\w*)*)\(\)`)
	callWithArgs = regexp.MustCompile(`\b([A-Za-z_]\w*)\(([\w\s,*&.>\[\]"'-]+)\)`)
	snakeInProse = regexp.MustCompile(`\b[A-Za-z][A-Za-z0-9]*(?:_[A-Za-z0-9]+)+\b`)
	flagInText   = regexp.MustCompile(`(?:^|[\s"'(=])(--[a-z][a-z0-9]*(?:-[a-z0-9]+)*)\b`)
	commitInText = regexp.MustCompile(`(?i)\bcommit\s+([0-9a-f]{7,40})\b`)
	qualified    = regexp.MustCompile(`^[A-Za-z_]\w*(?:(?:\.|::|->)[A-Za-z_]\w*)*$`)
	callLike     = regexp.MustCompile(`^([A-Za-z_]\w*(?:(?:\.|::|->)[A-Za-z_]\w*)*)\s*\(`)
	callAnywhere = regexp.MustCompile(`([A-Za-z_]\w*)\s*\(`)
	members      = regexp.MustCompile(`\.|::|->`)
	hexOnly      = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	digitsOnly   = regexp.MustCompile(`(?:^|_)\d+(?:_|$)`)
)

// products are names that look like files but are software, such as Node.js.
var products = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`node.js next.js nuxt.js vue.js express.js react.js angular.js ember.js backbone.js
		three.js d3.js chart.js nest.js solid.js alpine.js moment.js day.js knockout.js meteor.js gatsby.js p5.js pixi.js
		leaflet.js highlight.js prism.js popper.js socket.io deno.js bun.js svelte.js preact.js remix.js astro.js`) {
		products[w] = true
	}
}

// domains end host names, which look like a.b.
var domains = map[string]bool{"com": true, "org": true, "net": true, "edu": true, "gov": true, "info": true, "biz": true,
	"xyz": true, "localhost": true, "localdomain": true, "example": true, "onion": true, "arpa": true}

// notSymbols are words that are code but say nothing about a repository.
var notSymbols = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`true false null nil none undefined void int char long short float double bool
		string str byte bytes size_t ssize_t uint int32_t int64_t uint8_t uint32_t uint64_t unsigned signed const static
		struct enum union class def func function return if else for while do switch case break continue new delete
		this self super import from package public private protected try catch throw raise async await var let
		print printf main test tests todo fixme yes no ok err error sizeof`) {
		notSymbols[w] = true
	}
}

// Extract finds the references to code in text, which is usually Markdown.
func Extract(text string) Text {
	x := &extractor{index: map[string]int{}, loose: Block{Words: map[string]bool{}, Defines: map[string]bool{}}}
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	for i := 0; i < len(lines); i++ {
		fence, info, ok := openFence(lines[i])
		if !ok {
			x.prose(lines[i], i+1)
			continue
		}
		j := i + 1
		for j < len(lines) && !closesFence(lines[j], fence) {
			j++
		}
		x.block(info, lines[i+1:min(j, len(lines))], i+1)
		i = j
	}
	if len(x.loose.Words) > 0 {
		x.loose.Shell = true
		x.blocks = append(x.blocks, x.loose)
	}
	return Text{Refs: x.refs, Blocks: x.blocks}
}

type extractor struct {
	refs    []Ref
	blocks  []Block
	loose   Block          // code pasted without a fence
	heading string         // the last Markdown heading, lower-cased
	index   map[string]int // where each reference is in refs, so it is kept once
	recent  []string       // the last few lines of prose, for what a code block is for
	symbols []string       // the symbols named on the line being read
	named   []mention      // the symbols and files named on the line being read
}

// mention is a reference as one line words it.
type mention struct {
	ref  int
	text string
}

// add records a reference once, and returns where it is in refs.
func (x *extractor) add(r Ref) int {
	key := string(r.Kind) + "|" + r.Name + "|" + r.Site + "|" + r.Path + "|" + strconv.Itoa(r.Line) + "|" + r.Code
	if i, ok := x.index[key]; ok {
		return i
	}
	x.index[key] = len(x.refs)
	x.refs = append(x.refs, r)
	return len(x.refs) - 1
}

// openFence recognises the start of a fenced code block, ``` or ~~~.
func openFence(line string) (fence, info string, ok bool) {
	t := strings.TrimSpace(line)
	for _, c := range []string{"`", "~"} {
		n := len(t) - len(strings.TrimLeft(t, c))
		if n >= 3 {
			return t[:n], strings.TrimSpace(t[n:]), true
		}
	}
	return "", "", false
}

func closesFence(line, fence string) bool {
	t := strings.TrimSpace(line)
	return strings.HasPrefix(t, fence) && strings.Trim(t, fence[:1]) == ""
}

// notCode are the languages whose code blocks hold commands, output or
// prose; they are read like prose instead.
var notCode = map[string]bool{"sh": true, "bash": true, "shell": true, "console": true, "zsh": true, "powershell": true,
	"ps": true, "cmd": true, "bat": true, "text": true, "txt": true, "plain": true, "plaintext": true, "output": true,
	"log": true, "http": true, "markdown": true, "md": true}

// block handles a fenced code block. Code is checked line by line; commands
// and logs are searched for paths, functions and flags.
func (x *extractor) block(info string, code []string, at int) {
	lang := strings.ToLower(strings.Fields(info + " x")[0])
	if lang == "x" {
		lang = ""
	}
	b := Block{Lang: lang, Words: map[string]bool{}, Defines: map[string]bool{}}
	for _, line := range code {
		words(line, func(w string) { b.Words[w] = true })
		for _, d := range definitions {
			if m := d.FindStringSubmatch(line); m != nil {
				b.Defines[m[1]] = true
			}
		}
		for _, f := range filePath.FindAllStringSubmatch(" "+line, -1) {
			b.Words[path.Base(f[1])] = true
		}
	}
	switch {
	case lang == "diff" || lang == "patch":
		b.Aim = "quote"
		x.add(Ref{Kind: Snippet, Text: "code block", Lang: lang, At: at, Code: strings.Join(code, "\n"), Diff: true, Aim: b.Aim})
	case notCode[lang] || lang == "" && !looksLikeCode(code):
		b.Shell = commands[lang] || lang == ""
		for i, line := range code {
			x.prose(line, at+1+i)
		}
	default:
		b.Aim = aim(x.recent, x.heading, code)
		if b.Aim == "" && program(code) {
			b.Aim = "other" // a whole program, such as a proof of concept, is the writer's own
		}
		x.add(Ref{Kind: Snippet, Text: "code block", Lang: lang, At: at, Code: strings.Join(code, "\n"), Aim: b.Aim})
	}
	x.blocks = append(x.blocks, b)
}

// commands are the block languages that hold commands someone typed.
var commands = map[string]bool{"sh": true, "bash": true, "shell": true, "console": true, "zsh": true, "powershell": true,
	"ps": true, "cmd": true, "bat": true}

// program reports whether a code block is a whole program rather than an excerpt.
func program(code []string) bool {
	for _, l := range code {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "int main(") || strings.HasPrefix(t, "#include <") || strings.HasPrefix(t, "func main(") ||
			strings.HasPrefix(t, "if __name__") || strings.HasPrefix(t, "def main(") {
			return true
		}
	}
	return false
}

// Words before a code block that say what it is: code quoted from the
// repository, or the writer's own code, such as a fix or an exploit.
var (
	quoteWords = []string{"vulnerable code", "code location", "exact code", "current code", "existing code", "source code",
		"affected code", "the code", "snippet", "excerpt", "located", "implementation", "code:", "lines "}
	otherWords = []string{"fix", "instead", "replace", "recommend", "suggest", "patch", "mitigat", "alternative",
		"proof of concept", "poc", "exploit", "reproduc", "payload", "save ", "compile", "harness", "test case",
		"attacker", "malicious", "should be", "could be", "example", "remediat", "defen", "solution", "workaround",
		"harden", "need to", "you'll", "added", "modified", "updated version", "new version", "secure version", "safer"}
	// suggestions are words that make the names in a sentence proposals, not claims.
	suggestions = []string{"replace", "instead", "recommend", "suggest", "should use", "switch to", "migrate to",
		"consider using", "could use", "prefer "}
)

// aim reads the heading and the lines just before a code block, and the
// block's own comments, for what it is meant to be.
func aim(recent []string, heading string, code []string) string {
	quote, other := 0, 0
	for _, l := range code {
		t := strings.ToLower(strings.TrimSpace(l))
		if c := strings.Index(t, "//"); c >= 0 && (strings.Contains(t[c:], "added") || strings.Contains(t[c:], "fix") ||
			strings.Contains(t[c:], "new") || strings.Contains(t[c:], "changed") || strings.Contains(t[c:], "patched")) {
			other += 2 // "// Added global parameter": a modified version, not the original
		}
	}
	for _, l := range append(recent, heading) {
		for _, w := range quoteWords {
			if strings.Contains(l, w) {
				quote++
			}
		}
		for _, w := range otherWords {
			if strings.Contains(l, w) {
				other++
			}
		}
	}
	switch {
	case quote > other:
		return "quote"
	case other > quote:
		return "other"
	}
	return ""
}

// looksLikeCode guesses whether an unlabelled block is code rather than a
// shell session or its output.
func looksLikeCode(code []string) bool {
	score, shell := 0, 0
	for _, l := range code {
		t := strings.TrimSpace(l)
		for _, c := range []string{"$ ", "# ", "grep ", "sed ", "echo ", "curl ", "cat ", "gcc ", "make ", "./", "cd ", "export "} {
			if strings.HasPrefix(t, c) {
				shell++
				break
			}
		}
		switch {
		case strings.HasSuffix(t, ";"), strings.HasSuffix(t, "{"), t == "}", strings.HasPrefix(t, "#include"),
			strings.HasPrefix(t, "def "), strings.HasPrefix(t, "func "), strings.HasPrefix(t, "return "):
			score++
		}
	}
	return score >= 2 && score > shell
}

// prose finds references in a line of ordinary text.
func (x *extractor) prose(line string, at int) {
	if t := strings.ToLower(strings.TrimSpace(line)); strings.HasPrefix(t, "#") {
		x.heading = t
	}
	if t := strings.ToLower(strings.TrimSpace(line)); t != "" {
		x.recent = append(x.recent, t)
		if len(x.recent) > 2 {
			x.recent = x.recent[1:]
		}
	}
	x.symbols, x.named = nil, nil
	start := len(x.refs)
	if codeLine(line) {
		words(line, func(w string) { x.loose.Words[w] = true })
		for _, d := range definitions {
			if m := d.FindStringSubmatch(line); m != nil {
				x.loose.Defines[m[1]] = true
			}
		}
	}

	// Links to files on GitHub.
	for _, m := range githubBlob.FindAllStringSubmatch(line, -1) {
		p := strings.TrimRight(m[2], ".,;:!?") // the end of a sentence, not of the path
		if u, err := url.PathUnescape(p); err == nil {
			p = u // [trpc].ts, written %5Btrpc%5D.ts
		}
		i := x.add(Ref{Kind: File, Text: m[0], Path: p, Line: atoi(m[3]), End: atoi(m[4]), At: at, Site: m[1]})
		x.named = append(x.named, mention{i, m[0]})
	}
	line = githubBlob.ReplaceAllStringFunc(line, blank)

	// Files, with the line numbers written after or before them.
	plain := strings.ReplaceAll(line, "`", " ")
	for _, m := range filePath.FindAllStringSubmatchIndex(plain, -1) {
		if products[strings.ToLower(path.Base(plain[m[2]:m[3]]))] {
			continue // Node.js, not a file
		}
		r := Ref{Kind: File, Text: strings.TrimSpace(plain[m[2]:m[1]]), Path: plain[m[2]:m[3]], At: at}
		switch {
		case m[4] >= 0:
			r.Line, r.End = atoi(plain[m[4]:m[5]]), group(plain, m, 6)
		case m[8] >= 0:
			r.Line, r.End = atoi(plain[m[8]:m[9]]), group(plain, m, 10)
		default:
			if a := lineAfter.FindStringSubmatch(plain[m[1]:]); a != nil {
				r.Line, r.End = atoi(a[1]), atoi(a[2])
				r.Text += " " + strings.TrimSpace(strings.Trim(a[0], " ,("))
			} else if b := lineBefore.FindStringSubmatch(plain[:m[2]]); b != nil {
				r.Line, r.End = atoi(b[1]), atoi(b[2])
				r.Text = strings.TrimSpace(b[0]) + " " + r.Text
			}
		}
		x.named = append(x.named, mention{x.add(r), r.Path})
	}

	// Code spans: functions, names, flags and commits.
	for _, m := range codeSpan.FindAllStringSubmatch(line, -1) {
		x.span(strings.TrimSpace(m[1]), at)
	}

	// The rest of the line: foo_bar(), strcpy(dst, src), snake_case names,
	// --flags and "commit abc1234".
	rest := codeSpan.ReplaceAllStringFunc(line, blank)
	rest = filePath.ReplaceAllStringFunc(rest, blank)
	for _, m := range callInProse.FindAllStringSubmatch(rest, -1) {
		if codeish(m[1]) {
			x.symbol(m[0], m[1], at)
		}
	}
	for _, m := range callWithArgs.FindAllStringSubmatch(rest, -1) {
		x.symbol(m[0], m[1], at)
	}
	for _, m := range snakeInProse.FindAllString(callInProse.ReplaceAllStringFunc(rest, blank), -1) {
		if !digitsOnly.MatchString(m) {
			x.symbol(m, m, at)
		}
	}
	for _, m := range flagInText.FindAllStringSubmatch(rest, -1) {
		x.add(Ref{Kind: Flag, Text: m[1], Name: m[1], At: at, Cmd: command(line)})
	}
	for _, m := range commitInText.FindAllStringSubmatch(rest, -1) {
		x.add(Ref{Kind: Commit, Text: m[0], Name: m[1], At: at})
	}

	// A line number is a claim about what is there: remember the functions
	// named alongside it. A suggestion names things that need not exist yet.
	for i := start; i < len(x.refs); i++ {
		if x.refs[i].Kind == File && x.refs[i].Line > 0 {
			x.refs[i].Near = x.symbols
		}
	}

	// Sentence by sentence: a name in the same sentence as a file, as in
	// "Parse in parse.go", is said to be in that file. A name that only a
	// suggestion mentions, as in "replace X with Y", need not exist yet.
	claimed := map[int]bool{}
	for _, s := range sentences(line) {
		lowered, idea := strings.ToLower(s), false
		for _, w := range suggestions {
			idea = idea || strings.Contains(lowered, w)
		}
		var files []string
		for _, m := range x.named {
			if x.refs[m.ref].Kind == File && strings.Contains(s, m.text) {
				files = append(files, x.refs[m.ref].Path)
			}
		}
		for _, m := range x.named {
			r := &x.refs[m.ref]
			if r.Kind != Symbol || !strings.Contains(s, m.text) && !hasWord(s, r.Name) {
				continue
			}
			for _, f := range files {
				if !slices.Contains(r.Files, f) {
					r.Files = append(r.Files, f)
				}
			}
			if m.ref >= start {
				claimed[m.ref] = claimed[m.ref] || !idea
				r.Idea = !claimed[m.ref]
			}
		}
	}
}

// hasWord reports whether s has name as a whole word.
func hasWord(s, name string) bool {
	for i := 0; ; {
		j := strings.Index(s[i:], name)
		if j < 0 {
			return false
		}
		j += i
		end := j + len(name)
		if (j == 0 || !identChar(s[j-1])) && (end == len(s) || !identChar(s[end])) {
			return true
		}
		i = j + 1
	}
}

func identChar(c byte) bool { return identStart(c) || c >= '0' && c <= '9' }

// sentences splits a line of prose where its sentences end, keeping code
// spans whole.
func sentences(line string) []string {
	var out []string
	start, code := 0, false
	for i := 0; i < len(line); i++ {
		switch c := line[i]; {
		case c == '`':
			code = !code
		case !code && (c == '.' || c == '!' || c == '?') && i+2 < len(line) && line[i+1] == ' ' &&
			(line[i+2] >= 'A' && line[i+2] <= 'Z' || line[i+2] == '`' || line[i+2] == '"'):
			out = append(out, line[start:i+1])
			start = i + 2
		}
	}
	return append(out, line[start:])
}

// span classifies the contents of one `code span`. Paths in spans are found
// with the rest of the line, so that a line number after the span counts.
func (x *extractor) span(s string, at int) {
	switch {
	case !strings.ContainsAny(s, " \t") && filePath.MatchString(" "+s):
		// A path, read with the prose; a command that names a file falls through.
	case strings.HasPrefix(s, "--") && flagInText.MatchString(s):
		x.add(Ref{Kind: Flag, Text: s, Name: flagInText.FindStringSubmatch(s)[1], At: at})
	case hexOnly.MatchString(s) && strings.ContainsAny(s, "0123456789") && strings.ContainsAny(s, "abcdef"):
		x.add(Ref{Kind: Commit, Text: s, Name: s, At: at})
	case callLike.MatchString(s):
		x.symbol(s, callLike.FindStringSubmatch(s)[1], at)
	case qualified.MatchString(s):
		x.symbol(s, s, at)
	default:
		// An expression or a command: take the functions it calls and the flags it passes.
		for _, m := range callAnywhere.FindAllStringSubmatch(s, -1) {
			x.symbol(m[1]+"()", m[1], at)
		}
		for _, m := range flagInText.FindAllStringSubmatch(s, -1) {
			x.add(Ref{Kind: Flag, Text: m[1], Name: m[1], At: at, Cmd: command(s)})
		}
	}
}

// command returns the program a command line runs, such as curl in
// "$ sudo /usr/bin/curl --insecure https://example.com".
func command(line string) string {
	for _, w := range strings.Fields(line) {
		switch {
		case w == "$" || w == ">" || w == "#" || w == "sudo" || w == "env" || strings.Contains(w, "="):
			continue
		case strings.HasPrefix(w, "-"):
			return ""
		}
		return path.Base(strings.Trim(w, "`'\"$"))
	}
	return ""
}

// symbol records a function or name. Of a.b, A::b or a->b it keeps b, and
// what b is a member of.
func (x *extractor) symbol(text, name string, at int) {
	parts := members.Split(name, -1)
	name = parts[len(parts)-1]
	switch {
	case len(name) < 3, len(parts) == 1 && notSymbols[strings.ToLower(name)]:
		return // too short to tell, or a keyword; store.New is code, but new is a keyword
	case len(parts) > 1 && domains[strings.ToLower(name)]:
		return // evil.com is a host, not code
	}
	x.symbols = append(x.symbols, name)
	i := x.add(Ref{Kind: Symbol, Text: text, Name: name, At: at})
	if len(parts) > 1 && len(x.refs[i].Qual) == 0 {
		x.refs[i].Qual = parts[:len(parts)-1]
	}
	x.named = append(x.named, mention{i, text})
}

// codeish reports whether a word looks like code rather than English: it has
// an underscore, a capital after a lower-case letter, or a qualifier.
func codeish(s string) bool {
	if strings.ContainsAny(s, "_.:>") {
		return true
	}
	for i := 1; i < len(s); i++ {
		if s[i] >= 'A' && s[i] <= 'Z' && s[i-1] >= 'a' && s[i-1] <= 'z' {
			return true
		}
	}
	return false
}

func blank(s string) string { return strings.Repeat(" ", len(s)) }

func group(s string, m []int, i int) int {
	if m[i] < 0 {
		return 0
	}
	return atoi(s[m[i]:m[i+1]])
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
