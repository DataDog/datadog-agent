"""Jev (System One) client for the e2e tooling: questions, state building,
token handling and the SINGLE run/skip decision shared by all tools."""

from __future__ import annotations

import json
import math
import os
import shlex
import subprocess
import urllib.error
import urllib.request

from tasks.libs.dynamic_test.jev.pr_context import MAX_DESCRIPTION_BYTES, truncate
from tasks.libs.dynamic_test.jev.test_discovery import MAX_TEST_CODE_BYTES

SYSTEMONE_PATH = "/v1/systemone"


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


def get_ai_gateway_token(token: str | None = None, token_cmd: str | None = None, dc: str = "us1.ddbuild.io") -> str:
    if token:
        return token
    if os.environ.get("AI_GATEWAY_TOKEN"):
        return os.environ["AI_GATEWAY_TOKEN"]
    # token_cmd is a shell command string (JEV_TOKEN_CMD/--token-cmd);
    # otherwise authanywhere provides the infra token for the selected
    # datacenter (preinstalled in the CI build image, brew on laptops)
    cmd = (
        shlex.split(token_cmd)
        if token_cmd
        else ["authanywhere", "--audience", "rapid-ai-platform", "--raw", "--dc", dc]
    )
    res = subprocess.run(cmd, capture_output=True, text=True, timeout=60)
    if res.returncode != 0:
        raise RuntimeError(f"token command {shlex.join(cmd)} failed: {res.stderr.strip()}")
    return res.stdout.strip()


def ask_jev(token: str, state: str, *, model: str, dc: str, source: str) -> dict:
    payload = {
        "state": state,
        "model": model,
        "questions": QUESTIONS,
    }
    req = urllib.request.Request(
        f"https://ai-gateway.{dc}{SYSTEMONE_PATH}",
        data=json.dumps(payload).encode(),
        headers={
            "Content-Type": "application/json",
            "Authorization": f"Bearer {token}",
            "source": source,
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


def decide(answers: dict, run_threshold: float = DEFAULT_RUN_THRESHOLD) -> dict:
    """Single source of truth for the run/skip decision.

    A test runs only if BOTH:
      - its relation to the PR is not "unrelated" (code, infra or packaging link)
      - its should_execute score is at least run_threshold
    otherwise it is skipped. Any error in the pipeline fails open to run
    (see fail_open). Validation errors name the offending field, the value
    received and the full raw answers, so bad model output is diagnosable
    from the log alone.
    """
    try:
        should = answers["should_execute"]["noul"]
        relation = answers["relation"]["choice"]
        confidence = answers["confidence"]["score"]
    except (KeyError, TypeError) as e:
        raise ValueError(f"Jev answer is missing or malformed ({e!r}); answers: {json.dumps(answers)}") from e
    for field, value in (("should_execute", should), ("confidence", confidence)):
        if isinstance(value, bool) or not isinstance(value, int | float) or not math.isfinite(value) or not 0 <= value:
            raise ValueError(
                f"Jev returned an invalid {field} score: {value!r} "
                f"(expected a finite number higher than 0); answers: {json.dumps(answers)}"
            )
    if relation not in QUESTIONS["relation"]["criteria"]:
        raise ValueError(
            f"Jev returned an unknown relation: {relation!r} "
            f"(expected one of {sorted(QUESTIONS['relation']['criteria'])}); answers: {json.dumps(answers)}"
        )
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
