package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func run(t *testing.T, stdin string, args ...string) (int, string, string) {
	t.Helper()
	var out, errs strings.Builder
	code := cli(args, strings.NewReader(stdin), &out, &errs)
	return code, out.String(), errs.String()
}

func TestMadeUpCodeFails(t *testing.T) {
	code, out, _ := run(t, "", "-repo", "testdata/tiny", "testdata/report.md")
	if code != 1 || !strings.Contains(out, "✗ tiny_decode_url()") {
		t.Errorf("exit %d:\n%s", code, out)
	}
}

func TestRealCodePasses(t *testing.T) {
	code, out, _ := run(t, "The length check in `tiny_parse_url()` (lib/url.c:12) compares against `TINY_MAX_URL`.", "-repo", "testdata/tiny")
	if code != 0 || !strings.Contains(out, "Everything this cites from the repository is there: 3 pieces of code.") {
		t.Errorf("exit %d:\n%s", code, out)
	}
}

func TestJSON(t *testing.T) {
	_, out, _ := run(t, "", "-repo", "testdata/tiny", "-format", "json", "-", "testdata/report.md")
	dec := json.NewDecoder(strings.NewReader(out))
	var reports []struct {
		Source  string
		Missing int
		Results []struct{ Kind, Text, Status string }
	}
	for dec.More() {
		var r struct {
			Source  string
			Missing int
			Results []struct{ Kind, Text, Status string }
		}
		if err := dec.Decode(&r); err != nil {
			t.Fatal(err)
		}
		reports = append(reports, r)
	}
	if len(reports) != 2 || reports[0].Source != "standard input" || len(reports[0].Results) != 0 || reports[1].Missing != 4 {
		t.Errorf("reports = %+v", reports)
	}
}

func TestBadUse(t *testing.T) {
	if code, _, errs := run(t, "", "-format", "html"); code != 2 || !strings.Contains(errs, `unknown format "html"`) {
		t.Errorf("exit %d: %s", code, errs)
	}
	if code, _, errs := run(t, "", "-repo", "testdata/tiny", "no-such-file.md"); code != 2 || !strings.Contains(errs, "no-such-file.md") {
		t.Errorf("exit %d: %s", code, errs)
	}
}
