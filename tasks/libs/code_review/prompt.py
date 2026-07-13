from __future__ import annotations

import io
import json
import shlex
import sys
from dataclasses import dataclass
from pathlib import Path

import yaml

from tasks.libs.common.git import get_changed_files, get_origin_default_branch
from tasks.libs.common.utils import is_installed

CODE_REVIEW_ACTION_REPOSITORY = "DataDog/code-review-action"
CODE_REVIEW_ACTION_WORKFLOW = f"{CODE_REVIEW_ACTION_REPOSITORY}/.github/workflows/code-review.yml"
WORKFLOW_PATH = Path(".github/workflows/code-review.yml")


class CodeReviewError(RuntimeError):
    pass


@dataclass(frozen=True)
class Guideline:
    path: str
    content: str


@dataclass(frozen=True)
class ReviewPrompt:
    base: str
    changed_files: tuple[str, ...]
    guidelines: tuple[Guideline, ...]
    content: str


def build_review_prompt(
    *,
    ctx,
    repo_root: Path,
    base: str | None = None,
    extra_prompt: str | None = None,
    prompt: str | None = None,
) -> ReviewPrompt:
    """
    Build the prompt passed to review providers for the current git diff.
    """
    if prompt and extra_prompt:
        raise CodeReviewError(
            "--override-prompt replaces the generated prompt and cannot be combined with extra prompt"
        )

    resolved_base = base or get_origin_default_branch(ctx)

    if prompt:
        return ReviewPrompt(
            base=resolved_base,
            changed_files=(),
            guidelines=(),
            content=prompt.strip() + "\n",
        )

    changed_files = tuple(get_changed_files(ctx, resolved_base))
    prompt_file_pattern = _get_prompt_file_pattern(repo_root)
    _warn_deleted_prompt_files(ctx, resolved_base, prompt_file_pattern)
    guidelines = load_guidelines(ctx, repo_root, changed_files, prompt_file_pattern=prompt_file_pattern)
    content = render_prompt(guidelines, extra_prompt=extra_prompt)
    return ReviewPrompt(base=resolved_base, changed_files=changed_files, guidelines=guidelines, content=content)


def _warn_deleted_prompt_files(ctx, base: str, prompt_file_pattern: str) -> None:
    deleted_prompt_files = _get_deleted_prompt_files(ctx, base, prompt_file_pattern)
    if not deleted_prompt_files:
        return

    print(
        "Warning: deleted code review prompt file(s) match "
        f"{prompt_file_pattern}: {', '.join(deleted_prompt_files)}. "
        "They will not be included in local review prompts; make sure the deletion is intentional.",
        file=sys.stderr,
    )


def _get_deleted_prompt_files(ctx, base: str, prompt_file_pattern: str) -> tuple[str, ...]:
    base_to_head = shlex.quote(f"{base}...HEAD")
    prompt_file_pathspec = f":(glob){prompt_file_pattern}"
    result = ctx.run(
        f"git diff --name-only --diff-filter=D {base_to_head} -- {shlex.quote(prompt_file_pathspec)}",
        hide=True,
    )
    return tuple(line for line in result.stdout.splitlines() if line)


def render_prompt(guidelines: tuple[Guideline, ...], *, extra_prompt: str | None = None) -> str:
    """
    Render selected guidelines and optional user instructions into one prompt.
    """
    sections = [
        "# Code Review Prompt",
        "",
        "Use the following instructions when reviewing the current changes.",
    ]

    for guideline in guidelines:
        sections.extend(
            [
                "",
                f"## {guideline.path}",
                "",
                guideline.content.rstrip(),
            ]
        )

    if extra_prompt:
        sections.extend(
            [
                "",
                "## Extra Prompt",
                "",
                extra_prompt.strip(),
            ]
        )

    return "\n".join(sections).rstrip() + "\n"


def load_guidelines(
    ctx,
    repo_root: Path,
    changed_files: tuple[str, ...],
    *,
    prompt_file_pattern: str | None = None,
) -> tuple[Guideline, ...]:
    """
    Read the guideline files that apply to the changed files.
    """
    if not is_installed("npm"):
        raise CodeReviewError("Cannot compute review guidelines: `npm` is not installed or is not on PATH")

    result = _run_find_guidelines(
        ctx, repo_root, changed_files, prompt_file_pattern or _get_prompt_file_pattern(repo_root)
    )
    if result.get("error"):
        raise CodeReviewError(str(result["error"]))

    return tuple(Guideline(path=guideline["path"], content=guideline["content"]) for guideline in result["guidelines"])


def _run_find_guidelines(ctx, repo_root: Path, changed_files: tuple[str, ...], prompt_file_pattern: str) -> dict:
    command = (
        f"npm exec --yes --package {shlex.quote(_get_code_review_action_package(repo_root))} "
        "-- find-guidelines "
        f"--repo-root {shlex.quote(str(repo_root))} "
        f"--pattern {shlex.quote(prompt_file_pattern)} "
        "--changed-files -"
    )
    result = ctx.run(
        command,
        hide=True,
        warn=True,
        in_stream=io.StringIO("\n".join(changed_files)),
    )

    try:
        parsed_result = json.loads(result.stdout)
    except json.JSONDecodeError as e:
        raise CodeReviewError(result.stderr.strip() or f"Failed to parse guideline discovery output: {e}") from e

    if result.exited != 0 and not parsed_result.get("error"):
        raise CodeReviewError(result.stderr.strip() or "Guideline discovery failed")

    return parsed_result


def _get_code_review_action_package(repo_root: Path) -> str:
    return f"github:{CODE_REVIEW_ACTION_REPOSITORY}#{_get_code_review_action_ref(repo_root)}"


def _get_code_review_action_ref(repo_root: Path) -> str:
    uses = _get_code_review_job(repo_root).get("uses")
    if not isinstance(uses, str) or not uses.startswith(f"{CODE_REVIEW_ACTION_WORKFLOW}@"):
        raise CodeReviewError(f"Cannot find {CODE_REVIEW_ACTION_WORKFLOW} pin in {WORKFLOW_PATH}")

    return uses.removeprefix(f"{CODE_REVIEW_ACTION_WORKFLOW}@")


def _get_prompt_file_pattern(repo_root: Path) -> str:
    inputs = _get_code_review_job(repo_root).get("with")
    if not isinstance(inputs, dict):
        raise CodeReviewError(f"Cannot find code review workflow inputs in {WORKFLOW_PATH}")

    prompt_file_pattern = inputs.get("prompt_file_pattern")
    if not isinstance(prompt_file_pattern, str) or not prompt_file_pattern.strip():
        raise CodeReviewError(f"Cannot find prompt_file_pattern in {WORKFLOW_PATH}")

    return prompt_file_pattern.strip()


def _get_code_review_job(repo_root: Path) -> dict:
    workflow_path = repo_root / WORKFLOW_PATH
    if not workflow_path.is_file():
        raise CodeReviewError(f"Cannot find code review workflow: {WORKFLOW_PATH}")

    workflow = yaml.safe_load(workflow_path.read_text(encoding="utf-8"))
    try:
        job = workflow["jobs"]["review"]
    except (KeyError, TypeError) as e:
        raise CodeReviewError(f"Cannot find code review job in {WORKFLOW_PATH}") from e

    if not isinstance(job, dict):
        raise CodeReviewError(f"Cannot find code review job in {WORKFLOW_PATH}")

    return job
