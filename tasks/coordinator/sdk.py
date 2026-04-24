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
import subprocess
import time
from pathlib import Path
from typing import Any, Callable

import yaml

from . import token_log
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


# Where to write per-call SDK error artifacts (full exception context
# including any claude-CLI stderr we can pry loose). One file per failed
# call. Referenced from the `iter_impl_failed` PR comment so a human can
# actually see why it crashed.
_SDK_ERRORS_DIR = "sdk-errors"


def _dump_sdk_error(root: Path, exc: BaseException, purpose: str, model: str) -> Path:
    """Serialise every scrap of context we can get from a failed SDK call
    to a file under .coordinator/sdk-errors/. Return the path so callers
    can reference it in journal / PR comments."""
    import datetime as _dt
    import traceback

    errdir = Path(root) / ".coordinator" / _SDK_ERRORS_DIR
    errdir.mkdir(parents=True, exist_ok=True)
    ts = _dt.datetime.now().strftime("%Y%m%dT%H%M%S")
    p = errdir / f"{ts}-{purpose}.txt"

    lines: list[str] = []
    lines.append(f"timestamp: {_dt.datetime.now().isoformat(timespec='seconds')}")
    lines.append(f"purpose:   {purpose}")
    lines.append(f"model:     {model}")
    lines.append(f"exception type: {type(exc).__name__}")
    lines.append(f"exception repr: {exc!r}")
    lines.append(f"exception str:  {str(exc)[:2000]}")
    # Common subprocess-error attributes: cmd, returncode, stdout, stderr.
    for attr in ("cmd", "args", "returncode", "output", "stdout", "stderr"):
        v = getattr(exc, attr, None)
        if v is not None:
            if isinstance(v, (bytes, bytearray)):
                try:
                    v = v.decode("utf-8", errors="replace")
                except Exception:
                    v = repr(v)
            lines.append(f"\n--- exc.{attr} ---")
            lines.append(str(v)[:8000])
    # Walk the cause/context chain — claude-agent-sdk often wraps a
    # subprocess.CalledProcessError inside its own exception.
    cause = exc.__cause__ or exc.__context__
    depth = 0
    while cause is not None and depth < 5:
        depth += 1
        lines.append(f"\n--- cause chain depth={depth} ({type(cause).__name__}) ---")
        lines.append(f"repr: {cause!r}")
        for attr in ("cmd", "args", "returncode", "stdout", "stderr"):
            v = getattr(cause, attr, None)
            if v is not None:
                if isinstance(v, (bytes, bytearray)):
                    try:
                        v = v.decode("utf-8", errors="replace")
                    except Exception:
                        v = repr(v)
                lines.append(f"  {attr}: {str(v)[:4000]}")
        cause = cause.__cause__ or cause.__context__
    lines.append("\n--- traceback ---")
    lines.append("".join(traceback.format_exception(type(exc), exc, exc.__traceback__))[:8000])
    p.write_text("\n".join(lines))
    return p


def _run_query(
    prompt: str,
    model: str | None = None,
    *,
    purpose: str = "unknown",
    root: Path | None = None,
    iter_num: int | None = None,
    **options_kwargs,
) -> str:
    """Execute one SDK query with retries and return concatenated text.

    `model` selects a Claude model ID; callers typically pass
    CONFIG.model_deep (Opus — implement/review/propose) or
    CONFIG.model_light (Sonnet — interpret_inbox_message).

    `purpose` tags token-log records for post-run cost attribution:
    "implement" | "review" | "propose" | "inbox".

    `root` is the coordinator root (used to locate the token log +
    sdk-errors dir). Defaults to CWD if omitted — the driver always
    passes the right value.
    """
    query, ClaudeAgentOptions = _import_sdk()
    if model:
        options_kwargs["model"] = model
    family = token_log.model_family(model)
    root_path = Path(root) if root else Path(".")

    def _once() -> str:
        return _collect_text(
            query(prompt=prompt, options=ClaudeAgentOptions(**options_kwargs)),
            root=root_path,
            model=model or "",
            family=family,
            purpose=purpose,
            iter_num=iter_num,
        )

    try:
        return _with_retries(_once)
    except Exception as exc:
        # Capture full context to an sdk-errors file; then re-raise with a
        # breadcrumb so the driver's iter_impl_failed handler can include
        # the path in the PR comment.
        err_path = _dump_sdk_error(root_path, exc, purpose, model or "")
        raise RuntimeError(
            f"SDK call failed (purpose={purpose}, model={model}). "
            f"Full context: {err_path}"
        ) from exc


_MSG_TRACE_SEEN: set[str] = set()


def _trace_first_msg(root: Path, purpose: str, msg) -> None:
    """One-shot diagnostic: record the type + public-attr list of the
    first message seen for each purpose. Lets us see what shape the SDK
    actually emits when debugging missing-token-usage capture.
    """
    key = f"{purpose}:{type(msg).__name__}"
    if key in _MSG_TRACE_SEEN:
        return
    _MSG_TRACE_SEEN.add(key)
    try:
        log_path = Path(root) / ".coordinator" / "sdk-msg-types.log"
        log_path.parent.mkdir(parents=True, exist_ok=True)
        attrs = [a for a in dir(msg) if not a.startswith("_")][:30]
        usage_probe = {}
        for name in ("usage", "message"):
            v = getattr(msg, name, None)
            if v is not None:
                usage_probe[name] = f"{type(v).__name__}:{[a for a in dir(v) if not a.startswith('_')][:15]}"
        import datetime as _dt
        with log_path.open("a") as f:
            f.write(
                f"{_dt.datetime.now().isoformat(timespec='seconds')} "
                f"purpose={purpose} type={type(msg).__name__} "
                f"attrs={attrs} usage_probe={usage_probe}\n"
            )
    except Exception:
        pass


def _collect_text(
    async_iter,
    *,
    root: Path,
    model: str,
    family: str,
    purpose: str,
    iter_num: int | None,
) -> str:
    """Drain an SDK query's async iterator into a single text string.

    On every message that carries `.usage`, write one line to the token
    log immediately — tokens are durable from the instant they're billed,
    not from the end of the iteration. This means a mid-iter crash/kill
    cannot lose them.
    """
    chunks: list[str] = []

    async def _go():
        async for msg in async_iter:
            # Pry token usage out of the message. The SDK (v0.1.x) puts
            # token counts in different places across message types:
            # ResultMessage has .usage; AssistantMessage / Message may
            # have .usage, .message.usage (nested), or none at all.
            # Plumb through every shape we've seen and fall back to
            # best-effort attribute access.
            in_tok = 0
            out_tok = 0
            usage = (
                getattr(msg, "usage", None)
                or getattr(getattr(msg, "message", None), "usage", None)
            )
            if usage is not None:
                # usage may be an object (attrs) OR a dict.
                if isinstance(usage, dict):
                    in_tok = int(usage.get("input_tokens", 0) or 0)
                    out_tok = int(usage.get("output_tokens", 0) or 0)
                    # Also check cache-aware fields — older SDKs surface
                    # cache_creation_input_tokens separately from input_tokens.
                    in_tok += int(usage.get("cache_creation_input_tokens", 0) or 0)
                    in_tok += int(usage.get("cache_read_input_tokens", 0) or 0)
                else:
                    in_tok = int(getattr(usage, "input_tokens", 0) or 0)
                    out_tok = int(getattr(usage, "output_tokens", 0) or 0)
                    in_tok += int(getattr(usage, "cache_creation_input_tokens", 0) or 0)
                    in_tok += int(getattr(usage, "cache_read_input_tokens", 0) or 0)
            if in_tok or out_tok:
                token_log.append(
                    root=root,
                    model=model,
                    family=family,
                    purpose=purpose,
                    input_tok=in_tok,
                    output_tok=out_tok,
                    iter_num=iter_num,
                    success=True,
                )
            if hasattr(msg, "result") and msg.result is not None:
                chunks.append(str(msg.result))
            # Diagnostic trail: on the FIRST message of each call, dump
            # its type + available attrs so we can see what the SDK is
            # actually surfacing. Helps debug missing-usage cases without
            # a live print. Appended under .coordinator/sdk-msg-types.log
            # (truncate-safe; only the first msg of each call). Only when
            # usage capture hasn't fired yet — one-shot per call.
            _trace_first_msg(root, purpose, msg)
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

def interpret_inbox_message(content: str, root: Path = Path(".")) -> tuple[str, str]:
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
    text = _run_query(
        prompt, model=CONFIG.model_light, allowed_tools=[],
        purpose="inbox", root=root,
    )
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


# Files the implementation agent may NEVER modify (ground truth, scoring
# labels, eval framework, coordinator state, git internals). These are
# either scoring labels (reward-hack surface) or cross-iteration state
# (persistent compromise surface).
_FORBIDDEN_WRITE_PREFIXES = (
    "comp/observer/scenarios/",     # ground truth JSON + episode windows
    "tasks/q.py",                   # eval orchestration
    "tasks/libs/q/",                # eval helpers + scoring code
    ".coordinator/",                # coordinator state (db, inbox, journal)
    ".git/",                        # git internals (hooks, refs, config)
    "q_branch/",                    # scenario manifest
)
# Only this prefix is writable. Everything else is locked out.
_ALLOWED_WRITE_PREFIX = "comp/observer/"


def _path_in_forbidden(path_str: str, root: Path) -> str | None:
    """If `path_str` resolves to a forbidden location, return the matching
    forbidden prefix. Otherwise None. Paths are canonicalised to guard
    against '..' traversal.
    """
    try:
        p = Path(path_str)
        if not p.is_absolute():
            p = root / p
        p = p.resolve()
        try:
            rel = str(p.relative_to(root.resolve()))
        except ValueError:
            # Outside the repo entirely — block.
            return "<outside repo>"
    except (OSError, ValueError):
        return "<invalid path>"
    rel = rel.replace("\\", "/")
    for bad in _FORBIDDEN_WRITE_PREFIXES:
        if rel == bad.rstrip("/") or rel.startswith(bad):
            return bad
    # Positive allowlist: writes must be under comp/observer/ (minus
    # the scenarios/ exception already caught above).
    if not rel.startswith(_ALLOWED_WRITE_PREFIX) and rel != _ALLOWED_WRITE_PREFIX.rstrip("/"):
        return f"<outside {_ALLOWED_WRITE_PREFIX}>"
    return None


def _make_write_guard(root: Path):
    """Build a PreToolUse hook that blocks Edit/Write to forbidden paths."""
    async def _guard(input_data, tool_use_id, context):
        tool_input = input_data.get("tool_input") or {}
        file_path = tool_input.get("file_path") or ""
        if not file_path:
            return {}
        bad = _path_in_forbidden(file_path, root)
        if bad:
            return {
                "decision": "block",
                "reason": (
                    f"Path '{file_path}' is not writable: matches forbidden "
                    f"prefix '{bad}'. The agent may only modify files under "
                    f"comp/observer/ (excluding comp/observer/scenarios/, "
                    "which contains scoring labels). The eval framework "
                    "(tasks/q.py, tasks/libs/q/), coordinator state "
                    "(.coordinator/), git internals (.git/), and scenario "
                    "manifests (q_branch/) are intentional evaluation "
                    "boundaries — changing them would invalidate the run's "
                    "F1 measurements."
                ),
            }
        return {}
    return _guard


def _make_bash_guard(root: Path):
    """PreToolUse hook for Bash: block git + any command that writes to a
    forbidden path via common shell mechanisms (redirect, cp, mv, tee, sed
    -i, rm). This is a heuristic; the Edit/Write guard is the primary line
    of defense. Bash-as-write-vector is the secondary concern.
    """
    import re

    # Rough patterns for "shell tries to write somewhere." Not airtight; a
    # determined agent can evade with eval/printf tricks. The goal is to
    # catch casual attempts, not a nation-state.
    _WRITE_TOKENS = re.compile(
        r"(>\s*|>>\s*|\btee\s+|\bcp\s+|\bmv\s+|\bsed\s+-i|\brm\s+|\btouch\s+|\bmkdir\s+)"
    )

    async def _guard(input_data, tool_use_id, context):
        cmd = (input_data.get("tool_input") or {}).get("command", "") or ""
        if is_git_command(cmd):
            return {
                "decision": "block",
                "reason": (
                    "git commands are forbidden for the implementation agent."
                ),
            }
        if not _WRITE_TOKENS.search(cmd):
            return {}
        # Command looks like it writes something. Check each word that looks
        # like a path against the forbidden list. This is imprecise —
        # err on the side of blocking when in doubt.
        for tok in cmd.split():
            # Strip quoting, redirection syntax, leading dashes
            cleaned = tok.strip("\"'`").lstrip(">").lstrip()
            if not cleaned or cleaned.startswith("-"):
                continue
            if "/" not in cleaned and "." not in cleaned:
                continue
            bad = _path_in_forbidden(cleaned, root)
            if bad:
                return {
                    "decision": "block",
                    "reason": (
                        f"Bash command appears to write to '{cleaned}', "
                        f"which matches forbidden prefix '{bad}'. Use Edit/"
                        "Write tools for comp/observer/ files; no other "
                        "paths are modifiable."
                    ),
                }
        return {}
    return _guard


def _format_prior_work(prior_experiments: list[dict]) -> str:
    """Render up to 5 prior-experiment summaries into an agent prompt block.

    Input is a list of small dicts (not Experiment objects) so the driver
    can shape it however is most useful. Fields read: id, approach_family,
    score_delta, approved, rationales (list of strings).
    """
    if not prior_experiments:
        return "(no prior experiments in this approach family)"
    lines = []
    for e in prior_experiments[-5:]:
        approved = "✓" if e.get("approved") else "✗"
        delta = e.get("score_delta")
        delta_s = f"ΔF1 {delta:+.3f}" if delta is not None else "(not scored)"
        lines.append(
            f"- {e.get('id', '?')} [{e.get('approach_family', '?')}] {approved} {delta_s}"
        )
        for r in (e.get("rationales") or [])[:2]:
            # one rationale per reviewer, typically; keep it short
            r_short = r.replace("\n", " ")[:200]
            lines.append(f"    {r_short}")
    return "\n".join(lines)


async def _block_task_tool_hook(input_data, tool_use_id, context):
    """PreToolUse hook: block the Task tool entirely.

    Task spawns a sub-Opus conversation — a 17-minute runaway of that
    is what crashed iter 16 ($291 burned, 19M tokens). The implementer
    is Sonnet with a detailed plan from the proposer; there is nothing
    that needs delegating. If the plan is too big for one Sonnet call,
    the proposer should have broken it into phases.
    """
    return {
        "decision": "block",
        "reason": (
            "Task tool is disabled for implementation calls. You (Sonnet) "
            "are executing the proposer's detailed implementation_plan, "
            "not redesigning the task. If the plan seems too big to "
            "finish in your remaining tool turns, produce a partial "
            "implementation and note in DONE what you completed vs what "
            "the plan called for — the reviewer will evaluate."
        ),
    }


def implement_candidate(
    candidate: Candidate,
    root: Path,
    prior_experiments: list[dict] | None = None,
    iter_num: int | None = None,
) -> str:
    """Execute the proposer's implementation_plan against the working tree.

    Runs on **Sonnet** (CONFIG.model_light). The proposer (Opus) already
    did the design work — file list, interface contract, algorithm steps,
    tests. Sonnet follows it mechanically. This split solves two problems:

      1. Cost: Sonnet is ~5× cheaper per token. A typical implementation
         of ~6M tokens at Opus pricing is ~$100; at Sonnet pricing it's
         ~$20.
      2. Failure mode: Opus-with-vague-instructions reached for the Task
         tool and spawned sub-agents that crashed after 17 min. Sonnet
         with a detailed plan doesn't need to delegate; the Task tool
         is also explicitly blocked here (see `_block_task_tool_hook`).

    If the proposer emitted no plan (`candidate.implementation_plan`
    empty), fall back to Opus with the old "design and implement"
    prompt — that's a bug upstream but we don't want to fail the
    iteration.
    """
    try:
        from claude_agent_sdk import HookMatcher
    except ImportError as e:
        raise RuntimeError("claude-agent-sdk HookMatcher unavailable") from e

    prior_block = _format_prior_work(prior_experiments or [])
    has_plan = bool(candidate.implementation_plan and len(candidate.implementation_plan) > 50)

    # Plan-first prompt (Sonnet). Short and prescriptive — no design
    # framing, no "consider alternatives." Execute what's specified.
    if has_plan:
        prompt = f"""You are a mechanical implementer. The proposer (Opus) already
designed this change; your job is to execute the plan faithfully.

Working directory: {root.resolve()}
Candidate ID: {candidate.id}
Approach family: {candidate.approach_family}
Target components: {', '.join(candidate.target_components)}

## Implementation plan (from the proposer — follow this)

{candidate.implementation_plan}

## Candidate description (context only; the plan is authoritative)

{candidate.description}

## Prior same-family experiments

{prior_block}

## Execution rules

1. **Follow the plan.** Do not redesign. If a plan step is ambiguous,
   pick the most mechanical interpretation (e.g. "maintain a ring buffer
   of size N" → allocate a Go slice with exactly N slots).
2. **Discovery is already in the plan.** The proposer already named
   files, interfaces, and line ranges. Read those files to confirm
   the current code matches what the plan expects, then apply the
   edits. If a file or symbol isn't where the plan says it should be,
   make your best effort and note the deviation in your DONE message.
3. **If you must deviate from the plan** (e.g. the interface has
   changed since the proposer read it, a dependency is missing, a
   test will fail with the plan as written), do the minimum deviation
   that makes the change compile + pass existing tests, and DOCUMENT
   it in your DONE message so the reviewer can judge. A net-positive
   deviation is acceptable; a deviation that abandons the plan's
   intent is not.
4. **Only touch files under `comp/observer/`.** Do NOT edit
   `comp/observer/scenarios/` (scoring labels), `tasks/q.py`,
   `tasks/libs/q/`, or anything outside `comp/observer/`. A hook will
   block those writes.
5. **Do NOT run `git`.** Coordinator owns git state.
6. **Do NOT use the Task tool.** It's disabled. You have Read, Edit,
   Write, Bash (non-destructive), Grep, Glob — that is sufficient for
   any reasonable implementation plan. If the plan seems too big,
   produce a partial implementation and note what's left.
7. **Finish with `DONE:`** — a 2-4 line summary of (a) which plan
   steps you completed, (b) which files you actually modified, (c)
   any deviations and why, (d) per-tick cost if you measured it.
"""
    else:
        # Fallback: no plan from the proposer. Very rare — the proposer
        # prompt demands a plan — but possible if the proposer errored
        # or output was malformed. Use the old Opus-design prompt,
        # shorter version.
        prompt = f"""You are implementing one candidate change in the observer
AD pipeline. Working directory: {root.resolve()}

Candidate ID: {candidate.id}
Approach family: {candidate.approach_family}
Target components: {', '.join(candidate.target_components)}

## Description

{candidate.description}

## Prior same-family experiments

{prior_block}

## Constraints

- Only modify files under comp/observer/ (minus scenarios/). Do NOT
  touch tasks/q.py, tasks/libs/q, or the eval framework.
- Do NOT use git commands. Do NOT use the Task tool (it's blocked).
- Read the target detector + its sibling + the Detector/SeriesDetector
  interface in comp/observer/def/component.go before editing.
- Keep the change small: if the candidate calls for a big algorithm,
  stub the shape and note in DONE that a proper implementation needs
  a more detailed plan next iteration.
- Finish with DONE: listing files changed + what you did.
"""

    write_guard = _make_write_guard(root)
    bash_guard = _make_bash_guard(root)
    # Sonnet with a detailed plan. max_turns ~25 is enough for: read
    # cited files, make edits, run build+test. If it can't finish in 25
    # turns, the plan was too big (proposer problem, not implementer).
    text = _run_query(
        prompt,
        model=CONFIG.model_light if has_plan else CONFIG.model_deep,
        purpose="implement",
        root=root,
        iter_num=iter_num,
        allowed_tools=["Read", "Edit", "Write", "Bash", "Grep", "Glob"],
        cwd=str(root),
        hooks={
            "PreToolUse": [
                HookMatcher(matcher="Edit", hooks=[write_guard]),
                HookMatcher(matcher="Write", hooks=[write_guard]),
                HookMatcher(matcher="Bash", hooks=[bash_guard]),
                # Hard-block Task: this is the iter-16-crash mitigation.
                # Without it, the agent can still reach the Task tool
                # because claude-agent-sdk exposes it implicitly.
                HookMatcher(matcher="Task", hooks=[_block_task_tool_hook]),
            ],
        },
        max_turns=25 if has_plan else 40,
    )
    # Extract the "DONE:" summary line.
    for line in text.splitlines():
        if line.strip().startswith("DONE:"):
            return line.strip()
    return text.strip().splitlines()[-1] if text.strip() else "[no summary]"


# ---------------------------------------------------------------------------
# 3. Review
# ---------------------------------------------------------------------------

def _format_scoring_for_review(scoring: ScoringResult) -> str:
    rows = []
    for d in scoring.per_scenario_delta.values():
        rows.append(
            f"  - {d.scenario}: F1 {d.baseline.f1:.3f} → {d.observed.f1:.3f} "
            f"(Δ{d.df1:+.3f}), recall Δ{d.drecall:+.3f}, "
            f"FPs {d.baseline.num_baseline_fps} → {d.observed.num_baseline_fps}"
        )
    deltas = "\n".join(rows)
    return (
        f"Detector: {scoring.detector}\n"
        f"Baseline mean F1: {scoring.baseline_mean_f1:.4f}  →  "
        f"Observed: {scoring.mean_f1:.4f}  (Δ{scoring.mean_df1:+.4f})\n"
        f"Baseline total FPs: {scoring.baseline_total_fps}  →  "
        f"Observed: {scoring.total_fps}  (Δ{scoring.total_dfps:+d}, "
        f"FP reduction {scoring.fp_reduction_pct:.1%})\n"
        f"Catastrophe-filter regressions: {scoring.strict_regressions or '(none)'}\n"
        f"Recall-floor violations: {scoring.recall_floor_violations or '(none)'}\n"
        f"Per-scenario:\n{deltas}"
    )


def _working_tree_diff(root: Path) -> str:
    """Snapshot the candidate's changes for reviewer context.

    The implementation agent writes to the working tree; at review time
    those edits are uncommitted. `git diff HEAD -- comp/observer` captures
    them (matches git_ops.WATCH_PATHS — only paths the coordinator actually
    commits are relevant to the review).
    """
    try:
        r = subprocess.run(
            ["git", "diff", "HEAD", "--", "comp/observer"],
            cwd=root, capture_output=True, text=True, timeout=30,
        )
    except (subprocess.TimeoutExpired, FileNotFoundError, OSError):
        return "(diff unavailable — git subprocess failed)"
    out = r.stdout
    # 200KB cap — larger than any honest single-candidate diff and below
    # anything that would dominate the prompt token cost.
    if len(out) > 200_000:
        out = out[:200_000] + "\n... (diff truncated at 200KB)"
    return out or "(no working-tree diff)"


def _redact_scenario_names(text: str, scenarios: list[str]) -> str:
    """Replace literal scenario names in reviewer rationales with a token.

    Panel-flagged leakage chain: reviewer rationale names specific
    scenarios → rationale is persisted as ReviewDecision.rationale →
    re-rendered into the implementation agent's prompt on future
    iterations via _format_prior_work. The future-iteration agent then
    "learns" which scenarios matter, biasing its work toward preserving
    them — a form of lockbox leakage through the prompt chain.

    We replace exact matches of scenario names with "<scenario>" so the
    rationale still communicates what happened ("one scenario lost
    recall") without naming which.
    """
    if not scenarios or not text:
        return text
    # Replace longest names first so "food_delivery_redis" isn't partly
    # clobbered by "redis" if both were in the list.
    redacted = text
    for s in sorted(scenarios, key=len, reverse=True):
        if s:
            redacted = redacted.replace(s, "<scenario>")
    return redacted


def _check_evidence_fields(data: dict) -> bool:
    """Structured-output enforcement: every check must have a non-empty
    evidence string, and `approve: true` requires all checks to pass.

    Returns True iff output is well-formed AND approved. Malformed output
    is treated as reject so a reviewer that emits only `{approve: true}`
    without filling the checks block cannot slip a candidate through.
    """
    checks = data.get("checks")
    if not isinstance(checks, dict) or not checks:
        return False
    for name, body in checks.items():
        if not isinstance(body, dict):
            return False
        status = str(body.get("status", "")).lower().strip()
        evidence = str(body.get("evidence", "") or "").strip()
        if status not in ("pass", "fail"):
            return False
        if len(evidence) < 20:
            # A one-word or empty evidence field is a vibes approval.
            return False
        if status == "fail":
            return False
    return bool(data.get("approve", False))


def review_experiment(
    experiment: Experiment,
    scoring: ScoringResult,
    phase: Phase,
    all_scenarios: list[str],
    root: Path,
    candidate: Candidate | None = None,
    iter_num: int | None = None,
) -> ReviewVerdict:
    """Invoke Phase-1 review (leakage_auditor + hack_detector + algorithm_expert).

    Each persona returns YAML with per-check evidence fields; unanimity
    required. Personas get the candidate's `implementation_plan` (written
    by the proposer) alongside the actual diff — lets them check plan
    fidelity and distinguish "clean execution" from "net-positive
    deviation" from "abandoned the plan and produced bad code."
    """
    personas = PHASE1_PERSONAS if phase == Phase.ONE else PHASE2_PERSONAS
    scoring_summary = _format_scoring_for_review(scoring)
    diff = _working_tree_diff(root)

    prior_block = "(no prior same-family experiments)"
    prior_rationales = getattr(experiment, "prior_rationales_summary", "") or ""
    if prior_rationales:
        prior_block = _redact_scenario_names(prior_rationales, all_scenarios)

    all_s = ", ".join(all_scenarios) or "(none)"

    # Plan for plan-fidelity checks. Empty string means "no plan authored" —
    # the reviewer persona can skip the fidelity check in that case.
    plan_block = (
        candidate.implementation_plan
        if candidate and candidate.implementation_plan
        else "(no implementation_plan was authored by the proposer for this candidate)"
    )

    decisions: list[ReviewDecision] = []
    for name, persona_prompt in personas.items():
        full_prompt = persona_prompt.format(
            diff=diff,
            all_scenarios=all_s,
            scoring_summary=scoring_summary,
            prior_block=prior_block,
            implementation_plan=plan_block,
        )
        full_prompt += f"\n\n--- Experiment context ---\n{scoring_summary}\n"
        text = _run_query(
            full_prompt,
            model=CONFIG.model_deep,  # review is a judgement call — Opus
            purpose=f"review:{name}",
            root=root,
            iter_num=iter_num,
            allowed_tools=["Read", "Grep", "Glob"],
            cwd=str(root),
            # Hard ceiling on tool hops. A reviewer that runs Grep across
            # the whole repo 50 times can burn a day of budget in one call.
            # 12 hops covers "read diff, grep for scenario names in changed
            # files, grep for suspect constants, read 2 call sites."
            max_turns=12,
        )
        data = _parse_yaml_block(text)
        approved = _check_evidence_fields(data)
        rationale = str(data.get("rationale", "") or "")
        if not approved and "checks" not in data:
            rationale = (rationale + " [auto-reject: structured output missing checks block]").strip()
        # Redact scenario names before persisting — stops the leakage
        # chain where a rationale naming specific scenarios trains future
        # implementation agents on the train/lockbox partition via
        # _format_prior_work.
        rationale = _redact_scenario_names(rationale, all_scenarios)
        decisions.append(
            ReviewDecision(
                persona=name,
                approve=approved,
                rationale=rationale,
            )
        )

    unanimous = all(d.approve for d in decisions) if decisions else False
    return ReviewVerdict(unanimous_approve=unanimous, decisions=decisions)
