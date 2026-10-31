# phantom on curl's AI slop reports

curl's lead developer keeps [a public list](https://gist.github.com/bagder/07f7581f6e3d78ef37dfbfc81fd1d1cd) of the reports sent to curl's bug bounty on HackerOne that he judged to be AI slop. All 49 on the list are public, so they make a fair test: real reports, written to look convincing, about a large C project with a long history.

## Method

- Each report's text comes from HackerOne's public JSON for it (`https://hackerone.com/reports/<id>.json`), with its title as the first line.
- Each is checked against the curl release that was current on the day it was filed, not today's code, so code that has since moved doesn't count against it.
- phantom was built and tuned on the first 37 reports it could fetch. The other 12 (marked † below) were rate-limited at first and came in later, so they were a test it had not seen.

To repeat it:

```bash
mkdir slop && cd slop
curl -sL https://gist.githubusercontent.com/bagder/07f7581f6e3d78ef37dfbfc81fd1d1cd/raw -o gist.md
grep -oE '^[0-9]+\. .*hackerone.com/reports/[0-9]+' gist.md | grep -oE '[0-9]+$' | while read -r id; do
  curl -s -A "Mozilla/5.0" "https://hackerone.com/reports/$id.json" -o "r$id.json"
  sleep 4
done
git clone https://github.com/curl/curl.git
cd .. && go build -o phantom . && python3 eval/curl.py slop ./phantom
```

## Results

Of 49 reports, citing 609 pieces of code between them:

| | Reports |
|---|---:|
| ❌ cite code that is not in curl | 5 |
| ⚠️ cite code that is there, but not as described | 10 |
| ✅ cite only code that is there | 32 |
| mention no code that can be checked | 2 |

The five that cite code that does not exist:

- **#2199174** calls `curl_easy_setopt_timeout()` and `curl_easy_setopt_custom()`, which curl has never had, and cites commit `9414975`, which is not in its history.
- **#3125820** quotes six lines it calls a "simplified snippet" of `lib/http2.c`. None of them are in curl 8.13.0, and the buffer functions they call left curl in 2020.
- **#3295650** points at `docs/examples/http-auth-example.txt`, a file curl has never had.
- **#3400831** † cites `include/tool_binmode.h`, which has never existed; the macro it is about lives in `lib/curlx/binmode.h`.
- **#3418528** † names `Curl_ldap_err2string`. The real function is `Curl_ldap_err2string_a`, and the line it points at is blank.

The made-up names, paths and commit were also searched for in curl's whole git history (`git log --all`). None of them ever existed.

The ten doubtful ones are mostly line numbers that land on a comment or a blank line, functions named beside a line number that are not near it, and code quoted in pieces. None of that proves a report was made up, but it tells a maintainer where to look first.

## The 12 reports phantom had not seen

On its first run on them, phantom flagged three reports as citing code that is not there. Two were right (#3400831 and #3418528, above). The third was wrong: it took OpenSSL's header `openssl/ssl.h` for a misspelling of curl's `lib/psl.h`. Two rules changed after that run:

- A name one letter away from a real file now counts as a misspelling only if it is at least 8 characters long. `ssl.h` and `psl.h` are too short to tell.
- A path that starts with the project's own name, like `curl/lib/telnet.c`, is read as `lib/telnet.c`. This had marked three paths as doubtful.

The numbers above are from after those changes, on all 49 reports.

## What this shows

phantom finds made-up references quickly: about 2 seconds per report on curl's 4,150 files, with no AI and no network. But most of these reports cite only real code. What makes them slop is the reasoning about that code, and phantom can't judge reasoning. It is a first filter, not a judge.

## Every report

† not seen while phantom was being built.

| Report | Filed | Checked against | References | Result | What phantom flagged |
|---|---|---|---:|---|---|
| [#2199174](https://hackerone.com/reports/2199174) | 2023-10-10 | 8.3.0 | 13 | ❌ cites code that is not there | ❌ `curl_easy_setopt_timeout(handle, timeout)`: named like this project's code, but appears nowhere in it<br>❌ `curl_easy_setopt_custom(handle, option, param)`: named like this project's code, but appears nowhere in it<br>❌ `commit 9414975`: no such commit in this checkout |
| [#2298307](https://hackerone.com/reports/2298307) | 2023-12-28 | 8.5.0 | 3 | ✅ all there |  |
| [#2819666](https://hackerone.com/reports/2819666) | 2024-11-04 | 8.10.1 | 6 | ✅ all there |  |
| [#2823554](https://hackerone.com/reports/2823554) | 2024-11-06 | 8.11.0 | 16 | ✅ all there |  |
| [#2871792](https://hackerone.com/reports/2871792) | 2024-11-30 | 8.11.0 | 4 | ✅ all there |  |
| [#2887487](https://hackerone.com/reports/2887487) | 2024-12-08 | 8.11.0 | 14 | ⚠️ doubtful | ⚠️ `code block at line 36 (c)`: only 4 of its 14 lines appear<br>⚠️ `code block at line 86 (c)`: only 3 of its 11 lines appear |
| [#2905552](https://hackerone.com/reports/2905552) | 2024-12-18 | 8.11.1 | 1 | ✅ all there |  |
| [#2912277](https://hackerone.com/reports/2912277) | 2024-12-23 | 8.11.1 | 0 | no code to check |  |
| [#2981245](https://hackerone.com/reports/2981245) | 2025-02-07 | 8.12.0 | 9 | ✅ all there |  |
| [#3100073](https://hackerone.com/reports/3100073) | 2025-04-18 | 8.13.0 | 8 | ✅ all there |  |
| [#3101127](https://hackerone.com/reports/3101127) | 2025-04-19 | 8.13.0 | 9 | ✅ all there |  |
| [#3116935](https://hackerone.com/reports/3116935) | 2025-04-29 | 8.13.0 | 4 | ✅ all there |  |
| [#3117697](https://hackerone.com/reports/3117697) | 2025-04-29 | 8.13.0 | 5 | ✅ all there |  |
| [#3125820](https://hackerone.com/reports/3125820) | 2025-05-04 | 8.13.0 | 11 | ❌ cites code that is not there | ❌ `code block at line 14 (c)`: none of its 6 lines appear in the repository<br>⚠️ `code block at line 112 (diff)`: its 2 lines do not appear, though so short a quote may be paraphrased |
| [#3125832](https://hackerone.com/reports/3125832) | 2025-05-04 | 8.13.0 | 24 | ✅ all there |  |
| [#3137657](https://hackerone.com/reports/3137657) | 2025-05-09 | 8.13.0 | 16 | ✅ all there |  |
| [#3158093](https://hackerone.com/reports/3158093) | 2025-05-22 | 8.13.0 | 46 | ✅ all there |  |
| [#3230082](https://hackerone.com/reports/3230082) † | 2025-06-30 | 8.14.1 | 21 | ✅ all there |  |
| [#3231321](https://hackerone.com/reports/3231321) † | 2025-07-01 | 8.14.1 | 17 | ✅ all there |  |
| [#3242005](https://hackerone.com/reports/3242005) † | 2025-07-09 | 8.14.1 | 23 | ✅ all there |  |
| [#3249936](https://hackerone.com/reports/3249936) † | 2025-07-13 | 8.14.1 | 6 | ✅ all there |  |
| [#3250490](https://hackerone.com/reports/3250490) | 2025-07-14 | 8.14.1 | 25 | ✅ all there |  |
| [#3272982](https://hackerone.com/reports/3272982) | 2025-07-26 | 8.15.0 | 0 | no code to check |  |
| [#3293884](https://hackerone.com/reports/3293884) | 2025-08-10 | 8.15.0 | 11 | ✅ all there |  |
| [#3295650](https://hackerone.com/reports/3295650) | 2025-08-12 | 8.15.0 | 3 | ❌ cites code that is not there | ❌ `docs/examples/http-auth-example.txt`: no such file |
| [#3335085](https://hackerone.com/reports/3335085) | 2025-09-11 | 8.16.0 | 5 | ✅ all there |  |
| [#3340109](https://hackerone.com/reports/3340109) | 2025-09-16 | 8.16.0 | 21 | ✅ all there |  |
| [#3346118](https://hackerone.com/reports/3346118) | 2025-09-18 | 8.16.0 | 4 | ✅ all there |  |
| [#3392174](https://hackerone.com/reports/3392174) | 2025-10-21 | 8.16.0 | 5 | ⚠️ doubtful | ⚠️ `lib/ws.c:1287`: line 1287 holds no code, only */ |
| [#3395218](https://hackerone.com/reports/3395218) | 2025-10-22 | 8.16.0 | 16 | ⚠️ doubtful | ⚠️ `Progress.c`: no such path, but lib/progress.c differs only in case<br>⚠️ `src/tool_progress.c at line 94`: line 94 is blank<br>⚠️ `code block at line 65`: only 6 of its 10 lines appear |
| [#3395227](https://hackerone.com/reports/3395227) | 2025-10-22 | 8.16.0 | 14 | ⚠️ doubtful | ⚠️ `src/tool_main.c at line 122`: line 122 is strcpy(fname, env);, and memory_tracking_init is not within 3 lines of it |
| [#3399774](https://hackerone.com/reports/3399774) † | 2025-10-25 | 8.16.0 | 8 | ✅ all there |  |
| [#3400831](https://hackerone.com/reports/3400831) † | 2025-10-27 | 8.16.0 | 10 | ❌ cites code that is not there | ❌ `include/tool_binmode.h`: no such file |
| [#3403880](https://hackerone.com/reports/3403880) † | 2025-10-29 | 8.16.0 | 6 | ✅ all there |  |
| [#3418528](https://hackerone.com/reports/3418528) † | 2025-11-10 | 8.17.0 | 12 | ❌ cites code that is not there | ❌ `Curl_ldap_err2string`: named like this project's code, but appears nowhere in it (closest: `Curl_ldap_err2string_a`)<br>⚠️ `packages/OS400/os400sys.c:741`: line 741 is blank |
| [#3418646](https://hackerone.com/reports/3418646) † | 2025-11-10 | 8.17.0 | 10 | ✅ all there |  |
| [#3452725](https://hackerone.com/reports/3452725) | 2025-12-05 | 8.17.0 | 17 | ⚠️ doubtful | ⚠️ `code block at line 140 (c)`: only 1 of its 4 lines appears |
| [#3459636](https://hackerone.com/reports/3459636) | 2025-12-09 | 8.17.0 | 9 | ✅ all there |  |
| [#3462525](https://hackerone.com/reports/3462525) | 2025-12-12 | 8.17.0 | 6 | ✅ all there |  |
| [#3465094](https://hackerone.com/reports/3465094) | 2025-12-15 | 8.17.0 | 22 | ⚠️ doubtful | ⚠️ `file.c:192-193`: lines 192 to 193 hold no code<br>⚠️ `lib/urlapi.c:836`: line 836 is }, and dedotdotify or ISSLASH is not within 3 lines of it<br>⚠️ `lib/urlapi.c:1233`: line 1233 is u->path[pathlen] = 0;, and dedotdotify is not within 3 lines of it<br>⚠️ `lib/file.c:192`: line 192 holds no code, only * FILE does a special check on setting up the connection …<br>⚠️ `code block at line 101 (c)`: only 1 of its 2 lines appears<br>⚠️ `code block at line 107 (c)`: only 2 of its 3 lines appear<br>⚠️ `code block at line 118 (c)`: only 3 of its 6 lines appear |
| [#3466883](https://hackerone.com/reports/3466883) | 2025-12-16 | 8.17.0 | 33 | ✅ all there |  |
| [#3466896](https://hackerone.com/reports/3466896) | 2025-12-16 | 8.17.0 | 16 | ⚠️ doubtful | ⚠️ `code block at line 150 (c)`: its 1 line does not appear, though so short a quote may be paraphrased |
| [#3468410](https://hackerone.com/reports/3468410) | 2025-12-17 | 8.17.0 | 4 | ✅ all there |  |
| [#3470095](https://hackerone.com/reports/3470095) | 2025-12-18 | 8.17.0 | 15 | ✅ all there |  |
| [#3473182](https://hackerone.com/reports/3473182) | 2025-12-20 | 8.17.0 | 11 | ✅ all there |  |
| [#3506159](https://hackerone.com/reports/3506159) | 2026-01-10 | 8.18.0 | 34 | ⚠️ doubtful | ⚠️ `lib/http2.c:1642`: line 1642 is h = curl_maprintf("%s:%s", name, value);, and on_header is not within 3 lines of it |
| [#3514263](https://hackerone.com/reports/3514263) † | 2026-01-17 | 8.18.0 | 6 | ✅ all there |  |
| [#3516186](https://hackerone.com/reports/3516186) † | 2026-01-19 | 8.18.0 | 15 | ⚠️ doubtful | ⚠️ `code block at line 40 (c)`: its 1 line does not appear, though so short a quote may be paraphrased |
| [#3516202](https://hackerone.com/reports/3516202) † | 2026-01-19 | 8.18.0 | 15 | ⚠️ doubtful | ⚠️ `lib/cookie.c:915`: line 915 is co->creationtime = repl->creationtime;, and replace_existing is not within 3 lines of it<br>⚠️ `lib/cookie.c:1234`: line 1234 is blank<br>⚠️ `lib/http.c:567`: line 567 is blank |
