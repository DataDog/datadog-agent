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
import concurrent.futures
import json
import os
import re
import shutil
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request

# make tools/jev and the repo root (for tasks.libs...) importable
REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, REPO_ROOT)
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from jev_e2e_selector import (  # noqa: E402
    GITHUB_API,
    _annotate_diff,
    ask_jev,
    build_state,
    files_from_diff,
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


def fetch_pr_diff(pr_number: int) -> str:
    """Full PR diff from the GitHub API (computed against the PR base, so it
    does not depend on local git history — works in shallow clones)."""
    req = urllib.request.Request(f"{GITHUB_API}/pulls/{pr_number}")
    if os.environ.get("GITHUB_TOKEN"):
        req.add_header("Authorization", f"Bearer {os.environ['GITHUB_TOKEN']}")
    req.add_header("Accept", "application/vnd.github.diff")
    with urllib.request.urlopen(req, timeout=60) as resp:
        return resp.read().decode(errors="ignore")


def git(*args: str, cwd: str | None = None) -> str:
    return run_cmd(["git", *args], cwd=cwd)


def run_cmd(cmd: list[str], cwd: str | None = None) -> str:
    import subprocess

    res = subprocess.run(cmd, capture_output=True, text=True, cwd=cwd)
    if res.returncode != 0:
        raise RuntimeError(f"command {' '.join(cmd)} failed: {res.stderr.strip()}")
    return res.stdout.strip()


def detect_pr_number() -> int | None:
    """PR number of the current branch (CI env first, GitHub lookup fallback)."""
    branch = None
    for env in ("CI_COMMIT_BRANCH", "CI_COMMIT_REF_NAME"):
        if os.environ.get(env) and os.environ[env] != "HEAD":
            branch = os.environ[env]
            break
    if not branch:
        branch = git("rev-parse", "--abbrev-ref", "HEAD")
    if not branch or branch == "HEAD" or branch == "main":
        return None
    req = urllib.request.Request(
        f"{GITHUB_API}/pulls?head={REPO.split(':')[0]}:{urllib.parse.quote(branch)}&state=open&per_page=1"
    )
    if os.environ.get("GITHUB_TOKEN"):
        req.add_header("Authorization", f"Bearer {os.environ['GITHUB_TOKEN']}")
    with urllib.request.urlopen(req, timeout=30) as resp:
        pulls = json.load(resp)
    return pulls[0]["number"] if pulls else None


# ---------------------------------------------------------------- executed tests


def fetch_executed_e2e_tests(query: str, days: int) -> dict:
    """Root-level e2e tests actually executed, from CI Visibility.

    Self-contained implementation of the CI Visibility test events query
    (same endpoint as datadog_api_client's list_ci_app_test_events, but
    stdlib-only so the script runs on a bare laptop). Requires DD_API_KEY
    and DD_APP_KEY in the environment.

    Returns {entry_point: {"status": "pass|fail", "flaky": bool, "jobs": [..]}}
    """
    from datetime import datetime, timedelta, timezone

    api_key = os.environ.get("DD_API_KEY")
    app_key = os.environ.get("DD_APP_KEY") or os.environ.get("DD_APPLICATION_KEY")
    if not api_key or not app_key:
        print("[warn] DD_API_KEY / DD_APP_KEY not set, skipping the executed-test lookup")
        return {}
    site = os.environ.get("DD_SITE", "datadoghq.com")
    url = f"https://api.{site}/api/v2/ci/tests/events"
    # query params as the datadog_api_client serializes them (bracket attributes)
    params = {
        "filter[query]": query,
        "filter[from]": (datetime.now(timezone.utc) - timedelta(days=days)).isoformat(),
        "filter[to]": datetime.now(timezone.utc).isoformat(),
        "page[limit]": 1000,
    }
    headers = {
        "DD-API-KEY": api_key,
        "DD-APPLICATION-KEY": app_key,
        "Accept": "application/json",
    }

    events = []
    pages = 0
    print(f"[info] CI Visibility query: {query}")
    print(f"[info] CI Visibility request: GET {url} (lookback {days} days)")
    while True:
        req = urllib.request.Request(url + "?" + urllib.parse.urlencode(params), headers=headers)
        try:
            with urllib.request.urlopen(req, timeout=60) as resp:
                payload = json.load(resp)
        except urllib.error.HTTPError as e:
            body = e.read().decode(errors="ignore")[:500]
            raise RuntimeError(f"CI Visibility API HTTP {e.code} on {url}: {body}") from e
        events.extend(payload.get("data", []))
        pages += 1
        cursor = (payload.get("meta", {}).get("page", {}) or {}).get("after")
        if not cursor:
            break
        params["page[cursor]"] = cursor

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
    print(
        f"[info] CI Visibility result: {len(events)} test events over {pages} page(s), "
        f"of which {len(executed)} root e2e tests (new-e2e, no '/'); "
        f"rejected samples: {sample_names[:3]}"
    )
    print(f"[info] executed root e2e tests: {len(executed)}: {sorted(executed)[:10]}{' ...' if len(executed) > 10 else ''}")
    if not executed:
        print(f"[warn] no executed e2e tests matched; sample test names seen: {sample_names}")
    return executed


# ---------------------------------------------------------------- GitLab artifacts


def get_gitlab_token() -> str:
    """GitLab API token, for querying the pipeline's e2e job artifacts.

    Uses the same mechanism as tasks.libs.ciproviders.gitlab_api.get_gitlab_token:
    a short-lived token from the bti-ci-api service, authorized with an 'sdm'
    infra token (authanywhere in CI, ddtool locally). $GITLAB_TOKEN overrides.
    """
    if os.environ.get("GITLAB_TOKEN"):
        return os.environ["GITLAB_TOKEN"]
    if os.environ.get("CI"):
        cmd = ["./authanywhere"] if os.path.exists("./authanywhere") else ["authanywhere"]
        out = run_cmd([*cmd, "--audience", "sdm"]).strip()
    else:
        out = run_cmd(
            ["ddtool", "auth", "token", "sdm", "--datacenter", "us1.ddbuild.io", "--http-header"]
        ).strip()
    bearer = out.removeprefix("Authorization: ").strip()
    req = urllib.request.Request(
        "https://bti-ci-api.us1.ddbuild.io/internal/ci/gitlab/token?owner=DataDog&repository=datadog-agent",
        headers={"Authorization": bearer},
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)["token"]


def fetch_executed_from_gitlab(pipeline_id: str) -> tuple[dict, dict]:
    """Root e2e tests executed in a GitLab pipeline, from the e2e jobs'
    e2e_test_output.json artifacts (same pipeline, no CI Visibility keys needed).

    Returns ({entry: {"status": "pass|fail", "flaky": False, "jobs": [..]}},
             {entry: [job names]}) where the second dict holds the tests the
    coverage-based --impacted selection skipped in those jobs.
    """
    base = os.environ.get("CI_API_V4_URL", "https://gitlab.ddbuild.io/api/v4")
    project = os.environ.get("CI_PROJECT_ID") or "DataDog%2Fdatadog-agent"
    headers = {"PRIVATE-TOKEN": get_gitlab_token()}

    # Only jobs that actually ran have artifacts; manual/canceled/created ones
    # would just 404 on the artifact download.
    jobs, page = [], 1
    while True:
        url = (
            f"{base}/projects/{project}/pipelines/{pipeline_id}/jobs"
            f"?per_page=100&page={page}&scope[]=success&scope[]=failed"
        )
        with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=60) as resp:
            batch = json.load(resp)
        jobs.extend(batch)
        if len(batch) < 100:
            break
        page += 1
    e2e_jobs = [j for j in jobs if j["name"].startswith("new-e2e")]

    def fetch_result_json(job: dict) -> tuple[dict, str]:
        art = f"{base}/projects/{project}/jobs/{job['id']}/artifacts/e2e_test_output.json"
        try:
            with urllib.request.urlopen(urllib.request.Request(art, headers=headers), timeout=60) as resp:
                return job, resp.read().decode(errors="ignore")
        except urllib.error.HTTPError:
            return job, ""

    executed: dict = {}
    coverage_skipped: dict = {}
    with concurrent.futures.ThreadPoolExecutor(max_workers=16) as pool:
        for job, content in pool.map(fetch_result_json, e2e_jobs):
            if not content:
                continue  # job has no result json (skipped job, other artifact shape)
            for line in content.splitlines():
                try:
                    d = json.loads(line)
                except json.JSONDecodeError:
                    continue
                action, test = d.get("Action"), d.get("Test")
                if not test or "/" in test:  # root entry points only
                    continue
                if action in ("pass", "fail", "run"):
                    e = executed.setdefault(test, {"status": "pass", "flaky": False, "jobs": []})
                    if action == "fail":
                        e["status"] = "fail"
                    e["jobs"].append(job["name"])
                elif action == "skip":
                    coverage_skipped.setdefault(test, []).append(job["name"])
    print(
        f"[info] {len(executed)} root e2e tests executed in pipeline {pipeline_id} "
        f"({len(e2e_jobs)} new-e2e jobs, {len(coverage_skipped)} tests skipped by the coverage selection)"
    )
    return executed, coverage_skipped


# ---------------------------------------------------------------- main


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--pr", type=int, default=None, help="PR number to replay (default: detect from the current branch)")
    parser.add_argument(
        "--from-artifacts", metavar="PIPELINE_ID", default=None,
        help="Fetch the executed tests from the pipeline's e2e_test_output.json "
        "artifacts (GitLab API) instead of CI Visibility",
    )
    parser.add_argument("--suite", default=None, help="Restrict to one e2e suite (default: all)")
    parser.add_argument("--pipeline-id", default=None, help="GitLab pipeline id of the executed run to compare with")
    parser.add_argument("--days", type=int, default=90, help="CI Visibility lookback window in days")
    parser.add_argument(
        "--concurrency", type=int, default=8,
        help="parallel Jev calls (the CI Visibility lookup runs concurrently too); 1 = sequential",
    )
    parser.add_argument("--dry-run", action="store_true", help="Skip Jev calls and the executed lookup (debug)")
    parser.add_argument("--model", default="datadoginternal/openjev-medium")
    parser.add_argument("--dc", default="us1.ddbuild.io", help="AI Gateway datacenter")
    parser.add_argument("--source", default="datadog-agent", help="source header for AI Gateway")
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
    if (
        not args.token
        and not args.token_cmd
        and not os.environ.get("AI_GATEWAY_TOKEN")
        and args.dc == "us1.ddbuild.io"
    ):
        print("[info] local run without a token command: using the us1.staging.dog gateway (ddtool token)")
        args.dc = "us1.staging.dog"

    # Checkout the PR head in a worktree. The PR head is fetched from GitHub
    # directly (GitLab mirrors do not carry refs/pull/*), depth-1: only the
    # test code is read from it; the diff comes from the GitHub API.
    git("fetch", "--depth=1", "https://github.com/DataDog/datadog-agent.git", f"pull/{pr_number}/head")
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
        pr_info = {"branch": worktree, "title": title, "description": pr.get("body") or "", "author": pr["user"]["login"]}
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
                (suite, name, os.path.relpath(path, worktree), code, suite_def_code)
                for name, path, code in entries
            )

        token = None if args.dry_run else get_ai_gateway_token(args)

        def evaluate(entry):
            """One Jev decision for one test entry point (thread-safe: pure inputs)."""
            suite, name, path, code, suite_def_code = entry
            state = build_state(
                name, path, code, suite, suite, pr_info, files, merge_base, diff,
                suite_def_code=suite_def_code,
            )
            try:
                answer = ask_jev(args, token, state)
            except Exception as e:
                return {"test": name, "suite": suite, "decision": "run", "error": str(e)}
            a = answer["answers"]
            should = a["should_execute"]["noul"]
            relation = a["relation"]["choice"]
            decision = "run" if should >= 0.5 or relation != "unrelated" else "skip"
            return {
                "test": name, "suite": suite, "decision": decision,
                "should_execute": should, "relation": relation,
                "confidence": a["confidence"]["score"],
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
                    # Query-side filtering to keep the payload small:
                    #   @ci.job.name:new-e2e*  -> only the e2e suite jobs
                    #   -@test.name:*/*        -> exclude sub-tests (root entry
                    #                           points only). The client-side
                    #                           root/new-e2e filter is kept as a
                    #                           safety net for query syntax quirks.
                    # Note: no env filter - the e2e jobs tag their test events
                    # with env:nativetest (see the .new_e2e_template), so
                    # filtering on env:prod (as the dyntest evaluator does for
                    # non-e2e jobs) would match nothing.
                    executed_future = pool.submit(
                        fetch_executed_e2e_tests,
                        f"@ci.pipeline.name:{PIPELINE_NAME} {scope} @ci.job.name:new-e2e* -@test.name:*/*",
                        args.days,
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
            print(f"[info] {len(decisions)} Jev decisions in {time.monotonic() - started:.0f}s (concurrency {args.concurrency})")
    finally:
        git("worktree", "remove", "--force", worktree)
        shutil.rmtree(worktree, ignore_errors=True)

    decisions.sort(key=lambda d: (d["suite"], d["test"]))  # stable report despite parallel completion order

    would_run = {d["test"] for d in decisions if d["decision"] == "run"}
    would_skip = {d["test"] for d in decisions if d["decision"] == "skip"}
    ran = set(executed)
    report = {
        "pr": pr_number, "title": title, "head": head,
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
    print(f"  - of which FAILED (flaky excluded):       {len(c['false_negatives'])} {c['false_negatives']}")
    print(f"  - of which failed but flaky:              {len(c['flaky_skipped'])}")
    print(f"Tests Jev would run that did not run:      {len(c['extra_runs'])}")
    print(f"Failures kept by Jev (ran & failed):       {len(c['kept_failures'])}")
    print(f"Savings (skipped share of executed):       {savings:.0f}%")
    if coverage_skipped:
        cov = report["comparison"]["coverage_selection"]
        print(f"Coverage-based --impacted skipped:         {len(cov['skipped'])}")
        print(f"  - Jev also skips:                          {len(cov['jev_also_skips'])}")
        print(f"  - Jev would keep running (disagreement):  {len(cov['jev_disagrees'])} {cov['jev_disagrees']}")
    if c["false_negatives"]:
        print("\n!! Jev would have skipped tests that FAILED on this PR - see 'false_negatives'")
    output = args.output or f"jev_e2e_eval_{pr_number}.json"
    with open(output, "w") as f:
        json.dump(report, f, indent=2)
    print(f"[info] full report written to {output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
