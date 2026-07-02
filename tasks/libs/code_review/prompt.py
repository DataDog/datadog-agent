from __future__ import annotations

from dataclasses import dataclass
from pathlib import Path, PurePosixPath

from tasks.libs.common.git import get_changed_files, get_origin_default_branch


DEFAULT_PROMPT_FILES = (
    "codereview_guideline.md",
    "bazel/codereview_guideline.md",
    ".claude/skills/codereview_guideline.md",
)
WORKFLOW_PATH = ".github/workflows/code-review.yml"


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
    if prompt and extra_prompt:
        raise CodeReviewError("--prompt replaces the generated prompt and cannot be combined with --extra-prompt")

    resolved_base = base or get_origin_default_branch(ctx)

    if prompt:
        return ReviewPrompt(
            base=resolved_base,
            changed_files=(),
            guidelines=(),
            content=prompt.strip() + "\n",
        )

    changed_files = tuple(get_changed_files(ctx, resolved_base))
    guidelines = load_guidelines(repo_root, changed_files)
    content = render_prompt(guidelines, extra_prompt=extra_prompt)
    return ReviewPrompt(base=resolved_base, changed_files=changed_files, guidelines=guidelines, content=content)


def load_guidelines(
    repo_root: Path,
    changed_files: tuple[str, ...],
    prompt_files: tuple[str, ...] | None = None,
) -> tuple[Guideline, ...]:
    guidelines = []
    for prompt_file in select_guideline_paths(changed_files, prompt_files or get_prompt_files(repo_root)):
        path = repo_root / prompt_file
        if not path.is_file():
            raise CodeReviewError(f"Review prompt file does not exist: {prompt_file}")
        guidelines.append(Guideline(path=prompt_file, content=path.read_text(encoding="utf-8").strip()))
    return tuple(guidelines)


def select_guideline_paths(
    changed_files: tuple[str, ...],
    prompt_files: tuple[str, ...] = DEFAULT_PROMPT_FILES,
) -> tuple[str, ...]:
    return tuple(prompt_file for prompt_file in prompt_files if _guideline_applies(prompt_file, changed_files))


def _guideline_applies(prompt_file: str, changed_files: tuple[str, ...]) -> bool:
    path = PurePosixPath(prompt_file)
    if str(path.parent) == ".":
        return True

    prefix = f"{path.parent.as_posix().rstrip('/')}/"
    return any(changed_file.startswith(prefix) for changed_file in changed_files)


def get_prompt_files(repo_root: Path) -> tuple[str, ...]:
    workflow_path = repo_root / WORKFLOW_PATH
    if not workflow_path.is_file():
        return DEFAULT_PROMPT_FILES

    workflow = workflow_path.read_text(encoding="utf-8")
    prompt_files = _prompt_files_from_block_scalar(workflow)
    return prompt_files or DEFAULT_PROMPT_FILES


def _prompt_files_from_block_scalar(workflow: str) -> tuple[str, ...]:
    lines = workflow.splitlines()
    for index, line in enumerate(lines):
        stripped = line.strip()
        if stripped != "prompt_file: |":
            continue

        prompt_indent = len(line) - len(line.lstrip())
        prompt_files = []
        for block_line in lines[index + 1 :]:
            if not block_line.strip():
                continue

            block_indent = len(block_line) - len(block_line.lstrip())
            if block_indent <= prompt_indent:
                break
            prompt_files.append(block_line.strip())

        return tuple(prompt_files)

    return ()


def render_prompt(guidelines: tuple[Guideline, ...], *, extra_prompt: str | None = None) -> str:
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
