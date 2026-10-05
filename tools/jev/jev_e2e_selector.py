#!/usr/bin/env python3
"""Jev-based E2E test selector.

Decides, for each E2E test of a given suite (or a single test), whether it
should be executed on the current PR, by asking a Jev (TypeSafe System One)
model through the AI Gateway. See tools/jev/jev_client.py for the questions
and the run/skip decision (single source shared with the eval), and the
sibling modules for context gathering (pr_context.py), diff processing
(diff_utils.py) and test discovery (test_discovery.py).

Usage (from repo root):
    GITHUB_TOKEN=... AI_GATEWAY_TOKEN=... python3 tools/jev/jev_e2e_selector.py \
        --suite fleet [--test TestFleetConfig] [--base main]

Token acquisition:
    - CI:      download authanywhere and pass --token-cmd 'authanywhere --audience rapid-ai-platform --raw --dc us1.ddbuild.io'
    - laptop:  ddtool auth token rapid-ai-platform --datacenter us1.staging.dog (default)
Jev docs: https://datadoghq.atlassian.net/wiki/spaces/AIP/pages/7265386822
"""

from __future__ import annotations  # python 3.9 compat

import argparse
import json
import os
import re
import sys

from diff_utils import MAX_DIFF_BYTES, MAX_DIFF_PER_FILE, pr_diff
from jev_client import (
    DEFAULT_RUN_THRESHOLD,
    QUESTIONS,
    SYSTEMONE_PATH,
    ask_jev,
    build_state,
    decide,
    fail_open,
    get_ai_gateway_token,
    print_collapsible,
)
from pr_context import changed_files, fetch_ddci_metadata, fetch_pr_info
from test_discovery import E2E_TESTS_DIR, list_suites, suite_definition


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument("--suite", required=True, help="E2E suite under test/new-e2e/tests/ (e.g. fleet)")
    parser.add_argument("--test", help="Restrict to a single test entry point or suite method")
    parser.add_argument(
        "--base", default=os.environ.get("COMPARE_TO_BRANCH", "main"), help="Base branch to diff against"
    )
    parser.add_argument("--team", default=None, help="Owning team (defaults to the suite directory name)")
    parser.add_argument("--model", default="datadoginternal/openjev-medium")
    parser.add_argument("--dc", default="us1.ddbuild.io", help="AI Gateway datacenter")
    parser.add_argument("--source", default="datadog-agent", help="source header for AI Gateway")
    parser.add_argument("--token", help="Raw internal auth token (default: $AI_GATEWAY_TOKEN)")
    parser.add_argument("--token-cmd", help="Command producing a raw token, e.g. authanywhere invocation")
    parser.add_argument(
        "--run-threshold",
        type=float,
        default=DEFAULT_RUN_THRESHOLD,
        help="should_execute value above which the test runs (see jev_client.decide)",
    )
    parser.add_argument("--output", default="jev_e2e_decisions.json", help="JSON output path")
    parser.add_argument(
        "--dry-run", action="store_true", help="Print the state that would be sent, without calling Jev"
    )
    args = parser.parse_args()

    suite_dir = os.path.join(E2E_TESTS_DIR, args.suite)
    if not os.path.isdir(suite_dir):
        print(f"error: no e2e suite at {suite_dir}", file=sys.stderr)
        return 1
    team = args.team or args.suite

    print(f"[info] suite={args.suite} team={team} base={args.base}")
    ddci = fetch_ddci_metadata()
    pr = fetch_pr_info(args.base, ddci)
    files, merge_base = changed_files(args.base, ddci)
    print(f"[info] {len(files)} changed files (merge base {str(merge_base)[:8]})")
    diff = pr_diff(merge_base)
    print(f"[info] full diff: {len(diff)} chars (per-file cap {MAX_DIFF_PER_FILE}, total cap {MAX_DIFF_BYTES})")

    suites = list_suites(suite_dir)
    if args.test:
        # --test matches an entry point, or a suite method: use the entry point of its file
        suites = [s for s in suites if s[0] == args.test]
        if not suites:
            for root, _, go_files in os.walk(suite_dir):
                for f in sorted(go_files):
                    if not f.endswith(".go"):
                        continue
                    path = os.path.join(root, f)
                    code = open(path, encoding="utf-8", errors="ignore").read()
                    if re.search(rf"^func \(s \*\w+\) {re.escape(args.test)}\(", code, re.MULTILINE):
                        entries = re.findall(r"^func (Test\w+)\(", code, re.MULTILINE)
                        suites = [s for s in list_suites(suite_dir) if s[0] in entries]
        if not suites:
            print(f"error: test {args.test} not found in {suite_dir}", file=sys.stderr)
            return 1
    print(f"[info] evaluating {len(suites)} tests: {', '.join(s[0] for s in suites)}")

    token = None if args.dry_run else get_ai_gateway_token(args)

    suite_def_path, suite_def_code = suite_definition(suite_dir)
    if suite_def_code:
        print(f"[info] suite definition included: {suite_def_path} ({len(suite_def_code)} chars)")

    decisions = []
    for name, path, code in suites:
        state = build_state(
            name,
            path,
            code,
            args.suite,
            team,
            pr,
            files,
            merge_base,
            diff,
            ddci=ddci,
            suite_def_code=suite_def_code,
        )
        if args.dry_run:
            print(f"--- state for {name} (dry run, not sent) ---\n{state}\n")
            decisions.append({"test": name, "dry_run": True})
            continue

        # Print exactly what is sent to Jev for every call, in a collapsed section
        section_name = "jev_call_" + re.sub(r"\W+", "_", name)
        section_body = (
            f"endpoint: https://ai-gateway.{args.dc}{SYSTEMONE_PATH}  model: {args.model}  source: {args.source}\n"
            f"state ({len(state)} chars):\n{state}\n"
            f"questions: {json.dumps(QUESTIONS)}"
        )
        print_collapsible(section_name, f"Jev call input: {name}", section_body)

        try:
            answer = ask_jev(args, token, state)
        except Exception as e:  # fail open: if Jev is unavailable, run the test
            print(f"[warn] Jev call failed for {name}: {e} -> defaulting to RUN")
            decisions.append({"test": name, **fail_open(e)})
            continue

        # Single decision source: jev_client.decide (shared with the eval)
        row = {
            "test": name,
            **decide(answer["answers"], args.run_threshold),
            "usage": answer.get("usage"),
        }
        decisions.append(row)
        print(
            f"[jev] {name:<45} -> {row['decision'].upper():4}  should_execute={row['should_execute']:.2f}  "
            f"relation={row['relation']}  confidence={row['confidence']:.2f}"
        )

    if not args.dry_run:
        to_run = sorted({d["test"] for d in decisions if d["decision"] == "run"})
        to_skip = sorted({d["test"] for d in decisions if d["decision"] == "skip"})
        summary = {
            "suite": args.suite,
            "team": team,
            "base": args.base,
            "pr": pr.get("number"),
            "changed_files": files,
            "run": to_run,
            "skip": to_skip,
            "decisions": decisions,
        }
        with open(args.output, "w") as f:
            json.dump(summary, f, indent=2)
        print(f"\n[summary] run: {to_run or 'none'}")
        print(f"[summary] skip: {to_skip or 'none'}")
        print(
            f"[summary] go test flag: --skip '{'|'.join(to_skip)}'" if to_skip else "[summary] go test flag: (run all)"
        )
        print(f"[summary] decisions written to {args.output}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
