# ABOUTME: Runs gremlins mutation testing across the Logs Agent Go packages (pkg/logs, comp/logs).
# ABOUTME: Resumable background bulk sweep — builds patched gremlins, scores each package, writes summaries.
from __future__ import annotations

import argparse
import importlib.util
import json
import os
import shutil
import subprocess
import sys
import tempfile
import time
from collections import defaultdict
from dataclasses import asdict, dataclass
from io import StringIO
from pathlib import Path

DATADOG_AGENT_DEFAULT = Path.home() / "repos" / "datadog-agent"
RESULTS_DEFAULT = Path.home() / "research" / "logs-agent-mutation-results"

SCOPE_ROOTS = {
    "pkg-logs": ["pkg/logs"],
    "comp-logs": ["comp/logs"],
    "all": ["pkg/logs", "comp/logs"],
}

GREMLINS_VERSION = "v0.6.0"
GREMLINS_REPO = "https://github.com/go-gremlins/gremlins.git"
PATCH_REL = ".gitlab/mutation-testing/patches/0001-add-test-cmd-and-no-coverage-flags.patch"

# Gremlins runs the --test-cmd from the mutated package's directory, where `dda inv` can't find
# the root go.mod. This wrapper cd's back to the module root first; the mutation is applied
# in-place to the real source, so `dda --targets=./<pkg>` from the root still compiles it.
# GOTMPDIR is unset because gremlins points it at its own workdir, which trips uv/dda.
# --timeout caps go test's own timeout (MUT_TIMEOUT): mutations in this concurrency-heavy code
# frequently deadlock, and the default 180s would dominate runtime. A mutant that hits the cap
# is counted killed; the wrapper records whether each invocation timed out (to MUT_LOG) so the
# report can show what fraction were cap-killed vs detected by a real assertion — that fraction
# tells us if the cap is too tight (legit-but-slow tests) or just catching deadlocks.
# Repo root, target, cap, and log path arrive via env (MUT_REPO_ROOT / MUT_TARGET / MUT_TIMEOUT / MUT_LOG).
DDA_WRAPPER = """#!/usr/bin/env bash
unset GOTMPDIR
cd "$MUT_REPO_ROOT" || exit 2
out=$(dda inv -- -e test --targets="./$MUT_TARGET" --skip-flakes --timeout "${MUT_TIMEOUT:-60}" 2>&1)
rc=$?
if printf '%s' "$out" | grep -q "test timed out"; then t=1; else t=0; fi
printf 'rc=%s timeout=%s\\n' "$rc" "$t" >> "$MUT_LOG"
exit $rc
"""

# Filenames excluded from "does this dir contain mutable source" — mirrors muttest.sh.
EXCLUDE_SUFFIXES = (
    "_test.go",
    ".pb.go",
    "_vtproto.pb.go",
    "_gen.go",
    "_generated.go",
)
EXCLUDE_DIR_PARTS = ("vendor", "mocks")


@dataclass
class Summary:
    target: str  # repo-relative posix package path
    status: str  # ok | baseline_failed | skipped | error | timed_out
    message: str
    files_mutated: int = 0
    total: int = 0
    killed: int = 0
    survived: int = 0
    other: int = 0
    score: float = 0.0
    duration_seconds: float = 0.0
    timeout_mutants: int = 0  # mutants whose test hit the MUT_TIMEOUT cap (counted as killed)


@dataclass
class Target:
    path: str  # repo-relative posix
    slug: str  # path with '/' -> '__'
    test_files: int
    skip_reason: str  # "" if actionable, else requires_build_tags | no_default_tag_tests
    is_submodule: bool = False  # has its own go.mod; gremlins must run from the pkg dir


# muttest_render.py owns the gremlins JSON schema and the killed/survived status
# sets. Import it rather than re-deriving the parsing/scoring logic here.
def _load_renderer(repo: Path):
    mod_path = repo / ".gitlab" / "mutation-testing" / "muttest_render.py"
    spec = importlib.util.spec_from_file_location("muttest_render", mod_path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def main() -> int:
    parser = argparse.ArgumentParser(description="Bulk gremlins mutation testing sweep over the Logs Agent.")
    parser.add_argument("targets", nargs="*", help="Explicit package paths (repo-relative); overrides --scope discovery")
    parser.add_argument("--repo", type=Path, default=DATADOG_AGENT_DEFAULT, help="Path to datadog-agent")
    parser.add_argument("--results-dir", type=Path, default=RESULTS_DEFAULT, help="Where to write reports/status")
    parser.add_argument("--per-package-timeout", type=int, default=7200, help="Wall-clock cap per package (seconds)")
    parser.add_argument("--test-timeout", type=int, default=60, help="go test timeout per mutant (seconds); deadlocking mutants are killed at this cap")
    parser.add_argument("--timeout-coefficient", default="5", help="Gremlins per-mutant timeout multiplier")
    parser.add_argument("--scope", choices=sorted(SCOPE_ROOTS), default="all", help="Which Logs Agent roots to sweep")
    parser.add_argument("--no-dda", action="store_true", help="Use 'go test' instead of 'dda inv test' (escape hatch)")
    parser.add_argument("--gremlins-bin", type=Path, default=None, help="Path to patched gremlins binary (built if absent)")
    parser.add_argument("--force", action="store_true", help="Re-run targets that already have a report.md")
    parser.add_argument("--list", action="store_true", help="Print discovered targets + classification, then exit")
    args = parser.parse_args()

    args.results_dir.mkdir(parents=True, exist_ok=True)
    repo = args.repo.expanduser().resolve()

    if not args.no_dda and shutil.which("dda") is None:
        print("error: 'dda' not on PATH. Install with `brew install --cask dda`, or pass --no-dda.", flush=True)
        return 1

    roots = SCOPE_ROOTS[args.scope]
    targets = discover_targets(repo, roots)
    if args.targets:
        wanted = {t.rstrip("/") for t in args.targets}
        targets = [t for t in targets if t.path in wanted]

    if args.list:
        _print_listing(targets)
        return 0

    if not targets:
        print("No targets discovered.", flush=True)
        return 1

    gremlins_bin = args.gremlins_bin or (repo / ".gitlab" / "mutation-testing" / ".gremlins")
    if not ensure_gremlins(repo, gremlins_bin, args.results_dir / ".gremlins-build.log"):
        print(f"error: could not build patched gremlins. See {args.results_dir / '.gremlins-build.log'}", flush=True)
        return 1

    wrapper_path = args.results_dir / ".ddawrap.sh"
    wrapper_path.write_text(DDA_WRAPPER)
    wrapper_path.chmod(0o755)

    renderer = _load_renderer(repo)

    actionable = [t for t in targets if not t.skip_reason]
    skipped = [t for t in targets if t.skip_reason]
    # Smallest test suites first: bank quick wins early, run the long tail last.
    actionable.sort(key=lambda t: (t.test_files, t.path))

    status_log = args.results_dir / "status.jsonl"
    summaries: list[Summary] = []

    # Record every skipped package so the summary accounts for it.
    for t in skipped:
        s = Summary(t.path, "skipped", t.skip_reason)
        summaries.append(s)
        _append_status(status_log, s)

    to_run = actionable
    if not args.force:
        remaining = [t for t in actionable if not (args.results_dir / t.slug / "report.md").exists()]
        n_skipped = len(actionable) - len(remaining)
        if n_skipped:
            print(f"Skipping {n_skipped} package(s) with existing report.md (use --force to re-run)", flush=True)
        to_run = remaining

    print(f"Running {len(to_run)} package(s); {len(skipped)} skipped (build tags / no default-tag tests).", flush=True)
    print(f"Results dir: {args.results_dir}", flush=True)

    for t in to_run:
        print(f"\n{'='*70}\n{t.path}  (test files: {t.test_files})\n{'='*70}", flush=True)
        s = run_target(
            target=t,
            repo=repo,
            results_dir=args.results_dir,
            gremlins_bin=gremlins_bin,
            timeout_coefficient=str(args.timeout_coefficient),
            per_package_timeout=args.per_package_timeout,
            test_timeout=args.test_timeout,
            use_dda=not args.no_dda,
            wrapper_path=wrapper_path,
            renderer=renderer,
        )
        summaries.append(s)
        _append_status(status_log, s)
        print(f"[{s.status}] {s.target}: {s.message} ({s.duration_seconds:.0f}s)", flush=True)

    write_overall_summary(summaries, args.results_dir, merge_existing=not args.force)
    return 0


def _append_status(path: Path, summary: Summary) -> None:
    with path.open("a") as f:
        f.write(json.dumps(asdict(summary)) + "\n")


def _print_listing(targets: list[Target]) -> None:
    actionable = [t for t in targets if not t.skip_reason]
    skipped = [t for t in targets if t.skip_reason]
    print(f"Discovered {len(targets)} package(s): {len(actionable)} actionable, {len(skipped)} skipped.\n")
    print("Actionable (sorted by test-file count):")
    for t in sorted(actionable, key=lambda x: (x.test_files, x.path)):
        print(f"  {t.test_files:>3} test files  {t.path}")
    print("\nSkipped:")
    for t in sorted(skipped, key=lambda x: x.path):
        print(f"  {t.skip_reason:<22} {t.path}")


def discover_targets(repo: Path, roots: list[str]) -> list[Target]:
    """Find every package dir under the scope roots that holds mutable source, and classify it.

    A package is actionable iff it builds under default tags AND has >=1 default-tag test
    file. The two skip reasons (requires_build_tags, no_default_tag_tests) are recorded so
    the summary accounts for every package rather than silently dropping it.
    """
    pkg_dirs: set[str] = set()
    for root in roots:
        root_dir = repo / root
        if not root_dir.is_dir():
            continue
        for go_file in root_dir.rglob("*.go"):
            if go_file.name.endswith(EXCLUDE_SUFFIXES):
                continue
            rel = go_file.relative_to(repo).as_posix()
            if any(f"/{part}/" in f"/{rel}" for part in EXCLUDE_DIR_PARTS):
                continue
            pkg_dirs.add(go_file.parent.relative_to(repo).as_posix())

    targets: list[Target] = []
    for pkg in sorted(pkg_dirs):
        buildable, n_tests = probe_default_tag_tests(repo, pkg)
        if not buildable:
            reason = "requires_build_tags"
        elif n_tests == 0:
            reason = "no_default_tag_tests"
        else:
            reason = ""
        is_sub = (repo / pkg / "go.mod").is_file()
        targets.append(Target(path=pkg, slug=pkg.replace("/", "__"), test_files=n_tests, skip_reason=reason, is_submodule=is_sub))
    return targets


def probe_default_tag_tests(repo: Path, pkg: str) -> tuple[bool, int]:
    """Return (buildable_under_default_tags, n_test_files) for a package.

    Uses `go list -tags='' -f '{{len .TestGoFiles}}+{{len .XTestGoFiles}}' .` — stronger than
    muttest.sh's exit-code-only check: catches packages that build via stub files but carry no
    default-tag tests (e.g. journald/windowsevent launchers), which would otherwise waste a run.
    """
    proc = subprocess.run(
        ["go", "list", "-tags=", "-f", "{{len .TestGoFiles}}+{{len .XTestGoFiles}}", "."],
        cwd=repo / pkg, capture_output=True, text=True, check=False,
    )
    if proc.returncode != 0:
        return False, 0
    try:
        a, b = proc.stdout.strip().split("+")
        return True, int(a) + int(b)
    except (ValueError, AttributeError):
        return True, 0


def ensure_gremlins(repo: Path, gremlins_bin: Path, log_path: Path) -> bool:
    """Build the patched gremlins binary once. No-op if it already exists and is executable.

    Mirrors muttest.sh: clone v0.6.0 into a temp dir, apply the --test-cmd/--no-coverage patch,
    `go build`. Never touches the datadog-agent repo itself.
    """
    if os.access(gremlins_bin, os.X_OK):
        return True

    patch = repo / PATCH_REL
    if not patch.is_file():
        log_path.write_text(f"patch not found: {patch}\n")
        return False

    print(f"Building patched gremlins {GREMLINS_VERSION} -> {gremlins_bin}", flush=True)
    gremlins_bin.parent.mkdir(parents=True, exist_ok=True)
    src = Path(tempfile.mkdtemp(prefix="gremlins-src-"))
    log = StringIO()
    try:
        steps = [
            ["git", "clone", "--depth", "1", "--branch", GREMLINS_VERSION, GREMLINS_REPO, str(src)],
            ["git", "apply", str(patch)],
            ["go", "build", "-o", str(gremlins_bin), "./cmd/gremlins"],
        ]
        for i, cmd in enumerate(steps):
            cwd = src if i > 0 else None
            log.write(f"$ {' '.join(cmd)}\n")
            proc = subprocess.run(cmd, cwd=cwd, capture_output=True, text=True, check=False, timeout=600)
            log.write(proc.stdout + proc.stderr + "\n")
            if proc.returncode != 0:
                log_path.write_text(log.getvalue())
                return False
    except subprocess.TimeoutExpired as e:
        log.write(f"\nTIMEOUT: {e}\n")
        log_path.write_text(log.getvalue())
        return False
    finally:
        shutil.rmtree(src, ignore_errors=True)

    log_path.write_text(log.getvalue())
    return os.access(gremlins_bin, os.X_OK)


def run_target(
    *,
    target: Target,
    repo: Path,
    results_dir: Path,
    gremlins_bin: Path,
    timeout_coefficient: str,
    per_package_timeout: int,
    test_timeout: int,
    use_dda: bool,
    wrapper_path: Path,
    renderer,
) -> Summary:
    start = time.time()
    out_dir = results_dir / target.slug
    out_dir.mkdir(parents=True, exist_ok=True)
    result_file = out_dir / "results.json"
    if result_file.exists():
        result_file.unlink()
    mutant_log = out_dir / "mutant_runs.log"
    if mutant_log.exists():
        mutant_log.unlink()

    env = dict(os.environ)
    if use_dda:
        # The wrapper cd's to the repo root and runs dda for MUT_TARGET (see DDA_WRAPPER).
        test_cmd = str(wrapper_path)
        env["MUT_REPO_ROOT"] = str(repo)
        env["MUT_TARGET"] = target.path
        env["MUT_TIMEOUT"] = str(test_timeout)
        env["MUT_LOG"] = str(mutant_log)
    else:
        test_cmd = "go test"

    # Gremlins v0.6.0 looks for go.mod in cwd only (doesn't traverse in a workspace).
    # Sub-modules have their own go.mod, so run from the package dir with `./`.
    # Root-module packages must run from the repo root with the package path as target.
    if target.is_submodule:
        gremlins_cwd = repo / target.path
        gremlins_target = "./"
    else:
        gremlins_cwd = repo
        gremlins_target = f"./{target.path}"

    cmd = [
        str(gremlins_bin), "unleash",
        "--silent",
        "--no-coverage",
        # Serial: concurrent dda launches race in uv's shared tool dir and fail spuriously,
        # which gremlins would miscount as killed.
        "--workers", "1",
        "--test-cmd", test_cmd,
        "--output", str(result_file),
        "--threshold-efficacy", "0",
        "--threshold-mcover", "0",
        "--timeout-coefficient", timeout_coefficient,
        gremlins_target,
    ]

    timed_out = False
    returncode = None
    with (out_dir / "gremlins.log").open("w") as logf:
        logf.write(f"$ cd {gremlins_cwd.relative_to(repo)} && {' '.join(cmd)}\n  (test-cmd -> {test_cmd}, MUT_TARGET={target.path}, submodule={target.is_submodule})\n\n")
        logf.flush()
        try:
            proc = subprocess.run(
                cmd, cwd=gremlins_cwd, stdout=logf, stderr=subprocess.STDOUT,
                timeout=per_package_timeout, check=False, env=env,
            )
            returncode = proc.returncode
        except subprocess.TimeoutExpired:
            timed_out = True
            logf.write(f"\n\nTIMEOUT after {per_package_timeout}s\n")

    mutants = renderer._load_mutants(out_dir) if result_file.exists() else []
    killed = sum(1 for m in mutants if m["status"] in renderer.KILLED)
    survived = sum(1 for m in mutants if m["status"] in renderer.SURVIVED)
    run_errors = sum(1 for m in mutants if m["status"] == "RUN_ERROR")
    total = len(mutants)
    other = total - killed - survived
    denom = killed + survived
    score = (killed / denom * 100) if denom else 0.0
    files_mutated = len({m["file"] for m in mutants})
    timeout_mutants = _count_timeout_mutants(mutant_log)

    report = generate_report(mutants, target=target.path, killed=killed, survived=survived, other=other, score=score)
    (out_dir / "report.md").write_text(report)

    duration = time.time() - start
    to_note = f", {timeout_mutants} hit {test_timeout}s cap" if timeout_mutants else ""
    if timed_out:
        status = "timed_out"
        message = f"(partial) {total} mutants, {killed} killed, {survived} survived{to_note} — hit {per_package_timeout}s package cap"
    elif total == 0:
        # gremlins ran but emitted nothing: nonzero exit => build/baseline failure, else genuinely no mutants.
        if returncode not in (0, None):
            status, message = "baseline_failed", _tail_log(out_dir, returncode)
        else:
            status, message = "ok", "no mutants generated"
    elif denom == 0 and run_errors:
        status = "baseline_failed"
        message = f"all {total} mutants RUN_ERROR — tests likely fail at baseline"
    else:
        status = "ok"
        message = f"{total} mutants, {killed} killed, {survived} survived, {other} other{to_note}"

    return Summary(
        target=target.path, status=status, message=message, files_mutated=files_mutated,
        total=total, killed=killed, survived=survived, other=other, score=score,
        duration_seconds=duration, timeout_mutants=timeout_mutants,
    )


def _count_timeout_mutants(mutant_log: Path) -> int:
    """Count mutant runs whose test hit the MUT_TIMEOUT cap (wrapper logged `timeout=1`)."""
    if not mutant_log.exists():
        return 0
    return sum(1 for line in mutant_log.read_text().splitlines() if line.strip().endswith("timeout=1"))


def _tail_log(out_dir: Path, returncode: int) -> str:
    try:
        tail = "; ".join((out_dir / "gremlins.log").read_text().strip().splitlines()[-3:])[:400]
    except OSError:
        tail = ""
    return f"gremlins exit {returncode}: {tail}"


def generate_report(mutants: list[dict], *, target: str, killed: int, survived: int, other: int, score: float) -> str:
    out = StringIO()
    out.write(f"# Mutation Testing: {target}\n\n")
    if not mutants:
        out.write("No mutants generated.\n")
        return out.getvalue()

    total = len(mutants)
    out.write(
        f"**Score: {score:.1f}%** ({killed} killed / {killed + survived} actionable, "
        f"{survived} survived, {other} non-actionable, {total} total)\n\n"
    )

    by_file: dict[str, list[dict]] = defaultdict(list)
    for m in mutants:
        by_file[m["file"]].append(m)

    out.write("## Per-file\n\n")
    out.write("| File | Total | Killed | Survived | Other | Score |\n")
    out.write("|------|------:|-------:|---------:|------:|------:|\n")
    for f, ms in sorted(by_file.items()):
        from_killed = sum(1 for m in ms if m["status"] in {"KILLED"})
        from_survived = sum(1 for m in ms if m["status"] in {"LIVED", "NOT_COVERED"})
        from_other = len(ms) - from_killed - from_survived
        denom = from_killed + from_survived
        fs = (from_killed / denom * 100) if denom else 0.0
        out.write(f"| `{f}` | {len(ms)} | {from_killed} | {from_survived} | {from_other} | {fs:.1f}% |\n")

    survivors = [m for m in mutants if m["status"] in {"LIVED", "NOT_COVERED"}]
    if survivors:
        out.write(f"\n## Surviving mutants ({len(survivors)})\n\n")
        by_file_surv: dict[str, list[dict]] = defaultdict(list)
        for m in survivors:
            by_file_surv[m["file"]].append(m)
        for f, ms in sorted(by_file_surv.items()):
            out.write(f"### `{f}`\n\n")
            for m in sorted(ms, key=lambda x: (x["line"], x["column"])):
                out.write(f"- **Line {m['line']}** ({m['type']}) — `{m['status']}`\n")
                if m["diff"]:
                    out.write(f"```diff\n{m['diff']}\n```\n")

    return out.getvalue()


def write_overall_summary(summaries: list[Summary], results_dir: Path, *, merge_existing: bool) -> None:
    all_summaries: list[Summary] = []
    if merge_existing:
        status_log = results_dir / "status.jsonl"
        if status_log.exists():
            seen = set()
            for line in reversed(status_log.read_text().splitlines()):
                if not line.strip():
                    continue
                try:
                    data = json.loads(line)
                except json.JSONDecodeError:
                    continue
                if data["target"] in seen:
                    continue
                seen.add(data["target"])
                all_summaries.append(Summary(**data))
            all_summaries.reverse()
    else:
        all_summaries = summaries

    (results_dir / "summary.json").write_text(json.dumps([asdict(s) for s in all_summaries], indent=2))

    by_status: dict[str, int] = defaultdict(int)
    for s in all_summaries:
        by_status[s.status] += 1

    ok = [s for s in all_summaries if s.status == "ok"]
    agg_killed = sum(s.killed for s in ok)
    agg_survived = sum(s.survived for s in ok)
    agg_other = sum(s.other for s in ok)
    agg_denom = agg_killed + agg_survived
    agg_score = (agg_killed / agg_denom * 100) if agg_denom else 0.0
    agg_timeout = sum(s.timeout_mutants for s in ok)

    md = StringIO()
    md.write("# Logs Agent Mutation Testing Summary\n\n")
    md.write(f"**Total packages processed:** {len(all_summaries)}\n\n")
    md.write("**By status:** " + ", ".join(f"{k}={v}" for k, v in sorted(by_status.items())) + "\n\n")
    md.write(
        f"**Aggregate (ok packages):** {agg_killed} killed / {agg_denom} actionable "
        f"({agg_score:.1f}%), {agg_survived} survived, {agg_other} non-actionable\n\n"
    )
    if agg_killed:
        md.write(
            f"**Test-timeout kills:** {agg_timeout} of {agg_killed} killed mutants "
            f"({agg_timeout / agg_killed * 100:.0f}%) hit the per-mutant test timeout rather than a "
            f"real assertion. A high fraction in a package with slow tests may mean the cap is too tight; "
            f"in concurrency-heavy code it usually just reflects deadlock-inducing mutations.\n\n"
        )

    md.write("## Per package\n\n")
    md.write("| Target | Status | Score | Killed | Survived | Total | Timeout-kills | Duration(s) |\n")
    md.write("|--------|--------|------:|-------:|---------:|------:|--------------:|------------:|\n")
    for s in sorted(all_summaries, key=lambda x: (x.status != "ok", -x.score, x.target)):
        if s.status == "skipped":
            continue
        md.write(
            f"| {s.target} | {s.status} | {s.score:.1f}% | {s.killed} | "
            f"{s.survived} | {s.total} | {s.timeout_mutants} | {s.duration_seconds:.1f} |\n"
        )

    skipped = [s for s in all_summaries if s.status == "skipped"]
    if skipped:
        md.write("\n## Skipped\n\n")
        for s in sorted(skipped, key=lambda x: x.target):
            md.write(f"- `{s.target}` — {s.message}\n")

    failed = [s for s in all_summaries if s.status in ("baseline_failed", "error", "timed_out")]
    if failed:
        md.write("\n## Non-successful details\n\n")
        for s in failed:
            md.write(f"- **{s.target}** ({s.status}): {s.message}\n")

    (results_dir / "summary.md").write_text(md.getvalue())
    print(f"\nSummary written to {results_dir / 'summary.md'}", flush=True)


if __name__ == "__main__":
    sys.exit(main())
