"""Jev-based E2E test selector.

Decides, for each E2E test of a given suite (or a single test), whether it
should be executed on the current PR, by asking a Jev (TypeSafe System One)
model through the AI Gateway. Before the per-test calls, one LLM call to
the AI Gateway chat completions endpoint (pr_summary.py) generates a summary
of the PR changes that replaces the raw diff in every Jev state. select_suite()
is the only entry point, called in-process by the executor
(tasks/libs/dynamic_test/jev_selection.py). See jev_client.py for the
questions and the run/skip decision, and the sibling modules for context
gathering (pr_context.py), diff processing (diff_utils.py), PR summarization
(pr_summary.py) and test discovery (test_discovery.py).

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
)
from tasks.libs.dynamic_test.jev.pr_context import changed_files, fetch_ddci_metadata, fetch_pr_info
from tasks.libs.dynamic_test.jev.pr_summary import CHAT_COMPLETIONS_PATH, summarize_pr
from tasks.libs.dynamic_test.jev.test_discovery import E2E_TESTS_DIR, list_suites, suite_definition

# The context (what every Jev call for a suite sees: the PR, the diff, the
# suite definition) is printed once per unique (base, merge base): all the
# suites of a selection share the same context, so printing it per suite
# would repeat the same diff over and over
_printed_contexts: set[tuple[str, str]] = set()

# The LLM PR summary (pr_summary.py) is also shared by every suite of a
# selection run against the same (base, merge base): computed once, reused
# by the later select_suite calls of the same run
_pr_summaries: dict[tuple[str, str], str] = {}

# The AI Gateway model generating the PR summary (chat completions endpoint,
# same gateway and token as the Jev calls); an empty value disables the
# summary and every Jev state carries the raw diff instead
DEFAULT_SUMMARY_MODEL = "gpt-4o-mini"


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
    summary_model: str | None = None,
) -> dict | None:
    """Run the Jev selection for one suite and return its summary dict.

    Returns None on a dry run (full per-test states printed, nothing decided).
    Unless summary_model is set to "" (or JEV_SUMMARY_MODEL is empty), one LLM
    call to the AI Gateway chat completions endpoint (pr_summary.py) produces
    a summary of the PR changes, computed once per (base, merge base) and
    shared by every suite of the run; that summary replaces the raw diff in
    every per-test Jev state. If the summary call fails, the selector fails
    open to the raw diff. The
    summary is {"suite", "team", "base", "pr", "changed_files", "run", "skip",
    "decisions"}. The shared context (PR, diff, suite definition - everything
    but the per-test code) is printed once per run (per unique base/merge
    base) so the passed diff is inspectable; the per-test states are not
    printed (use --dry-run for those). Fail-open is per test: a failed Jev
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

    # One LLM call per (base, merge base) - shared by every suite and every
    # per-test Jev state of the run - producing the PR summary that replaces
    # the raw diff (pr_summary.py). An explicit summary_model="" or an empty
    # JEV_SUMMARY_MODEL disables it; a failed call falls back to the diff.
    summary_model = (
        summary_model if summary_model is not None else os.environ.get("JEV_SUMMARY_MODEL", DEFAULT_SUMMARY_MODEL)
    )
    pr_summary = ""
    if not dry_run and summary_model:
        pr_summary = _pr_summaries.get((base, str(merge_base)), "")
        if not pr_summary:
            try:
                pr_summary = summarize_pr(token, pr, files, merge_base, diff, model=summary_model, dc=dc, source=source)
                _pr_summaries[(base, str(merge_base))] = pr_summary
                print(
                    f"[info] LLM PR summary from {summary_model} ({len(pr_summary)} chars) replaces the diff "
                    "in every Jev state"
                )
            except Exception as e:
                pr_summary = ""
                print(f"[warn] PR summary generation failed: {e} -> sending the raw diff instead")
    elif dry_run and summary_model:
        print("[info] dry run: the PR summary that would replace the diff is not generated (no gateway call)")

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
                + (
                    f"PR summary: https://ai-gateway.{dc}{CHAT_COMPLETIONS_PATH}  model: {summary_model}\n"
                    if pr_summary
                    else "PR summary: none, the raw diff is passed\n"
                )
                + f"questions: {json.dumps(QUESTIONS)}\n"
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
