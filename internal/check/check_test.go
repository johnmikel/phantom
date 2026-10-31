package check

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func sample(t *testing.T) (*Repo, Text) {
	t.Helper()
	r, err := Open("../../testdata/tiny")
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile("../../testdata/report.md")
	if err != nil {
		t.Fatal(err)
	}
	return r, Extract(string(text))
}

// TestSampleReport checks every reference in testdata/report.md against
// testdata/tiny, a small C project.
func TestSampleReport(t *testing.T) {
	r, text := sample(t)
	got := map[string]Status{}
	for _, res := range r.Check(text) {
		got[res.Text] = res.Status
	}
	want := map[string]Status{
		"tiny_parse_url":                     Exists,   // from the heading; the later tiny_parse_url() is the same name
		"lib/url.c at line 9":                Exists,   // and tiny_parse_url is on it
		"tiny_decode_url()":                  Missing,  // named like the project's code, but made up
		"TINY_MAX_URL":                       Exists,   //
		"lib/url.c:7":                        Doubtful, // a comment, not code
		"code block at line 9 (c)":           Doubtful, // two of its three lines are real
		"src/url_parser.c line 40":           Missing,  // no such file in src/
		"line 400 of lib/url.c":              Missing,  // the file is not that long
		"--insecure":                         Exists,   //
		"--follow-redirects":                 Missing,  // given to tiny, which has no such option
		"poc.c":                              Foreign,  // the writer's own file
		"--leak-check":                       Foreign,  // valgrind's option
		"tiny_safe_copy()":                   Foreign,  // proposed as a fix, not claimed to exist
		"tiny --insecure --follow-redirects": "",       // not a reference in itself
		"tiny_unsafe_copy":                   "",       // checked as part of the quoted code, not on its own
		"make_payload":                       "",       // the writer's proof of concept
	}
	for text, status := range want {
		if status == "" {
			if _, ok := got[text]; ok {
				t.Errorf("%q should not be a reference", text)
			}
			continue
		}
		if got[text] != status {
			t.Errorf("%q: %q, want %q", text, got[text], status)
		}
	}
	for text := range got {
		if _, ok := want[text]; !ok {
			t.Errorf("unexpected reference %q (%s)", text, got[text])
		}
	}
}

func TestExtractDetails(t *testing.T) {
	_, text := sample(t)
	byText := map[string]Ref{}
	for _, r := range text.Refs {
		byText[r.Text] = r
	}
	if r := byText["lib/url.c at line 9"]; r.Path != "lib/url.c" || r.Line != 9 || !reflect.DeepEqual(r.Near, []string{"tiny_parse_url"}) {
		t.Errorf("line reference = %+v", r)
	}
	if r := byText["line 400 of lib/url.c"]; r.Line != 400 {
		t.Errorf("line written before the file = %+v", r)
	}
	if r := byText["--follow-redirects"]; r.Cmd != "tiny" {
		t.Errorf("flag command = %q, want tiny", r.Cmd)
	}
	if r := byText["tiny_safe_copy()"]; !r.Idea {
		t.Error("a name in a suggestion should be marked as an idea")
	}
	var aims []string
	for _, b := range text.Blocks {
		aims = append(aims, b.Aim)
	}
	if !reflect.DeepEqual(aims[:2], []string{"quote", "other"}) {
		t.Errorf("block aims = %v, want the quote and then the proof of concept", aims)
	}
}

func TestGitHubLinks(t *testing.T) {
	refs := Extract("See https://github.com/acme/tiny/blob/0a1b2c3/lib/url.c#L9-L14 for the copy.").Refs
	if len(refs) != 1 || refs[0].Path != "lib/url.c" || refs[0].Line != 9 || refs[0].End != 14 {
		t.Fatalf("refs = %+v", refs)
	}
}

func TestDiffChecksRemovedLinesOnly(t *testing.T) {
	r, _ := sample(t)
	text := Extract("```diff\n-  memcpy(out->buf, text, len + 1);\n+  memcpy(out->buf, text, min(len, TINY_MAX_URL));\n   return tiny_split_url(out);\n```")
	res := r.Check(text)
	if len(res) != 1 || res[0].Status != Exists {
		t.Fatalf("a patch whose removed and context lines exist should pass: %+v", res)
	}
}

func TestHelpers(t *testing.T) {
	for line, want := range map[string]bool{"*/": true, "* text": true, "// x": true, "#include <a.h>": false,
		"# a comment": true, "*p = 0;": false, "-- sql comment": true, "--i;": false} {
		if noCode(line) != want {
			t.Errorf("noCode(%q) = %v", line, !want)
		}
	}
	if got := even("    strcpy(keyval,   randstr);  // VULNERABLE!"); got != "strcpy(keyval, randstr);" {
		t.Errorf("even = %q", got)
	}
	if distance("curl_easy_setopt", "curl_easy_setop", 2) != 1 || distance("abc", "xyz", 1) != 2 {
		t.Error("distance is wrong")
	}
	for line, want := range map[string]string{"$ sudo /usr/bin/curl --insecure x": "curl", "FOO=1 tiny -v": "tiny", "--x": ""} {
		if got := command(line); got != want {
			t.Errorf("command(%q) = %q, want %q", line, got, want)
		}
	}
	for line, want := range map[string]bool{"    char* user_input = \"%x\";  // read": true, "conn.send_headers(1, [(':s', '2')])": true,
		"The function copies the URL.": false, "for event in events:": true, "- a list item;": false} {
		if codeLine(line) != want {
			t.Errorf("codeLine(%q) = %v", line, !want)
		}
	}
}

func TestReports(t *testing.T) {
	r, text := sample(t)
	rep := NewReport("report.md", r.Check(text))
	if rep.Missing != 4 || rep.Doubtful != 2 {
		t.Errorf("missing %d, doubtful %d; want 4 and 2", rep.Missing, rep.Doubtful)
	}
	var b strings.Builder
	rep.Text(&b, false)
	for _, want := range []string{"and 4 of them are not in the repository", "✗ tiny_decode_url()", "named like this project's code",
		"lib/url.c has only 22 lines", "Not from this repository"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("text report lacks %q:\n%s", want, b.String())
		}
	}
	b.Reset()
	rep.Markdown(&b)
	if !strings.Contains(b.String(), "| ❌ | `tiny_decode_url()` |") || !strings.Contains(b.String(), "<details>") {
		t.Errorf("markdown report:\n%s", b.String())
	}
}

func TestHeadlines(t *testing.T) {
	r := func(statuses ...Status) Report {
		var res []Result
		for _, s := range statuses {
			res = append(res, Result{Status: s})
		}
		return NewReport("x", res)
	}
	for want, rep := range map[string]Report{
		"This mentions no code that can be checked.":                                                         r(),
		"None of the code this mentions is from this repository.":                                            r(Foreign),
		"2 pieces of code this mentions could not be checked.":                                               r(Unchecked, Unchecked, Foreign),
		"Everything this cites from the repository is there: 1 piece of code.":                               r(Exists, Foreign),
		"Everything this cites from the repository is there: 2 pieces of code. 1 more could not be checked.": r(Exists, Exists, Unchecked),
		"This mentions 2 pieces of code. 1 is there, but not as described.":                                  r(Exists, Doubtful),
		"This mentions 3 pieces of code, and 2 of them are not in the repository.":                           r(Missing, Doubtful, Missing),
	} {
		if got := rep.Headline(); got != want {
			t.Errorf("Headline() = %q, want %q", got, want)
		}
	}
}

func TestFileNameInTheWrongCase(t *testing.T) {
	r, _ := sample(t)
	res := r.Check(Extract("The overflow is in URL.c, see `Lib/Url.h`."))
	if len(res) != 2 {
		t.Fatalf("results = %+v", res)
	}
	for _, x := range res {
		if x.Status != Doubtful || !strings.Contains(x.Note, "only in case") {
			t.Errorf("%s: %s, %s", x.Text, x.Status, x.Note)
		}
	}
}

func TestLibraryHeaders(t *testing.T) {
	r, _ := sample(t)
	for _, text := range []string{"It includes `openssl/ssl.h`.", "See `uri.h`."} {
		res := r.Check(Extract(text))
		if len(res) != 1 || res[0].Status != Foreign {
			t.Errorf("%s: %+v", text, res)
		}
	}
	if res := r.Check(Extract("See `lib/curl.h`.")); len(res) != 1 || res[0].Status != Missing {
		t.Errorf("a file in the repository's own folder should be missing: %+v", res)
	}
}

func TestPathStartingWithTheProjectName(t *testing.T) {
	r, _ := sample(t)
	res := r.Check(Extract("The copy is in tiny/lib/url.c:14."))
	if len(res) != 1 || res[0].Status != Exists {
		t.Errorf("results = %+v", res)
	}
}
