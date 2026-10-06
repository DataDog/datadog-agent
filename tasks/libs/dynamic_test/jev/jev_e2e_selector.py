#!/usr/bin/env python3
"""Jev-based E2E test selector.

Decides, for each E2E test of a given suite (or a single test), whether it
should be executed on the current PR, by asking a Jev (TypeSafe System One)
model through the AI Gateway. select_suite() is the in-process entry point
used by the executor (tasks/libs/dynamic_test/jev_selection.py); main() is the
command-line wrapper. See jev_client.py for the questions and the run/skip
decision, and the sibling modules for context gathering (pr_context.py), diff
processing (diff_utils.py) and test discovery (test_discovery.py).

Must run from the repository root (git context and relative paths).

Usage:
    GITHUB_TOKEN=... AI_GATEWAY_TOKEN=... dda run i python -m tasks.libs.dynamic_test.jev.jev_e2e_selector \
        --suite fleet [--test TestFleetConfig] [--base main]

Token acquisition:
    - CI/laptop: preinstalled authanywhere (rapid-ai-platform audience, selected --dc)
    - overrides: AI_GATEWAY_TOKEN or --token-cmd
Jev docs: https://datadoghq.atlassian.net/wiki/spaces/AIP/pages/7265386822
"""

from __future__ import annotations

import argparse
import json
import os
import re
import sys
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from tasks.libs.dynamic_test.jev.diff_utils import MAX_DIFF_BYTES, MAX_DIFF_PER_FILE, pr_diff
from tasks.libs.dynamic_test.jev.jev_client import (
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
from tasks.libs.dynamic_test.jev.pr_context import changed_files, fetch_ddci_metadata, fetch_pr_info
from tasks.libs.dynamic_test.jev.test_discovery import E2E_TESTS_DIR, list_suites, suite_definition


def select_suite(
    suite: str,
    *,
    test: str | None = None,
    base: str | None = None,
    team: str | None = None,
    model: str = "datadoginternal/openjev-medium",
    dc: str = "us1.ddbuild.io",
    source: str = "datadog-agent",
    token: str | None = None,
    token_cmd: str | None = None,
    run_threshold: float = DEFAULT_RUN_THRESHOLD,
    workers: int = 8,
    output: str | None = None,
    dry_run: bool = False,
) -> dict | None:
    """Run the Jev selection for one suite and return its summary dict.

    Returns None on a dry run (states printed, nothing decided). The summary is
    {"suite", "team", "base", "pr", "changed_files", "run", "skip", "decisions"}.
    Fail-open is per test: a failed Jev call yields a RUN decision, never a
    skip. Raises ValueError on invalid arguments.
    """
    if workers < 1 or not 0 <= run_threshold <= 1:
        raise ValueError("workers must be positive and run_threshold must be between 0 and 1")
    base = base or os.environ.get("COMPARE_TO_BRANCH", "main")
    # The AI Gateway client takes a namespace-like argument object
    client_args = argparse.Namespace(model=model, dc=dc, source=source, token=token, token_cmd=token_cmd)

    suite_dir = os.path.join(E2E_TESTS_DIR, suite)
    if not os.path.isdir(suite_dir):
        raise ValueError(f"no e2e suite at {suite_dir}")
    team = team or suite

    print(f"[info] suite={suite} team={team} base={base}")
    ddci = fetch_ddci_metadata()
    pr = fetch_pr_info(base, ddci)
    files, merge_base = changed_files(base, ddci)
    print(f"[info] {len(files)} changed files (merge base {str(merge_base)[:8]})")
    diff = pr_diff(merge_base)
    print(f"[info] full diff: {len(diff)} chars (per-file cap {MAX_DIFF_PER_FILE}, total cap {MAX_DIFF_BYTES})")

    suites = list_suites(suite_dir)
    if test:
        # --test matches an entry point, or a suite method: use the entry point of its file
        suites = [s for s in suites if s[0] == test]
        if not suites:
            for root, _, go_files in os.walk(suite_dir):
                for f in sorted(go_files):
                    if not f.endswith(".go"):
                        continue
                    path = os.path.join(root, f)
                    code = Path(path).read_text(encoding="utf-8", errors="ignore")
                    if re.search(rf"^func \(s \*\w+\) {re.escape(test)}\(", code, re.MULTILINE):
                        entries = re.findall(r"^func (Test\w+)\(", code, re.MULTILINE)
                        suites = [s for s in list_suites(suite_dir) if s[0] in entries]
        if not suites:
            raise ValueError(f"test {test} not found in {suite_dir}")
    print(f"[info] evaluating {len(suites)} tests: {', '.join(s[0] for s in suites)}")

    token = None if dry_run else get_ai_gateway_token(client_args)

    suite_def_path, suite_def_code = suite_definition(suite_dir)
    if suite_def_code:
        print(f"[info] suite definition included: {suite_def_path} ({len(suite_def_code)} chars)")

    def select_test(entry):
        name, path, code = entry
        state = build_state(
            name, path, code, suite, team, pr, files, merge_base, diff, ddci=ddci, suite_def_code=suite_def_code
        )
        if dry_run:
            print(f"--- state for {name} (dry run, not sent) ---\n{state}\n")
            return {"test": name, "dry_run": True}

        # Print exactly what is sent to Jev for every call, in a collapsed section
        section_name = "jev_call_" + re.sub(r"\W+", "_", name)
        section_body = (
            f"endpoint: https://ai-gateway.{dc}{SYSTEMONE_PATH}  model: {model}  source: {source}\n"
            f"state ({len(state)} chars):\n{state}\n"
            f"questions: {json.dumps(QUESTIONS)}"
        )
        print_collapsible(section_name, f"Jev call input: {name}", section_body)

        try:
            answer = ask_jev(client_args, token, state)
            row = {
                "test": name,
                **decide(answer["answers"], run_threshold),
                "usage": answer.get("usage"),
            }
        except Exception as e:  # fail open: if Jev is unavailable, run the test
            print(f"[warn] Jev call failed for {name}: {e} -> defaulting to RUN")
            return {"test": name, **fail_open(e)}
        print(
            f"[jev] {name:<45} -> {row['decision'].upper():4}  should_execute={row['should_execute']:.2f}  "
            f"relation={row['relation']}  confidence={row['confidence']:.2f}"
        )
        return row

    with ThreadPoolExecutor(max_workers=workers) as pool:
        decisions = list(pool.map(select_test, suites))

    if dry_run:
        return None
    to_run = sorted({d["test"] for d in decisions if d["decision"] == "run"})
    # Bare names may occur in several packages/platform files. RUN wins.
    to_skip = sorted({d["test"] for d in decisions if d["decision"] == "skip"} - set(to_run))
    summary = {
        "suite": suite,
        "team": team,
        "base": base,
        "pr": pr.get("number"),
        "changed_files": files,
        "run": to_run,
        "skip": to_skip,
        "decisions": decisions,
    }
    print(f"\n[summary] run: {to_run or 'none'}")
    print(f"[summary] skip: {to_skip or 'none'}")
    print(f"[summary] go test flag: --skip '{'|'.join(to_skip)}'" if to_skip else "[summary] go test flag: (run all)")
    if output:
        with open(output, "w") as f:
            json.dump(summary, f, indent=2)
        print(f"[summary] decisions written to {output}")
    return summary


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
    parser.add_argument("--workers", type=int, default=8, help="Maximum concurrent Jev requests")
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
    try:
        select_suite(
            args.suite,
            test=args.test,
            base=args.base,
            team=args.team,
            model=args.model,
            dc=args.dc,
            source=args.source,
            token=args.token,
            token_cmd=args.token_cmd,
            run_threshold=args.run_threshold,
            workers=args.workers,
            output=args.output,
            dry_run=args.dry_run,
        )
    except ValueError as e:
        print(f"error: {e}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
