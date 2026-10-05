#!/usr/bin/env python3
"""Evaluate the Jev-based E2E test selection against a past PR.

Replays the selection on a merged/closed PR: fetches the PR (head, title,
description, diff), checks it out in a temporary git worktree, asks Jev for
every E2E test entry point, then compares with the tests that actually
executed in CI, and reports savings, false negatives and disagreements with
the coverage-based selection. The run/skip decision comes from the single
source in jev_client.decide, shared with the selector.

Usage (from the repo root, a full git checkout):
    GITHUB_TOKEN=... DD_API_KEY=... DD_APP_KEY=... python3 tools/jev/jev_e2e_eval.py --pr 57508

Executed-test lookup: by default queries CI Visibility by the PR head commit.
Pass --pipeline-id <gitlab pipeline id> to scope to a specific pipeline, or
--from-artifacts <pipeline id> to read the pipeline's e2e_test_output.json
job artifacts instead (no CI Visibility keys needed).

Environment:
    GITHUB_TOKEN   optional, avoids GitHub API rate limits
    DD_API_KEY     Datadog API key, for the CI Visibility lookup
    DD_APP_KEY     Datadog app key, for the CI Visibility lookup
    (AI Gateway token: same as jev_e2e_selector.py --token/--token-cmd/ddtool)
"""

from __future__ import annotations  # python 3.9 compat

import argparse
import concurrent.futures
import json
import os
import shutil
import sys
import tempfile
import threading
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from diff_utils import _annotate_diff, files_from_diff
from executed_lookup import (
    PIPELINE_NAME,
    allow_failure_jobs,
    fetch_executed_e2e_tests,
    fetch_executed_from_gitlab,
)
from jev_client import DEFAULT_RUN_THRESHOLD, ask_jev, build_state, decide, fail_open, get_ai_gateway_token
from pr_context import detect_pr_number, fetch_pr, fetch_pr_diff, git
from test_discovery import E2E_TESTS_DIR, list_suites, suite_definition

REPO_URL = "https://github.com/DataDog/datadog-agent.git"


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument(
        "--pr", type=int, default=None, help="PR number to replay (default: detect from the current branch)"
    )
    parser.add_argument(
        "--from-artifacts",
        metavar="PIPELINE_ID",
        default=None,
        help="Fetch the executed tests from the pipeline's e2e_test_output.json "
        "artifacts (GitLab API) instead of CI Visibility",
    )
    parser.add_argument("--suite", default=None, help="Restrict to one e2e suite (default: all)")
    parser.add_argument("--pipeline-id", default=None, help="GitLab pipeline id of the executed run to compare with")
    parser.add_argument("--days", type=int, default=90, help="CI Visibility lookback window in days")
    parser.add_argument(
        "--concurrency",
        type=int,
        default=8,
        help="parallel Jev calls (the executed lookup runs concurrently too); 1 = sequential",
    )
    parser.add_argument("--dry-run", action="store_true", help="Skip Jev calls and the executed lookup (debug)")
    parser.add_argument("--model", default="datadoginternal/openjev-medium")
    parser.add_argument("--dc", default="us1.ddbuild.io", help="AI Gateway datacenter")
    parser.add_argument("--source", default="datadog-agent", help="source header for AI Gateway")
    parser.add_argument(
        "--run-threshold",
        type=float,
        default=DEFAULT_RUN_THRESHOLD,
        help="should_execute threshold of the decision (see jev_client.decide)",
    )
    parser.add_argument("--token", default=None)
    parser.add_argument("--token-cmd", default=None)
    parser.add_argument("--output", default=None, help="JSON output path (default jev_e2e_eval_<pr>.json)")
    args = parser.parse_args()

    pr_number = args.pr or detect_pr_number()
    if not pr_number:
        print("error: no PR number found (pass --pr, or run on a PR branch)", file=sys.stderr)
        return 1
    pr = fetch_pr(pr_number)
    head, title = pr["head"]["sha"], pr["title"]
    print(f"[info] PR #{pr_number}: {title}")
    print(f"[info] head {head[:12]} base {pr['base']['sha'][:12]} state {pr['state']}")

    # Local runs: no authanywhere in CI, so the token comes from ddtool for
    # us1.staging.dog; align the gateway DC so the token and the endpoint match.
    if not args.token and not args.token_cmd and not os.environ.get("AI_GATEWAY_TOKEN") and args.dc == "us1.ddbuild.io":
        print("[info] local run without a token command: using the us1.staging.dog gateway (ddtool token)")
        args.dc = "us1.staging.dog"

    # Checkout the PR head in a worktree. The PR head is fetched from GitHub
    # directly (GitLab mirrors do not carry refs/pull/*), depth-1: only the
    # test code is read from it; the diff comes from the GitHub API.
    git("fetch", "--depth=1", REPO_URL, f"pull/{pr_number}/head")
    worktree = tempfile.mkdtemp(prefix=f"jev-eval-pr{pr_number}-")
    git("worktree", "add", "--detach", worktree, "FETCH_HEAD")
    try:
        # Diff and changed files from the GitHub API (PR base as merge base)
        raw_diff = fetch_pr_diff(pr_number)
        merge_base = pr["base"]["sha"]
        files = files_from_diff(raw_diff)
        old = os.getcwd()
        os.chdir(worktree)  # file line counts for the percentage annotations
        try:
            diff = _annotate_diff(raw_diff, f"{len(files)} files changed (GitHub PR diff)")
        finally:
            os.chdir(old)
        pr_info = {
            "branch": worktree,
            "title": title,
            "description": pr.get("body") or "",
            "author": pr["user"]["login"],
        }
        print(f"[info] {len(files)} changed files, diff {len(diff)} chars (base {merge_base[:12]})")

        # Decide which suites to evaluate
        suites_root = os.path.join(worktree, E2E_TESTS_DIR)
        suite_names = sorted(os.listdir(suites_root)) if not args.suite else [args.suite]
        # Discover all suites/tests first, then fan the Jev calls out in parallel
        tasks = []
        for suite in suite_names:
            suite_dir = os.path.join(suites_root, suite)
            if not os.path.isdir(suite_dir):
                continue
            entries = list_suites(suite_dir)
            if not entries:
                continue
            _, suite_def_code = suite_definition(suite_dir)
            print(f"[info] suite {suite}: {len(entries)} tests")
            tasks.extend(
                (suite, name, os.path.relpath(path, worktree), code, suite_def_code) for name, path, code in entries
            )

        token = None if args.dry_run else get_ai_gateway_token(args)

        def evaluate(entry):
            """One Jev decision for one test entry point (thread-safe: pure inputs)."""
            suite, name, path, code, suite_def_code = entry
            state = build_state(
                name,
                path,
                code,
                suite,
                suite,
                pr_info,
                files,
                merge_base,
                diff,
                suite_def_code=suite_def_code,
            )
            try:
                answer = ask_jev(args, token, state)
            except Exception as e:
                # Single decision source, fail open: if Jev is unavailable, run
                return {"test": name, "suite": suite, **fail_open(e)}
            # Single decision source: jev_client.decide (shared with the selector)
            return {
                "test": name,
                "suite": suite,
                "usage": answer.get("usage"),
                **decide(answer["answers"], args.run_threshold),
            }

        print_lock = threading.Lock()

        def report_result(d):
            line = f"{d['suite']}/{d['test']:<45}"
            if "error" in d:
                with print_lock:
                    print(f"[warn] {line} -> Jev failed: {d['error']} -> defaulting to RUN")
            else:
                with print_lock:
                    print(f"[jev] {line} -> {d['decision'].upper()}  {d['should_execute']:.2f} {d['relation']}")

        started = time.monotonic()
        coverage_skipped: dict = {}
        if args.dry_run:
            decisions = [{"test": t[1], "suite": t[0], "decision": "run", "dry_run": True} for t in tasks]
            executed = {}
        else:
            # The executed-test lookup is independent of the Jev calls: run it
            # concurrently in the same pool (+1 worker for it).
            with concurrent.futures.ThreadPoolExecutor(max_workers=args.concurrency + 1) as pool:
                if args.from_artifacts:
                    executed_future = pool.submit(fetch_executed_from_gitlab, args.from_artifacts)
                else:
                    scope = f"@ci.pipeline.id:{args.pipeline_id}" if args.pipeline_id else f"@git.commit.sha:{head}"
                    # exclude allow_failure jobs: their failures do not gate
                    # the pipeline, counting them would overstate selection risk
                    af_jobs = allow_failure_jobs(args.pipeline_id, head)
                    # Query-side filtering: e2e jobs only (see executed_lookup
                    # for the full filter rationale, incl. the env:nativetest
                    # caveat).
                    executed_future = pool.submit(
                        fetch_executed_e2e_tests,
                        f"@ci.pipeline.name:{PIPELINE_NAME} {scope} @ci.job.name:new-e2e* -@test.name:*/*",
                        args.days,
                        af_jobs,
                    )
                if args.concurrency <= 1:
                    decisions = []
                    for entry in tasks:
                        d = evaluate(entry)
                        decisions.append(d)
                        report_result(d)
                else:
                    futures = [pool.submit(evaluate, entry) for entry in tasks]
                    decisions = []
                    for fut in concurrent.futures.as_completed(futures):
                        d = fut.result()
                        decisions.append(d)
                        report_result(d)
                executed = ({}, {})
                try:
                    executed = executed_future.result()
                except Exception as e:
                    # fail open on lookup errors: the comparison just runs
                    # against an empty executed set
                    print(f"[warn] executed-test lookup failed: {e} -> comparison skipped")
                if isinstance(executed, dict):
                    executed, coverage_skipped = executed, {}
                else:
                    executed, coverage_skipped = executed
            print(
                f"[info] {len(decisions)} Jev decisions in {time.monotonic() - started:.0f}s (concurrency {args.concurrency})"
            )
    finally:
        git("worktree", "remove", "--force", worktree)
        shutil.rmtree(worktree, ignore_errors=True)

    decisions.sort(key=lambda d: (d["suite"], d["test"]))  # stable report despite parallel completion order

    would_run = {d["test"] for d in decisions if d["decision"] == "run"}
    would_skip = {d["test"] for d in decisions if d["decision"] == "skip"}
    ran = set(executed)
    report = {
        "pr": pr_number,
        "title": title,
        "head": head,
        "pipeline_id": args.pipeline_id,
        "jev": {
            "run": sorted(would_run),
            "skip": sorted(would_skip),
            "decisions": decisions,
        },
        "executed": executed,
        "comparison": {
            "ran_total": len(ran),
            "correctly_skipped": sorted(ran & would_skip - {t for t, v in executed.items() if v["status"] == "fail"}),
            "skipped_but_ran": sorted(ran & would_skip),
            "false_negatives": sorted(
                t for t in (ran & would_skip) if executed[t]["status"] == "fail" and not executed[t]["flaky"]
            ),
            "flaky_skipped": sorted(
                t for t in (ran & would_skip) if executed[t]["status"] == "fail" and executed[t]["flaky"]
            ),
            "extra_runs": sorted(would_run - ran),
            "kept_failures": sorted(t for t in (ran & would_run) if executed[t]["status"] == "fail"),
        },
    }
    if coverage_skipped:
        report["comparison"]["coverage_selection"] = {
            "skipped": sorted(coverage_skipped),
            "jev_also_skips": sorted(set(coverage_skipped) & would_skip),
            "jev_disagrees": sorted(set(coverage_skipped) & would_run),
        }
    c = report["comparison"]
    savings = (len(c["correctly_skipped"]) / len(ran) * 100) if ran else 0.0
    print("\n================ EVALUATION SUMMARY ================")
    print(f"PR #{pr_number}: {title}")
    print(f"E2E entry points actually executed:        {len(ran)}")
    print(f"Jev would run / skip (all suites):          {len(would_run)} / {len(would_skip)}")
    print(f"Tests Jev skipped that actually ran:        {len(c['skipped_but_ran'])}")
    print(f"  - of which passed (correct skips):        {len(c['correctly_skipped'])}")
    print(f"  - of which FAILED (flaky excluded):       {len(c['false_negatives'])}")
    print(f"  - of which failed but flaky:              {len(c['flaky_skipped'])}")
    print(f"Tests Jev would run that did not run:      {len(c['extra_runs'])}")
    print(f"Failures kept by Jev (ran & failed):       {len(c['kept_failures'])}")
    for t in c["kept_failures"]:
        print(f"   [kept-failure] {t} (jobs: {executed.get(t, {}).get('jobs', [])})")
    for t in c["flaky_skipped"]:
        print(f"   [flaky-skipped] {t} (jobs: {executed.get(t, {}).get('jobs', [])})")
    print(f"Savings (skipped share of executed):       {savings:.0f}%")
    if coverage_skipped:
        cov = report["comparison"]["coverage_selection"]
        print(f"Coverage-based --impacted skipped:         {len(cov['skipped'])}")
        print(f"  - Jev also skips:                          {len(cov['jev_also_skips'])}")
        print(f"  - Jev would keep running (disagreement):  {len(cov['jev_disagrees'])} {cov['jev_disagrees']}")
    if c["false_negatives"]:
        print("\n!! Jev would have skipped tests that FAILED on this PR:")
        verdicts = {x["test"]: x for x in decisions}
        for t in c["false_negatives"]:
            v = verdicts.get(t, {})
            print(
                f"   {t} (jobs: {executed.get(t, {}).get('jobs', [])}) "
                f"suite={v.get('suite')} relation={v.get('relation')} "
                f"should_execute={v.get('should_execute')} confidence={v.get('confidence')}"
            )
    output = args.output or f"jev_e2e_eval_{pr_number}.json"
    with open(output, "w") as f:
        json.dump(report, f, indent=2)
    print(f"[info] full report written to {output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
