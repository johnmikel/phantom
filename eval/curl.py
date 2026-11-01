"""Runs phantom on the AI slop reports sent to curl's bug bounty.

Each report is checked against the curl release that was current on the day
it was filed. See docs/curl-evaluation.md for how to fetch the reports.

    python3 eval/curl.py SLOP_DIR PATH_TO_PHANTOM

SLOP_DIR holds the reports as r<id>.json and a clone of curl in curl/.
"""
import glob
import json
import os
import subprocess
import sys

slop, phantom = sys.argv[1], sys.argv[2]
curl = os.path.join(slop, "curl")
tags = subprocess.run(["git", "-C", curl, "for-each-ref", "--sort=creatordate",
                       "--format=%(refname:short) %(creatordate:short)", "refs/tags/curl-*"],
                      capture_output=True, text=True, check=True).stdout.split("\n")
tags = [t.split() for t in tags if t.strip()]
rows = []
for f in sorted(glob.glob(os.path.join(slop, "r[0-9]*.json"))):
    try:
        d = json.load(open(f))
    except ValueError:
        print(f"skipping {f}: not JSON (HackerOne may have rate-limited it)", file=sys.stderr)
        continue
    text = d.get("vulnerability_information") or ""
    if not text:
        continue
    day = d["created_at"][:10]
    tag = [t for t, dt in tags if dt <= day][-1]
    subprocess.run(["git", "-C", curl, "-c", "advice.detachedHead=false", "checkout", "-q", tag], check=True)
    md = os.path.join(slop, f"r{d['id']}.md")
    open(md, "w").write("# " + d["title"] + "\n\n" + text)
    out = subprocess.run([phantom, "-repo", curl, "-format", "json", md], capture_output=True, text=True).stdout
    rep = json.loads(out)
    rows.append({"id": d["id"], "day": day, "tag": tag, "mentioned": rep["mentioned"],
                 "missing": [(r["text"], r["note"], r.get("closest")) for r in rep["results"] if r["status"] == "missing"],
                 "doubtful": [(r["text"], r["note"]) for r in rep["results"] if r["status"] == "doubtful"]})
json.dump(rows, open(os.path.join(slop, "results.json"), "w"), indent=1)
print(f"{len(rows)} reports, {sum(r['mentioned'] for r in rows)} references; "
      f"{sum(1 for r in rows if r['missing'])} cite code that does not exist; "
      f"{sum(1 for r in rows if r['doubtful'] and not r['missing'])} more have doubtful references; "
      f"{sum(1 for r in rows if r['mentioned'] == 0)} mention no checkable code")
for r in rows:
    mark = "MISSING" if r["missing"] else ("doubt" if r["doubtful"] else "ok")
    print(f"{r['day']} #{r['id']} {r['tag']:<12} {mark:<7} {r['mentioned']:>2} refs")
    for t, n, c in r["missing"]:
        print(f"      x {t[:45]:<45} {n}" + (f" (closest: {', '.join(c)})" if c else ""))
    for t, n in r["doubtful"]:
        print(f"      ~ {t[:45]:<45} {n}")
