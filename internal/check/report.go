package check

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Report is what was found for one piece of text.
type Report struct {
	Source    string   `json:"source"`
	Mentioned int      `json:"mentioned"`
	Missing   int      `json:"missing"`  // not in the repository at all
	Doubtful  int      `json:"doubtful"` // partly there, or at another path
	Results   []Result `json:"results"`
}

// NewReport sums up the results for one piece of text.
func NewReport(source string, results []Result) Report {
	r := Report{Source: source, Mentioned: len(results), Results: results}
	for _, res := range results {
		switch res.Status {
		case Missing:
			r.Missing++
		case Doubtful:
			r.Doubtful++
		}
	}
	if r.Results == nil {
		r.Results = []Result{}
	}
	return r
}

// Headline says in one sentence what was found.
func (r Report) Headline() string {
	switch {
	case r.Mentioned == 0:
		return "This mentions no code that can be checked."
	case r.Missing == 0 && r.Doubtful == 0:
		found, unchecked := 0, 0
		for _, res := range r.Results {
			switch res.Status {
			case Exists:
				found++
			case Unchecked:
				unchecked++
			}
		}
		switch {
		case found == 0 && unchecked == 0:
			return "None of the code this mentions is from this repository."
		case found == 0:
			return fmt.Sprintf("%s this mentions could not be checked.", count(unchecked, "piece of code", "pieces of code"))
		case unchecked > 0:
			return fmt.Sprintf("Everything this cites from the repository is there: %s. %d more could not be checked.",
				count(found, "piece of code", "pieces of code"), unchecked)
		}
		return fmt.Sprintf("Everything this cites from the repository is there: %s.", count(found, "piece of code", "pieces of code"))
	case r.Missing == 0:
		return fmt.Sprintf("This mentions %s. %d %s there, but not as described.", count(r.Mentioned, "piece of code", "pieces of code"),
			r.Doubtful, pick(r.Doubtful, "is", "are"))
	}
	return fmt.Sprintf("This mentions %s, and %d of them %s not in the repository.", count(r.Mentioned, "piece of code", "pieces of code"),
		r.Missing, pick(r.Missing, "is", "are"))
}

// groups orders results for reading: the missing ones first.
var groups = []struct {
	title  string
	mark   string
	color  string
	status []Status
}{
	{"Not in the repository", "✗", "31", []Status{Missing}},
	{"There, but not as described", "~", "33", []Status{Doubtful}},
	{"In the repository", "✓", "32", []Status{Exists}},
	{"Not from this repository: the writer's own code, the system, other programs", "·", "2", []Status{Foreign}},
	{"Not checked", "?", "2", []Status{Unchecked}},
}

// Text writes the report for a terminal, in colour if color is set.
func (r Report) Text(w io.Writer, color bool) {
	paint := func(code, s string) string {
		if !color {
			return s
		}
		return "\x1b[" + code + "m" + s + "\x1b[0m"
	}
	fmt.Fprintf(w, "%s\n%s\n", paint("1", r.Source), r.Headline())
	width := 0
	for _, res := range r.Results {
		width = max(width, len([]rune(short(res.Text))))
	}
	for _, g := range groups {
		var rows []Result
		for _, res := range r.Results {
			if has(g.status, res.Status) {
				rows = append(rows, res)
			}
		}
		if len(rows) == 0 {
			continue
		}
		fmt.Fprintf(w, "\n%s\n", paint("1", g.title))
		for _, res := range rows {
			note := res.Note
			if len(res.Closest) > 0 {
				note += " (closest: " + strings.Join(res.Closest, ", ") + ")"
			}
			text := short(res.Text)
			fmt.Fprintf(w, "  %s %s%s  %s\n", paint(g.color, g.mark), text, strings.Repeat(" ", width-len([]rune(text))), note)
		}
	}
	if r.Missing > 0 {
		fmt.Fprintln(w, "\nText that cites code which is not there was probably made up, or written about")
		fmt.Fprintln(w, "another version. Check the missing pieces before spending time on the rest.")
	}
	fmt.Fprintln(w)
}

// Markdown writes the report as a comment for an issue or pull request.
func (r Report) Markdown(w io.Writer) {
	fmt.Fprintf(w, "**phantom:** %s\n\n", r.Headline())
	var shown, found []Result
	for _, res := range r.Results {
		if res.Status == Missing || res.Status == Doubtful {
			shown = append(shown, res)
		} else {
			found = append(found, res)
		}
	}
	marks := map[Status]string{Missing: "❌", Doubtful: "⚠️", Exists: "✅", Foreign: "➖", Unchecked: "❔"}
	row := func(res Result) {
		note := res.Note
		if len(res.Closest) > 0 {
			note += ". Closest: `" + strings.Join(res.Closest, "`, `") + "`"
		}
		fmt.Fprintf(w, "| %s | %s | %s |\n", marks[res.Status], cell(short(res.Text)), strings.ReplaceAll(note, "|", `\|`))
	}
	if len(shown) > 0 {
		fmt.Fprintln(w, "| | Mentioned | What the repository has |")
		fmt.Fprintln(w, "|---|---|---|")
		for _, res := range shown {
			row(res)
		}
		fmt.Fprintln(w)
	}
	if len(found) > 0 {
		summary := "The other " + count(len(found), "piece of code", "pieces of code")
		if len(found) == 1 {
			summary = "The other piece of code"
		}
		if len(shown) == 0 {
			summary = "What was checked"
		}
		fmt.Fprintf(w, "<details><summary>%s</summary>\n\n", summary)
		fmt.Fprintln(w, "| | Mentioned | What the repository has |")
		fmt.Fprintln(w, "|---|---|---|")
		for _, res := range found {
			row(res)
		}
		fmt.Fprintln(w, "\n</details>")
	}
	if r.Missing > 0 {
		fmt.Fprintln(w, "\nCode cited here that is not in the repository was probably made up, or written about another version.")
	}
}

// JSON writes the report as JSON.
func (r Report) JSON(w io.Writer) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// short shortens a reference for a report column.
func short(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 44 {
		return string(r[:41]) + "…"
	}
	return s
}

func cell(s string) string {
	return "`" + strings.ReplaceAll(strings.ReplaceAll(s, "`", "'"), "|", `\|`) + "`"
}

func count(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return thousands(n) + " " + many
}

func has(ss []Status, s Status) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}
