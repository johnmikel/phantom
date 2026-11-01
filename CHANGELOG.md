# Changelog

## v0.1.2 (1 November 2026)

Fixes four bugs found while writing [a demo issue](https://github.com/johnmikel/phantom/issues/1) for the GitHub Action:

- A file's length was counted one line too long, because the newline that ends the last line was read as the start of another. "line 22 of lib/url.c", for a file of 21 lines, is now missing rather than a blank line.
- "Line 640 of extract.go" at the start of a sentence is read as a line reference, as "line 640 of extract.go" already was.
- The options in a command that also names a file, such as `--follow-redirects` in `tiny --follow-redirects urls.txt`, are checked. Only the file used to be.
- A quoted line found in the repository reads "its line appears", not "1 of its 1 lines appears", and a Markdown report's one remaining piece of code is "The other piece of code".

curl's 49 AI slop reports, and the docs of cobra, requests, FastAPI and tRPC with and without made-up names, get the same results as with v0.1.1.

## v0.1.1 (1 November 2026)

Catches made-up names in projects whose code, unlike curl's, has no name prefixes, such as most Go, Python and TypeScript projects. On four of them, it flags 81% of made-up members such as `Session.sendSafely()`, against 2% before. See [docs/languages-evaluation.md](docs/languages-evaluation.md).

Finds more made-up code:

- A member of the project's own package, module, folder, file, type or class, such as `store.Open()` or `AuthService.logout()`, is missing when that member is nowhere in the code.
- A member of a variable the code uses, such as `cmd.Purge()`, is doubtful when the member is nowhere in the code.
- A name mentioned in the same sentence as one of the project's files, as in "`useToken()` in src/auth.ts", is missing when it is nowhere in the repository.
- "Use the `X()` method" is read as a claim that `X` exists, not as a suggestion. Suggestions are judged sentence by sentence, not line by line.
- Members whose names are also keywords, such as `store.New()` and `Store.Delete()`, are checked.

Raises fewer false alarms:

- A GitHub link into another repository is "not from this repository", not a missing file.
- Software names such as Node.js and Express.js are not read as files.
- Paths relative to the page, such as `../../docs_src/main.py`, are found.
- URL-encoded GitHub links, and a quote mark or full stop after a link, no longer break its path.
- A path into a folder the repository doesn't have is no longer taken for a misspelling of a similar file name.
- Host names such as `evil.com` are not read as code.

Also:

- In a git checkout, new files that git does not ignore are read too, not only the files it tracks.
- Where a name is first found is reported from the code, rather than from a README that mentions it.
- The Action uses `actions/setup-go@v7`, so it no longer causes GitHub's warning that Node.js 20 is deprecated.
- curl's 49 AI slop reports get the same results as with v0.1.0.

## v0.1.0 (31 October 2026)

The first release: the `phantom` command, the GitHub Action, and the evaluation on curl's AI slop reports.
