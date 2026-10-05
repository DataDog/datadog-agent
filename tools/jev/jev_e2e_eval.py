#!/usr/bin/env python3
"""Evaluate the Jev-based E2E test selection against a past PR.

Replays the selection on a merged/closed PR: fetches the PR (head, title,
description, diff), checks it out in a temporary git worktree, asks Jev for
every E2E test entry point whether it should run, then compares with the
tests that were actually executed in CI (Datadog CI Visibility), and reports:

- tests Jev would run vs tests that actually ran
- skipped-and-passed (savings), skipped-but-failed (false negatives),
  flaky-skipped (excluded from false negatives)
- extra runs Jev would add

Usage (from the repo root, a full git checkout):
    GITHUB_TOKEN=... DD_API_KEY=... python3 tools/jev/jev_e2e_eval.py --pr 57508

Executed-test lookup: by default queries test events for the PR head commit
(branch pipelines). Pass --pipeline-id <gitlab pipeline id> to evaluate a
specific pipeline instead (e.g. the merge-queue pipeline, whose commit differs
from the PR head).

Environment:
    GITHUB_TOKEN   optional, avoids GitHub API rate limits
    DD_API_KEY     Datadog API key, required for the executed-test lookup
    (AI Gateway token: same as jev_e2e_selector.py --token/--token-cmd/ddtool)
"""

from __future__ import annotations  # python 3.9 compat

import argparse
import json
import os
import re
import shutil
import sys
import tempfile
import urllib.request

# make tools/jev and the repo root (for tasks.libs...) importable
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, REPO_ROOT)
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from jev_e2e_selector import (  # noqa: E402
    GITHUB_API,
    ask_jev,
    build_state,
    get_ai_gateway_token,
    list_suites,
    suite_definition,
)

E2E_TESTS_DIR = "test/new-e2e/tests"
PIPELINE_NAME = "DataDog/datadog-agent"


# ---------------------------------------------------------------- PR checkout


def fetch_pr(pr_number: int) -> dict:
    """PR metadata from the GitHub API (token optional for public repos)."""
    req = urllib.request.Request(f"{GITHUB_API}/pulls/{pr_number}")
    if os.environ.get("GITHUB_TOKEN"):
        req.add_header("Authorization", f"Bearer {os.environ['GITHUB_TOKEN']}")
    req.add_header("Accept", "application/vnd.github+json")
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)


def git(*args: str, cwd: str | None = None) -> str:
    import subprocess

    res = subprocess.run(["git", *args], capture_output=True, text=True, cwd=cwd)
    if res.returncode != 0:
        raise RuntimeError(f"git {' '.join(args)} failed: {res.stderr.strip()}")
    return res.stdout.strip()


# ---------------------------------------------------------------- executed tests


def fetch_executed_e2e_tests(query: str, days: int) -> dict:
    """Root-level e2e tests actually executed, from CI Visibility.

    Returns {entry_point: {"status": "pass|fail", "flaky": bool, "jobs": [..]}}
    """
    from tasks.libs.common.datadog_api import get_ci_test_events

    events = get_ci_test_events(query, days)
    executed: dict = {}
    sample_names = []
    for item in events:
        attrs = item.get("attributes", {}).get("attributes", {})
        test_attrs = attrs.get("test", {})
        name = test_attrs.get("name") or ""
        # e2e tests only, root-level only (sub-tests contain '/')
        if "new-e2e" not in name or "/" in name:
            if len(sample_names) < 5:
                sample_names.append(name)
            continue
        m = re.search(r"(Test\w+)\s*$", name)
        if not m:
            continue
        entry = m.group(1)
        status = "pass" if test_attrs.get("status") == "pass" else "fail"
        flaky = test_attrs.get("agent_is_flaky_failure", "false") == "true"
        e = executed.setdefault(entry, {"status": "pass", "flaky": False, "jobs": []})
        if status == "fail":
            e["status"] = "fail"
        if flaky:
            e["flaky"] = True
        e["jobs"].append((attrs.get("ci", {}).get("job", {}) or {}).get("name", ""))
    print(f"[info] {len(executed)} root e2e tests actually executed ({query})")
    if not executed:
        print(f"[warn] no executed e2e tests matched; sample test names seen: {sample_names}")
    return executed


# ---------------------------------------------------------------- main


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--pr", type=int, required=True, help="PR number to replay")
    parser.add_argument("--suite", default=None, help="Restrict to one e2e suite (default: all)")
    parser.add_argument("--pipeline-id", default=None, help="GitLab pipeline id of the executed run to compare with")
    parser.add_argument("--base", default="origin/main", help="Base ref used for the merge base in the worktree")
    parser.add_argument("--days", type=int, default=90, help="CI Visibility lookback window in days")
    parser.add_argument("--dry-run", action="store_true", help="Skip Jev calls and the executed lookup (debug)")
    parser.add_argument("--model", default="datadoginternal/openjev-medium")
    parser.add_argument("--dc", default="us1.ddbuild.io", help="AI Gateway datacenter")
    parser.add_argument("--source", default="datadog-agent", help="source header for AI Gateway")
    parser.add_argument("--token", default=None)
    parser.add_argument("--token-cmd", default=None)
    parser.add_argument("--output", default=None, help="JSON output path (default jev_e2e_eval_<pr>.json)")
    args = parser.parse_args()

    pr = fetch_pr(args.pr)
    head, title = pr["head"]["sha"], pr["title"]
    print(f"[info] PR #{args.pr}: {title}")
    print(f"[info] head {head[:12]} base {pr['base']['sha'][:12]} state {pr['state']}")

    # Checkout the PR head in a worktree to read the test code as of that PR
    git("fetch", "origin", f"pull/{args.pr}/head")
    worktree = tempfile.mkdtemp(prefix=f"jev-eval-pr{args.pr}-")
    git("worktree", "add", "--detach", worktree, "FETCH_HEAD")
    try:
        merge_base = git("merge-base", "HEAD", args.base, cwd=worktree)
        files = [(f, "") for f in git("diff", "--name-only", merge_base, "HEAD", cwd=worktree).splitlines()]
        diff = pr_diff_module_diff(merge_base, worktree)
        pr_info = {"branch": worktree, "title": title, "description": pr.get("body") or "", "author": pr["user"]["login"]}
        print(f"[info] {len(files)} changed files, diff {len(diff)} chars (merge base {merge_base[:12]})")

        # Decide which suites to evaluate
        suites_root = os.path.join(worktree, E2E_TESTS_DIR)
        suite_names = sorted(os.listdir(suites_root)) if not args.suite else [args.suite]
        decisions = []
        token = None if args.dry_run else get_ai_gateway_token(args)
        for suite in suite_names:
            suite_dir = os.path.join(suites_root, suite)
            if not os.path.isdir(suite_dir):
                continue
            entries = list_suites(suite_dir)
            if not entries:
                continue
            _, suite_def_code = suite_definition(suite_dir)
            print(f"[info] suite {suite}: {len(entries)} tests")
            for name, path, code in entries:
                path = os.path.relpath(path, worktree)
                if args.dry_run:
                    decisions.append({"test": name, "suite": suite, "decision": "run", "dry_run": True})
                    continue
                state = build_state(
                    name, path, code, suite, suite, pr_info, files, merge_base, diff,
                    suite_def_code=suite_def_code,
                )
                try:
                    answer = ask_jev(args, token, state)
                except Exception as e:
                    print(f"[warn] Jev failed for {suite}/{name}: {e} -> defaulting to RUN")
                    decisions.append({"test": name, "suite": suite, "decision": "run", "error": str(e)})
                    continue
                a = answer["answers"]
                should = a["should_execute"]["noul"]
                relation = a["relation"]["choice"]
                decision = "run" if should >= 0.5 or relation != "unrelated" else "skip"
                decisions.append(
                    {
                        "test": name, "suite": suite, "decision": decision,
                        "should_execute": should, "relation": relation,
                        "confidence": a["confidence"]["score"],
                    }
                )
                print(f"[jev] {suite}/{name:<45} -> {decision.upper()}  {should:.2f} {relation}")

        # What actually ran
        if args.dry_run:
            executed = {}
        else:
            scope = f"@ci.pipeline.id:{args.pipeline_id}" if args.pipeline_id else f"@git.commit.sha:{head}"
            executed = fetch_executed_e2e_tests(
                f"env:prod @ci.pipeline.name:{PIPELINE_NAME} {scope}", args.days
            )
    finally:
        git("worktree", "remove", "--force", worktree)
        shutil.rmtree(worktree, ignore_errors=True)

    would_run = {d["test"] for d in decisions if d["decision"] == "run"}
    would_skip = {d["test"] for d in decisions if d["decision"] == "skip"}
    ran = set(executed)
    report = {
        "pr": args.pr, "title": title, "head": head,
        "pipeline_id": args.pipeline_id,
        "jev": {
            "run": sorted(would_run), "skip": sorted(would_skip),
            "decisions": decisions,
        },
        "executed": executed,
        "comparison": {
            "ran_total": len(ran),
            "correctly_skipped": sorted(ran & would_skip - {t for t, v in executed.items() if v["status"] == "fail"}),
            "skipped_but_ran": sorted(ran & would_skip),
            "false_negatives": sorted(
                t for t in (ran & would_skip)
                if executed[t]["status"] == "fail" and not executed[t]["flaky"]
            ),
            "flaky_skipped": sorted(
                t for t in (ran & would_skip) if executed[t]["status"] == "fail" and executed[t]["flaky"]
            ),
            "extra_runs": sorted(would_run - ran),
            "kept_failures": sorted(
                t for t in (ran & would_run) if executed[t]["status"] == "fail"
            ),
        },
    }
    c = report["comparison"]
    savings = (len(c["correctly_skipped"]) / len(ran) * 100) if ran else 0.0
    print("\n================ EVALUATION SUMMARY ================")
    print(f"PR #{args.pr}: {title}")
    print(f"E2E entry points actually executed:        {len(ran)}")
    print(f"Jev would run / skip (all suites):          {len(would_run)} / {len(would_skip)}")
    print(f"Tests Jev skipped that actually ran:        {len(c['skipped_but_ran'])}")
    print(f"  - of which passed (correct skips):        {len(c['correctly_skipped'])}")
    print(f"  - of which FAILED (flaky excluded):       {len(c['false_negatives'])} {c['false_negatives']}")
    print(f"  - of which failed but flaky:              {len(c['flaky_skipped'])}")
    print(f"Tests Jev would run that did not run:      {len(c['extra_runs'])}")
    print(f"Failures kept by Jev (ran & failed):       {len(c['kept_failures'])}")
    print(f"Savings (skipped share of executed):       {savings:.0f}%")
    if c["false_negatives"]:
        print("\n!! Jev would have skipped tests that FAILED on this PR - see 'false_negatives'")
    output = args.output or f"jev_e2e_eval_{args.pr}.json"
    with open(output, "w") as f:
        json.dump(report, f, indent=2)
    print(f"[info] full report written to {output}")
    return 0


def pr_diff_module_diff(merge_base: str, cwd: str) -> str:
    """pr_diff() from the selector, executed in the worktree directory."""
    import jev_e2e_selector as sel

    old = os.getcwd()
    try:
        os.chdir(cwd)
        return sel.pr_diff(merge_base)
    finally:
        os.chdir(old)


if __name__ == "__main__":
    sys.exit(main())
