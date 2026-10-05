"""Jev (System One) client for the e2e tooling: questions, state building,
token handling and the SINGLE run/skip decision shared by all tools."""

from __future__ import annotations  # python 3.9 compat

import argparse
import json
import os
import re
import time
import urllib.error
import urllib.request

from pr_context import MAX_DESCRIPTION_BYTES, run_cmd, truncate
from test_discovery import MAX_SUITE_DEFINITION_BYTES, MAX_TEST_CODE_BYTES

SYSTEMONE_PATH = "/v1/systemone"
# https://datadoghq.atlassian.net/wiki/spaces/DEVX/pages/5423334716/DDCI+Metadata+Service


QUESTIONS = {
    "should_execute": {
        "type": "noul",
        "instructions": (
            "Given this PR's changed files and stated intent, could this PR "
            "plausibly affect what this e2e test verifies? 1.0 means the test "
            "should be executed, 0.0 means it is safe to skip."
        ),
    },
    "relation": {
        "type": "choice",
        "instructions": "What best describes the relationship between this PR and this e2e test?",
        "criteria": {
            "code_under_test": "The PR modifies code or behavior this test directly exercises",
            "shared_infra": "The PR modifies shared infrastructure, framework or configuration the test depends on",
            "packaging_ci": "The PR modifies build, packaging, dependencies or CI config affecting the test environment",
            "unrelated": "No plausible relationship between this PR and this test",
        },
    },
    "confidence": {
        "type": "score",
        "instructions": "How confident are you in your decision?",
        "criteria": [
            "Unsure, run the test to be safe",
            "Moderately confident",
            "Highly confident, decision is clear from the inputs",
        ],
    },
}


# Default of the single run/skip decision (see decide): a test runs only if
# its relation to the PR is not "unrelated" AND its should_execute score is
# at least this threshold. Errors always fail open to run.
DEFAULT_RUN_THRESHOLD = 0.1


def get_ai_gateway_token(args: argparse.Namespace) -> str:
    if args.token:
        return args.token
    if os.environ.get("AI_GATEWAY_TOKEN"):
        return os.environ["AI_GATEWAY_TOKEN"]
    if args.token_cmd:
        return run_cmd(args.token_cmd.split()).strip()
    # laptop fallback: ddtool prints the raw internal service token by default
    return run_cmd(["ddtool", "auth", "token", "rapid-ai-platform", "--datacenter", "us1.staging.dog"]).strip()


def ask_jev(args: argparse.Namespace, token: str, state: str) -> dict:
    payload = {
        "state": state,
        "model": args.model,
        "questions": QUESTIONS,
    }
    req = urllib.request.Request(
        f"https://ai-gateway.{args.dc}{SYSTEMONE_PATH}",
        data=json.dumps(payload).encode(),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
            "source": args.source,
            "org-id": "2",
            "x-dd-tag-ddagent-ci": "innovation-week-experiment",
        },
        method="POST",
    )
    try:
        with urllib.request.urlopen(req, timeout=120) as resp:
            return json.load(resp)
    except urllib.error.HTTPError as e:
        body = e.read().decode(errors="ignore")
        raise RuntimeError(f"AI Gateway HTTP {e.code}: {body}") from e


def print_collapsible(name: str, title: str, body: str) -> None:
    """Print `body` under `title`, in a collapsed GitLab log section when in CI.

    Uses the GitLab CI collapsible-section ANSI markers; falls back to plain
    printing when not running in GitLab CI (e.g. local dry runs).
    """
    if os.environ.get("GITLAB_CI") or os.environ.get("CI_PIPELINE_ID"):
        start = int(time.time())
        print(f"\033[0Ksection_start:{start}:{name}[collapsed=true]\r\033[0K{title}")
        print(body)
        print(f"\033[0Ksection_end:{int(time.time())}:{name}\r\033[0K")
    else:
        print(f"--- {title} ---\n{body}")


def build_state(
    name: str,
    path: str,
    code: str,
    suite: str,
    team: str,
    pr: dict,
    files: list,
    merge_base: str,
    diff: str,
    ddci: dict | None = None,
    suite_def_code: str = "",
) -> str:
    """Assemble the System One state sent to Jev for one test entry point."""
    files_section = "\n".join(f"- {f} ({kind})" if kind else f"- {f}" for f, kind in files)
    author = f", author: @{pr['author']}" if pr.get("author") else ""
    impacted = ""
    if ddci and ddci.get("impacted_targets"):
        impacted = "\n\n## Impacted build targets (from DDCI build impact analysis)\n" + ", ".join(
            ddci["impacted_targets"][:100]
        )
    diff_section = f"\n## Full PR diff (per-file patches, truncated to fit)\n```diff\n{diff}\n```" if diff else ""
    suite_def_section = (
        (
            f"\n## E2E suite provisioning definition (base suite: platforms, components, install method)\n"
            f"```go\n{suite_def_code}\n```"
        )
        if suite_def_code
        else ""
    )
    return (
        "## PR under review\n"
        f"Title: {pr.get('title') or '(unknown)'}{author}\n"
        f"Description:\n{truncate(pr.get('description') or '(none)', MAX_DESCRIPTION_BYTES, 'description')}\n"
        f"Owning team of the E2E suite: {team}\n\n"
        f"## Files changed in this PR (merge base {str(merge_base)[:12]}, {len(files)} files)\n"
        f"{files_section}{diff_section}{impacted}\n\n"
        "## E2E test under evaluation\n"
        f"Test: {name}\n"
        f"Suite: {suite} ({path})\n"
        f"Code:\n```go\n{truncate(code, MAX_TEST_CODE_BYTES, 'test code')}\n```{suite_def_section}\n\n"
        "Context: this is a test in the datadog-agent repository, a large Go monorepo. "
        "E2E tests provision real VMs and are expensive to run. Decide whether this PR "
        "plausibly affects what this test verifies."
    )


# ---------------------------------------------------------------- main


def decide(answers: dict, run_threshold: float = DEFAULT_RUN_THRESHOLD) -> dict:
    """Single source of truth for the run/skip decision.

    Used by both the selector (live decisions) and the eval (replays), so the
    two can never drift again. A test runs only if BOTH:
      - its relation to the PR is not "unrelated" (code, infra or packaging link)
      - its should_execute score is at least run_threshold
    otherwise it is skipped. Any error in the pipeline fails open to run
    (see fail_open).
    """
    should = answers["should_execute"]["noul"]
    relation = answers["relation"]["choice"]
    confidence = answers["confidence"]["score"]
    decision = "run" if should >= run_threshold and relation != "unrelated" else "skip"
    return {
        "should_execute": should,
        "relation": relation,
        "confidence": confidence,
        "decision": decision,
    }


def fail_open(error: Exception | str) -> dict:
    """Decision used whenever Jev cannot be reached: run the test."""
    return {
        "should_execute": 1.0,
        "relation": "fail_open",
        "confidence": None,
        "decision": "run",
        "error": str(error),
    }
