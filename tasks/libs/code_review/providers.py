from __future__ import annotations

import shutil
import subprocess
import sys
from dataclasses import dataclass
from datetime import datetime, timezone
from pathlib import Path

from tasks.libs.code_review.prompt import CodeReviewError, ReviewPrompt


PROVIDERS = ("codex", "claude", "gemini")
PROVIDER_CHOICES = (*PROVIDERS, "all")


@dataclass(frozen=True)
class ProviderInvocation:
    provider: str
    command: tuple[str, ...]
    stdin: str | None
    output_path: Path


def expand_providers(provider: str) -> tuple[str, ...]:
    if provider == "all":
        return PROVIDERS
    if provider not in PROVIDER_CHOICES:
        raise CodeReviewError(f"Unknown provider {provider!r}. Expected one of: {', '.join(PROVIDER_CHOICES)}")
    return (provider,)


def create_artifact_dir(repo_root: Path) -> Path:
    stamp = datetime.now(timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    artifact_dir = repo_root / ".tmp" / "code-review" / stamp
    artifact_dir.mkdir(parents=True, exist_ok=True)
    return artifact_dir


def build_provider_invocation(
    *,
    provider: str,
    review_prompt: ReviewPrompt,
    prompt_path: Path,
    artifact_dir: Path,
) -> ProviderInvocation:
    output_path = artifact_dir / f"{provider}.md"

    if provider == "codex":
        prompt = (
            f"Review the current git changes against {review_prompt.base}.\n"
            f"Use `git diff --find-renames {review_prompt.base}...HEAD` to inspect the patch.\n"
            "Do not modify files. Return only review findings and an overall correctness verdict.\n\n"
            f"{review_prompt.content}"
        )
        return ProviderInvocation(
            provider=provider,
            command=("codex", "exec", "--sandbox", "read-only", "-"),
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
            command=("claude", "-p", instruction),
            stdin=None,
            output_path=output_path,
        )

    if provider == "gemini":
        return ProviderInvocation(
            provider=provider,
            command=("gemini", "-p", instruction),
            stdin=None,
            output_path=output_path,
        )

    raise CodeReviewError(f"Unknown provider {provider!r}")


def _ensure_command_exists(command: str) -> None:
    if shutil.which(command) is None:
        raise CodeReviewError(f"Cannot run review provider: `{command}` is not installed or is not on PATH")


def run_provider(invocation: ProviderInvocation, *, cwd: Path) -> None:
    _ensure_command_exists(invocation.command[0])
    print(f"Running {invocation.provider} review...", file=sys.stderr)
    completed = subprocess.run(
        list(invocation.command),
        cwd=cwd,
        input=invocation.stdin,
        text=True,
        check=False,
        capture_output=True,
    )

    output = ""
    if completed.stdout:
        output += completed.stdout
        print(completed.stdout, end="")
    if completed.stderr:
        output += completed.stderr
        print(completed.stderr, end="", file=sys.stderr)

    invocation.output_path.write_text(output, encoding="utf-8")

    if completed.returncode != 0:
        raise CodeReviewError(
            f"{invocation.provider} review failed with exit code {completed.returncode}. "
            f"Output saved to {invocation.output_path}"
        )


def run_review(
    *,
    repo_root: Path,
    review_prompt: ReviewPrompt,
    provider: str,
) -> Path:
    artifact_dir = create_artifact_dir(repo_root)
    prompt_path = artifact_dir / "prompt.md"
    prompt_path.write_text(review_prompt.content, encoding="utf-8")
    print(f"Review prompt written to {prompt_path}", file=sys.stderr)

    invocations = [
        build_provider_invocation(
            provider=provider_name,
            review_prompt=review_prompt,
            prompt_path=prompt_path,
            artifact_dir=artifact_dir,
        )
        for provider_name in expand_providers(provider)
    ]

    for invocation in invocations:
        _ensure_command_exists(invocation.command[0])

    for invocation in invocations:
        run_provider(invocation, cwd=repo_root)

    return artifact_dir
