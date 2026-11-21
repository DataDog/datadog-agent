#!/usr/bin/env python3
"""Static hard checks for a new Agent Health signal (SPEC section 7, checks 1-38).

Usage:
  python3 -I scripts/check_signal.py --worktree <datadog-agent worktree> --pkg <issues pkg dir> [--json]
      [--path A|B] [--ack-drop-category] [--owner-file F ...] [--pr-body F] [--names "A B,C D"]
      [--catalog-dir D --generated-dir D2] [--gazelle]
      [--detection todo|implemented|partial] [--catalog-shim] [--tag-sets TAGS ...] [--skip-build]

--detection enforces checks 24 and 25 (leftover scaffold markers, skipped scaffold tests) and the partial
clause of check 33; --catalog-shim says a no-op eslint shim was used for the catalog jest gate (check 33).

Exit code 1 when any check fails. Stdlib only. Checks are static (regex plus brace matching);
they gate, they do not replace `dda inv test` / `dda inv linter.go`.
"""
import argparse
import json
import re
import subprocess
import sys
from dataclasses import dataclass, asdict
from pathlib import Path

sys.dont_write_bytecode = True  # never write __pycache__ into the skill dir

DROP_CATEGORIES = {"configuration", "autodiscovery"}  # prod drop flag, verify with check-mosaic-drift
NAME_RE = re.compile(r"^[A-Z][a-zA-Z0-9 -]*$")
KEBAB_RE = re.compile(r"^[a-z0-9]+(-[a-z0-9]+)*$")
SNAKE_RE = re.compile(r"^[a-z][a-z0-9_]*$")
REQUIRED_ISSUE_FIELDS = ["IssueName", "IssueType", "Title", "Description", "Category",
                         "Location", "Severity", "Source", "Remediation"]
DIAG_HINTS = ("`", "datadog-agent ", "agent status", "diagnose", "kubectl", "systemctl", "docker ")
EM_DASH = chr(0x2014)


@dataclass
class Result:
    id: int
    name: str
    status: str  # pass | fail | warn | skip
    detail: str = ""


# ---------- helpers ----------

def pkg_dir(worktree: Path, pkg: str) -> Path:
    return worktree / "comp" / "healthplatform" / "issues" / pkg


def go_files(d: Path, tests=False):
    out = {}
    if not d.is_dir():
        return out
    for p in sorted(d.glob("*.go")):
        is_test = p.name.endswith("_test.go")
        if is_test == tests:
            out[p.name] = p.read_text()
    return out


def detect_path(d: Path) -> str:
    mod = d / "module.go"
    if mod.exists() and "RegisterModuleFactory" in mod.read_text():
        return "A"
    return "B"


def match_brace(text: str, open_idx: int) -> int:
    """Index of the brace closing the one at open_idx (ignores braces in strings/comments loosely)."""
    depth, i, n = 0, open_idx, len(text)
    in_str = None
    while i < n:
        c = text[i]
        if in_str:
            if c == "\\" and in_str != "`":
                i += 2
                continue
            if c == in_str:
                in_str = None
        else:
            if c in "\"`'":
                in_str = c
            elif c == "/" and text[i:i + 2] == "//":
                nl = text.find("\n", i)
                i = n if nl < 0 else nl
                continue
            elif c == "{":
                depth += 1
            elif c == "}":
                depth -= 1
                if depth == 0:
                    return i
        i += 1
    return n


def literals(text: str, header_re: str):
    """Yield bodies of composite literals whose header matches header_re (header ends with '{')."""
    for m in re.finditer(header_re, text):
        start = m.end() - 1
        end = match_brace(text, start)
        yield text[start + 1:end]


def const_strings(text: str):
    return {m.group(1): m.group(2) for m in re.finditer(r'(?m)^\s*(?:const\s+)?(\w+)\s*=\s*"([^"]*)"', text)}


def strip_comments(text: str) -> str:
    """Drop // line comments (keeps strings intact enough for the regex heuristics here)."""
    return re.sub(r"(?m)^\s*//.*$|(?<=[\s;{}),])//[^\n\"`]*$", "", text)


def all_src(d: Path) -> str:
    return "\n".join(go_files(d).values())


def prefixed(text: str, suffix: str):
    """Map prefix -> literal value for consts like <Prefix>IssueName = "..."."""
    out = {}
    for m in re.finditer(r'(?m)^\s*(\w*?)%s\s*=\s*"([^"]+)"' % suffix, text):
        out[m.group(1)] = m.group(2)
    return out


# ---------- checks ----------

def c1(d, a):
    names = prefixed(a["src"], "IssueName")
    if not names:
        return Result(1, "issue_name_regex", "fail", "no IssueName const with string literal found")
    bad = [f"{v!r}" for v in names.values() if not NAME_RE.match(v)]
    return Result(1, "issue_name_regex", "fail" if bad else "pass",
                  f"violates ^[A-Z][a-zA-Z0-9 -]*$: {', '.join(bad)}" if bad else f"{len(names)} name(s) ok")


def c2(d, a):
    names, types = prefixed(a["src"], "IssueName"), prefixed(a["src"], "IssueType")
    problems = []
    for pre, name in names.items():
        want = name.lower().replace(" ", "_")
        got = types.get(pre)
        if got is None:
            problems.append(f"missing {pre}IssueType const")
        elif got != want:
            problems.append(f"{pre}IssueType={got!r} want {want!r}")
    issue_src = "\n".join(t for n, t in a["files"].items() if n != "module.go" or a["path"] == "B")
    if not re.search(r"\bIssueType:\s*\w+|\.IssueType\s*=\s*\w+", issue_src) \
            and not re.search(r"\w+\.BuildIssue\(", issue_src):
        problems.append("BuildIssue does not set IssueType")
    if a["path"] == "A":
        mod = a["files"].get("module.go", "")
        if not re.search(r"\)\s*IssueType\(\)\s*string", mod):
            problems.append("module.go lacks IssueType() string method")
    return Result(2, "issue_type_derivation", "fail" if problems else "pass", "; ".join(problems))


def c3(d, a):
    ids = prefixed(a["src"], "IssueID")
    if not ids:
        return Result(3, "issue_id_scope", "fail", "no IssueID base const found")
    bad = [v for v in ids.values() if not KEBAB_RE.match(v)]
    if bad:
        return Result(3, "issue_id_scope", "fail", f"base not kebab-case: {bad}")
    src = a["src"]
    if re.search(r"IssueID:\s+[\w.]*IssueID\s*[,}\n]", src):
        return Result(3, "issue_id_scope", "fail", "report uses bare IssueID const; scope per host/instance")
    scoped = (re.search(r"IssueDiscriminator|instanceIssueID", src) or re.search(r'IssueID\s*\+\s*":"', src)
              or re.search(r'Sprintf\("%s:[^"]*",\s*[\w.]*IssueID', src))
    if a["path"] == "B":
        return Result(3, "issue_id_scope", "pass", "base ok; Path B caller appends instance suffix (verify in owner)")
    if scoped:
        return Result(3, "issue_id_scope", "pass", "base kebab and scoped ID construction found")
    return Result(3, "issue_id_scope", "warn", "base ok but no per-host/instance scoping found")


def c4(d, a):
    problems, warns = [], []
    for name, text in a["files"].items():
        if name in ("check.go", "check_noop.go"):
            continue
        text = strip_comments(text)
        for m in re.finditer(r"(?<![A-Za-z])Id:\s", text):
            problems.append(f"{name}: BuildIssue sets Id")
        if re.search(r"\.Id\s*=[^=]", text):
            problems.append(f"{name}: assigns .Id")
    src = a["src"]
    accesses = len(re.findall(r"\bcontext\[[^\]]+\]", src))
    defaults = len(re.findall(r'==\s*""', src))
    if accesses > defaults:
        warns.append(f"{accesses} context reads but {defaults} empty-string defaults")
    if re.search(r'\bcontext\["', src):
        warns.append("context keys read as string literals; declare as consts")
    if problems and a["path"] == "A" and not builtin_present(d):
        warns = problems + warns
        problems = []
        warns.insert(0, "externally reported module sets Id in BuildIssue (documented exception when it has no built-in check)")
    if problems:
        return Result(4, "buildissue_contract", "fail", "; ".join(problems))
    return Result(4, "buildissue_contract", "warn" if warns else "pass", "; ".join(warns))


HEALTHY_RE = re.compile(r"return\s+nil,\s*nil|return\s+\[\]runnerdef\.IssueReport\{\},\s*nil|return\s+reports?,\s*nil")


def c5(d, a, owner_files):
    if a["path"] == "B":
        if not owner_files:
            return Result(5, "healthy_resolve", "skip", "Path B: pass --owner-file to verify owner calls ResolveIssue")
        txt = "\n".join(Path(f).read_text() for f in owner_files)
        if "ReportIssue" in txt and "ResolveIssue" not in txt and "ResolveAllIssues" not in txt:
            return Result(5, "healthy_resolve", "fail", "owner reports but never resolves")
        return Result(5, "healthy_resolve", "pass", "owner resolves")
    mod = a["files"].get("module.go", "")
    chk = a["files"].get("check.go", "") + "\n" + mod
    if not builtin_present(d):
        return Result(5, "healthy_resolve", "skip", "no built-in check (externally reported); owner must resolve")
    if not HEALTHY_RE.search(chk):
        return Result(5, "healthy_resolve", "fail", "no `return nil, nil` healthy path; scheduler cannot resolve")
    return Result(5, "healthy_resolve", "pass", "healthy path returns nil, nil")


def c6(d, a, worktree, pkg):
    bundle = (worktree / "comp/healthplatform/bundle.go")
    if not bundle.exists():
        return Result(6, "bundle_import", "fail", "bundle.go not found")
    present = bool(re.search(r'_\s+"[^"]*/comp/healthplatform/issues/%s"' % re.escape(pkg), bundle.read_text()))
    if a_path(d) == "A":
        return Result(6, "bundle_import", "pass" if present else "fail",
                      "blank import present" if present else "Path A needs blank import in bundle.go (else silent 'no issue template')")
    return Result(6, "bundle_import", "fail" if present else "pass",
                  "Path B must NOT be blank-imported" if present else "no blank import (correct for Path B)")


def a_path(d):
    return detect_path(d)


def bazel_list(text: str, rule_re: str):
    out = set()
    for m in re.finditer(rule_re + r"\(\s*name\s*=[^)]*?srcs\s*=\s*\[(.*?)\]", text, re.S):
        out |= set(re.findall(r'"([^"]+)"', m.group(1)))
    return out


def c7(d, a, gazelle_out=None):
    b = d / "BUILD.bazel"
    if not b.exists():
        return Result(7, "build_bazel", "fail", "BUILD.bazel missing; run gazelle")
    text = b.read_text()
    lib = bazel_list(text, r"go_library")
    tst = bazel_list(text, r"(?:dd_agent_go_test|go_test)")
    want_lib = set(a["files"])
    want_tst = set(go_files(d, tests=True))
    problems = []
    if want_lib - lib:
        problems.append(f"srcs missing {sorted(want_lib - lib)}")
    if lib - want_lib:
        problems.append(f"srcs has stale {sorted(lib - want_lib)}")
    if want_tst - tst:
        problems.append(f"test srcs missing {sorted(want_tst - tst)}")
    if gazelle_out:
        problems.append(gazelle_out)
    return Result(7, "build_bazel", "fail" if problems else "pass", "; ".join(problems))


def c8(d, a, pr=None):
    problems, warns = [], []
    steps_seen = False
    first_text = None
    for name, text in a["files"].items():
        for body in literals(text, r"\[\]\*healthplatform\.RemediationStep\s*\{"):
            steps_seen = True
            orders = [int(x) for x in re.findall(r"Order:\s*(\d+)", body)]
            if orders and orders != list(range(1, len(orders) + 1)):
                problems.append(f"{name}: step Order not contiguous 1..n: {orders}")
            if first_text is None:
                m = re.search(r"Text:\s*(.*?)\}", body, re.S)
                first_text = m.group(1) if m else ""
        for rx in (r'(?i)(password|secret|token|api_?key)\s*[:=]\s*"[^"]{4,}"',
                   r"[\w.+-]+@[\w-]+\.[\w.-]+", r"AKIA[0-9A-Z]{16}"):
            if re.search(rx, text):
                problems.append(f"{name}: possible secret/PII literal ({rx[:18]}...)")
    if steps_seen and first_text is not None and not any(h in first_text for h in DIAG_HINTS):
        warns.append("remediation step 1 does not look like a diagnostic command")
    if steps_seen and first_text:
        has_cmd = "`" in first_text
        if has_cmd and re.search(r"(?i)\b(search|look for|look in|locate)\b", first_text):
            warns.append("remediation step 1 is a search instruction: allowed only when the file path varies by install; "
                         "it must name the exact string to search and the default paths per OS, otherwise use a literal command")
        if re.search(r"(?i)\blogs?\b", first_text) and not re.search(r"kubectl logs|journalctl", first_text):
            warns.append("remediation step 1 reads logs without a per-platform command: the Agent logs to stdout on "
                         "Kubernetes (kubectl logs), journalctl or the default log path per OS elsewhere")
    if not steps_seen:
        warns.append("no RemediationStep literals found (dynamic builder?)")
    if problems:
        return Result(8, "remediation_hygiene", "fail", "; ".join(problems))
    return Result(8, "remediation_hygiene", "warn" if warns else "pass", "; ".join(warns))


def c9(d, a):
    consts = const_strings(a["src"])
    bad = []
    for name, text in a["files"].items():
        for body in literals(text, r"structpb\.NewStruct\(\s*map\[string\]any\s*\{"):
            for m in re.finditer(r'(?m)^\s*("([^"]+)"|(\w+))\s*:', body):
                key = m.group(2) or consts.get(m.group(3))
                if key is not None and not SNAKE_RE.match(key):
                    bad.append(f"{name}: Extra key {key!r}")
    return Result(9, "extra_keys_snake_case", "fail" if bad else "pass", "; ".join(bad))


def c10(d, a, ack):
    cats = set(re.findall(r'Category:\s*"([^"]+)"', a["src"]))
    hit = cats & DROP_CATEGORIES
    if not hit:
        return Result(10, "category_not_dropped_in_prod", "pass", f"categories {sorted(cats)}")
    if ack:
        return Result(10, "category_not_dropped_in_prod", "warn", f"{sorted(hit)} dropped in prod (acknowledged)")
    return Result(10, "category_not_dropped_in_prod", "fail",
                  f"{sorted(hit)} dropped in prod by agthealth-worker-drop-issue; pass --ack-drop-category after user ack")


def fixture_candidates(name: str):
    """Names a generated fixture may carry in the catalog. The README convention is `<type>.md` for a
    static template and `<type>.<variant>.md` for variants; the generator may emit `.default.md`."""
    parts = name.split(".")
    cands = [name]
    if len(parts) == 2:                       # <type>.md
        cands.append(f"{parts[0]}.default.md")
    elif len(parts) == 3 and parts[1] == "default":   # <type>.default.md
        cands.append(f"{parts[0]}.md")
    return cands


def c11(catalog_dir, generated_dir):
    if not catalog_dir or not generated_dir:
        return Result(11, "catalog_fixture_generated", "skip", "no --catalog-dir/--generated-dir provided")
    cd, gd = Path(catalog_dir), Path(generated_dir)
    diffs, notes = [], []
    for g in sorted(gd.rglob("*")):
        if not g.is_file():
            continue
        rel = g.relative_to(gd)
        found = None
        for cand in fixture_candidates(rel.name):
            c = cd / rel.parent / cand
            if c.exists():
                found = c
                break
        if found is None:
            tried = " or ".join(fixture_candidates(rel.name))
            diffs.append(f"missing: catalog has neither {tried}")
        elif found.read_bytes() != g.read_bytes():
            diffs.append(f"differs: {found.name} is not byte-identical to the generated {rel.name}")
        elif found.name != rel.name:
            notes.append(f"{rel.name} matched {found.name} (unsuffixed convention for static templates)")
    return Result(11, "catalog_fixture_generated", "fail" if diffs else "pass", "; ".join(diffs or notes))


def negation_equivalent(expr: str, other: str) -> bool:
    """De Morgan check for simple tag expressions: `a || b` vs `!a && !b` and the reverse."""
    def parse(e):
        e = e.strip()
        for op in ("||", "&&"):
            if op in e and ("&&" if op == "||" else "||") not in e:
                return op, sorted(t.strip() for t in e.split(op))
        return "", [e]
    op, terms = parse(expr)
    if op == "":
        return other.strip().replace(" ", "") == f"!{terms[0]}"
    flipped = {"||": "&&", "&&": "||"}[op]
    want = sorted(t[1:] if t.startswith("!") else "!" + t for t in terms)
    o_op, o_terms = parse(other)
    return o_op == flipped and o_terms == want


def c12(d, a):
    chk = a["files"].get("check.go")
    if chk is None:
        return Result(12, "check_noop_present", "skip", "no check.go")
    m = re.search(r"(?m)^//go:build\s+(.+)$", chk)
    if not m:
        return Result(12, "check_noop_present", "pass", "check.go has no build constraint")
    expr = m.group(1).strip()
    noop = a["files"].get("check_noop.go")
    if noop is None:
        return Result(12, "check_noop_present", "fail", f"check.go gated by `{expr}` but check_noop.go missing")
    nm = re.search(r"(?m)^//go:build\s+(.+)$", noop)
    if not nm:
        return Result(12, "check_noop_present", "fail", "check_noop.go lacks //go:build")
    got = nm.group(1).strip().replace(" ", "")
    ok = {f"!{expr}".replace(" ", ""), f"!({expr})".replace(" ", "")}
    if got not in ok and negation_equivalent(expr, nm.group(1)):
        ok.add(got)
    return Result(12, "check_noop_present", "pass" if got in ok else "fail",
                  "" if got in ok else f"noop tag `{got}` is not the negation of `{expr}`")


def method_body(text: str, method: str):
    m = re.search(method + r"\(\)[^{]*\{", text)
    if not m:
        return None
    return text[m.end():match_brace(text, m.end() - 1)]


def builtin_present(d: Path) -> bool:
    """True when the module defines a non-trivial built-in periodic or startup check."""
    mod = d / "module.go"
    if not mod.exists():
        return False
    text = mod.read_text()
    for name in ("BuiltInPeriodicHealthCheck", "BuiltInStartupHealthCheck"):
        body = method_body(text, name)
        if body is not None and not re.fullmatch(r"\s*return nil\s*", body):
            return True
    return False


def periodic_source(d):
    mod = d / "module.go"
    if not mod.exists():
        return None
    text = mod.read_text()
    m = re.search(r"BuiltInPeriodicHealthCheck\(\)[^{]*\{", text)
    if not m:
        return None
    end = match_brace(text, m.end() - 1)
    body = text[m.end():end]
    if re.fullmatch(r"\s*return nil\s*", body):
        return None
    sm = re.search(r'Source:\s*("([^"]+)"|(\w+))', body)
    if not sm:
        return None
    return sm.group(2) or const_strings(all_src(d)).get(sm.group(3))


def c13(d, a, worktree, pkg):
    mine = periodic_source(d)
    if mine is None:
        return Result(13, "periodic_source_unique", "skip", "no periodic check")
    for other in (worktree / "comp/healthplatform/issues").iterdir():
        if other.is_dir() and other.name != pkg and periodic_source(other) == mine:
            return Result(13, "periodic_source_unique", "fail", f"Source {mine!r} also used by {other.name}")
    return Result(13, "periodic_source_unique", "pass", f"Source {mine!r} unique")


def c14(d, a):
    problems = []
    found = False
    for name, text in a["files"].items():
        assigned = set(re.findall(r"\.(\w+)\s*=[^=]", text))
        for body in literals(text, r"&healthplatform\.Issue\s*\{"):
            found = True
            keys = set(re.findall(r"(?m)^\s*(\w+):", body)) | assigned
            miss = [f for f in REQUIRED_ISSUE_FIELDS if f not in keys]
            if miss:
                problems.append(f"{name}: Issue literal missing {miss}")
            if re.search(r"Severity:\s*[\w.]*UNSPECIFIED", body):
                problems.append(f"{name}: Severity UNSPECIFIED (worker drops)")
    if not found:
        if re.search(r"\w+\.BuildIssue\(", a["src"]):
            return Result(14, "worker_required_fields", "warn",
                          "delegates to another BuildIssue; run its package through this check too")
        return Result(14, "worker_required_fields", "fail", "no &healthplatform.Issue literal found")
    return Result(14, "worker_required_fields", "fail" if problems else "pass",
                  "; ".join(problems) or "title, description, severity, category, names present (hostname set by egress)")


def c15(d, a, worktree):
    files = list(d.glob("*_test.go")) + list((worktree / "comp/healthplatform").glob("*_test.go"))
    hits = [f.name for f in files if "STATE_OPEN" in f.read_text()]
    return Result(15, "no_state_open_assert", "fail" if hits else "pass",
                  f"tests assert STATE_OPEN (worker never emits it): {hits}" if hits else "")


def c16(d, a):
    problems = []
    for name, text in a["files"].items():
        for body in literals(text, r"&healthplatform\.Issue\s*\{"):
            m = re.search(r"Severity:\s*([^,\n]+)", body)
            if not m:
                continue
            v = m.group(1).strip()
            if re.fullmatch(r"\d+", v) or v.startswith('"'):
                problems.append(f"{name}: Severity must use IssueSeverity enum, got {v}")
            elif "UNSPECIFIED" in v:
                problems.append(f"{name}: Severity UNSPECIFIED")
    return Result(16, "severity_enum", "fail" if problems else "pass", "; ".join(problems))


def c17(d, a, worktree, pkg, detection=None):
    names = prefixed(a["src"], "IssueName")
    name = next(iter(names.values()), None)
    cands = list(d.glob("*_test.go")) + list((worktree / "comp/healthplatform").glob("*_test.go"))
    importer = []
    for f in cands:
        t = f.read_text()
        has_flow = ("GetAgentHealth" in t and "Bundle()" in t and ("GetTitle()" in t or "GetRemediation()" in t))
        if not has_flow or not (name and f'"{name}"' in t):
            continue
        imports_pkg = bool(re.search(r'issues/%s"' % re.escape(pkg), t))
        if imports_pkg and a["path"] == "A":
            importer.append(f.name)  # init() would register the module and hide a missing bundle.go import
            continue
        need_resolved = detection in IMPLEMENTED_DETECTION
        missing = [w for w, rx in ((("RESOLVED", r"RESOLVED|Resolved"),) if need_resolved else ()) + ( ("Location", r"GetLocation|Location"),
                                   ("Source", r"GetSource|Source"), ("Extra", r"GetExtra|Extra")) if not re.search(rx, t)]
        if missing:
            return Result(17, "tier1_registration_test", "warn",
                          f"{f.name} runs Bundle() but never asserts: {', '.join(missing)}. Assert the wire fields "
                          "(location, source, extra) and that clearing the condition reaches RESOLVED; when the module's real "
                          "check can run in the test, drive it through Bundle() instead of only a synthetic runner")
        return Result(17, "tier1_registration_test", "pass", f.name)
    if importer:
        return Result(17, "tier1_registration_test", "fail",
                      f"{importer} imports the package; its init() registers the module even without the bundle.go "
                      "import, so the test cannot catch a missing import. Use string literals for IssueID/IssueName/IssueType")
    return Result(17, "tier1_registration_test", "fail",
                  "need a test running Bundle() -> runner/registry -> store -> fakeintake that asserts title and "
                  "remediation are populated and references the IssueName literal (Path A: without importing the package)")


def c19(d, a, worktree, pkg):
    """Release note and registered checks must not claim detection that does not exist yet."""
    notes_dir = worktree / "releasenotes" / "notes"
    notes = sorted(notes_dir.glob(f"{pkg}-*.yaml")) if notes_dir.is_dir() else []
    marker = "TODO(onboardhealthsignal)"
    marker_in_code = any(marker in t for t in a["files"].values())
    stub = any(re.search(r"detectCondition\(\)[^{]*\{\s*return nil, nil\s*\}", t) for t in a["files"].values())
    todo_in_code = marker_in_code or stub
    why = f"check code still has {marker}" if marker_in_code else "detectCondition is still the stub (return nil, nil)"
    problems, warns = [], []
    if todo_in_code and builtin_present(d):
        problems.append(f"a built-in check is registered but {why}: a registered check that cannot report only "
                        "costs a goroutine and ticker everywhere; register it once detection exists")
    for n in notes:
        has_marker = marker in n.read_text()
        if todo_in_code and not has_marker:
            problems.append(f"{n.name} claims detection but {why}")
        if has_marker and not todo_in_code:
            warns.append(f"{n.name} still carries the {marker} marker; rewrite the note")
    if not notes and not problems:
        return Result(19, "release_note_honest", "skip", "no release note named after the package")
    if problems:
        return Result(19, "release_note_honest", "fail", "; ".join(problems))
    return Result(19, "release_note_honest", "warn" if warns else "pass", "; ".join(warns))


HEAVY_AGENT = ("pkg/logs", "comp/logs", "pkg/util/kubernetes/kubelet", "pkg/util/kubernetes/apiserver", "pkg/util/docker",
               "pkg/util/containerd", "pkg/util/crio", "pkg/collector", "pkg/security", "pkg/ebpf", "pkg/process")
HEAVY_EXTERNAL = ("github.com/docker/docker", "github.com/containerd", "k8s.io", "google.golang.org/grpc")
AGENT_PREFIX = "github.com/DataDog/datadog-agent/"
NON_GUARD_TAGS = {"test", "ignore", "genfixture"}


def prefix_match(path: str, prefix: str) -> bool:
    return path == prefix or path.startswith(prefix + "/")


def heavy_imports_of(text: str, extra=()):
    found = []
    paths = []
    for blk in re.finditer(r"import\s*\(([^)]*)\)", text, re.S):
        paths += re.findall(r'"([^"]+)"', blk.group(1))
    paths += re.findall(r'(?m)^import\s+(?:\w+\s+)?"([^"]+)"', text)
    for path in paths:
        hit = None
        if path.startswith(AGENT_PREFIX):
            rel = path[len(AGENT_PREFIX):]
            hit = next((p for p in HEAVY_AGENT if prefix_match(rel, p)), None)
        else:
            hit = next((p for p in HEAVY_EXTERNAL + tuple(extra) if prefix_match(path, p)), None)
        if hit:
            found.append((path, hit))
    return found


def build_expr(text: str):
    m = re.search(r"(?m)^//go:build\s+(.+)$", text)
    return m.group(1).strip() if m else None


def is_real_guard(expr) -> bool:
    if not expr:
        return False
    tokens = set(re.findall(r"[A-Za-z_][A-Za-z0-9_]*", expr))
    return bool(tokens - NON_GUARD_TAGS)


def c20(d, a, extra):
    """Heavy or platform-specific imports must sit in a build-tag-gated file; guarded files need a noop stub."""
    unguarded, no_stub, guarded_ok = [], [], 0
    exprs = {n: build_expr(t) for n, t in a["files"].items()}
    for name, text in a["files"].items():
        heavy = heavy_imports_of(text, extra)
        if not heavy:
            continue
        expr = exprs[name]
        shown = ", ".join(sorted({h[1] for h in heavy}))
        if not is_real_guard(expr):
            unguarded.append(f"{name} imports {shown} without a feature or platform //go:build tag")
            continue
        stub = any(n != name and e and (negation_equivalent(expr, e) or e.replace(" ", "") in
                                          {f"!{expr}".replace(" ", ""), f"!({expr})".replace(" ", "")})
                   for n, e in exprs.items())
        if stub:
            guarded_ok += 1
        else:
            no_stub.append(f"{name} (tag `{expr}`) has no file with the negated tag (noop stub)")
    if unguarded:
        return Result(20, "heavy_imports_guarded", "fail",
                      "unguarded heavy import: " + "; ".join(unguarded) +
                      ". Move it into a build-tag-gated file and add a check_noop.go stub with the negated tag")
    if no_stub:
        return Result(20, "heavy_imports_guarded", "warn", "; ".join(no_stub))
    return Result(20, "heavy_imports_guarded", "pass",
                  f"{guarded_ok} guarded heavy import file(s) with noop stubs" if guarded_ok else "no heavy imports")


def c21(d, a):
    """issue_test.go must carry the AGENTS.md mandatory BuildIssue assertions."""
    files = [p for p in sorted(d.glob("*_test.go")) if p.name == "issue_test.go" or p.name.endswith("_issue_test.go")]
    if not files:
        return Result(21, "issue_test_mandatory_assertions", "fail",
                      "no issue_test.go (or *_issue_test.go): the mandatory BuildIssue assertions are missing")
    text = "\n".join(p.read_text() for p in files)
    missing = []
    if not re.search(r"(?:assert|require)\.Equal\([^\n]*\.Title\b", text):
        missing.append("exact title assertion (assert.Equal(..., issue.Title))")
    if not re.search(r"(?:assert|require)\.Contains\([^\n]*\.Description\b", text):
        missing.append("description substring assertion (assert.Contains(..., issue.Description, ...))")
    if not re.search(r"(?:assert|require)\.Len\([^\n]*Steps|len\([^)]*Steps\)", text):
        missing.append("step count assertion")
    if not re.search(r"context:\s*nil|map\[string\]string\{\}|BuildIssue\(nil\)", text):
        missing.append("a nil or empty context case (defaults applied)")
    if missing:
        return Result(21, "issue_test_mandatory_assertions", "fail", "missing: " + "; ".join(missing))
    return Result(21, "issue_test_mandatory_assertions", "pass", ", ".join(p.name for p in files))


MARKER = "TODO(onboardhealthsignal)"
IMPLEMENTED_DETECTION = ("implemented", "partial")

TOOL_NAMES = ("onboardhealthsignal", "check_signal", "render_templates", "gen_catalog_fixture", "preflight.sh",
              "baseline_check", "audit_contamination", "verification_bullets", "deps_delta")


def c22(d, a, worktree, pkg, detection=None):
    """Skill and script names must not leak into repo code, comments, or release notes.

    The TODO scaffold marker is tolerated only while detection is a TODO (or unspecified). With
    --detection implemented|partial a leftover marker is NOT stripped, so it is reported here too."""
    files = sorted(d.glob("*.go")) + sorted((worktree / "comp" / "healthplatform").glob(f"{pkg}*_integration_test.go"))
    notes_dir = worktree / "releasenotes" / "notes"
    if notes_dir.is_dir():
        files += sorted(notes_dir.glob(f"{pkg}-*.yaml"))
    hits = []
    for f in files:
        raw = f.read_text()
        text = (raw if detection in IMPLEMENTED_DETECTION else raw.replace(MARKER, "")).lower()
        found = [t for t in TOOL_NAMES if t in text]
        if found:
            hits.append(f"{f.name}: {', '.join(found)}")
    if hits:
        return Result(22, "no_tool_names_in_repo_code", "fail",
                      "tool names leak into repo files (upstream readers cannot resolve them): " + "; ".join(hits))
    return Result(22, "no_tool_names_in_repo_code", "pass", "")


def c23(catalog_ts_dir):
    """Generated catalog TypeScript must be prettier-formatted: keys are quoted only when hyphenated."""
    if not catalog_ts_dir:
        return Result(23, "catalog_ts_formatting", "skip", "no --catalog-ts-dir provided")
    bad = []
    for f in sorted(Path(catalog_ts_dir).glob("*.ts")):
        for m in re.finditer(r'(?m)^\s*"([A-Za-z0-9_]+)":\s', f.read_text()):
            bad.append(f"{f.name}: key {m.group(1)!r} is quoted but has no hyphen")
    if bad:
        return Result(23, "catalog_ts_formatting", "fail",
                      "; ".join(bad) + ". Prettier unquotes it: run the real formatter (do not hand-splice generated files); "
                      "if prettier cannot run, mark the catalog slice BLOCKED")
    return Result(23, "catalog_ts_formatting", "pass", "")


def c18(pr_body, names):
    if not pr_body:
        return Result(18, "pr_text_hygiene", "skip", "no --pr-body")
    text = Path(pr_body).read_text()
    problems = []
    if EM_DASH in text:
        problems.append("em dash present")
    for m in re.finditer(r"(?<![\w/])@(?!DataDog/)([A-Za-z0-9][A-Za-z0-9-]*)", text):
        problems.append(f"@{m.group(1)} mention (person handles not allowed)")
    for n in names:
        if n and re.search(r"\b%s\b" % re.escape(n), text, re.I):
            problems.append(f"person name {n!r}")
    if "qa/done" in text:
        problems.append("suggests the qa/done label; use it only when real QA was done, otherwise qa/no-code-change or qa/skip-qa")
    return Result(18, "pr_text_hygiene", "fail" if problems else "pass", "; ".join(problems))


# ---------- round 3 checks (24-33) ----------

def read_all(paths):
    out = {}
    for p in paths:
        p = Path(p)
        if p.is_file():
            out[p.name] = p.read_text()
    return out


def test_texts(d, worktree, pkg):
    """Test files of the package plus the package-prefixed healthplatform integration test."""
    paths = sorted(d.glob("*_test.go")) + sorted((worktree / "comp" / "healthplatform").glob(f"{pkg}*_integration_test.go"))
    return read_all(paths)


def nontest_texts(d):
    return {n: t for n, t in go_files(d).items()}


def build_line(text: str) -> str:
    m = re.search(r"(?m)^//go:build\s+(.+)$", text)
    return m.group(1).strip() if m else ""


def release_notes(worktree, pkg):
    nd = worktree / "releasenotes" / "notes"
    return sorted(nd.glob(f"{pkg}-*.yaml")) if nd.is_dir() else []


def c24(d, a, worktree, pkg, owner_files, detection):
    """No leftover scaffold marker, and no registered check that cannot report, once detection exists."""
    if detection not in IMPLEMENTED_DETECTION:
        return Result(24, "no_leftover_scaffold_markers", "skip",
                      "pass --detection implemented|partial to enforce (the marker is allowed while detection is a TODO)")
    texts = read_all(sorted(d.glob("*.go")) + sorted((worktree / "comp" / "healthplatform").glob(f"{pkg}*_integration_test.go"))
                     + [Path(f) for f in owner_files] + release_notes(worktree, pkg))
    problems = []
    for name, t in texts.items():  # raw text: the marker is never stripped here
        n = t.count(MARKER)
        if n:
            problems.append(f"{name}: {n} leftover {MARKER}")
    stub = any(re.search(r"detectCondition\([^)]*\)[^{]*\{\s*return nil, nil\s*\}", t) for t in nontest_texts(d).values())
    if stub and builtin_present(d):
        problems.append("a built-in check is registered but detectCondition is still the stub (return nil, nil): it can never report")
    if problems:
        return Result(24, "no_leftover_scaffold_markers", "fail",
                      f"detection is {detection} but: " + "; ".join(problems))
    return Result(24, "no_leftover_scaffold_markers", "pass", "")


SCAFFOLD_SKIP_RE = re.compile(r"(?i)scaffold|todo|skeleton|not wired|gate[- ]through|throughthemodule|fill (?:this )?in|replace this")


def c25(d, worktree, pkg, detection):
    """Scaffold tests that t.Skip must not ship once detection is implemented."""
    if detection not in IMPLEMENTED_DETECTION:
        return Result(25, "no_skipped_scaffold_tests", "skip", "pass --detection implemented|partial to enforce")
    bad = []
    for name, t in test_texts(d, worktree, pkg).items():
        for m in re.finditer(r"t\.Skip(?:f|Now)?\(([^)]*)\)", t):
            if SCAFFOLD_SKIP_RE.search(m.group(1)):
                bad.append(f"{name}: {m.group(0)[:80]}")
        for m in re.finditer(r"(?m)^\s*//.*\n\s*t\.Skip(?:f|Now)?\(", t):
            if SCAFFOLD_SKIP_RE.search(m.group(0)):
                bad.append(f"{name}: skip under a scaffold comment")
    if bad:
        return Result(25, "no_skipped_scaffold_tests", "fail",
                      f"detection is {detection} but scaffold tests are skipped (a skipped test proves nothing): "
                      + "; ".join(sorted(set(bad))))
    return Result(25, "no_skipped_scaffold_tests", "pass", "")


EXPVAR_NEW_RE = re.compile(r'expvar\.New(?:Map|Int|String|Float)\(\s*"([^"]+)"\s*\)')


def c26(d, worktree, pkg):
    """Fixed expvar names published from tests panic on `go test -count=2` (duplicate registration)."""
    bad = []
    for name, t in test_texts(d, worktree, pkg).items():
        for m in EXPVAR_NEW_RE.finditer(t):
            lit = re.escape(m.group(1))
            guarded = re.search(r'expvar\.Get\(\s*"%s"' % lit, t) or "sync.Once" in t
            if not guarded:
                bad.append(f"{name}: expvar.New*({m.group(1)!r})")
    if bad:
        return Result(26, "no_fixed_expvar_names_in_tests", "fail",
                      "fixed names panic with `go test -count=2` (duplicate registration); look the variable up with "
                      "expvar.Get first, use sync.Once, or build a unique name: " + "; ".join(bad))
    return Result(26, "no_fixed_expvar_names_in_tests", "pass", "")


def c27(d, worktree, pkg):
    """Reading another package's expvar by string literal goes silently dead if it is renamed."""
    lits = []
    for name, t in nontest_texts(d).items():
        for m in re.finditer(r'expvar\.Get\(\s*"([^"]+)"\s*\)', strip_comments(t)):
            lits.append((name, m.group(1)))
    if not lits:
        return Result(27, "cross_package_string_reads", "pass", "no expvar reads by string literal")
    shared = any(re.search(r"expvar\.Get\(\s*[A-Za-z_]\w*\.[A-Za-z_]\w*\s*\)", t)
                 for t in test_texts(d, worktree, pkg).values())
    if shared:
        return Result(27, "cross_package_string_reads", "pass", "a test reads the real variable through a shared constant")
    return Result(27, "cross_package_string_reads", "warn",
                  "reads expvar by string literal (" + ", ".join(f"{n}: {l!r}" for n, l in lits) + ") with no test that "
                  "reads the real variable or a constant shared with the owner package: a rename kills the check silently")


# Real socket use only: a ".sock" path inside sample data or a context map is not a socket.
UNIX_SOCKET_RE = re.compile(
    r'net\.(?:Listen|Dial|DialTimeout|ListenPacket)\(\s*"unix(?:packet|gram)?"'
    r'|\b(?:net\.)?(?:ListenUnix|DialUnix|ResolveUnixAddr)\('
    r'|grpc\.(?:Dial|DialContext|NewClient)\([^)\n]*unix:'
    r'|\bnet\.Dialer\b[^\n]*unix')


def c28(d, worktree, pkg):
    """Tests that use unix sockets need an OS constraint: tag-only constraints still build on Windows."""
    fails, warns = [], []
    for name, t in test_texts(d, worktree, pkg).items():
        if not UNIX_SOCKET_RE.search(strip_comments(t)):  # prose about sockets in comments is not a socket
            continue
        expr = build_line(t)
        if not expr:
            warns.append(f"{name}: unix sockets, no //go:build constraint (runs on Windows)")
        elif not (re.search(r"(?<![!\w])(linux|unix|darwin)\b", expr) or "!windows" in expr):
            fails.append(f"{name}: unix sockets under `//go:build {expr}` with no OS constraint")
    if fails:
        return Result(28, "unix_socket_tests_os_constrained", "fail",
                      "; ".join(fails) + "; add `&& linux` (or `!windows`): feature tags are present in Windows builds "
                      "where the client uses named pipes")
    if warns:
        return Result(28, "unix_socket_tests_os_constrained", "warn", "; ".join(warns))
    return Result(28, "unix_socket_tests_os_constrained", "pass", "")


OS_FILE_RE = re.compile(r"_(?:linux|unix|nix|darwin|windows)\.go$")
OS_API_RE = re.compile(r'golang\.org/x/sys/unix|\bsyscall\.|net\.Dial\(\s*"unix"|net\.Listen\(\s*"unix"|"/proc/|"/sys/')


def c29(d, worktree, pkg):
    """Release note must state platform scope when the signal reads OS-specific state."""
    specific = []
    for name, t in nontest_texts(d).items():
        expr = build_line(t)
        if OS_FILE_RE.search(name) or re.search(r"(?<![!\w])(linux|unix|darwin|windows)\b|!windows", expr) \
                or OS_API_RE.search(strip_comments(t)):
            specific.append(name)
    notes = release_notes(worktree, pkg)
    if not specific:
        return Result(29, "release_note_platform_scope", "pass", "no OS-specific code in the package")
    if not notes:
        return Result(29, "release_note_platform_scope", "skip", "OS-specific code but no release note named after the package")
    bad = [n.name for n in notes if not re.search(r"(?i)linux|macos|darwin|windows|unix|platform", n.read_text())]
    if bad:
        return Result(29, "release_note_platform_scope", "warn",
                      f"{', '.join(bad)} does not say which platforms report this issue (OS-specific code in "
                      f"{', '.join(specific)}): state platform scope, e.g. 'Linux and macOS only'")
    return Result(29, "release_note_platform_scope", "pass", "")


STORE_DEF_RE = re.compile(r'healthplatform/store/def"|\bstoredef\b|\bhealthplatformdef\b')


def git_changed(worktree: Path, rel: str):
    try:
        r = subprocess.run(["git", "diff", "--name-only", "HEAD", "--", rel], cwd=worktree,
                           capture_output=True, text=True, timeout=30)
    except (OSError, subprocess.SubprocessError):
        return None
    if r.returncode != 0:
        return None
    return bool(r.stdout.strip())


def c30(d, a, worktree, owner_files):
    """Path B structure: no module, no bundle change, issue file present, owner reports and resolves with an Id."""
    if a["path"] != "B":
        return Result(30, "path_b_structure", "skip", "Path A")
    problems, warns = [], []
    if (d / "module.go").exists():
        problems.append("module.go present (Path B has no module registration)")
    if git_changed(worktree, "comp/healthplatform/bundle.go"):
        problems.append("bundle.go is modified (Path B packages are not blank-imported)")
    issue_files = [n for n in a["files"] if n == "issue.go" or n.endswith("_issue.go")]
    if not issue_files:
        problems.append("no issue.go or *_issue.go")
    if not owner_files:
        warns.append("pass --owner-file to verify the owner edit")
    else:
        txt = "\n".join(Path(f).read_text() for f in owner_files)
        if not STORE_DEF_RE.search(txt):
            problems.append("owner edit does not use the health platform store def interface (def packages only)")
        if "ReportIssue(" not in txt:
            problems.append("owner edit never calls ReportIssue")
        if not re.search(r"ResolveIssue\(|ResolveAllIssues\(", txt):
            problems.append("owner edit never calls ResolveIssue")
        both = txt + "\n" + "\n".join(a["files"][n] for n in issue_files)
        has_id = re.search(r"\.Id\s*=\s*[^\s\"]|\.Id\s*=\s*\"[^\"]+\"|\bId:\s*[^\s\",}]|\bId:\s*\"[^\"]+\"", both)
        if not has_id:
            problems.append("no non-empty Id is set before ReportIssue (store.ReportIssue rejects an empty id)")
    if problems:
        return Result(30, "path_b_structure", "fail", "; ".join(problems))
    return Result(30, "path_b_structure", "warn" if warns else "pass", "; ".join(warns))


THRESHOLD_RE = re.compile(r"(?i)(?:consecutive[_ ]?failures?|failure[_ ]?threshold|failures?[_ ]?to[_ ]?report)\w*\s*(?::=|=|:)\s*(\d+)")
ERROR_RETURN_RE = re.compile(r"return\s+[^\n]*,\s*(?:fmt\.Errorf|errors\.New|err\b|\w*[Ee]rr\w*\b|\w+\.Err\w+)")


def c31(d, a):
    """Hysteresis above 1 needs a pending-confirmation error path so a restart cannot make the issue vanish."""
    src = strip_comments(a["src"])
    thresholds = [int(m.group(1)) for m in THRESHOLD_RE.finditer(src)]
    if not thresholds or max(thresholds) <= 1:
        return Result(31, "hysteresis_pending_error", "pass", "no hysteresis above 1")
    if ERROR_RETURN_RE.search(src):
        return Result(31, "hysteresis_pending_error", "pass", f"threshold {max(thresholds)} with an error return path")
    return Result(31, "hysteresis_pending_error", "warn",
                  f"threshold {max(thresholds)} but no error return found: after a restart the scheduler resolves seeded ids "
                  "the first run does not return, so an instance found but not yet confirmed should return an error "
                  "(not nil, nil and not a first-run flag)")


REMEDIATION_CMD_RE = re.compile(r"`(?:sudo\s+)?(?:datadog-agent|agent)\s+([a-z][a-z-]*)[^`]*`")


def grep_go(worktree: Path, pattern: str, dirs):
    existing = [x for x in dirs if (worktree / x).is_dir()]
    if not existing:
        return None
    try:
        r = subprocess.run(["grep", "-rIlE", "--include=*.go", pattern] + existing, cwd=worktree,
                           capture_output=True, text=True, timeout=90)
    except (OSError, subprocess.SubprocessError):
        return None
    return bool(r.stdout.strip())


def c32(d, a, worktree):
    """Subcommands named in remediation text must exist in the tree; otherwise mark them UNVERIFIED."""
    subs = {}
    for name, t in nontest_texts(d).items():
        if name == "issue.go" or name.endswith("_issue.go"):
            for m in REMEDIATION_CMD_RE.finditer(t):
                subs.setdefault(m.group(1), name)
    if not subs:
        return Result(32, "remediation_commands_exist", "pass", "no datadog-agent/agent subcommands named in backticks")
    if not (worktree / "cmd").is_dir():
        return Result(32, "remediation_commands_exist", "skip", "cmd/ not present in the worktree (cannot verify subcommands)")
    unverified, checked = [], 0
    for sub in sorted(subs):
        found = grep_go(worktree, r'Use:\s*"%s[ "]' % re.escape(sub), ["cmd", "comp", "pkg/cli"])
        if found is None:
            return Result(32, "remediation_commands_exist", "skip", "cmd/ not present in the worktree")
        checked += 1
        if not found:
            unverified.append(f"`{sub}` ({subs[sub]})")
    if unverified:
        return Result(32, "remediation_commands_exist", "warn",
                      "UNVERIFIED subcommand(s) named in remediation: " + ", ".join(unverified)
                      + "; no cobra command with that Use was found under cmd/ comp/ pkg/cli; verify or mark UNVERIFIED in the PR body")
    return Result(32, "remediation_commands_exist", "pass", f"{checked} subcommand(s) found")


LIMIT_RE = re.compile(r"(?i)cannot|can not|does not|doesn't|not yet|limit|unable|only |without|lack|missing|no per-")


def c33(pr_body, catalog_shim, detection):
    """PR body honesty about the eslint shim and about partial detection."""
    needs_shim = bool(catalog_shim)
    needs_partial = detection == "partial"
    if not (needs_shim or needs_partial):
        return Result(33, "pr_body_claims", "skip", "pass --catalog-shim and/or --detection partial to enforce")
    if not pr_body:
        return Result(33, "pr_body_claims", "skip", "no --pr-body")
    text = Path(pr_body).read_text()
    problems = []
    if needs_shim and not re.search(r"(?is)eslint[^.\n]*(not (?:been )?run|was not run|were not run|skipped|bypass|shim)"
                                    r"|(?:not run|shim)[^.\n]*eslint", text):
        problems.append("a no-op eslint shim was used for the catalog gate but the PR body does not say eslint was not run")
    if needs_partial:
        m = re.search(r"(?is)#+\s*what does this pr do\??\s*\n(.*?)(?=\n#+\s|\Z)", text)
        if not m:
            problems.append("no 'What does this PR do' section to carry the partial-detection limitation")
        elif not LIMIT_RE.search(m.group(1)):
            problems.append("detection is partial but 'What does this PR do' states no limitation "
                            "(not only Additional Notes)")
    if problems:
        return Result(33, "pr_body_claims", "fail", "; ".join(problems))
    return Result(33, "pr_body_claims", "pass", "")


def owner_dirs(owner_files):
    return sorted({Path(f).resolve().parent for f in owner_files if Path(f).is_file()})


WIRING_STORE_RE = re.compile(r"(?i)(?:fake|mock|stub)\w*store|store\w*(?:fake|mock)|storemock|storedef\.|ReportIssue")
WIRING_LINK_RE = re.compile(r"Requires\{[^}]*(?:HealthPlatform|Store)|HealthPlatform|option\.New\(")
WIRING_ATTACH_RE = re.compile(r"(?i)attach|\bwire|addLauncher|new\w*agent|Requires\{|launcher|observer")


def c34(a, owner_files):
    """Path B: a test must exercise the owner wiring (attach path) with a fake store, not only the adapter in isolation."""
    if a["path"] != "B":
        return Result(34, "path_b_wiring_tests", "skip", "Path A")
    if not owner_files:
        return Result(34, "path_b_wiring_tests", "skip", "pass --owner-file to locate the owner tests")
    seen, linked = [], []
    for dd in owner_dirs(owner_files):
        for t in sorted(dd.glob("*_test.go")):
            txt = t.read_text()
            if WIRING_STORE_RE.search(txt) and WIRING_ATTACH_RE.search(txt):
                seen.append(t.name)
                if WIRING_LINK_RE.search(txt):
                    linked.append(t.name)
    if seen and not linked:
        return Result(34, "path_b_wiring_tests", "warn",
                      f"{', '.join(sorted(set(seen)))} exercise the observer but not the dependency link: add a test that fails "
                      "when the owner's fx Requires field (HealthPlatform) is no longer handed to the store field")
    if seen:
        return Result(34, "path_b_wiring_tests", "pass", ", ".join(sorted(set(seen))))
    return Result(34, "path_b_wiring_tests", "warn",
                  "no test in the owner directories constructs or attaches the owner component with a fake store: the wiring "
                  "that hands the store to the reporter is untested (a missing hand-off silently disables the signal)")


HASH_RE = re.compile(r"fnv\.New|sha256\.|sha1\.|md5\.|crc32\.|crc64\.|maphash|%x|hex\.EncodeToString")
VACUOUS_RE = re.compile(r"(?:assert|require)\.NotEmpty\([^\n]*(?:[Ii]d\b|ID\b|[Ss]uffix|[Hh]ost)")


def c35(d, worktree, pkg, owner_files):
    """A non-empty assertion on an id built from a hash proves nothing: a hash is never empty."""
    code = "\n".join(strip_comments(t) for t in nontest_texts(d).values())
    code += "\n".join(strip_comments(Path(f).read_text()) for f in owner_files if Path(f).is_file())
    if not HASH_RE.search(code):
        return Result(35, "no_vacuous_id_tests", "pass", "ids are not built from a hash")
    tests = dict(test_texts(d, worktree, pkg))
    for dd in owner_dirs(owner_files):
        tests.update(read_all(sorted(dd.glob("*_test.go"))))
    hits = sorted(n for n, t in tests.items() if VACUOUS_RE.search(t))
    if hits:
        return Result(35, "no_vacuous_id_tests", "warn",
                      f"{', '.join(hits)} assert a non-empty id/suffix/host but the id is built from a hash (never empty): "
                      "assert the exact id for a known hostname, or that two hosts produce different ids")
    return Result(35, "no_vacuous_id_tests", "pass", "")


def refs_to_pkg(worktree: Path, rel: str, exclude: Path):
    try:
        r = subprocess.run(["grep", "-rIl", "--include=*.go", "-F", f'datadog-agent/{rel}"', "."], cwd=worktree,
                           capture_output=True, text=True, timeout=120)
    except (OSError, subprocess.SubprocessError):
        return None
    out = []
    for line in r.stdout.splitlines():
        p = (worktree / line).resolve()
        if p.name.endswith("_test.go") or exclude in p.parents or p.parent == exclude:
            continue
        out.append(p)
    return out


def c36(worktree, owner_files, tag_sets, skip_build):
    """Owner files referenced only from build-tagged callers must build under the alternate tag sets."""
    dirs = owner_dirs(owner_files)
    if not dirs:
        return Result(36, "owner_tag_consistency", "skip", "pass --owner-file to evaluate")
    worktree = Path(worktree).resolve()
    problems, warns, notes = [], [], []
    for dd in dirs:
        try:
            rel = str(dd.relative_to(worktree))
        except ValueError:
            continue
        refs = refs_to_pkg(worktree, rel, dd)
        if refs is None:
            return Result(36, "owner_tag_consistency", "skip", "grep unavailable")
        tagged_only = bool(refs) and all(build_line(p.read_text()) for p in refs)
        if not tagged_only:
            notes.append(f"{rel}: referenced from untagged code or not at all")
            continue
        if not tag_sets:
            problems.append(f"{rel} is referenced only from build-tagged files "
                            f"({', '.join(sorted(str(p.relative_to(worktree)) for p in refs)[:3])}); pass --tag-sets to verify the build "
                            "under the alternate tag sets")
        elif skip_build:
            warns.append(f"{rel}: tagged-only callers; build under {tag_sets} skipped (--skip-build)")
        else:
            for ts in tag_sets:
                try:
                    r = subprocess.run(["go", "build", "-tags", ts, f"./{rel}/..."], cwd=worktree,
                                       capture_output=True, text=True, timeout=900)
                except (OSError, subprocess.SubprocessError) as e:
                    return Result(36, "owner_tag_consistency", "skip", f"go build unavailable: {e}")
                if r.returncode != 0:
                    problems.append(f"{rel}: go build -tags {ts!r} failed: {(r.stderr or r.stdout)[-200:].strip()}")
                else:
                    notes.append(f"{rel}: go build -tags {ts!r} ok")
    if problems:
        return Result(36, "owner_tag_consistency", "fail", "; ".join(problems))
    return Result(36, "owner_tag_consistency", "warn" if warns else "pass", "; ".join(warns + notes))


BACKEND_NAMES = ("agthealth", "event-workers", "recommendation-processor", "dd-source", "dd-go", "redapl", "worker.ts")
COMMENT_RE = re.compile(r"//[^\n]*|/\*.*?\*/", re.S)


def c37(d, worktree, pkg, owner_files, detection=None):
    """Comments in repo code must not name backend services or tool/skill names.

    The scaffold marker is exempt only in todo mode: no check registered and no release note (a detection=todo scaffold
    ships neither). With --detection implemented|partial, or whenever a check or a note exists, it is not stripped."""
    files = sorted(d.glob("*.go")) + sorted((worktree / "comp" / "healthplatform").glob(f"{pkg}*_integration_test.go"))
    files += [Path(f) for f in owner_files if Path(f).is_file()]
    todo_scaffold = (detection not in IMPLEMENTED_DETECTION and not builtin_present(d)
                     and not release_notes(worktree, pkg))
    hits = []
    for f in files:
        text = f.read_text()
        if todo_scaffold:
            text = text.replace(MARKER, "")
        comments = " ".join(COMMENT_RE.findall(text)).lower()
        found = [n for n in BACKEND_NAMES + TOOL_NAMES if n.lower() in comments]
        if found:
            hits.append(f"{f.name}: {', '.join(found)}")
    if hits:
        return Result(37, "no_backend_or_tool_names_in_comments", "fail",
                      "comments name a backend service or a tool (upstream readers cannot resolve them; describe the behavior "
                      "instead): " + "; ".join(hits))
    return Result(37, "no_backend_or_tool_names_in_comments", "pass", "")


def codeowners_rules(path: Path):
    rules = []
    for line in path.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        parts = line.split()
        rules.append((parts[0], parts[1:]))
    return rules


def c38(worktree, pkg, owner_files):
    """Report the recommended CODEOWNERS co-owner line when sibling modules carry a feature-team co-owner (report only)."""
    co = worktree / ".github" / "CODEOWNERS"
    if not co.is_file():
        return Result(38, "codeowners_recommendation", "skip", "no .github/CODEOWNERS in the worktree")
    rules = codeowners_rules(co)
    mine = f"/comp/healthplatform/issues/{pkg}"
    if any(p.rstrip("/") == mine for p, _ in rules):
        return Result(38, "codeowners_recommendation", "pass", "a CODEOWNERS line already exists for the package")
    siblings = [(p, o) for p, o in rules
                if p.startswith("/comp/healthplatform/issues/") and p.rstrip("/") != mine
                and "@DataDog/fleet-remediation" in o and len(o) >= 2]
    if not siblings:
        return Result(38, "codeowners_recommendation", "pass", "no sibling module carries a feature-team co-owner")
    feature = None
    wt = Path(worktree).resolve()
    for f in owner_files:
        try:
            rel = "/" + str(Path(f).resolve().relative_to(wt))
        except ValueError:
            continue
        match = None
        for p, o in rules:  # last matching rule wins
            if rel.startswith(p.rstrip("/") + "/") or rel == p:
                match = o
        if match:
            feature = [x for x in match if x != "@DataDog/fleet-remediation"]
            break
    owner_txt = " ".join(feature) if feature else "@DataDog/<feature-team>"
    line = f"{mine}/ {owner_txt} @DataDog/fleet-remediation"
    return Result(38, "codeowners_recommendation", "warn",
                  f"no CODEOWNERS line for the new directory, while {len(siblings)} sibling module(s) co-own with a feature "
                  f"team. Recommended line (report only, not applied): {line}")


def c39(worktree, pkg):
    """The module table in comp/healthplatform/README.md needs a row for the new package (not generated)."""
    readme = Path(worktree) / "comp" / "healthplatform" / "README.md"
    if not readme.is_file():
        return Result(39, "readme_module_row", "skip", "no comp/healthplatform/README.md in the worktree")
    if pkg in readme.read_text():
        return Result(39, "readme_module_row", "pass", "")
    return Result(39, "readme_module_row", "warn",
                  f"comp/healthplatform/README.md does not mention {pkg}: add a row to the module table (the renderer does not "
                  "write it) or list it in the PR review checklist")


STATE_KEY_RE = re.compile(r'contextKey\w*\s*=\s*"(?:reason|state|status|mode|kind|failure\w*)"')


def c40(d, a):
    """Distinct states (reasons) must not share one title, description and remediation."""
    if not STATE_KEY_RE.search(a["src"]):
        return Result(40, "state_specific_text", "skip", "no reason/state context key in the issue")
    body = strip_comments(a["src"])
    if re.search(r"\bswitch\b|\bcase\b|\bif\b[^\n]*(?:reason|state|status|mode|kind)", body):
        return Result(40, "state_specific_text", "pass", "")
    return Result(40, "state_specific_text", "warn",
                  "the issue carries a reason/state context key but BuildIssue has one text for every state: give each state "
                  "its own title, description and remediation steps, and one issue_test case per state")


MUTANT_ALL_RE = re.compile(r"(?i)\b(all|every|each)\b[^.\n]{0,30}\bmutants?\b[^.\n]{0,40}\b(killed|caught|detected|fail)")
HINT_LEAK_RE = re.compile(r"<!--|\bTODO\b|\bFIXME\b|\{\{|\(fill in|\[describe|REPLACE_ME|\bTBD\b")


def c41(pr_body):
    """PR body: no template hints, no truncated verification section, no blanket mutant-kill claim."""
    if not pr_body:
        return Result(41, "pr_body_sections", "skip", "no --pr-body")
    text = Path(pr_body).read_text()
    warns = []
    if HINT_LEAK_RE.search(text):
        warns.append("template hints or placeholders leak into the PR body")
    m = re.search(r"(?is)#+\s*[^\n]*(?:verification|testing|how was this tested)[^\n]*\n(.*?)(?=\n#+\s|\Z)", text)
    if m is not None:
        sec = m.group(1).rstrip()
        if not re.search(r"(?m)^\s*(?:[-*]|\d+\.)\s+\S", sec) or sec.endswith(("...", "\u2026")):
            warns.append("the verification section has no bullets or looks truncated")
    if MUTANT_ALL_RE.search(text):
        warns.append("claims every mutant was killed: state per mutant the test that failed, and do not count a compile error "
                     "as a kill")
    return Result(41, "pr_body_sections", "warn" if warns else "pass", "; ".join(warns))


def run_gazelle(worktree: Path):
    try:
        r = subprocess.run(["bazel", "run", "//:gazelle", "--", "./comp/healthplatform"], cwd=worktree,
                           capture_output=True, text=True, timeout=1800)
        if r.returncode != 0:
            return f"gazelle failed: {r.stderr[-300:]}"
        d = subprocess.run(["git", "diff", "--stat", "--", "comp/healthplatform"], cwd=worktree,
                           capture_output=True, text=True)
        return None if not d.stdout.strip() else None
    except FileNotFoundError:
        return "bazel not found; gazelle not run"


def run_checks(worktree, pkg, *, path=None, category_ack=False, owner_files=(), pr_body=None, names=(),
               catalog_dir=None, generated_dir=None, gazelle=False, heavy_imports=(), catalog_ts_dir=None,
               detection=None, catalog_shim=False, tag_sets=(), skip_build=False):
    worktree = Path(worktree)
    d = pkg_dir(worktree, pkg)
    if not d.is_dir():
        raise SystemExit(f"package dir not found: {d}")
    files = go_files(d)
    a = {"files": files, "src": "\n".join(files.values()), "path": path or detect_path(d)}
    gz = run_gazelle(worktree) if gazelle else None
    res = [
        c1(d, a), c2(d, a), c3(d, a), c4(d, a), c5(d, a, owner_files), c6(d, a, worktree, pkg),
        c7(d, a, gz), c8(d, a), c9(d, a), c10(d, a, category_ack), c11(catalog_dir, generated_dir),
        c12(d, a), c13(d, a, worktree, pkg), c14(d, a), c15(d, a, worktree), c16(d, a),
        c17(d, a, worktree, pkg, detection), c18(pr_body, names), c19(d, a, worktree, pkg), c20(d, a, heavy_imports),
        c21(d, a), c22(d, a, worktree, pkg, detection), c23(catalog_ts_dir),
        c24(d, a, worktree, pkg, owner_files, detection), c25(d, worktree, pkg, detection),
        c26(d, worktree, pkg), c27(d, worktree, pkg), c28(d, worktree, pkg), c29(d, worktree, pkg),
        c30(d, a, worktree, owner_files), c31(d, a), c32(d, a, worktree), c33(pr_body, catalog_shim, detection),
        c34(a, owner_files), c35(d, worktree, pkg, owner_files), c36(worktree, owner_files, list(tag_sets), skip_build),
        c37(d, worktree, pkg, owner_files, detection), c38(worktree, pkg, owner_files),
        c39(worktree, pkg), c40(d, a), c41(pr_body),
    ]
    return res


def main(argv=None):
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--worktree", required=True)
    ap.add_argument("--pkg", required=True)
    ap.add_argument("--path", choices=["A", "B"])
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--ack-drop-category", action="store_true")
    ap.add_argument("--owner-file", action="append", default=[])
    ap.add_argument("--pr-body")
    ap.add_argument("--names", default="")
    ap.add_argument("--catalog-dir")
    ap.add_argument("--generated-dir")
    ap.add_argument("--gazelle", action="store_true", help="also run real gazelle (slow, needs bazel)")
    ap.add_argument("--catalog-ts-dir", help="dd-source catalog generated/ directory (check 23: prettier formatting)")
    ap.add_argument("--heavy-import", action="append", default=[], metavar="PREFIX",
                    help="extra import path prefix to treat as heavy (repeatable)")
    ap.add_argument("--detection", choices=["todo", "implemented", "partial"],
                    help="detection state (checks 24, 25, 33): the scaffold marker is only allowed while 'todo'")
    ap.add_argument("--catalog-shim", action="store_true",
                    help="a no-op eslint shim was used for the catalog jest gate (check 33: PR body must say eslint was not run)")
    ap.add_argument("--tag-sets", action="append", default=[], metavar="TAGS",
                    help="alternate go build tag set to compile owner packages under (check 36), repeatable, e.g. serverless")
    ap.add_argument("--skip-build", action="store_true", help="do not run `go build` for check 36 (offline unit tests)")
    args = ap.parse_args(argv)
    results = run_checks(args.worktree, args.pkg, path=args.path, category_ack=args.ack_drop_category,
                         detection=args.detection, catalog_shim=args.catalog_shim,
                         tag_sets=args.tag_sets, skip_build=args.skip_build,
                         owner_files=args.owner_file, pr_body=args.pr_body,
                         names=[n.strip() for n in args.names.split(",") if n.strip()],
                         catalog_dir=args.catalog_dir, generated_dir=args.generated_dir, gazelle=args.gazelle,
                         heavy_imports=args.heavy_import, catalog_ts_dir=args.catalog_ts_dir)
    d = pkg_dir(Path(args.worktree), args.pkg)
    ok = not any(r.status == "fail" for r in results)
    payload = {"pkg": args.pkg, "path": args.path or detect_path(d), "ok": ok,
               "results": [asdict(r) for r in results]}
    if args.json:
        print(json.dumps(payload, indent=2))
    else:
        for r in results:
            print(f"[{r.status.upper():4}] {r.id:>2} {r.name}: {r.detail}")
        print("OK" if ok else "FAILED")
    return 0 if ok else 1


if __name__ == "__main__":
    sys.exit(main())
