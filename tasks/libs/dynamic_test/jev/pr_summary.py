"""LLM-generated PR change summary for the Jev e2e tooling.

The Jev (System One) endpoint only returns structured decisions (noul/choice/
score questions), so it cannot generate free text. To give the selector a
compact description of the PR instead of the raw diff, the PR title,
description, changed files and annotated diff are sent once per selection to
the AI Gateway's OpenAI-compatible chat completions endpoint - the same
gateway (https://ai-gateway.{dc}), the same Bearer token (see
get_ai_gateway_token in jev_client.py) and the same request headers as the
Jev calls - and the resulting summary replaces the diff in every per-test
Jev state (see build_context_state in jev_client.py).
"""

from __future__ import annotations

import json
import urllib.error
import urllib.request

from tasks.libs.dynamic_test.jev.pr_context import MAX_DESCRIPTION_BYTES, truncate

CHAT_COMPLETIONS_PATH = "/v1/chat/completions"

# The summary replaces a diff of up to MAX_DIFF_BYTES (40k chars) in every
# per-test Jev state; a few hundred tokens capture the PR intent at a
# fraction of the size.
MAX_SUMMARY_CHARS = 2_000
SUMMARY_MAX_TOKENS = 400

SUMMARY_SYSTEM_PROMPT = (
    "You summarize pull request changes for a CI e2e-test-selection system. "
    "Given a PR's title, description, changed files and diff, produce a concise "
    "factual summary of WHAT the PR changes and WHY: the components, packages and "
    "behavior touched, the platforms or install flows affected, and the PR's stated "
    "intent. Do not speculate beyond the diff, do not judge whether tests should run, "
    "and do not restate the file list line by line. Plain text, no markdown headings, "
    "at most a dozen sentences."
)


def summarize_pr(
    token: str,
    pr: dict,
    files: list,
    merge_base: str,
    diff: str,
    *,
    model: str = "gpt-4o-mini",
    dc: str = "us1.ddbuild.io",
    source: str = "datadog-agent",
) -> str:
    """One LLM call returning a short summary of the PR's changes.

    Uses the AI Gateway chat completions endpoint with the same token,
    gateway host and headers as the Jev (System One) calls - see
    get_ai_gateway_token/ask_jev in jev_client.py. Raises RuntimeError on
    HTTP or response-shape errors; callers fail open to the raw diff.
    """
    files_section = "\n".join(f"- {f} ({kind})" if kind else f"- {f}" for f, kind in files)
    user_prompt = (
        f"## PR title\n{pr.get('title') or '(unknown)'}\n\n"
        f"## PR description\n{truncate(pr.get('description') or '(none)', MAX_DESCRIPTION_BYTES, 'description')}\n\n"
        f"## Changed files ({len(files)}, merge base {str(merge_base)[:12]})\n{files_section}\n\n"
        f"## Annotated diff\n```diff\n{diff}\n```\n\n"
        "Summarize what this PR changes and why, for deciding which e2e tests it may affect."
    )
    payload = {
        "model": model,
        "messages": [
            {"role": "system", "content": SUMMARY_SYSTEM_PROMPT},
            {"role": "user", "content": user_prompt},
        ],
        "temperature": 0.2,
        "max_tokens": SUMMARY_MAX_TOKENS,
    }
    req = urllib.request.Request(
        f"https://ai-gateway.{dc}{CHAT_COMPLETIONS_PATH}",
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
            body = json.load(resp)
    except urllib.error.HTTPError as e:
        detail = e.read().decode(errors="ignore")
        raise RuntimeError(f"AI Gateway HTTP {e.code}: {detail}") from e
    try:
        summary = body["choices"][0]["message"]["content"]
    except (KeyError, TypeError, IndexError) as e:
        raise RuntimeError(f"unexpected chat completions response: {json.dumps(body)[:500]}") from e
    if not summary or not summary.strip():
        raise RuntimeError(f"empty summary from model {model}")
    return truncate(summary.strip(), MAX_SUMMARY_CHARS, "summary")
