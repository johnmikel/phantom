# phantom on Go, Python and TypeScript projects

curl names its code with prefixes (`curl_`, `Curl_`, `CURLOPT_`), so a made-up name like `curl_easy_setopt_timeout()` stands out. Most projects have no such prefixes. v0.1.1 adds two checks that don't depend on them:

- **A member of the project's own code.** `store.Open()` is missing when the project has a `store` package, module, folder or file but no `Open` anywhere. The same goes for types and classes, as in `Store.Delete()` or `AuthService.logout()`. A member of a variable the code uses, like `cmd.Purge()`, is only doubtful.
- **A name tied to a file.** In "`useToken()` in src/auth.ts", the name is said to be in a file the project has, so it is missing if it appears nowhere in the repository.

Both were tested on four projects, checking each project's own docs against its own code:

| Project | Language | Commit | Docs checked |
|---|---|---|---|
| [cobra](https://github.com/spf13/cobra) | Go | adbc881 (10 Jul 2026) | 17 Markdown pages |
| [requests](https://github.com/psf/requests) | Python | 3c7baaa (26 Oct 2026) | README and 15 reStructuredText pages |
| [FastAPI](https://github.com/fastapi/fastapi) | Python | 45cb225 (31 Oct 2026) | README and 156 English docs pages |
| [tRPC](https://github.com/trpc/trpc) | TypeScript | 87e8b38 (31 Oct 2026) | README and 90 docs pages |

## False alarms

Docs that ship with a project should cite code that is in it, so most of what phantom flags in them is a false alarm, or a doc gone out of date.

| Project | References | Flagged by v0.1.0 (missing, doubtful) | Flagged by v0.1.1 |
|---|---:|---:|---:|
| cobra | 258 | 1, 0 | 0, 0 |
| requests | 220 | 0, 2 | 0, 0 |
| FastAPI, apart from its changelog | 1,958 | 1, 365 | 0, 5 |
| tRPC | 1,152 | 9, 40 | 0, 4 |

The new checks flag nothing in these docs. v0.1.1 also fixes false alarms v0.1.0 raised:

- links to files in other GitHub repositories, such as Redocly's, were checked against this one;
- software names were read as files: "Express.js", "React.js/Next.js/Express.js";
- paths relative to the page, like `../../docs_src/first_steps/tutorial001_py310.py`, were not resolved;
- a URL-encoded link (`%5Btrpc%5D.ts` for `[trpc].ts`) and a quote mark after a link's path (`LICENSE"`) broke the path;
- cobra's `cmd/completion.go`, a file in the reader's own project, was taken for a misspelling of `completions.go`.

What v0.1.1 still flags, all checked by hand: FastAPI's 5 and tRPC's 4 are paths in the reader's own app, such as `app/routers/items.py` and `trpc/query-client.ts`. phantom marks them doubtful and names the example files they match.

FastAPI's changelog is counted apart. It cites files from older versions, and v0.1.1 flags 21 as missing and 105 as doubtful, which is right for code that has since moved.

## Made-up names

To see what the checks catch, real names in the same docs were swapped for made-up ones. For each project, up to 150 members (`a.b`, `a.b()`) and 150 bare calls (`b()`) were picked at random from the docs' code spans, keeping only those phantom finds in the code. Each was renamed by adding a word, so `Session.send()` became `Session.sendSafely()` and `url_for()` became `url_for_safely()`, then checked along with the rest of its line.

| Project | Members flagged by v0.1.0 | Members flagged by v0.1.1 (missing + doubtful) | Bare names flagged by v0.1.1 |
|---|---:|---:|---:|
| cobra | 0 of 12 | 12 of 12 (7 + 5) | 0 of 31 |
| requests | 0 of 17 | 16 of 17 (6 + 10) | 0 of 3 |
| FastAPI | 5 of 129 | 95 of 129 (63 + 32) | 2 of 121 |
| tRPC | 0 of 62 | 56 of 62 (32 + 24) | 3 of 38 |
| **All** | **5 of 220 (2%)** | **179 of 220 (81%)** | **5 of 193 (3%)** |

v0.1.0 flagged no bare names.

Of the 41 members v0.1.1 did not flag:

- 22 are members of a standard library or another package, such as `datetime`, `io`, `json` and Starlette's `Request`. phantom leaves these alone on purpose, since a text can name them without claiming they are in the project.
- 11 sit in sentences that read as suggestions ("replace", "instead", "consider using"), which phantom takes as proposals rather than claims.
- 5 aren't code (`Pipfile.lock`, `uv.lock`).
- 3 hang off names the code only reaches through something else: tRPC's code has `ctx.wssHandler`, never `wssHandler` on its own.

Bare names are the gap. A made-up `sendSafely()` gives phantom nothing to go on unless it is mentioned with one of the project's files, named like the project's code, or one letter away from a real name of 8 or more characters.

These made-up names are all of one kind: a real name with a word added. For the member and file checks, how a name was made up doesn't matter, only whether it is in the code.

## Repeating it

From a checkout of phantom:

```bash
git clone --depth 1 https://github.com/spf13/cobra.git slop/cobra
GOBIN="$PWD/bin/v0.1.0" go install github.com/johnmikel/phantom@v0.1.0
go build -o bin/phantom .
python3 eval/docs.py slop/cobra bin/v0.1.0/phantom bin/phantom '*.md' 'site/content/**/*.md'
python3 eval/mutate.py slop/cobra bin/v0.1.0/phantom bin/phantom slop/mutants-cobra '*.md' 'site/content/**/*.md'
```

The other projects use these patterns: requests `'README.md' 'docs/**/*.rst'`, FastAPI `'docs/en/docs/**/*.md' 'README.md'`, tRPC `'www/docs/**/*.md' 'www/docs/**/*.mdx' 'README.md'`.
