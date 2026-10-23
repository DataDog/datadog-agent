#!/usr/bin/env python3
"""Generate dd-source catalog fixtures (the expected half) from the REAL BuildIssue.

Usage: python3 -I scripts/gen_catalog_fixture.py --spec spec.json --worktree <agent worktree>
           --cases cases.json --out <dir> [--label LABEL] [--no-run]

cases.json: [{"context": {"target": "/x", "error": "boom"}}]   (static template: no label, no suffix)
            [{"label": "linux", "context": {...}}, ...]       (variant template: label = variant value)
Writes <out>/<issue_type>[.<label>].md in the dd-source catalog README fixture format (front matter =
reporter `extra`, body = resolved fields). A case without its own "label" gets --label (default: none, so a
static template yields `<issue_type>.md`, the README convention). A per-case label always wins. Copy the files to
domains/evp-workers/apps/agthealth-worker/src/catalog/testdata/, then run `npm run gen-templates` twice.

How: renders references/templates/common/gen_catalog_fixture_test.go.tmpl into the package as a temporary
zz_gen_catalog_fixture_test.go (build tag genfixture), runs `go test -tags genfixture,test` for that one test,
and always deletes the temp file. This is the one place raw `go test` is used (a scratch generator, never
committed); product tests still go through `dda inv test`.
Fixtures come from the reporter so agent and catalog text cannot drift (docker_file_tailing_disabled did).
"""
import argparse
import importlib.util
import json
import os
import subprocess
import sys
from pathlib import Path

sys.dont_write_bytecode = True  # never write __pycache__ into the skill dir

SKILL = Path(__file__).resolve().parent.parent
TEMP_NAME = "zz_gen_catalog_fixture_test.go"


def load_renderer():
    spec = importlib.util.spec_from_file_location("render_templates", SKILL / "scripts" / "render_templates.py")
    mod = importlib.util.module_from_spec(spec)
    sys.modules["render_templates"] = mod
    spec.loader.exec_module(mod)
    return mod


def quoting_warnings(out_dir):
    """Flag front-matter metadata values the catalog loader could misparse.

    The fixture front matter is YAML; each metadata value is parsed as JSON when it can be. A string value
    that is not JSON-quoted but contains ': ', ' #', a leading indicator (one of {[&*!|>%@'"), or leading or
    trailing spaces will not round-trip. Template placeholders that quote a value (for example "{{path}}")
    also break when the value contains a double quote: keep values quote-free or escape them."""
    warns = []
    for f in sorted(Path(out_dir).glob("*.md")):
        lines = f.read_text(errors="replace").splitlines()
        if not lines or lines[0].strip() != "---":
            continue
        for ln in lines[1:]:
            if ln.strip() == "---":
                break
            s = ln.strip()
            if ":" not in s or s.startswith(("metadata:", "#")):
                continue
            key, _, val = s.partition(":")
            val = val.strip()
            if not val or val[0] == '"':
                if val and val.count('"') > 2 and '\\"' not in val:
                    warns.append(f"{f.name}: value of '{key}' contains an unescaped double quote")
                continue
            if ": " in val or " #" in val or val[0] in "{[&*!|>%@'":
                warns.append(f"{f.name}: value of '{key}' is not JSON-quoted but contains characters YAML may misparse: {val[:60]!r}")
    return warns


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--spec", required=True)
    ap.add_argument("--worktree", required=True)
    ap.add_argument("--cases", required=True)
    ap.add_argument("--out", required=True)
    ap.add_argument("--label", default="", help="label for cases that have none (default: no suffix)")
    ap.add_argument("--no-run", action="store_true", help="render and clean up only (self-test)")
    args = ap.parse_args(argv)

    rt = load_renderer()
    spec = json.loads(Path(args.spec).read_text())
    values, *_ = rt.build_values(spec)
    wt = Path(args.worktree).resolve()
    pkgdir = wt / "comp" / "healthplatform" / "issues" / values["PKG_DIR"]
    if not pkgdir.is_dir():
        raise SystemExit(f"package dir not found: {pkgdir}")
    raw_cases = json.loads(Path(args.cases).read_text())
    effective = [{**c, "label": c["label"] if "label" in c else args.label} for c in raw_cases]
    import tempfile
    cases_tmp = tempfile.NamedTemporaryFile("w", suffix=".json", delete=False)
    json.dump(effective, cases_tmp)
    cases_tmp.close()
    cases = Path(cases_tmp.name)
    out = Path(args.out).resolve()
    out.mkdir(parents=True, exist_ok=True)

    tmp = pkgdir / TEMP_NAME
    tmpl = (SKILL / "references/templates/common/gen_catalog_fixture_test.go.tmpl").read_text()
    tmp.write_text(rt.render(tmpl, values, "gen_catalog_fixture_test.go"))
    try:
        if args.no_run:
            print(json.dumps({"rendered": str(tmp), "ran": False, "cases": effective}))
        else:
            env = dict(os.environ)
            env["PATH"] = f"/opt/homebrew/opt/go@1.25/bin:{Path.home()}/go/bin:" + env.get("PATH", "")
            env["FIXTURE_CASES"] = str(cases)
            env["FIXTURE_OUT"] = str(out)
            r = subprocess.run(["go", "test", "-tags", "genfixture,test", "-run", "TestGenCatalogFixtures", "-count=1",
                                f"./comp/healthplatform/issues/{values['PKG_DIR']}/"],
                               cwd=wt, env=env, capture_output=True, text=True)
            sys.stdout.write(r.stdout[-2000:])
            sys.stderr.write(r.stderr[-2000:])
            written = sorted(p.name for p in out.glob("*.md"))
            warns = quoting_warnings(out)
            for w in warns:
                sys.stderr.write("gen_catalog_fixture: warning: " + w + "\n")
            print(json.dumps({"ran": True, "returncode": r.returncode, "fixtures": written, "quoting_warnings": warns}))
            if r.returncode != 0:
                return r.returncode
    finally:
        if tmp.exists():
            tmp.unlink()
        cases.unlink(missing_ok=True)
    return 0


if __name__ == "__main__":
    sys.exit(main())
