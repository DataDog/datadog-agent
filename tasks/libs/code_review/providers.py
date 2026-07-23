from __future__ import annotations

import io
import shlex
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

from tasks.libs.code_review.prompt import CodeReviewError, ReviewPrompt
from tasks.libs.common.utils import is_installed

PROVIDERS = ("codex", "claude", "gemini")
PROVIDER_CHOICES = (*PROVIDERS, "all")


@dataclass(frozen=True)
class ProviderInvocation:
    provider: str
    executable: str
    command: str
    stdin: str | None
    output_path: Path


def run_review(
    *,
    ctx,
    repo_root: Path,
    review_prompt: ReviewPrompt,
    provider: str,
) -> Path:
    artifact_dir = create_artifact_dir(repo_root)
    prompt_path = artifact_dir / "prompt.md"
    prompt_path.write_text(review_prompt.content, encoding="utf-8")
    print(f"Review prompt written to {prompt_path}", file=sys.stderr)

    invocations = []
    for provider_name in expand_providers(provider):
        review_diff = None
        if provider_name == "codex":
            review_diff = collect_review_diff(ctx, repo_root, review_prompt.base)
            diff_path = artifact_dir / "codex-diff.md"
            diff_path.write_text(review_diff, encoding="utf-8")
            print(f"Codex review diff written to {diff_path}", file=sys.stderr)

        invocations.append(
            build_provider_invocation(
                provider=provider_name,
                review_prompt=review_prompt,
                prompt_path=prompt_path,
                artifact_dir=artifact_dir,
                review_diff=review_diff,
            )
        )

    for invocation in invocations:
        _ensure_command_exists(invocation.executable)

    for invocation in invocations:
        run_provider(ctx, invocation, cwd=repo_root)

    return artifact_dir


def run_provider(ctx, invocation: ProviderInvocation, *, cwd: Path) -> None:
    _ensure_command_exists(invocation.executable)
    print(f"Running {invocation.provider} review...", file=sys.stderr)
    kwargs = {"hide": True, "warn": True}
    if invocation.stdin is not None:
        kwargs["in_stream"] = io.StringIO(invocation.stdin)

    result = ctx.run(f"cd {shlex.quote(str(cwd))} && {invocation.command}", **kwargs)

    output = ""
    if result.stdout:
        output += result.stdout
        print(result.stdout, end="")
    if result.stderr:
        output += result.stderr
        print(result.stderr, end="", file=sys.stderr)

    invocation.output_path.write_text(output, encoding="utf-8")

    if result.exited != 0:
        raise CodeReviewError(
            f"{invocation.provider} review failed with exit code {result.exited}. "
            f"Output saved to {invocation.output_path}"
        )


def build_provider_invocation(
    *,
    provider: str,
    review_prompt: ReviewPrompt,
    prompt_path: Path,
    artifact_dir: Path,
    review_diff: str | None = None,
) -> ProviderInvocation:
    output_path = artifact_dir / f"{provider}.md"

    if provider == "codex":
        prompt = (
            f"Review the current git changes against {review_prompt.base} using the precomputed diff below.\n"
            "Do not modify files. Return only review findings and an overall correctness verdict.\n\n"
            f"{review_prompt.content}\n"
            "## Precomputed Diff\n\n"
            f"{review_diff or 'No diff was provided.'}"
        )
        return ProviderInvocation(
            provider=provider,
            executable="codex",
            command="codex exec --sandbox read-only -",
            stdin=prompt,
            output_path=output_path,
        )

    instruction = (
        f"Review the current git changes against {review_prompt.base}. "
        f"Use the review instructions in {prompt_path}. "
        "Return actionable findings with exact file and line references."
    )

    if provider == "claude":
        return ProviderInvocation(
            provider=provider,
            executable="claude",
            command=f"claude -p {shlex.quote(instruction)}",
            stdin=None,
            output_path=output_path,
        )

    if provider == "gemini":
        return ProviderInvocation(
            provider=provider,
            executable="gemini",
            command=f"gemini -p {shlex.quote(instruction)}",
            stdin=None,
            output_path=output_path,
        )

    raise CodeReviewError(f"Unknown provider {provider!r}")


def collect_review_diff(ctx, repo_root: Path, base: str) -> str:
    diff_range = shlex.quote(f"{base}...HEAD")
    sections = []
    for title, command in (
        ("DIFF STAT", f"git diff --find-renames --stat {diff_range}"),
        ("PATCH", f"git diff --find-renames {diff_range}"),
    ):
        result = ctx.run(f"cd {shlex.quote(str(repo_root))} && {command}", hide=True, warn=True)
        if result.exited != 0:
            raise CodeReviewError(result.stderr.strip() or f"{command} failed")
        sections.extend([f"--- {title} ---", result.stdout.strip() or "(empty)", ""])

    return "\n".join(sections).rstrip() + "\n"


def expand_providers(provider: str) -> tuple[str, ...]:
    if provider == "all":
        return PROVIDERS
    if provider not in PROVIDER_CHOICES:
        raise CodeReviewError(f"Unknown provider {provider!r}. Expected one of: {', '.join(PROVIDER_CHOICES)}")
    return (provider,)


def create_artifact_dir(repo_root: Path) -> Path:
    """
    Create a durable artifact directory so users can inspect prompts and provider output.
    """
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    artifact_dir = repo_root / ".tmp" / "code-review" / stamp
    artifact_dir.mkdir(parents=True, exist_ok=True)
    return artifact_dir


def _ensure_command_exists(command: str) -> None:
    if not is_installed(command):
        raise CodeReviewError(f"Cannot run review provider: `{command}` is not installed or is not on PATH")
