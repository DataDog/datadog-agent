"""SDK integration shims.

Wraps `claude_agent_sdk.query` calls for the three agent invocation
points in the coordinator loop:

  1. interpret_inbox_message: small, turns user markdown into (interpretation, planned_change).
  2. implement_candidate:     code-writing agent; makes changes in the working tree.
  3. review_experiment:       persona-based review; returns ReviewVerdict.

SDK is imported lazily so the rest of the coordinator remains importable
without the SDK installed (relevant for unit tests, local db inspection,
baseline import, etc.).
"""

from __future__ import annotations

import asyncio
import json
import re
import time
from pathlib import Path
from typing import Any, Callable

import yaml

from .config import CONFIG
from .reviewer import PHASE1_PERSONAS, PHASE2_PERSONAS
from .schema import (
    Candidate,
    Experiment,
    Phase,
    ReviewDecision,
    ReviewVerdict,
)
from .scoring import ScoringResult


# Patterns in an exception repr that indicate a transient failure worth
# retrying (network, rate limit, 5xx). Anything else propagates.
_TRANSIENT_PATTERNS = (
    "rate_limit",
    "rate limit",
    "429",
    "overloaded",
    "500 ",
    "502 ",
    "503 ",
    "504 ",
    "timeout",
    "connection",
    "temporarily unavailable",
    "server error",
    "service unavailable",
)


def _is_transient(exc: BaseException) -> bool:
    txt = f"{type(exc).__name__}: {exc}".lower()
    return any(p in txt for p in _TRANSIENT_PATTERNS)


def _with_retries(fn: Callable, *args, **kwargs):
    """Run `fn(*args, **kwargs)` with exponential backoff on transient errors."""
    last: BaseException | None = None
    for attempt in range(CONFIG.sdk_retry_max_attempts):
        try:
            return fn(*args, **kwargs)
        except BaseException as e:
            last = e
            if not _is_transient(e):
                raise
            if attempt == CONFIG.sdk_retry_max_attempts - 1:
                raise
            sleep_for = CONFIG.sdk_retry_base_seconds * (2 ** attempt)
            time.sleep(sleep_for)
    assert last is not None
    raise last


# Match `git` only when it is the command being run, not when it appears as
# an argument. Command boundary = start of string or one of `; && || | \n`.
# Trailing boundary = whitespace or end of string (so `gitk` / `git-foo` don't
# match).
_GIT_CMD_RE = re.compile(r"(?:^|[;&|\n])\s*git(?:\s|$)")


def is_git_command(command: str) -> bool:
    """Return True if the shell command runs any `git` executable.

    Used by the PreToolUse hook to block the implementation agent from
    running git — the coordinator owns all git state end-to-end.
    """
    if not command:
        return False
    return bool(_GIT_CMD_RE.search(command))


def _import_sdk():
    try:
        from claude_agent_sdk import ClaudeAgentOptions, query  # noqa: F401

        return query, ClaudeAgentOptions
    except ImportError as e:
        raise RuntimeError(
            "claude-agent-sdk not installed. Run: pip install claude-agent-sdk"
        ) from e


def _run_query(prompt: str, model: str | None = None, **options_kwargs) -> str:
    """Execute one SDK query with retries and return concatenated text.

    `model` selects a Claude model ID; callers typically pass
    CONFIG.model_deep (Opus — implement/review/propose) or
    CONFIG.model_light (Sonnet — interpret_inbox_message). An empty
    string or None falls back to the SDK's default model.
    """
    query, ClaudeAgentOptions = _import_sdk()
    if model:
        options_kwargs["model"] = model

    def _once() -> str:
        return _collect_text(query(prompt=prompt, options=ClaudeAgentOptions(**options_kwargs)))

    return _with_retries(_once)


def _collect_text(async_iter) -> str:
    """Drain an SDK query's async iterator into a single text string."""
    chunks: list[str] = []

    async def _go():
        async for msg in async_iter:
            # ResultMessage has .result; other message types are skipped.
            if hasattr(msg, "result") and msg.result is not None:
                chunks.append(str(msg.result))
        return "".join(chunks)

    return asyncio.run(_go())


def _parse_yaml_block(text: str) -> dict[str, Any]:
    """Extract the first YAML block (or whole text) and parse."""
    # Prefer fenced ```yaml blocks
    m = re.search(r"```(?:yaml)?\s*\n(.*?)```", text, re.DOTALL)
    blob = m.group(1) if m else text
    try:
        data = yaml.safe_load(blob)
    except yaml.YAMLError:
        return {}
    return data if isinstance(data, dict) else {}


# ---------------------------------------------------------------------------
# 1. Inbox interpretation
# ---------------------------------------------------------------------------

def interpret_inbox_message(content: str) -> tuple[str, str]:
    """Ask Claude to interpret a user inbox message.

    Returns (interpretation, planned_change). On parse failure returns
    ("[unparsed]", "[no action]") with the raw text preserved in the
    interpretation field.
    """
    query, ClaudeAgentOptions = _import_sdk()
    prompt = f"""The user wrote this to the coordinator inbox:

---
{content}
---

Reply in YAML with two fields:
  interpretation: <one sentence summarizing what the user wants>
  planned_change: <one sentence describing how the coordinator should change
                   its behaviour, OR "no action: <reason>">
"""
    # Lightweight one-shot summary: Sonnet is plenty.
    text = _run_query(prompt, model=CONFIG.model_light, allowed_tools=[])
    data = _parse_yaml_block(text)
    return (
        str(data.get("interpretation", "[unparsed]") or "[unparsed]"),
        str(data.get("planned_change", "[no action]") or "[no action]"),
    )


# ---------------------------------------------------------------------------
# 2. Implementation agent
# ---------------------------------------------------------------------------

async def _block_git_hook(input_data, tool_use_id, context):
    """PreToolUse hook: block any Bash call that invokes `git`.

    The coordinator owns all git state. The implementation agent must not
    run git — it could otherwise push, switch branches, reset history, or
    otherwise corrupt the scratch-branch contract.
    """
    cmd = (input_data.get("tool_input") or {}).get("command", "") or ""
    if is_git_command(cmd):
        return {
            "decision": "block",
            "reason": (
                "git commands are forbidden for the implementation agent. "
                "The coordinator manages all git state. Make file edits only; "
                "the coordinator will commit on the scratch branch after review."
            ),
        }
    return {}


def implement_candidate(candidate: Candidate, root: Path) -> str:
    """Spawn the implementation agent on the current working tree.

    Returns a short string summarizing what the agent changed (extracted
    from its final message). The caller is responsible for evaluating
    the change and deciding commit-or-revert.

    The agent's Bash access is filtered by a PreToolUse hook that blocks
    all `git` invocations — coordinator owns git state end-to-end.
    """
    try:
        from claude_agent_sdk import HookMatcher
    except ImportError as e:
        raise RuntimeError("claude-agent-sdk HookMatcher unavailable") from e

    prompt = f"""You are the implementation agent for the observer AD improvement
coordinator. Your job is to implement ONE candidate change in the repo at
{root.resolve()}.

Candidate ID: {candidate.id}
Target components: {', '.join(candidate.target_components)}

Instructions:
{candidate.description}

Constraints:
- Only modify files under comp/observer/. Do not touch tests outside that path,
  do not edit CLAUDE.md / AGENTS.md, do not add new top-level dependencies.
- Keep the change minimal — no refactors unrelated to the candidate.
- DO NOT run any git command. The coordinator manages all git state;
  attempting `git` will be blocked. Just make file edits.
- When done, print a 1-3 sentence summary starting with "DONE:" describing
  exactly which files you changed and how.
"""
    text = _run_query(
        prompt,
        model=CONFIG.model_deep,  # deep thinking — Opus
        allowed_tools=["Read", "Edit", "Write", "Bash", "Grep", "Glob"],
        cwd=str(root),
        hooks={
            "PreToolUse": [
                HookMatcher(matcher="Bash", hooks=[_block_git_hook]),
            ],
        },
    )
    # Extract the "DONE:" summary line.
    for line in text.splitlines():
        if line.strip().startswith("DONE:"):
            return line.strip()
    return text.strip().splitlines()[-1] if text.strip() else "[no summary]"


# ---------------------------------------------------------------------------
# 3. Review
# ---------------------------------------------------------------------------

def _format_scoring_for_review(
    experiment: Experiment,
    scoring: ScoringResult,
) -> str:
    deltas = "\n".join(
        f"  - {d.scenario}: F1 {d.baseline.f1:.3f} → {d.observed.f1:.3f} "
        f"(Δ{d.df1:+.3f}), FPs {d.baseline.num_baseline_fps} → {d.observed.num_baseline_fps}"
        for d in scoring.per_scenario_delta.values()
    )
    return f"""Experiment: {experiment.id}
Detector: {scoring.detector}
Baseline mean F1: {scoring.baseline_mean_f1:.4f}
Observed mean F1: {scoring.mean_f1:.4f}  (Δ{scoring.mean_df1:+.4f})
Baseline total FPs: {scoring.baseline_total_fps}
Observed total FPs: {scoring.total_fps}  (Δ{scoring.total_dfps:+d})
FP reduction pct: {scoring.fp_reduction_pct:.2%}
Strict F1 regressions: {scoring.strict_regressions or '(none)'}
Recall-floor violations: {scoring.recall_floor_violations or '(none)'}
Per-scenario deltas:
{deltas}
"""


def review_experiment(
    experiment: Experiment,
    scoring: ScoringResult,
    phase: Phase,
) -> ReviewVerdict:
    """Invoke the 2-persona Phase-1 review (or 5-persona Phase-2+).

    Each persona returns YAML with {persona, approve, rationale}.
    Unanimity required for approval.
    """
    personas = PHASE1_PERSONAS if phase == Phase.ONE else PHASE2_PERSONAS
    context = _format_scoring_for_review(experiment, scoring)

    decisions: list[ReviewDecision] = []
    for name, persona_prompt in personas.items():
        full_prompt = f"{persona_prompt}\n\n--- Experiment context ---\n{context}"
        text = _run_query(
            full_prompt,
            model=CONFIG.model_deep,  # review is a judgement call — Opus
            allowed_tools=["Read", "Grep", "Glob"],
        )
        data = _parse_yaml_block(text)
        decisions.append(
            ReviewDecision(
                persona=name,
                approve=bool(data.get("approve", False)),
                rationale=str(data.get("rationale", "") or ""),
            )
        )

    unanimous = all(d.approve for d in decisions) if decisions else False
    return ReviewVerdict(unanimous_approve=unanimous, decisions=decisions)
