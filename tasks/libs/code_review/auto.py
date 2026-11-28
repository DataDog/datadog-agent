"""
`--provider auto`: review with the first available provider other than the AI agent running the review.

An agent reviewing its own change tends to agree with itself, so `auto` detects the running agent from
the environment variables its harness sets and skips it. Candidates are tried in order, and the next one
is tried when a review fails:
  1. codex on the host, the same provider as the PR `Code review` workflow
  2. claude on the host
  3. gemini on the host
  4. claude in a `dda env dev` developer environment, which authenticates through `ddtool`

Only the providers in `PROVIDERS` can review. Supporting another harness, such as opencode or pi, means
adding it there, to `build_provider_invocation` and to `RUNNING_AGENT_VARIABLES`.
"""

from __future__ import annotations

import os
import re
import subprocess
import sys
from collections.abc import Callable, Mapping, Sequence
from dataclasses import dataclass
from pathlib import Path

from tasks.libs.code_review.prompt import CodeReviewError, ReviewPrompt
from tasks.libs.code_review.providers import run_review
from tasks.libs.common.utils import is_installed, join_command

AUTO = "auto"

# Environment variables each harness sets for the commands it runs
RUNNING_AGENT_VARIABLES = {
    "claude": ("CLAUDECODE",),
    "codex": ("CODEX_THREAD_ID", "CODEX_SANDBOX", "CODEX_SANDBOX_NETWORK_DISABLED"),
    "gemini": ("GEMINI_CLI",),
}

# How to check that a provider installed on the host is logged in
LOGIN_CHECKS = {
    "codex": (("codex", "login", "status"), "codex login"),
    "claude": (("claude", "auth", "status"), "claude auth login"),
}

DEV_ENV_ID_PREFIX = "create-pr-review-"


@dataclass(frozen=True)
class Candidate:
    provider: str
    in_dev_env: bool = False

    @property
    def label(self) -> str:
        return f"{self.provider} in a developer environment" if self.in_dev_env else self.provider


def _log(message: str) -> None:
    print(f"code-review auto: {message}", file=sys.stderr, flush=True)


def detect_running_agent(env: Mapping[str, str]) -> str | None:
    """
    Return the provider of the AI agent running this command, or None when it can't tell.

    Child processes inherit these variables, so a Codex session started from a Claude Code shell has
    both: with more than one match, no provider is skipped rather than guessing.
    """
    matches = [provider for provider, names in RUNNING_AGENT_VARIABLES.items() if any(env.get(name) for name in names)]
    if len(matches) > 1:
        _log(f"environment variables of several agents are set ({', '.join(matches)}), so none is skipped")
        return None
    return matches[0] if matches else None


def auto_candidates(running_agent: str | None) -> tuple[Candidate, ...]:
    host = tuple(Candidate(provider) for provider in ("codex", "claude", "gemini") if provider != running_agent)
    dev_env = () if running_agent == "claude" else (Candidate("claude", in_dev_env=True),)
    return host + dev_env


def review_with_first_available(
    candidates: Sequence[Candidate],
    unavailable_reason: Callable[[Candidate], str | None],
    review: Callable[[Candidate], None],
) -> Candidate:
    """
    Run `review` with each available candidate until one succeeds, and return it.

    Raises CodeReviewError listing why each candidate was skipped or failed.
    """
    skipped, failed = [], []
    for candidate in candidates:
        reason = unavailable_reason(candidate)
        if reason:
            _log(f"skipping {candidate.label}: {reason}")
            skipped.append(f"{candidate.label}: {reason}")
            continue
        _log(f"reviewing with {candidate.label}")
        try:
            review(candidate)
            return candidate
        except CodeReviewError as e:
            _log(f"the review with {candidate.label} failed: {e}")
            failed.append(f"{candidate.label}: {e}")

    if failed:
        raise CodeReviewError("every attempted review failed:\n  " + "\n  ".join(failed + skipped))
    raise CodeReviewError("no reviewer is available:\n  " + "\n  ".join(skipped or ["no candidate"]))


def run_auto_review(
    *,
    ctx,
    repo_root: Path,
    build_prompt: Callable[[], ReviewPrompt],
    task_options: Mapping[str, str | None],
    env: Mapping[str, str] = os.environ,
) -> Candidate:
    """
    Review with the first available provider other than the running agent.

    The prompt is built only when a host reviewer needs it: a developer environment builds its own from
    `task_options`, the other `code-review.run` options, so it can still review when building the prompt
    fails on the host, e.g. because npm is broken there.
    """
    running_agent = detect_running_agent(env)
    if running_agent:
        _log(f"running in {running_agent}, so {running_agent} doesn't review")

    # Built once, by the first host reviewer that is available
    prompt: list[ReviewPrompt] = []
    prompt_error: list[str] = []

    def unavailable_reason(candidate: Candidate) -> str | None:
        if candidate.in_dev_env:
            return dev_env_options_problem(task_options) or _prepare_dev_env(ctx, repo_root)
        reason = _host_unavailable_reason(ctx, candidate.provider)
        if reason is None and not prompt and not prompt_error:
            try:
                prompt.append(build_prompt())
            except CodeReviewError as e:
                prompt_error.append(f"the review prompt can't be built on the host: {e}")
        return reason or (prompt_error[0] if prompt_error else None)

    def review(candidate: Candidate) -> None:
        if not candidate.in_dev_env:
            run_review(ctx=ctx, repo_root=repo_root, review_prompt=prompt[0], provider=candidate.provider)
            return
        command = [*_in_dev_env(dev_env_id(repo_root)), "dda", "inv", "code-review.run", "--provider=claude"]
        options = [f"--{name}={value}" for name, value in task_options.items() if value]
        result = ctx.run(join_command([*command, *options]), warn=True, encoding="utf-8")
        if result.exited != 0:
            raise CodeReviewError(f"exit code {result.exited}")

    return review_with_first_available(auto_candidates(running_agent), unavailable_reason, review)


# `dda env dev run` passes its command through the container's shell again, quoting an argument only
# when it has a space. Only these characters mean the same there quoted or not.
SHELL_SAFE_VALUE = re.compile(r"[\w .,:/@+=%-]*")


def dev_env_options_problem(task_options: Mapping[str, str | None]) -> str | None:
    """Return why the options can't be passed to a developer environment unchanged, or None."""
    for name, value in task_options.items():
        if value and not SHELL_SAFE_VALUE.fullmatch(value):
            return (
                f"--{name} has characters the developer environment's shell would interpret: "
                "use only letters, digits, spaces and . , : / @ + = % - _"
            )
    return None


def _succeeds(ctx, *command: str) -> bool:
    return ctx.run(join_command(command), hide=True, warn=True).exited == 0


def _host_unavailable_reason(ctx, provider: str) -> str | None:
    if not is_installed(provider):
        return "not installed"
    if provider in LOGIN_CHECKS:
        check, login_command = LOGIN_CHECKS[provider]
        if not _succeeds(ctx, *check):
            return f"not logged in, run `{login_command}`"
    return None


def dev_env_id(repo_root: Path) -> str:
    """
    One environment per checkout, reused across runs since creating it pulls a large image.

    The suffix is the POSIX `cksum` of the checkout path, as earlier versions of the /create-pr skill
    computed it, so the environments they created keep being reused.
    """
    out = subprocess.run(["cksum"], input=str(repo_root), capture_output=True, text=True, check=True).stdout
    return DEV_ENV_ID_PREFIX + out.split()[0]


def _in_dev_env(env_id: str) -> list[str]:
    return ["dda", "env", "dev", "run", "--id", env_id, "--"]


def _prepare_dev_env(ctx, repo_root: Path) -> str | None:
    """Start the developer environment if needed; return why it can't review, or None when it can."""
    if Path("/.started").exists():
        return "already running in a developer environment"
    if not is_installed("docker"):
        return "docker, which developer environments need, is not installed"
    if not is_installed("cksum"):
        return "cksum, which names the environment, is not installed"

    env_id = dev_env_id(repo_root)
    status = ctx.run(join_command(["dda", "env", "dev", "status", "--id", env_id]), hide=True, warn=True)
    state = next(
        (line.removeprefix("State: ") for line in status.stdout.splitlines() if line.startswith("State: ")), ""
    )
    if state != "started":
        _log(f"starting developer environment {env_id} (state: {state or 'unknown'}), the first start pulls the image")
        start = ["dda", "env", "dev", "start", "--id", env_id, "--repo", str(repo_root)]
        if ctx.run(join_command(start), warn=True, out_stream=sys.stderr).exited != 0:
            return f"`dda env dev start --id {env_id}` failed"

    # An environment missing a prerequisite is removed, so the next run recreates it with newer versions
    in_env = _in_dev_env(env_id)
    for check, problem in (
        (
            ("git", "rev-parse", "--git-dir"),
            "git doesn't work in it, which happens with Git worktrees and an older dda",
        ),
        (("npm", "--version"), "npm is missing, so its image predates claude support"),
    ):
        if not _succeeds(ctx, *in_env, *check):
            _log(f"removing developer environment {env_id}: {problem}")
            if _succeeds(ctx, "dda", "env", "dev", "stop", "--id", env_id):
                _succeeds(ctx, "dda", "env", "dev", "remove", "--id", env_id)
            return f"{problem}; it was removed, so the next run recreates it"
    if not _succeeds(ctx, *in_env, "ddtool", "auth", "token", "rapid-ai-platform", "--datacenter", "us1.ddbuild.io"):
        return "claude has no credentials in it: `ddtool auth token rapid-ai-platform` failed"

    _log(f"developer environment {env_id} is kept for later runs, remove it with `dda env dev remove --id {env_id}`")
    return None
