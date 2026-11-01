package check

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// Repo is an index of the code in a repository.
type Repo struct {
	Root    string
	Git     bool                // whether Root is a git checkout, so commits can be checked
	Shallow bool                // whether the checkout lacks older history, as in CI
	files   map[string][]string // the lines of each file, by slash-separated path
	bases   map[string][]string // file name → paths
	folded  map[string][]string // file name in lower case → paths
	dirs    map[string]bool
	idents  map[string]*seen // identifiers in the code
	flags   map[string]*seen // --long-options anywhere in the files
	lines   map[string]*seen // lines of code, with their spacing evened out
	byChar  map[byte][]string
	langs   map[string]int  // code files per language
	main    map[string]bool // the languages the project is written in
	styles  map[string]bool // name prefixes typical of the project, such as curl_ and CURLOPT_
	// The modules, packages, folders and types the project defines, which
	// a name can be written as a member of, as check in check.Parse.
	declared map[string]bool
	used     map[string]bool // names the code takes members from, as cmd in cmd.Flags()
	name     string          // the project's name, from its folder
	remote   string          // the GitHub repository it was cloned from, as owner/name, if known
}

// common are name prefixes too ordinary to say a name belongs to a project.
var common = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`test tests get set init do is has my new old tmp temp str buf data file read write
		the to on max min num len use no sys free alloc open close send recv parse handle create make run main user
		multi http https ssl tls dns url api app lib util utils internal private public base error err log debug`) {
		common[w] = true
	}
}

// families maps file extensions and code block labels to languages.
var families = map[string]string{
	"c": "c", "h": "c", "cc": "c", "cpp": "c", "cxx": "c", "hpp": "c", "hh": "c", "c++": "c", "m": "c", "mm": "c", "objc": "c",
	"go": "go", "golang": "go", "py": "python", "pyi": "python", "python": "python", "python3": "python",
	"js": "js", "mjs": "js", "cjs": "js", "jsx": "js", "javascript": "js", "node": "js", "ts": "js", "tsx": "js", "typescript": "js",
	"rs": "rust", "rust": "rust", "java": "java", "kt": "kotlin", "kts": "kotlin", "kotlin": "kotlin", "scala": "scala",
	"rb": "ruby", "ruby": "ruby", "php": "php", "cs": "csharp", "csharp": "csharp", "c#": "csharp", "swift": "swift",
	"pl": "perl", "pm": "perl", "perl": "perl", "lua": "lua", "sql": "sql", "dart": "dart", "zig": "zig",
	"ex": "elixir", "exs": "elixir", "elixir": "elixir", "erl": "erlang", "hs": "haskell", "haskell": "haskell",
}

// scripts are languages people write throwaway files in, such as an exploit.py.
var scripts = map[string]bool{"python": true, "js": true, "ruby": true, "perl": true, "php": true, "go": true, "c": true}

// seen counts where a word or line appears.
type seen struct {
	count, files int
	first        string // path:line
	last         string // the last file it was counted in
}

func (s *seen) add(file string, line int) {
	s.count++
	if s.last != file {
		s.files++
		s.last = file
	}
	if s.first == "" {
		s.first = file + ":" + itoa(line)
	}
}

// skipDirs are folders that hold no code of the project's own.
var skipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
	"target": true, ".venv": true, "venv": true, "__pycache__": true, ".next": true}

// Open indexes the repository at root: every text file git tracks there, or
// every text file below it when it is not a git checkout.
func Open(root string) (*Repo, error) {
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	r := &Repo{Root: abs, files: map[string][]string{}, bases: map[string][]string{}, folded: map[string][]string{}, dirs: map[string]bool{},
		idents: map[string]*seen{}, flags: map[string]*seen{}, lines: map[string]*seen{}, byChar: map[byte][]string{},
		langs: map[string]int{}, main: map[string]bool{}, styles: map[string]bool{}, declared: map[string]bool{}, used: map[string]bool{},
		name: filepath.Base(abs)}
	r.declared[r.name] = true
	paths, git := tracked(abs)
	if !git {
		if paths, err = walk(abs); err != nil {
			return nil, err
		}
	}
	// Code first, so that where a name is first found is in the code, not
	// in a README that mentions it.
	sort.SliceStable(paths, func(i, j int) bool {
		return family(path.Base(paths[i])) != "" && family(path.Base(paths[j])) == ""
	})
	r.Git = git
	if git {
		out, _ := exec.Command("git", "-C", abs, "rev-parse", "--is-shallow-repository").Output()
		r.Shallow = strings.TrimSpace(string(out)) == "true"
		out, _ = exec.Command("git", "-C", abs, "remote", "get-url", "origin").Output()
		if m := githubRemote.FindStringSubmatch(strings.TrimSpace(string(out))); m != nil {
			r.remote = m[1]
		}
	}
	for _, p := range paths {
		data, err := os.ReadFile(filepath.Join(abs, filepath.FromSlash(p)))
		if err != nil || len(data) > 4<<20 || bytes.IndexByte(data[:min(len(data), 8000)], 0) >= 0 {
			continue // unreadable, huge or binary
		}
		r.index(p, string(data))
	}
	for id := range r.idents {
		c := lower(id[0])
		r.byChar[c] = append(r.byChar[c], id)
	}
	// The project's own name prefixes: its name, and the most used prefixes
	// that are not ordinary words.
	prefixes := map[string]int{}
	for id := range r.idents {
		if i := strings.Index(id, "_"); i >= 3 {
			prefixes[id[:i+1]]++
		}
	}
	type pc struct {
		p string
		n int
	}
	var ps []pc
	for p, n := range prefixes {
		if n >= 30 && !common[strings.ToLower(strings.TrimSuffix(p, "_"))] {
			ps = append(ps, pc{p, n})
		}
	}
	sort.Slice(ps, func(i, j int) bool { return ps[i].n > ps[j].n || ps[i].n == ps[j].n && ps[i].p < ps[j].p })
	for _, x := range ps[:min(8, len(ps))] {
		r.styles[x.p] = true
	}
	name := strings.ToLower(filepath.Base(abs))
	for p := range prefixes {
		if strings.ToLower(strings.TrimSuffix(p, "_")) == name {
			r.styles[p] = true
		}
	}
	// The project's languages are those with at least a sixth of its code files.
	total, top, topN := 0, "", 0
	for lang, n := range r.langs {
		total += n
		if n > topN || n == topN && lang < top {
			top, topN = lang, n
		}
	}
	for lang, n := range r.langs {
		if n*6 >= total {
			r.main[lang] = true
		}
	}
	if top != "" {
		r.main[top] = true
	}
	return r, nil
}

// family returns the language of a file or code block label, or "".
func family(name string) string {
	ext := strings.ToLower(name)
	if i := strings.LastIndex(ext, "."); i >= 0 {
		ext = ext[i+1:]
	}
	return families[ext]
}

// tracked lists the files under root that git tracks, or would if they were
// added (new files it does not ignore), if root is in a git checkout.
func tracked(root string) ([]string, bool) {
	out, err := exec.Command("git", "-C", root, "ls-files", "-z", "--cached", "--others", "--exclude-standard").Output()
	if err != nil {
		return nil, false
	}
	var paths []string
	for _, p := range strings.Split(string(out), "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, true
}

func walk(root string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		switch {
		case err != nil:
			return err
		case d.IsDir() && p != root && skipDirs[d.Name()]:
			return filepath.SkipDir
		case d.Type().IsRegular():
			rel, _ := filepath.Rel(root, p)
			paths = append(paths, filepath.ToSlash(rel))
		}
		return nil
	})
	return paths, err
}

func (r *Repo) index(file, text string) {
	// A final newline ends the last line; it does not start another.
	lines := strings.Split(strings.TrimSuffix(text, "\n"), "\n")
	r.files[file] = lines
	if lang := family(path.Base(file)); lang != "" {
		r.langs[lang]++
	}
	r.bases[path.Base(file)] = append(r.bases[path.Base(file)], file)
	r.folded[strings.ToLower(path.Base(file))] = append(r.folded[strings.ToLower(path.Base(file))], file)
	for d := path.Dir(file); d != "." && d != "/" && !r.dirs[d]; d = path.Dir(d) {
		r.dirs[d] = true
		r.declared[path.Base(d)] = true
	}
	code := family(path.Base(file)) != ""
	if code {
		r.declared[strings.TrimSuffix(path.Base(file), path.Ext(file))] = true
	}
	for i, line := range lines {
		n := i + 1
		words(line, func(w string) { note(r.idents, w, file, n) })
		for _, m := range flagInText.FindAllStringSubmatch(line, -1) {
			note(r.flags, m[1], file, n)
		}
		if l := even(line); l != "" {
			note(r.lines, l, file, n)
		}
		if code {
			if m := declares.FindStringSubmatch(line); m != nil {
				r.declared[m[1]] = true
			}
			roots(line, func(w string) { r.used[w] = true })
		}
	}
}

// githubRemote reads owner/name from a GitHub remote, over HTTPS or SSH.
var githubRemote = regexp.MustCompile(`github\.com[:/]([^/]+/[^/]+?)(?:\.git)?/?$`)

// declares finds the package, module or type a line of code defines, as in
// "type Repo struct", "export class Session" or "pub mod parser".
var declares = regexp.MustCompile(`^\s*(?:(?:export|default|declare|pub(?:\([\w:]+\))?|public|private|protected|internal|` +
	`abstract|final|sealed|static|data|case|open|partial)\s+)*(?:package|type|class|interface|enum|struct|union|trait|mod|` +
	`impl|namespace|module|object|record|protocol|extension)\s+([A-Za-z_]\w*)`)

func note(m map[string]*seen, key, file string, line int) {
	s := m[key]
	if s == nil {
		s = &seen{}
		m[key] = s
	}
	s.add(file, line)
}

// words calls f with each identifier in line.
func words(line string, f func(string)) {
	for i := 0; i < len(line); {
		if !identStart(line[i]) {
			i++
			continue
		}
		j := i + 1
		for j < len(line) && (identStart(line[j]) || line[j] >= '0' && line[j] <= '9') {
			j++
		}
		if i == 0 || !(line[i-1] >= '0' && line[i-1] <= '9') {
			f(line[i:j])
		}
		i = j
	}
}

// roots calls f with each name in line that a member is taken from, as cmd
// in cmd.Flags() or conn in conn->data, but not data.
func roots(line string, f func(string)) {
	for i := 0; i < len(line); {
		if !identStart(line[i]) {
			i++
			continue
		}
		j := i + 1
		for j < len(line) && (identStart(line[j]) || line[j] >= '0' && line[j] <= '9') {
			j++
		}
		rest := line[j:]
		after := strings.HasPrefix(rest, ".") && len(rest) > 1 && identStart(rest[1]) ||
			strings.HasPrefix(rest, "->") || strings.HasPrefix(rest, "::")
		before := i > 0 && (line[i-1] == '.' || line[i-1] == '>' || line[i-1] == ':' || line[i-1] >= '0' && line[i-1] <= '9')
		if after && !before {
			f(line[i:j])
		}
		i = j
	}
}

func identStart(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' }

// even trims a line of code and squeezes its spaces, so that code quoted with
// different indentation still matches. Lines too short or plain to tell
// anything apart, such as "}" or "break;", come back empty.
func even(line string) string {
	if i := strings.Index(line, " //"); i >= 0 {
		line = line[:i] // a comment at the end of the line, as in `strcpy(a, b); // VULNERABLE`
	}
	if i := strings.Index(line, "/*"); i > 0 && strings.HasSuffix(strings.TrimSpace(line), "*/") {
		line = line[:i]
	}
	l := strings.Join(strings.Fields(line), " ")
	if len(l) < 8 {
		return ""
	}
	letters := 0
	for i := 0; i < len(l); i++ {
		if identStart(l[i]) {
			letters++
		}
	}
	if letters < 4 {
		return ""
	}
	return l
}

func lower(c byte) byte {
	if c >= 'A' && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}

func itoa(n int) string { return strconv.Itoa(n) }
