"""Swaps one real name in a project's docs for a made-up one, and counts how
often each of two phantom builds notices.

    python3 eval/mutate.py REPO OLD_PHANTOM NEW_PHANTOM OUT_DIR 'docs/**/*.md' ...

Each mutant is one line of the docs with the last part of one code span
renamed, as Session.send() becomes Session.sendSafely(). Up to 150 members
(a.b) and 150 bare calls (b()) are picked at random, and only names phantom
finds in the code count. The mutants are written to OUT_DIR.
"""
import json, subprocess, sys, glob, os, re, random, shutil, collections
repo, old, new, out = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
patterns = sys.argv[5:]
random.seed(7)
files = sorted({f for p in patterns for f in glob.glob(os.path.join(repo, p), recursive=True)})
span = re.compile(r"`([A-Za-z_]\w*(?:(?:\.|::|->)[A-Za-z_]\w*)*)(\(\))?`")
cands = []
for f in files:
    fence = False
    for n, line in enumerate(open(f, errors="replace").read().split("\n"), 1):
        if line.lstrip().startswith(("```", "~~~")):
            fence = not fence
        if fence:
            continue
        for m in span.finditer(line):
            name = m.group(1)
            last = re.split(r"\.|::|->", name)[-1]
            if len(last) < 4 or not (m.group(2) or re.search(r"\.|::|->", name)):
                continue  # only calls and qualified names
            kind = "qualified" if re.search(r"\.|::|->", name) else "bare"
            cands.append((f, n, line, m, kind))
random.shuffle(cands)
picked, seen = [], set()
for c in cands:
    key = (c[0], c[1])
    if key not in seen and sum(1 for p in picked if p[4] == c[4]) < 150:
        seen.add(key); picked.append(c)
shutil.rmtree(out, ignore_errors=True); os.makedirs(out)
orig_files, mut_files, meta = [], [], []
for i, (f, n, line, m, kind) in enumerate(picked):
    name = m.group(1)
    parts = re.split(r"(\.|::|->)", name)
    last = parts[-1]
    fake = last + ("_safely" if "_" in last or last.islower() else "Safely")
    mutated_name = "".join(parts[:-1]) + fake
    mutated = line[:m.start(1)] + mutated_name + line[m.end(1):]
    a, b = os.path.join(out, f"o{i}.md"), os.path.join(out, f"m{i}.md")
    open(a, "w").write(line + "\n"); open(b, "w").write(mutated + "\n")
    orig_files.append(a); mut_files.append(b)
    meta.append((kind, name, mutated_name, fake, os.path.relpath(f, repo), n))
def run(binary, paths):
    res = subprocess.run([binary, "-repo", repo, "-format", "json", *paths], capture_output=True, text=True).stdout
    dec, j, reps = json.JSONDecoder(), 0, []
    while j < len(res):
        while j < len(res) and res[j].isspace(): j += 1
        if j >= len(res): break
        o, j = dec.raw_decode(res, j); reps.append(o)
    return reps
def status(rep, text):
    for r in rep["results"]:
        if r["kind"] == "symbol" and (r["text"].startswith(text) or r["name"] == re.split(r"\.|::|->", text)[-1]):
            return r["status"], r["note"]
    return "absent", ""
base = run(new, orig_files)
tally = collections.Counter()
examples = collections.defaultdict(list)
roots = collections.Counter()
for build, label in ((old, "v0.1.0"), (new, "new")):
    reps = run(build, mut_files)
    for (kind, name, mname, fake, src, n), rb, rm in zip(meta, base, reps):
        if status(rb, name)[0] != "exists":
            continue  # the original is not found as real, so a mutant proves nothing
        st, note = status(rm, mname)
        tally[(label, kind, "total")] += 1
        if st == "missing":
            tally[(label, kind, "caught")] += 1
        elif st == "doubtful":
            tally[(label, kind, "doubtful")] += 1
        elif label == "new":
            if len(examples[kind]) < 4:
                examples[kind].append(f"{src}:{n} `{mname}` -> {st}: {note}")
            if st != "doubtful":
                why = "proposed" if note.startswith("proposed") else ("other" if st != "foreign" else note.split(",")[0])
                root = re.split(r"\.|::|->", mname)[0] if kind == "qualified" else ""
                tally[("why", kind, why)] += 1
                if kind == "qualified" and why.startswith("appears nowhere"):
                    roots[root] += 1
for kind in ("qualified", "bare"):
    for label in ("v0.1.0", "new"):
        t, c = tally[(label, kind, "total")], tally[(label, kind, "caught")]
        d = tally[(label, kind, "doubtful")]
        print(f"  {kind:<9} {label:<6} missing {c:>3}, doubtful {d:>3}, of {t:>3}" + (f"  ({100*c//t}% + {100*d//t}%)" if t else ""))
for (a, kind, why), c in sorted(tally.items()):
    if a == "why":
        print(f"  missed {kind}: {c:>3} {why}")
print("  roots of missed members:", ", ".join(f"{r} {c}" for r, c in roots.most_common()))
for kind, ex in examples.items():
    print(f"  missed {kind}, for example:")
    for e in ex:
        print("    " + e)
