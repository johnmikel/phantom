"""Runs two phantom builds on a project's own docs, checked against its own
code, and prints what the second flags that the first did not.

    python3 eval/docs.py REPO OLD_PHANTOM NEW_PHANTOM 'docs/**/*.md' ...

Docs that ship with the code should cite code that is there, so what is
flagged is mostly false alarms, or docs gone out of date.
"""
import json, subprocess, sys, glob, os, collections
repo, old, new = sys.argv[1], sys.argv[2], sys.argv[3]
patterns = sys.argv[4:]
files = sorted({f for p in patterns for f in glob.glob(os.path.join(repo, p), recursive=True)})
def run(binary):
    out = subprocess.run([binary, "-repo", repo, "-format", "json", *files], capture_output=True, text=True).stdout
    dec, i, reports = json.JSONDecoder(), 0, []
    while i < len(out):
        while i < len(out) and out[i].isspace():
            i += 1
        if i >= len(out):
            break
        obj, i = dec.raw_decode(out, i)
        reports.append(obj)
    return reports
a, b = run(old), run(new)
counts = collections.Counter()
for ra, rb in zip(a, b):
    src = os.path.relpath(rb["source"], repo)
    before = {(r["text"], r["status"]) for r in ra["results"]}
    for r in rb["results"]:
        counts[r["status"]] += 1
        if r["status"] in ("missing", "doubtful") and (r["text"], r["status"]) not in before:
            print(f"NEW {r['status']:<8} {src}:{r['at']}  {r['text'][:60]}  -- {r['note']}" + (f" (closest: {', '.join(r['closest'])})" if r.get('closest') else ""))
print(f"{len(files)} files; new build: " + ", ".join(f"{k} {v}" for k, v in sorted(counts.items())))
print("old build missing:", sum(1 for r in a for x in r["results"] if x["status"] == "missing"),
      " new build missing:", sum(1 for r in b for x in r["results"] if x["status"] == "missing"))
