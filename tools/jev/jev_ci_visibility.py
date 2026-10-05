#!/usr/bin/env python3
"""Query CI Visibility for the E2E tests that actually ran for a given PR.

Standalone version of the executed-test lookup done by jev_e2e_eval.py:
resolves the PR head commit, builds the same query the eval uses (e2e jobs
only, root entry points only), fetches the results and prints them.

Usage (from the repo root, with CI Visibility keys for the org where the
agent e2e test events land - org 2, uploaded with agent-api-key-org-2):
    DD_API_KEY=... DD_APP_KEY=... python3 tools/jev/jev_ci_visibility.py --pr 57005

    # or scope by a specific pipeline (e.g. the merge-queue run) instead:
    DD_API_KEY=... DD_APP_KEY=... python3 tools/jev/jev_ci_visibility.py --pr 57005 --pipeline-id 140815637
"""

from __future__ import annotations  # python 3.9 compat

import argparse
import json
import os
import re
import sys
import urllib.request

REPO_ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, REPO_ROOT)
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from jev_e2e_eval import GITHUB_API, PIPELINE_NAME, fetch_executed_e2e_tests, fetch_pr  # noqa: E402


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--pr", type=int, required=True, help="PR number")
    parser.add_argument("--pipeline-id", default=None, help="Scope by this GitLab pipeline id instead of the PR head commit")
    parser.add_argument("--days", type=int, default=90, help="CI Visibility lookback window in days")
    args = parser.parse_args()

    pr = fetch_pr(args.pr)
    head = pr["head"]["sha"]
    print(f"[info] PR #{args.pr}: {pr['title']}")
    print(f"[info] head {head[:12]} state {pr['state']} merged {pr.get('merged_at')}")

    scope = f"@ci.pipeline.id:{args.pipeline_id}" if args.pipeline_id else f"@git.commit.sha:{head}"
    query = f"@ci.pipeline.name:{PIPELINE_NAME} {scope} @ci.job.name:new-e2e* -@test.name:*/*"

    executed, coverage_skipped = fetch_executed_e2e_tests(query, args.days)

    fails = sorted(t for t, v in executed.items() if v["status"] == "fail")
    flaky = sorted(t for t, v in executed.items() if v["flaky"])
    print(f"\n--- CI Visibility executed e2e tests for PR #{args.pr} ---")
    print(f"root tests executed: {len(executed)}")
    print(f"failed:              {len(fails)} {fails}")
    print(f"flaky failures:      {len(flaky)} {flaky}")
    print(f"skipped by coverage selection: {len(coverage_skipped)} {sorted(coverage_skipped)[:10]}")
    print(f"first tests:         {sorted(executed)[:10]}")
    if not executed and not os.environ.get("DD_API_KEY"):
        print("\n[warn] DD_API_KEY is not set - the lookup was skipped entirely. Export DD_API_KEY and DD_APP_KEY (org where the agent e2e events land).")
    return 0


if __name__ == "__main__":
    sys.exit(main())
