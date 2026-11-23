"""Jev-based E2E test selector.

Decides, for each E2E test of a given suite (or a single test), whether it
should be executed on the current PR, by asking a Jev (TypeSafe System One)
model through the AI Gateway. select_suite() is the only entry point, called
in-process by the executor (tasks/libs/dynamic_test/jev_selection.py). See
jev_client.py for the questions and the run/skip decision, and the sibling
modules for context gathering (pr_context.py), diff processing
(diff_utils.py) and test discovery (test_discovery.py).

Must run from the repository root (git context and relative paths).

For an input-only preview, without any Jev call:
    dda run i python -c 'from tasks.libs.dynamic_test.jev.jev_e2e_selector import select_suite; \
        select_suite("fleet", test="TestFleetConfig", dry_run=True)'

Token acquisition: preinstalled authanywhere (rapid-ai-platform audience,
the selected dc); AI_GATEWAY_TOKEN or token_cmd override it.
Jev docs: https://datadoghq.atlassian.net/wiki/spaces/AIP/pages/7265386822
"""

from __future__ import annotations

import json
import os
import re
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path

from tasks.libs.common.utils import gitlab_section
from tasks.libs.dynamic_test.jev.diff_utils import MAX_DIFF_BYTES, MAX_DIFF_PER_FILE, pr_diff
from tasks.libs.dynamic_test.jev.jev_client import (
    DEFAULT_RUN_THRESHOLD,
    QUESTIONS,
    SYSTEMONE_PATH,
    ask_jev,
    build_context_state,
    build_state,
    decide,
    fail_open,
    get_ai_gateway_token,
    summarize_pr,
)
from tasks.libs.dynamic_test.jev.pr_context import changed_files, fetch_ddci_metadata, fetch_pr_info
from tasks.libs.dynamic_test.jev.test_discovery import E2E_TESTS_DIR, list_suites, suite_definition

# The context (what every Jev call for a suite sees: the PR, the diff, the
# suite definition) is printed once per unique (base, merge base): all the
# suites of a selection share the same context, so printing it per suite
# would repeat the same diff over and over
_printed_contexts: set[tuple[str, str]] = set()

DEFAULT_SUMMARY_MODEL = "gpt-4o-mini"


def generate_pr_summary(
    *,
    base: str | None = None,
    model: str | None = None,
    dc: str = "us1.ddbuild.io",
    source: str = "datadog-agent",
    token: str | None = None,
    token_cmd: str | None = None,
) -> str:
    """One LLM call summarizing the current checkout's PR changes.

    Meant to run ONCE (dyntest.generate-jev-pr-summary, its own CI job) and
    the result passed to every selection as select_suite(pr_summary=...),
    instead of each selection calling the LLM. Raises on failure.
    """
    base = base or os.environ.get("COMPARE_TO_BRANCH", "main")
    model = model or os.environ.get("JEV_SUMMARY_MODEL") or DEFAULT_SUMMARY_MODEL
    ddci = fetch_ddci_metadata()
    pr = fetch_pr_info(base, ddci)
    files, merge_base = changed_files(base, ddci)
    diff = pr_diff(merge_base)
    print(f"[info] summarizing {len(files)} changed files, diff {len(diff)} chars (merge base {str(merge_base)[:8]})")
    token = get_ai_gateway_token(token=token, token_cmd=token_cmd, dc=dc)
    summary = summarize_pr(token, pr, files, merge_base, diff, model=model, dc=dc, source=source)
    print(f"[info] LLM PR summary from {model}: {len(summary)} chars")
    return summary


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
    pr_summary: str = "",
) -> dict | None:
    """Run the Jev selection for one suite and return its summary dict.

    Returns None on a dry run (full per-test states printed, nothing decided).
    The summary is {"suite", "team", "base", "pr", "changed_files", "run", "skip",
    "decisions"}. The shared context (PR, diff, suite definition - everything
    but the per-test code) is printed once per run (per unique base/merge
    base) so the passed diff is inspectable; the per-test states are not
    printed (use --dry-run for those). A non-empty pr_summary (generated
    beforehand by generate_pr_summary, never here) replaces the raw diff in
    every state. Fail-open is per test: a failed Jev
    call yields a RUN decision, never a skip. Raises ValueError on invalid
    arguments.
    """
    if workers < 1 or not 0 <= run_threshold <= 1:
        raise ValueError("workers must be positive and run_threshold must be between 0 and 1")
    base = base or os.environ.get("COMPARE_TO_BRANCH", "main")

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

    token = None if dry_run else get_ai_gateway_token(token=token, token_cmd=token_cmd, dc=dc)

    if pr_summary:
        print(f"[info] the given LLM PR summary ({len(pr_summary)} chars) replaces the diff")

    suite_def_path, suite_def_code = suite_definition(suite_dir)
    if suite_def_code:
        print(f"[info] suite definition included: {suite_def_path} ({len(suite_def_code)} chars)")

    # The context shared by every Jev call of this suite (everything but the
    # per-test code) is printed once - to see what diff will be passed. The
    # per-test states (with the test code) are not printed: use --dry-run to
    # inspect them without calling Jev.
    if not dry_run and (base, str(merge_base)) not in _printed_contexts:
        _printed_contexts.add((base, str(merge_base)))
        context = build_context_state(
            suite, team, pr, files, merge_base, diff, ddci=ddci, suite_def_code=suite_def_code, pr_summary=pr_summary
        )
        with gitlab_section(f"Jev input context (suite {suite})", collapsed=True, echo=True):
            print(
                f"endpoint: https://ai-gateway.{dc}{SYSTEMONE_PATH}  model: {model}  source: {source}\n"
                f"questions: {json.dumps(QUESTIONS)}\n"
                f"state without the per-test code ({len(context)} chars):\n{context}"
            )

    def select_test(entry):
        name, path, code = entry
        state = build_state(
            name,
            path,
            code,
            suite,
            team,
            pr,
            files,
            merge_base,
            diff,
            ddci=ddci,
            suite_def_code=suite_def_code,
            pr_summary=pr_summary,
        )
        if dry_run:
            print(f"--- state for {name} (dry run, not sent) ---\n{state}\n")
            return {"test": name, "dry_run": True}

        try:
            answer = ask_jev(token, state, model=model, dc=dc, source=source)
        except Exception as e:  # fail open: if Jev is unavailable, run the test
            print(f"[warn] Jev call failed for {name}: {e} -> defaulting to RUN")
            return {"test": name, **fail_open(e)}
        try:
            row = {
                "test": name,
                **decide(answer.get("answers") or {}, run_threshold),
                "usage": answer.get("usage"),
            }
        except ValueError as e:  # fail open: an answer we cannot interpret
            print(f"[warn] Invalid Jev answer for {name}: {e} -> defaulting to RUN")
            return {"test": name, **fail_open(e), "answers": answer.get("answers")}
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
