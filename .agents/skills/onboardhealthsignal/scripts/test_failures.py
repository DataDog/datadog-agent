#!/usr/bin/env python3
"""Extract failing test names from `dda inv test` logs and compare a dirty run with a clean baseline.

Usage: python3 -I scripts/test_failures.py <dirty.log> [<clean.log>] [--json]

One log: list its failures. Two logs: NEW = failures only in <dirty.log> (caused by your change),
baseline = failures in both (pre-existing at that SHA), fixed = only in <clean.log>.
Exit code 1 when there are NEW failures (two logs) or any failure (one log), else 0.
Names come from gotestsum `=== FAIL: <pkg> <Test> (...)` lines, with a fallback to Bazel
`//target FAILED` lines. ANSI codes are ignored.
"""
import json
import re
import sys

sys.dont_write_bytecode = True

ANSI = re.compile(r"\x1b\[[0-9;]*m|\[\d+(?:;\d+)*m")
GOTESTSUM = re.compile(r"^=== FAIL: (\S+) (\S+) \(")
BAZEL = re.compile(r"^(//\S+)\s+(?:FAILED|TIMEOUT|NO STATUS)\b")
SCHEMA_HINT = "run `dda inv schema.compress`"


def failures(text: str):
    clean = ANSI.sub("", text)
    found = set()
    for line in clean.splitlines():
        m = GOTESTSUM.match(line)
        if m:
            found.add(f"{m.group(1)} :: {m.group(2)}")
    if not found:
        for line in clean.splitlines():
            m = BAZEL.match(line)
            if m:
                found.add(f"{m.group(1)} :: (target)")
    return found


def main(argv=None):
    args = [a for a in (argv if argv is not None else sys.argv[1:]) if a != "--json"]
    as_json = "--json" in (argv if argv is not None else sys.argv[1:])
    if not 1 <= len(args) <= 2:
        print(__doc__, file=sys.stderr)
        return 2
    texts = [open(a, errors="replace").read() for a in args]
    dirty = failures(texts[0])
    hint = any(SCHEMA_HINT in t for t in texts)
    if len(args) == 1:
        out = {"failures": sorted(dirty)}
        rc = 1 if dirty else 0
    else:
        clean = failures(texts[1])
        out = {"new": sorted(dirty - clean), "baseline": sorted(dirty & clean), "fixed": sorted(clean - dirty)}
        rc = 1 if out["new"] else 0
    if as_json:
        print(json.dumps(out, indent=2))
        return rc
    if len(args) == 1:
        print(f"failures ({len(out['failures'])}):")
        for n in out["failures"]:
            print("  - " + n)
    else:
        for key, label in (("new", "NEW failures"), ("baseline", "baseline failures"), ("fixed", "fixed vs baseline")):
            print(f"{label} ({len(out[key])}):")
            for n in out[key]:
                print("  - " + n)
    if hint:
        print("hint: a log reports a missing embedded schema; run `dda inv schema.compress` once in the worktree "
              "(the tool's own hint, a generated artifact) and rerun before treating these as real failures.")
    return rc


if __name__ == "__main__":
    sys.exit(main())
