# phantom

**Catch made-up code in bug reports, pull requests and AI answers.**

AI tools now write a lot of the bug reports, pull request descriptions and answers that reach a project. They cite functions, files, line numbers and code, and some of it does not exist. A maintainer can lose an hour finding that out.

phantom reads the text, picks out every piece of code it mentions, and checks each one against the repository. It takes a second or two, with no AI and no network.

```console
$ phantom -repo testdata/tiny testdata/report.md
testdata/report.md
This mentions 13 pieces of code, and 4 of them are not in the repository.

Not in the repository
  ✗ tiny_decode_url()         named like this project's code, but appears nowhere in it
  ✗ src/url_parser.c line 40  no such file
  ✗ line 400 of lib/url.c     lib/url.c has only 22 lines
  ✗ --follow-redirects        not an option anywhere in the repository

There, but not as described
  ~ lib/url.c:7               line 7 holds no code, only * tiny_parse_url splits a URL into its parts.
  ~ code block at line 9 (c)  only 2 of its 3 lines appear

In the repository
  ✓ tiny_parse_url            in 2 files, first at lib/url.c:7
  ✓ lib/url.c at line 9       line 9: int tiny_parse_url(const char *text, struct tiny_url *out)
  ✓ TINY_MAX_URL              in 2 files, first at lib/url.c:12
  ✓ --insecure                in 2 files, first at docs/options.md:3

Not from this repository: the writer's own code, the system, other programs
  · poc.c                     not in the repository, so probably the writer's own file or a library's
  · --leak-check              not one of this project's options; perhaps another program's
  · tiny_safe_copy()          proposed by the writer, not claimed to exist
```

## Tried on curl's AI slop

curl's lead developer keeps [a public list](https://gist.github.com/bagder/07f7581f6e3d78ef37dfbfc81fd1d1cd) of the AI slop reports sent to curl's bug bounty. phantom checked all 49 of them, each against the curl release current on the day it was filed:

| | Reports |
|---|---:|
| ❌ cite code that is not in curl | 5 |
| ⚠️ cite code that is there, but not as described | 10 |
| ✅ cite only code that is there | 32 |
| mention no code that can be checked | 2 |

The five call functions curl has never had (`curl_easy_setopt_timeout()`), cite a commit that is not in its history, quote code that is not in the file they name, and point at files that never existed. [docs/curl-evaluation.md](docs/curl-evaluation.md) has the method, every result, and how phantom did on the 12 reports it had not seen while it was being built.

Most of the 49 cite only real code, though. What makes them slop is the reasoning about that code, and phantom can't judge reasoning. It is a first filter, not a judge.

## What it checks

| The text mentions | phantom checks |
|---|---|
| a function, type or constant: `tiny_parse_url()`, `TINY_MAX_URL` | that the name appears in the code |
| a file, or a GitHub link to one | that the file exists, and what it is really called if not |
| a line: `lib/url.c:9`, "line 400 of lib/url.c" | that the file is that long, the line holds code, and a function named beside it is near it |
| a block of quoted code | that its lines are in the repository (for a patch, the lines it removes and keeps) |
| a command-line option given to the project's own program | that the program has it |
| a commit | that it is in the checkout's history |

Code the writer brings along is not a claim about the repository, so phantom sets it aside: a proof of concept, the commands they ran, a fix they propose, C library functions and system headers, other programs' options. It tells these apart by the headings and sentences around a block ("Proof of concept", "Replace X with Y") and by the code itself, such as a `main()` that calls the library, or a block in a language the project is not written in.

## Install

```bash
go install github.com/johnmikel/phantom@latest
```

## Use

```bash
phantom -repo ~/src/curl report.md
```

```bash
pbpaste | phantom
```

```bash
gh issue view 123 --json title,body -q '"# " + .title + "\n\n" + .body' | phantom
```

With no file, phantom reads standard input, and `-repo` defaults to the current directory. `-format markdown` writes a report for an issue comment, and `-format json` one for other tools. It exits with status 1 when something mentioned is not in the repository, and 2 on an error.

## In GitHub Actions

This workflow comments on new and edited issues and pull requests that cite code which is not in the repository:

```yaml
name: phantom

on:
  issues:
    types: [opened, edited]
  pull_request:
    types: [opened, edited, synchronize]

permissions:
  contents: read
  issues: write
  pull-requests: write

jobs:
  check:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: johnmikel/phantom@v0.1.0
```

| Input | Default | |
|---|---|---|
| `repo` | `.` | the checked-out repository to check against |
| `comment` | `missing` | when to comment: `missing`, `doubtful` (also when something is there but not as described), `always` or `never` |
| `fail-on-missing` | `false` | fail the step when something mentioned is not in the repository |
| `file` | | check this file instead of the issue or pull request |
| `token` | `github.token` | the token to comment with |

It sets the outputs `mentioned`, `missing`, `doubtful` and `report` (the path of the Markdown report), and adds the report to the job summary.

- It keeps one comment per issue and updates it when the text is edited, so fixing a report clears the warning.
- The text is read from the event file, never pasted into a command, so nothing in an issue can run in the workflow.
- A pull request from a fork gets a read-only token, so its report goes only to the job summary.
- To check commits, give the checkout its history with `fetch-depth: 0`. In a shallow checkout, commits are marked as not checked rather than missing.

## Limits

- **It checks references, not reasoning.** A report that cites only real code can still be wrong, as most of curl's slop is.
- **It checks the checkout you give it.** A report about an older release may cite code that has since moved. Check that release out first, for example with `git worktree add ../curl-8.11 curl-8_11_0`.
- **It has to guess whose code a name is.** A missing name counts against the text only when it is named like the project's code (`curl_`, `CURLOPT_`), so a made-up name in another style passes as the writer's own.
- **It reads English text, and code in the common languages**: C and C++, Go, Python, JavaScript and TypeScript, Rust, Java, Ruby, PHP and more.

## License

MIT
