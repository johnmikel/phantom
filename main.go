// Command phantom checks the code that a piece of text talks about, such as a
// bug report, a pull request description or an AI's answer, against a
// repository, and lists the functions, files, lines and code that are not
// there.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/johnmikel/phantom/internal/check"
)

const usage = `phantom checks that the code a piece of text mentions exists in a repository:
functions, files, line numbers, quoted code, command-line flags and commits.

Usage:
  phantom [-repo dir] [-format text|markdown|json] [file ...]

With no file, or with -, it reads the text from standard input. It exits with
status 1 when something mentioned is not in the repository.

Flags:`

func main() { os.Exit(cli(os.Args[1:], os.Stdin, os.Stdout, os.Stderr)) }

func cli(args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("phantom", flag.ContinueOnError)
	fs.SetOutput(stderr)
	repo := fs.String("repo", ".", "the repository to check against")
	format := fs.String("format", "text", "report `format`: text, markdown or json")
	fs.Usage = func() { fmt.Fprintln(stderr, usage); fs.PrintDefaults() }
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *format != "text" && *format != "markdown" && *format != "json" {
		fmt.Fprintf(stderr, "phantom: unknown format %q\n", *format)
		return 2
	}
	r, err := check.Open(*repo)
	if err != nil {
		fmt.Fprintln(stderr, "phantom:", err)
		return 2
	}
	sources := fs.Args()
	if len(sources) == 0 {
		sources = []string{"-"}
	}
	status := 0
	for _, src := range sources {
		text, err := read(src, stdin)
		if err != nil {
			fmt.Fprintln(stderr, "phantom:", err)
			return 2
		}
		name := src
		if src == "-" {
			name = "standard input"
		}
		rep := check.NewReport(name, r.Check(check.Extract(text)))
		switch *format {
		case "json":
			rep.JSON(stdout)
		case "markdown":
			rep.Markdown(stdout)
		default:
			rep.Text(stdout, colorful(stdout))
		}
		if rep.Missing > 0 {
			status = 1
		}
	}
	return status
}

func read(src string, stdin io.Reader) (string, error) {
	var b []byte
	var err error
	if src == "-" {
		b, err = io.ReadAll(stdin)
	} else {
		b, err = os.ReadFile(src)
	}
	return string(b), err
}

// colorful reports whether w is a terminal that wants colour.
func colorful(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok || os.Getenv("NO_COLOR") != "" {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
