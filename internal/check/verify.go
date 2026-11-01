package check

import (
	"fmt"
	"os/exec"
	"path"
	"slices"
	"sort"
	"strings"
)

// Status is what became of a reference.
type Status string

const (
	Exists    Status = "exists"
	Missing   Status = "missing"   // not in the repository, though it claims to be or looks like it should be
	Doubtful  Status = "doubtful"  // there, but not as described: a wrong path, a line with no code, a partial quote
	Foreign   Status = "foreign"   // not in the repository, and not from it: the writer's own code, a system header, another tool
	Unchecked Status = "unchecked" // there was no way to check, as for a commit outside git
)

// Result is one reference and what the repository says about it.
type Result struct {
	Ref
	Status  Status   `json:"status"`
	Note    string   `json:"note"`              // what was found, in a few words
	Closest []string `json:"closest,omitempty"` // real names that look like a missing one
}

// Check looks up every reference in the repository.
func (r *Repo) Check(t Text) []Result {
	// Names that the writer's own code uses or defines, such as an exploit or
	// a command they ran, are theirs, not claims about the repository.
	own, defined := map[string]bool{}, map[string]bool{}
	for _, b := range t.Blocks {
		lang := family(b.Lang)
		mine := b.Aim == "other" || b.Shell || b.Aim != "quote" && (b.Lang == "" || lang == "" || !r.main[lang])
		for w := range b.Words {
			own[w] = own[w] || mine
		}
		for w := range b.Defines {
			defined[w], own[w] = true, true
		}
	}
	var out []Result
	for _, ref := range t.Refs {
		var res Result
		switch ref.Kind {
		case Symbol:
			res = r.symbol(ref, own, defined)
		case File:
			res = r.file(ref, own)
		case Snippet:
			var ok bool
			if res, ok = r.snippet(ref); !ok {
				continue // not a quotation of the repository, or nothing in it to look for
			}
		case Flag:
			res = r.flag(ref)
		case Commit:
			res = r.commit(ref)
		}
		out = append(out, res)
	}
	return out
}

// symbol checks a name. A name the writer's code defines is theirs; one named
// like the project's own code but nowhere in it is the surest sign of
// something made up, even when only the writer's code calls it.
func (r *Repo) symbol(ref Ref, own, defined map[string]bool) Result {
	if owner, part, ours := r.member(ref); part != "" && !defined[part] && !ref.Idea && (ours || !own[part]) {
		closest, _ := r.closest(part)
		if ours {
			return Result{Ref: ref, Status: Missing, Note: owner + " is in the repository, but " + part + " is not", Closest: closest}
		}
		return Result{Ref: ref, Status: Doubtful, Note: owner + " is used in the repository, but " + part + " appears nowhere in it",
			Closest: closest}
	}
	if s := r.idents[ref.Name]; s != nil {
		return Result{Ref: ref, Status: Exists, Note: places(s)}
	}
	closest, d := r.closest(ref.Name)
	file := r.claimedIn(ref.Files)
	switch {
	case defined[ref.Name]:
		return Result{Ref: ref, Status: Foreign, Note: "defined in the writer's own code"}
	case libc[ref.Name]:
		return Result{Ref: ref, Status: Foreign, Note: "a C library function"}
	case ref.Idea:
		return Result{Ref: ref, Status: Foreign, Note: "proposed by the writer, not claimed to exist"}
	case r.styled(ref.Name):
		return Result{Ref: ref, Status: Missing, Note: "named like this project's code, but appears nowhere in it", Closest: closest}
	case own[ref.Name]:
		return Result{Ref: ref, Status: Foreign, Note: "only in the writer's own code"}
	case file != "":
		return Result{Ref: ref, Status: Missing, Note: "mentioned with " + file + ", but appears nowhere in the repository", Closest: closest}
	case len(closest) > 0 && d <= 1 && len(ref.Name) >= 8:
		return Result{Ref: ref, Status: Missing, Note: "appears nowhere in the repository", Closest: closest}
	}
	return Result{Ref: ref, Status: Foreign, Note: "appears nowhere, and is not named like this project's code"}
}

// member checks a name written as a member of something, such as
// check.Parse, Repo::get or cmd.Flags(). When the project defines it or its
// code uses it, each part after it must be in the code too: member returns
// the first that is not, what it was a member of, and whether the project
// defines that (ours) or only uses it, as it uses a variable.
func (r *Repo) member(ref Ref) (owner, part string, ours bool) {
	if len(ref.Qual) == 0 {
		return "", "", false
	}
	root := ref.Qual[0]
	if libraries[root] && root != r.name || !r.declared[root] && !r.used[root] {
		return "", "", false // a standard library's module, such as os in os.path.join, or not this project's
	}
	parts := append(slices.Clone(ref.Qual[1:]), ref.Name)
	for i, p := range parts {
		if r.idents[p] == nil {
			return strings.Join(append([]string{root}, parts[:i]...), "."), p, r.declared[root]
		}
	}
	return "", "", false
}

// libraries are the modules and objects of languages' standard libraries and
// runtimes, which a project uses but does not define.
var libraries = map[string]bool{}

func init() {
	for _, w := range strings.Fields(`self this super cls me it module exports require
		fmt os io ioutil strings strconv bytes errors time sync atomic context http net url json xml filepath path
		sort regexp bufio log exec reflect unsafe math rand big template testing flag runtime signal syscall utf8
		unicode hex base64 sha256 md5 tls x509 crypto zip gzip tar heap list ring slices maps cmp iter
		sys re datetime random subprocess pathlib collections itertools functools typing logging threading
		multiprocessing asyncio socket struct hashlib pickle shutil glob string copy uuid urllib unittest pytest
		argparse csv sqlite3 tempfile traceback inspect contextlib dataclasses enum abc operator queue select ssl
		zlib zipfile tarfile html email decimal statistics secrets hmac binascii codecs platform warnings weakref
		ctypes np numpy pd pandas plt torch tf
		console Math JSON Object Array Promise process window document navigator Number String Date Reflect
		Symbol Buffer globalThis Intl Error Map Set WeakMap fetch fs util events stream child_process React
		ReactDOM jest expect vi describe
		System Arrays Collections List Optional Objects Integer Long Thread Files Paths Collectors
		std core alloc Vec Option Result Box Rc Arc HashMap HashSet BTreeMap env mem ptr thread boost`) {
		libraries[w] = true
	}
}

// same reports whether a GitHub repository, owner/name, is this one or a
// fork of it.
func (r *Repo) same(site string) bool {
	_, name, _ := strings.Cut(site, "/")
	_, remote, _ := strings.Cut(r.remote, "/")
	return strings.EqualFold(name, r.name) || remote != "" && strings.EqualFold(name, remote)
}

// claimedIn returns the first of files that is in the repository, as
// written or under a folder, or "".
func (r *Repo) claimedIn(files []string) string {
	for _, f := range files {
		p := cleanPath(f)
		if top, rest, ok := strings.Cut(p, "/"); ok && strings.EqualFold(top, r.name) {
			p = rest
		}
		if r.files[p] != nil {
			return p
		}
		for _, g := range r.bases[path.Base(p)] {
			if strings.HasSuffix(g, "/"+p) {
				return g
			}
		}
	}
	return ""
}

// styled reports whether a name starts the way the repository's own names
// do, as curl_easy_setopt_timeout starts like curl_easy_setopt.
func (r *Repo) styled(name string) bool {
	i := strings.Index(name, "_")
	return i >= 3 && r.styles[name[:i+1]]
}

// libc are C library and POSIX functions, which a report may name but a
// project does not define.
var libc = map[string]bool{}

func init() {
	for _, f := range strings.Fields(`malloc calloc realloc free memcpy memmove memset memcmp memchr memcpy_s memset_s
		strcpy strncpy strcat strncat strcmp strncmp strlen strnlen strchr strrchr strstr strdup strndup strtok strtol
		strtoul strtoll strtoull strerror strlcpy strlcat sprintf snprintf vsprintf vsnprintf printf fprintf scanf sscanf
		fopen fclose fread fwrite fgets fputs fseek ftell fflush getenv setenv putenv system exit abort atexit atoi atol
		open close read write lseek mmap munmap mprotect fork execve execvp waitpid kill signal sigaction pthread_create
		pthread_join pthread_mutex_lock pthread_mutex_unlock pthread_once socket connect bind listen accept recv send
		recvfrom sendto select poll getaddrinfo freeaddrinfo gethostbyname inet_ntop inet_pton htons ntohs htonl ntohl
		gettimeofday clock_gettime time localtime gmtime strftime sleep usleep nanosleep dlopen dlsym dlclose`) {
		libc[f] = true
	}
}

// headers are system headers, which a C repository includes but does not hold.
var headers = map[string]bool{}

func init() {
	for _, h := range strings.Fields(`assert.h ctype.h errno.h float.h limits.h locale.h math.h setjmp.h signal.h
		stdarg.h stddef.h stdio.h stdlib.h string.h strings.h time.h stdint.h stdbool.h inttypes.h unistd.h fcntl.h
		pthread.h dlfcn.h netdb.h poll.h dirent.h termios.h syslog.h windows.h winsock2.h ws2tcpip.h malloc.h
		iostream vector string map memory`) {
		headers[h] = true
	}
}

func (r *Repo) file(ref Ref, own map[string]bool) Result {
	if ref.Site != "" && !r.same(ref.Site) {
		return Result{Ref: ref, Status: Foreign, Note: "a file in another repository, " + ref.Site}
	}
	p := cleanPath(ref.Path)
	if top, rest, ok := strings.Cut(p, "/"); ok && strings.EqualFold(top, r.name) && (r.files[rest] != nil || r.dirs[rest]) {
		p = rest // curl/lib/ws.c, written in the curl repository
	}
	at := p
	if r.files[p] == nil && !r.dirs[strings.TrimSuffix(p, "/")] {
		var same, other []string // files whose path ends with p, and files that merely share its name
		for _, f := range r.bases[path.Base(p)] {
			if strings.HasSuffix(f, "/"+p) {
				same = append(same, f)
			} else {
				other = append(other, f)
			}
		}
		switch {
		case len(same) == 1:
			at = same[0] // "ws.c" for lib/ws.c
		case len(same) > 1:
			return Result{Ref: ref, Status: Exists, Note: "matches " + list(same)}
		case len(other) > 0:
			return Result{Ref: ref, Status: Doubtful, Note: "no such path, but " + list(other) + " " + pick(len(other), "exists", "exist")}
		case headers[p] || strings.HasPrefix(p, "sys/") || strings.HasPrefix(p, "arpa/") || strings.HasPrefix(p, "netinet/"):
			return Result{Ref: ref, Status: Foreign, Note: "a system header"}
		case own[path.Base(p)]:
			return Result{Ref: ref, Status: Foreign, Note: "the writer's own file"}
		case len(r.folded[strings.ToLower(path.Base(p))]) > 0:
			fold := r.folded[strings.ToLower(path.Base(p))]
			return Result{Ref: ref, Status: Doubtful, Note: "no such path, but " + list(fold) + " " + pick(len(fold), "differs", "differ") + " only in case"}
		default:
			// A path into one of the repository's folders should be there, and
			// so should a long name a letter away from a real one. A short one,
			// such as ssl.h for psl.h, is more likely a library's.
			closest, d := r.closestFile(p)
			top := strings.Split(p, "/")[0]
			if strings.Contains(p, "/") && r.dirs[top] || !strings.Contains(p, "/") && d <= 1 && len(p) >= 8 {
				return Result{Ref: ref, Status: Missing, Note: "no such file", Closest: closest}
			}
			return Result{Ref: ref, Status: Foreign, Note: "not in the repository, so probably the writer's own file or a library's"}
		}
	}
	lines := r.files[at]
	switch {
	case lines == nil:
		return Result{Ref: ref, Status: Exists, Note: "a folder"}
	case ref.Line == 0:
		return Result{Ref: ref, Status: Exists, Note: fmt.Sprintf("%s lines", thousands(len(lines)))}
	case ref.Line > len(lines) || ref.End > len(lines):
		return Result{Ref: ref, Status: Missing, Note: fmt.Sprintf("%s has only %s lines", at, thousands(len(lines)))}
	}
	code := strings.TrimSpace(lines[ref.Line-1])
	shown := code
	if len(shown) > 60 {
		shown = shown[:57] + "…"
	}
	if ref.End == 0 {
		switch {
		case code == "":
			return Result{Ref: ref, Status: Doubtful, Note: fmt.Sprintf("line %d is blank", ref.Line)}
		case noCode(code):
			return Result{Ref: ref, Status: Doubtful, Note: fmt.Sprintf("line %d holds no code, only %s", ref.Line, shown)}
		}
	} else {
		some := false
		for _, l := range lines[ref.Line-1 : ref.End] {
			l = strings.TrimSpace(l)
			some = some || l != "" && !noCode(l)
		}
		if !some {
			return Result{Ref: ref, Status: Doubtful, Note: fmt.Sprintf("lines %d to %d hold no code", ref.Line, ref.End)}
		}
		shown = fmt.Sprintf("%d lines", ref.End-ref.Line+1)
	}
	// The functions named alongside a line number should be near that line.
	if len(ref.Near) > 0 {
		lo, hi := max(1, ref.Line-3), min(len(lines), max(ref.Line, ref.End)+3)
		near := map[string]bool{}
		for _, l := range lines[lo-1 : hi] {
			words(l, func(w string) { near[w] = true })
		}
		var absent []string
		for _, name := range ref.Near {
			if !near[name] {
				absent = append(absent, name)
			}
		}
		if len(absent) == len(ref.Near) {
			return Result{Ref: ref, Status: Doubtful, Note: fmt.Sprintf("line %d is %s, and %s is not within 3 lines of it",
				ref.Line, shown, strings.Join(absent, " or "))}
		}
	}
	if ref.End > 0 {
		return Result{Ref: ref, Status: Exists, Note: fmt.Sprintf("lines %d to %d hold code", ref.Line, ref.End)}
	}
	return Result{Ref: ref, Status: Exists, Note: fmt.Sprintf("line %d: %s", ref.Line, shown)}
}

// noCode reports whether a line holds only a comment.
func noCode(line string) bool {
	switch {
	case line == "*", line == "/*", line == "*/":
		return true
	case strings.HasPrefix(line, "//"), strings.HasPrefix(line, "/*"), strings.HasPrefix(line, "* "),
		strings.HasPrefix(line, "*/"), strings.HasPrefix(line, "<!--"), strings.HasPrefix(line, "-- "):
		return true
	case strings.HasPrefix(line, "#"):
		directive := strings.Fields(strings.TrimPrefix(line, "#") + " x")[0]
		switch directive {
		case "include", "define", "if", "ifdef", "ifndef", "endif", "else", "elif", "undef", "pragma", "error", "line", "import", "!":
			return false
		}
		return !strings.HasPrefix(line, "#!")
	}
	return false
}

// snippet checks a code block line by line. Lines that appear all over the
// repository say little, so they count only when nothing else does. Code that
// is not quoted from the repository, such as a suggested fix or an exploit
// written in another language, is not expected to be in it and is skipped.
func (r *Repo) snippet(ref Ref) (Result, bool) {
	lang := family(ref.Lang)
	if !(ref.Aim == "quote" || ref.Aim == "" && (ref.Lang == "" || r.main[lang])) {
		return Result{}, false
	}
	var distinct, common []string
	for _, line := range strings.Split(ref.Code, "\n") {
		if ref.Diff {
			switch {
			case strings.HasPrefix(line, "---"), strings.HasPrefix(line, "+++"), strings.HasPrefix(line, "@@"),
				strings.HasPrefix(line, "diff "), strings.HasPrefix(line, "index "), strings.HasPrefix(line, "+"):
				continue // headers and added lines are not meant to exist yet
			case strings.HasPrefix(line, "-"), strings.HasPrefix(line, " "):
				line = line[1:]
			}
		}
		l := even(line)
		switch s := r.lines[l]; {
		case l == "":
		case s != nil && s.count >= 10:
			common = append(common, l)
		default:
			distinct = append(distinct, l)
		}
	}
	check := distinct
	if len(check) == 0 {
		check = common
	}
	if len(check) == 0 {
		return Result{}, false
	}
	found, first := 0, ""
	for _, l := range check {
		if s := r.lines[l]; s != nil {
			found++
			if first == "" {
				first = s.first
			}
		}
	}
	ref.Text = fmt.Sprintf("code block at line %d", ref.At)
	if ref.Lang != "" {
		ref.Text += " (" + ref.Lang + ")"
	}
	n := len(check)
	switch {
	case found == 0 && n < 3:
		return Result{Ref: ref, Status: Doubtful, Note: fmt.Sprintf("its %d %s %s not appear, though so short a quote may be paraphrased",
			n, pick(n, "line", "lines"), pick(n, "does", "do"))}, true
	case found*10 >= n*7:
		return Result{Ref: ref, Status: Exists, Note: fmt.Sprintf("%d of its %d lines %s, first at %s", found, n, pick(found, "appears", "appear"), first)}, true
	case found*4 >= n:
		return Result{Ref: ref, Status: Doubtful, Note: fmt.Sprintf("only %d of its %d lines %s", found, n, pick(found, "appears", "appear"))}, true
	case found == 0:
		return Result{Ref: ref, Status: Missing, Note: fmt.Sprintf("none of its %d %s appear%s in the repository", n, pick(n, "line", "lines"), pick(n, "s", ""))}, true
	}
	return Result{Ref: ref, Status: Missing, Note: fmt.Sprintf("only %d of its %d lines %s in the repository", found, n, pick(found, "appears", "appear"))}, true
}

// flag checks a command-line option. One given to this project's own
// program, or close to one of its options, should be one of them.
func (r *Repo) flag(ref Ref) Result {
	if s := r.flags[ref.Name]; s != nil {
		return Result{Ref: ref, Status: Exists, Note: places(s)}
	}
	var close []string
	for f := range r.flags {
		if distance(f, ref.Name, 2) <= 2 {
			close = append(close, f)
		}
	}
	sort.Strings(close)
	close = close[:min(3, len(close))]
	if ref.Cmd != "" && strings.EqualFold(ref.Cmd, r.name) {
		return Result{Ref: ref, Status: Missing, Note: "not an option anywhere in the repository", Closest: close}
	}
	return Result{Ref: ref, Status: Foreign, Note: "not one of this project's options; perhaps another program's"}
}

func (r *Repo) commit(ref Ref) Result {
	if !r.Git {
		return Result{Ref: ref, Status: Unchecked, Note: "not a git checkout"}
	}
	if exec.Command("git", "-C", r.Root, "cat-file", "-e", ref.Name+"^{commit}").Run() != nil {
		if r.Shallow {
			return Result{Ref: ref, Status: Unchecked, Note: "not in this shallow checkout, which lacks the older history"}
		}
		return Result{Ref: ref, Status: Missing, Note: "no such commit in this checkout"}
	}
	return Result{Ref: ref, Status: Exists, Note: "a commit in this checkout"}
}

// closest finds real identifiers within a typo or two of a missing one, and
// how far off the nearest is.
func (r *Repo) closest(name string) ([]string, int) {
	limit := 1
	if len(name) >= 8 {
		limit = 2
	}
	type cand struct {
		name string
		d, n int
	}
	var cs []cand
	for _, id := range r.byChar[lower(name[0])] {
		if abs(len(id)-len(name)) > limit {
			continue
		}
		if d := distance(strings.ToLower(id), strings.ToLower(name), limit); d <= limit {
			cs = append(cs, cand{id, d, r.idents[id].count})
		}
	}
	sort.Slice(cs, func(i, j int) bool {
		if cs[i].d != cs[j].d {
			return cs[i].d < cs[j].d
		}
		if cs[i].n != cs[j].n {
			return cs[i].n > cs[j].n
		}
		return cs[i].name < cs[j].name
	})
	if len(cs) == 0 {
		return nil, limit + 1
	}
	var out []string
	for _, c := range cs[:min(3, len(cs))] {
		out = append(out, c.name)
	}
	return out, cs[0].d
}

// closestFile finds files named within a typo or two of a missing one,
// ignoring case, and how far off the nearest is.
func (r *Repo) closestFile(p string) ([]string, int) {
	base := strings.ToLower(path.Base(p))
	best := 3
	byDist := map[int][]string{}
	for b, paths := range r.bases {
		if d := distance(strings.ToLower(b), base, 2); d <= 2 {
			byDist[d] = append(byDist[d], paths...)
			best = min(best, d)
		}
	}
	var out []string
	for d := 0; d <= 2; d++ {
		sort.Strings(byDist[d])
		out = append(out, byDist[d]...)
	}
	return out[:min(3, len(out))], best
}

// distance is the edit distance between a and b, or limit+1 once it is over limit.
func distance(a, b string, limit int) int {
	if abs(len(a)-len(b)) > limit {
		return limit + 1
	}
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		best := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			best = min(best, cur[j])
		}
		if best > limit {
			return limit + 1
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// cleanPath drops what a path picks up on its way into a report: a leading
// ./ or /, and the a/ or b/ of a diff.
func cleanPath(p string) string {
	p = strings.TrimPrefix(strings.TrimPrefix(p, "./"), "/")
	for _, pre := range []string{"a/", "b/"} {
		p = strings.TrimPrefix(p, pre)
	}
	p = path.Clean(p)
	for strings.HasPrefix(p, "../") {
		p = p[3:] // relative to a folder the text does not name
	}
	return p
}

func places(s *seen) string {
	if s.files == 1 {
		return fmt.Sprintf("at %s", s.first)
	}
	return fmt.Sprintf("in %s files, first at %s", thousands(s.files), s.first)
}

func list(paths []string) string {
	if len(paths) <= 2 {
		return strings.Join(paths, " and ")
	}
	return strings.Join(paths[:2], ", ") + fmt.Sprintf(" and %d more", len(paths)-2)
}

func pick(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func thousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + "," + s[i:]
	}
	return s
}
