from __future__ import annotations

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.code_review.auto import AUTO, run_auto_review
from tasks.libs.code_review.prompt import CodeReviewError, build_review_prompt
from tasks.libs.code_review.providers import run_review
from tasks.libs.common.utils import get_repo_root


@task(
    help={
        "base": "Base branch or ref to review against. Defaults to the repository default branch.",
        "provider": "Review provider to run: codex, claude, gemini, all, or auto (the first available one other than the AI agent running this). Default: codex.",
        "extra-prompt": "Additional instructions appended to the generated code review prompt.",
        "override-prompt": "Full prompt override. Cannot be combined with --extra-prompt.",
    }
)
def run(
    ctx,
    base: str | None = None,
    provider: str = "codex",
    extra_prompt: str | None = None,
    override_prompt: str | None = None,
) -> None:
    try:
        repo_root = get_repo_root()

        def build_prompt():
            return build_review_prompt(
                ctx=ctx,
                repo_root=repo_root,
                base=base,
                extra_prompt=extra_prompt,
                prompt=override_prompt,
            )

        if provider == AUTO:
            if override_prompt and extra_prompt:
                build_prompt()  # Reports the conflict
            task_options = {"base": base, "extra-prompt": extra_prompt, "override-prompt": override_prompt}
            run_auto_review(ctx=ctx, repo_root=repo_root, build_prompt=build_prompt, task_options=task_options)
        else:
            run_review(ctx=ctx, repo_root=repo_root, review_prompt=build_prompt(), provider=provider)
    except CodeReviewError as e:
        raise Exit(str(e), code=1) from e


@task(
    help={
        "base": "Base branch or ref to compute changed files against. Defaults to the repository default branch.",
        "extra-prompt": "Additional instructions appended to the generated code review prompt.",
    }
)
def show_prompt(ctx, base: str | None = None, extra_prompt: str | None = None) -> None:
    try:
        repo_root = get_repo_root()
        review_prompt = build_review_prompt(
            ctx=ctx,
            repo_root=repo_root,
            base=base,
            extra_prompt=extra_prompt,
        )
        print(review_prompt.content, end="")
    except CodeReviewError as e:
        raise Exit(str(e), code=1) from e
