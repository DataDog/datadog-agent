#!/usr/bin/env python3
"""Generate the PR 'how you validated your changes' bullets from FINAL logs only (never from memory).

Usage: python3 -I scripts/verification_bullets.py [--test-log "label=path" ...] [--lint-log path]
           [--check-json path] [--registration-log path]

Every bullet is parsed from a log or JSON file you pass, so a claim can never outlive the run that produced it:
skipped tests are counted and called out, a log with no result line is UNKNOWN (never PASSED), and no QA label
is ever suggested. Re-run it after the last code change and paste the output.
"""
import argparse
import importlib.util
import json
import re
import sys
from pathlib import Path

sys.dont_write_bytecode = True

ANSI = re.compile(r"\x1b\[[0-9;]*m|\[\d+(?:;\d+)*m")


def load_failures():
    spec = importlib.util.spec_from_file_location("test_failures", Path(__file__).resolve().parent / "test_failures.py")
    mod = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(mod)
    return mod.failures


def read(path):
    try:
        return ANSI.sub("", Path(path).read_text(errors="replace"))
    except OSError as e:
        raise SystemExit(f"verification_bullets: cannot read {path}: {e}")


def test_bullet(label, text, failures):
    names = sorted(failures(text))
    done = re.search(r"DONE (\d+) tests?(?:, (\d+) skipped)?(?:, (\d+) failures?)?", text)
    executed = re.search(r"Executed (\d+) out of (\d+) tests?", text)
    target_failed = bool(re.search(r"(?m)^//\S+\s+FAILED\b", text))
    skipped = 0
    if done:
        n, skipped, f = int(done.group(1)), int(done.group(2) or 0), int(done.group(3) or 0)
        detail = f"{n} tests" + (f", {skipped} skipped" if skipped else "") + (f", {f} failures" if f else "")
        status = "FAILED" if (f or names or target_failed) else "PASSED"
    elif executed:
        detail = f"{executed.group(1)} of {executed.group(2)} tests"
        status = "FAILED" if (names or target_failed) else "PASSED"
    else:
        return [f"- {label}: UNKNOWN (no result line found in the log; do not claim it passed)"]
    out = [f"- {label}: {status} ({detail})"]
    out += [f"  - failed: {n}" for n in names]
    if skipped:
        out.append(f"  - {skipped} skipped were not run (say so; do not claim they ran)")
    return out


def lint_bullet(text):
    if re.search(r"(?m)^0 issues\.?", text):
        return ["- linter: 0 issues"]
    m = re.search(r"(?m)^(\d+) issues?:", text)
    if not m:
        return ["- linter: UNKNOWN (no result line found in the log)"]
    lines = re.findall(r"(?m)^(\S+\.go:\d+:\d+: .+)$", text)[:5]
    return [f"- linter: {m.group(1)} issue(s)"] + [f"  - {l}" for l in lines]


def check_bullet(path):
    data = json.loads(Path(path).read_text())
    res = data["results"]
    count = lambda s: sum(1 for r in res if r["status"] == s)
    out = [f"- static checks: {count('pass')} passed, {count('warn')} warning(s), {count('fail')} failed, {count('skip')} skipped"]
    for r in res:
        if r["status"] in ("warn", "skip", "fail"):
            out.append(f"  - {r['status']}: {r['name']} ({r['detail'][:120]})")
    return out


def registration_bullet(text, label="registration test"):
    """Quote the final '--- PASS/FAIL: TestName' line; with several (mutated + unmutated runs) the last one wins."""
    found = re.findall(r"(?m)^\s*--- (PASS|FAIL): (Test\w+)", text)
    if not found:
        return [f"- {label}: UNKNOWN (no --- PASS/FAIL line; run with -test.v and confirm the test name)"]
    return [f"- {label}: --- {found[-1][0]}: {found[-1][1]}"]


def registration_path(path):
    """--registration-log may be the log itself or the verify_all out dir (which holds registration.log)."""
    p = Path(path)
    return p / "registration.log" if p.is_dir() else p


def split_spec(spec):
    """Split 'LABEL=PATH' where LABEL may itself contain '=' (for example 'go test -tags=x pkg').

    Try each '=' from the right and take the first whose right side is an existing file; fall back to the
    last '=' (so a missing file still reports a clear 'cannot read' for the path part)."""
    idxs = [i for i, ch in enumerate(spec) if ch == "="]
    if not idxs:
        raise SystemExit(f"verification_bullets: --test-log needs LABEL=PATH, got {spec!r}")
    for i in reversed(idxs):
        if Path(spec[i + 1:]).exists():
            return spec[:i], spec[i + 1:]
    i = idxs[-1]
    return spec[:i], spec[i + 1:]


def verify_bullets(path):
    """Bullets from a verify_all.sh summary.json: every gate status, mutation evidence, catalog caveat."""
    data = json.loads(Path(path).read_text())
    out = []
    names = {"gazelle": "gazelle", "pkg_tests": "package tests", "modified_tests": "modified-packages tests",
             "lint": "linter", "registration": "registration test (unmutated)", "deps_delta": "cluster agent dependency delta", "mutation": "mutation check",
             "baseline": "baseline comparison", "catalog": "catalog gates (gen-templates, prettier, jest)"}
    for g in data.get("gates", []):
        label = names.get(g["name"], g["name"])
        status = {"pass": "PASSED", "fail": "FAILED", "skip": "NOT RUN", "plan": "NOT RUN (dry run)"}.get(g["status"], g["status"].upper())
        if g["name"] == "mutation" and g.get("mutants"):  # the gate status derives from the mutant table
            status = "PASSED" if all(m["status"] == "KILLED" for m in g["mutants"]) else "FAILED"
        secs = f" in {g['seconds']}s" if g.get("seconds") else ""
        out.append(f"- {label}: {status}{secs}")
        if g["name"] == "registration" and g["status"] in ("pass", "fail") and g.get("log"):
            try:
                out += ["  " + l for l in registration_bullet(read(g["log"]), "result")]
            except SystemExit:
                out.append("  - result: UNKNOWN (registration log unreadable)")
        if g["name"] == "mutation" and g.get("mutants"):
            out += [f"  - mutant {m['name']}: {m['status']}" + (f" ({m['note']})" if m.get("note") else "") for m in g["mutants"]]
            bad = [m["name"] for m in g["mutants"] if m["status"] != "KILLED"]
            if bad:
                out.append("  - NOT killed by a test (list in the PR body): " + ", ".join(bad))
        if g["name"] == "mutation" and g.get("log"):
            try:
                obs = [l.strip() for l in Path(g["log"]).read_text(errors="replace").splitlines() if l.startswith("MUTATION")]
            except OSError:
                obs = []
            out += [f"  - {o}" for o in obs[:4]]
    if any(g["name"] == "catalog" and g["status"] != "skip" for g in data.get("gates", [])):
        out.append("- eslint was NOT run (a no-op shim lets turbo's tsc checks run); CI lints through Bazel")
    out.append(f"- dda inv test flags used: {data.get('dda_test_style', 'unknown')}")
    return out


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--test-log", action="append", default=[], metavar="LABEL=PATH")
    ap.add_argument("--lint-log")
    ap.add_argument("--check-json")
    ap.add_argument("--registration-log", help="registration.log from verify_all.sh (or its out dir)")
    ap.add_argument("--verify-json", help="summary.json written by scripts/verify_all.sh")
    args = ap.parse_args(argv)
    failures = load_failures()
    lines = []
    if args.verify_json:
        lines += verify_bullets(args.verify_json)
    for spec in args.test_log:
        label, path = split_spec(spec)
        lines += test_bullet(label.strip(), read(path), failures)
    if args.registration_log:
        lines += registration_bullet(read(registration_path(args.registration_log)))
    if args.lint_log:
        lines += lint_bullet(read(args.lint_log))
    if args.check_json:
        lines += check_bullet(args.check_json)
    print("\n".join(lines))
    return 0


if __name__ == "__main__":
    sys.exit(main())
