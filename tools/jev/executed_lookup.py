"""Executed-test lookups for the Jev e2e eval: CI Visibility (Datadog API)
and the pipeline's own e2e job artifacts (GitLab API)."""

from __future__ import annotations  # python 3.9 compat

import concurrent.futures
from datetime import datetime, timedelta, timezone
import json
import os
import re
import urllib.error
import urllib.request

from pr_context import run_cmd

PIPELINE_NAME = "DataDog/datadog-agent"


# ---------------------------------------------------------------- PR checkout


def fetch_executed_e2e_tests(query: str, days: int, allow_failure_jobs: set | None = None) -> tuple[dict, dict]:
    """Root-level e2e tests actually executed, from CI Visibility.

    Self-contained implementation of the CI Visibility test events query
    (same endpoint as datadog_api_client's list_ci_app_test_events, but
    stdlib-only so the script runs on a bare laptop). Requires DD_API_KEY
    and DD_APP_KEY in the environment.

    Returns (executed, coverage_skipped) where executed maps entry points to
    {"status": "pass|fail", "flaky": bool, "jobs": [..]} (only tests whose
    latest attempt actually ran and passed/failed), and coverage_skipped maps
    entry points to the jobs where the latest attempt was a skip (tests the
    coverage-based --impacted selection skipped).
    """
    from datetime import datetime, timedelta, timezone

    api_key = os.environ.get("DD_API_KEY")
    app_key = os.environ.get("DD_APP_KEY") or os.environ.get("DD_APPLICATION_KEY")
    if not api_key or not app_key:
        print("[warn] DD_API_KEY / DD_APP_KEY not set, skipping the executed-test lookup")
        return {}, {}
    site = os.environ.get("DD_SITE", "datadoghq.com")
    url = f"https://api.{site}/api/v2/ci/tests/events"
    # query params as the datadog_api_client serializes them (bracket attributes)
    params = {
        "filter[query]": query,
        "filter[from]": (datetime.now(timezone.utc) - timedelta(days=days)).isoformat(),
        "filter[to]": datetime.now(timezone.utc).isoformat(),
        "page[limit]": 1000,
        # chronological order (oldest first): with this sort, the last event
        # seen for a test is its latest attempt, which makes the retry logic
        # robust even when the timestamp attribute cannot be parsed
        "sort": "timestamp",
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
    skipped_subtests = skipped_empty = skipped_job = skipped_allow_failure = 0
    for item in events:
        attrs = item.get("attributes", {}).get("attributes", {})
        test_attrs = attrs.get("test", {})
        job_name = ((attrs.get("ci", {}) or {}).get("job", {}) or {}).get("name", "")
        name = test_attrs.get("name") or ""
        # CI Visibility test names are bare names (e.g. "TestServiceBehaviorPowerShell"
        # or "TestFleetConfig/linux/TestConfig" for sub-tests), with no package path,
        # so the e2e check must use the job name, not the test name.
        if not name:
            skipped_empty += 1
            continue
        if "/" in name:  # sub-tests
            skipped_subtests += 1
            continue
        if job_name and not job_name.startswith("new-e2e"):  # non-e2e job (query filter safety net)
            skipped_job += 1
            continue
        if allow_failure_jobs is not None and job_name in allow_failure_jobs:
            # failures of allow_failure jobs do not gate the pipeline: exclude
            # them from the executed set so they do not count as selection
            # misses
            skipped_allow_failure += 1
            continue
            continue
        m = re.search(r"(Test\w+)\s*$", name)
        if not m:
            if len(sample_names) < 5:
                sample_names.append(name)
            continue
        entry = m.group(1)
        # CI Visibility statuses: pass, fail, skip. Only the latest attempt
        # decides (see below); a latest status of skip means the test did not
        # run (the coverage-based --impacted selection skipped it in the job),
        # it must not be counted as executed nor as a failure.
        raw_status = test_attrs.get("status") or ""
        status = raw_status if raw_status in ("pass", "skip") else "fail"
        flaky = test_attrs.get("agent_is_flaky_failure", "false") == "true"
        # Retry semantics: a test may emit several events (one per attempt,
        # e.g. --max-retries). Only the LATEST attempt decides the status, so
        # a fail that was retried to a pass counts as passed. The timestamp
        # lives in the free-form attributes of the event (CIAppEventAttributes
        # has no top-level timestamp), with outer/`@timestamp` fallbacks; the
        # ascending sort guarantees chronological iteration order even when
        # it cannot be found at all.
        ts = str(attrs.get("timestamp") or item.get("attributes", {}).get("timestamp") or attrs.get("@timestamp") or "")
        e = executed.setdefault(entry, {"status": "pass", "flaky": False, "jobs": [], "ts": ""})
        if ts >= e["ts"]:
            e["status"] = status
            e["flaky"] = flaky
            e["ts"] = ts
        e["jobs"].append((attrs.get("ci", {}).get("job", {}) or {}).get("name", ""))
    # Split by the latest attempt: only tests that actually ran (latest
    # status pass/fail) count as executed; latest-status-skip tests are the
    # coverage selection's skips.
    coverage_skipped = {t: v["jobs"] for t, v in executed.items() if v["status"] == "skip"}
    executed = {t: v for t, v in executed.items() if v["status"] != "skip"}
    print(
        f"[info] CI Visibility result: {len(events)} test events over {pages} page(s) "
        f"(skipped: {skipped_subtests} sub-tests, {skipped_empty} without a name, {skipped_job} from non-e2e jobs, "
        f"{skipped_allow_failure} events from allow_failure jobs); "
        f"kept {len(executed)} executed root e2e tests and {len(coverage_skipped)} skipped by the coverage selection; "
        f"unmatched names: {sample_names[:3]}"
    )
    print(
        f"[info] executed root e2e tests: {len(executed)}: {sorted(executed)[:10]}{' ...' if len(executed) > 10 else ''}"
    )
    if not executed:
        print(f"[warn] no executed e2e tests matched; sample test names seen: {sample_names}")
    return executed, coverage_skipped


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
        out = run_cmd(["ddtool", "auth", "token", "sdm", "--datacenter", "us1.ddbuild.io", "--http-header"]).strip()
    bearer = out.removeprefix("Authorization: ").strip()
    req = urllib.request.Request(
        "https://bti-ci-api.us1.ddbuild.io/internal/ci/gitlab/token?owner=DataDog&repository=datadog-agent",
        headers={"Authorization": bearer},
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)["token"]


def _gitlab_pipeline_jobs(pipeline_id: str) -> list[dict]:
    """Jobs that ran (success/failed) in a GitLab pipeline, paginated."""
    base = os.environ.get("CI_API_V4_URL", "https://gitlab.ddbuild.io/api/v4")
    project = os.environ.get("CI_PROJECT_ID") or "DataDog%2Fdatadog-agent"
    headers = {"PRIVATE-TOKEN": get_gitlab_token()}
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
    return jobs


def allow_failure_jobs(pipeline_id: str | None = None, sha: str | None = None) -> set:
    """Names of the allow_failure e2e jobs to exclude from the executed set.

    Failures of allow_failure jobs do not gate the pipeline, so counting them
    as selection misses overstates the risk. Resolve by pipeline id, or by
    commit sha (all pipelines that ran that commit, e.g. branch re-runs).
    """
    base = os.environ.get("CI_API_V4_URL", "https://gitlab.ddbuild.io/api/v4")
    project = os.environ.get("CI_PROJECT_ID") or "DataDog%2Fdatadog-agent"
    headers = {"PRIVATE-TOKEN": get_gitlab_token()}
    if pipeline_id:
        pipeline_ids = [pipeline_id]
    elif sha:
        with urllib.request.urlopen(
            urllib.request.Request(f"{base}/projects/{project}/pipelines?sha={sha}&per_page=50", headers=headers),
            timeout=60,
        ) as resp:
            pipeline_ids = [p["id"] for p in json.load(resp)]
    else:
        return set()
    names = set()
    for pid in pipeline_ids:
        for job in _gitlab_pipeline_jobs(pid):
            if job.get("allow_failure") and job["name"].startswith("new-e2e"):
                names.add(job["name"])
    if names:
        print(f"[info] excluding {len(names)} allow_failure e2e job names from the executed set")
    return names


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
    # failures of allow_failure jobs do not gate the pipeline: exclude them
    allow_jobs = [j for j in e2e_jobs if not j.get("allow_failure")]
    if len(allow_jobs) < len(e2e_jobs):
        print(f"[info] excluding {len(e2e_jobs) - len(allow_jobs)} allow_failure e2e jobs from the executed set")
    e2e_jobs = allow_jobs

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
                    # Retry semantics: only the latest attempt decides (see
                    # fetch_executed_e2e_tests).
                    ts = str(d.get("Time") or "")
                    e = executed.setdefault(test, {"status": "pass", "flaky": False, "jobs": [], "ts": ""})
                    if ts >= e["ts"]:
                        if action in ("pass", "fail"):
                            e["status"] = action
                        e["ts"] = ts
                    e["jobs"].append(job["name"])
                elif action == "skip":
                    coverage_skipped.setdefault(test, []).append(job["name"])
    print(
        f"[info] {len(executed)} root e2e tests executed in pipeline {pipeline_id} "
        f"({len(e2e_jobs)} new-e2e jobs, {len(coverage_skipped)} tests skipped by the coverage selection)"
    )
    return executed, coverage_skipped


# ---------------------------------------------------------------- main
