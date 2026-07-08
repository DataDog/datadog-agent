from __future__ import annotations

import io
import json
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tasks.libs.code_review.prompt import (
    PROMPT_FILE_PATTERN,
    CodeReviewError,
    Guideline,
    build_review_prompt,
    load_guidelines,
    render_prompt,
)
from tasks.libs.code_review.providers import (
    ProviderInvocation,
    build_provider_invocation,
    expand_providers,
    run_provider,
)


class NoopContext:
    def run(self, *_args, **_kwargs):
        raise AssertionError("No command should be run in this test")


class FakeContext:
    def __init__(self):
        self.commands = []

    def run(self, command, **kwargs):
        self.commands.append((command, kwargs))
        return type("Result", (), {"exited": 0, "stdout": "review output\n", "stderr": "review warning\n"})()


class FakeGuidelineContext:
    def __init__(self, *, exited=0, stdout=None, stderr=""):
        self.commands = []
        self.stdin = None
        self.exited = exited
        self.stdout = stdout or json.dumps(
            {
                "error": None,
                "guidelines": [
                    {"path": "codereview_guideline.md", "content": "root rules"},
                    {"path": "bazel/codereview_guideline.md", "content": "bazel rules"},
                ],
            }
        )
        self.stderr = stderr

    def run(self, command, **kwargs):
        self.commands.append((command, kwargs))
        self.stdin = kwargs["in_stream"].read()
        return type("Result", (), {"exited": self.exited, "stdout": self.stdout, "stderr": self.stderr})()


class FakePromptContext:
    def __init__(self, *, changed_files="", deleted_prompt_files=""):
        self.changed_files = changed_files
        self.deleted_prompt_files = deleted_prompt_files
        self.commands = []

    def run(self, command, **_kwargs):
        self.commands.append(command)
        if "--diff-filter=D" in command:
            stdout = self.deleted_prompt_files
        else:
            stdout = self.changed_files
        return type("Result", (), {"stdout": stdout})()


def write_code_review_workflow(repo_root: Path, ref: str = "test-action-ref") -> None:
    workflow_dir = repo_root / ".github" / "workflows"
    workflow_dir.mkdir(parents=True)
    (workflow_dir / "code-review.yml").write_text(
        f"""
jobs:
  review:
    uses: DataDog/code-review-action/.github/workflows/code-review.yml@{ref} # v1.1.0
""".lstrip(),
        encoding="utf-8",
    )


class TestCodeReviewPrompt(unittest.TestCase):
    def test_load_guidelines_uses_code_review_action_helper(self):
        ctx = FakeGuidelineContext()

        with (
            tempfile.TemporaryDirectory() as tmp,
            patch("tasks.libs.code_review.prompt.is_installed", return_value=True),
        ):
            repo_root = Path(tmp)
            write_code_review_workflow(repo_root)
            guidelines = load_guidelines(ctx, repo_root, ("bazel/BUILD.bazel", "pkg/foo.go"))

        self.assertEqual(
            guidelines,
            (
                Guideline(path="codereview_guideline.md", content="root rules"),
                Guideline(path="bazel/codereview_guideline.md", content="bazel rules"),
            ),
        )
        self.assertIn("npm exec --yes --package", ctx.commands[0][0])
        self.assertIn("github:DataDog/code-review-action#test-action-ref", ctx.commands[0][0])
        self.assertIn("-- find-guidelines ", ctx.commands[0][0])
        self.assertIn(f"--pattern '{PROMPT_FILE_PATTERN}'", ctx.commands[0][0])
        self.assertIn("--changed-files -", ctx.commands[0][0])
        self.assertEqual(ctx.stdin, "bazel/BUILD.bazel\npkg/foo.go")

    def test_load_guidelines_reports_missing_npm(self):
        with (
            patch("tasks.libs.code_review.prompt.is_installed", return_value=False),
            self.assertRaisesRegex(CodeReviewError, "`npm` is not installed or is not on PATH"),
        ):
            load_guidelines(NoopContext(), Path("."), ("pkg/foo.go",))

    def test_load_guidelines_reports_action_error(self):
        ctx = FakeGuidelineContext(
            exited=1,
            stdout=json.dumps({"error": "prompt_file and prompt_file_pattern are mutually exclusive"}),
        )

        with (
            tempfile.TemporaryDirectory() as tmp,
            patch("tasks.libs.code_review.prompt.is_installed", return_value=True),
            self.assertRaisesRegex(CodeReviewError, "mutually exclusive"),
        ):
            repo_root = Path(tmp)
            write_code_review_workflow(repo_root)
            load_guidelines(ctx, repo_root, ("pkg/foo.go",))

    def test_load_guidelines_reports_unstructured_action_failure(self):
        ctx = FakeGuidelineContext(
            exited=1,
            stdout=json.dumps({"guidelines": []}),
            stderr="find-guidelines failed",
        )

        with (
            tempfile.TemporaryDirectory() as tmp,
            patch("tasks.libs.code_review.prompt.is_installed", return_value=True),
            self.assertRaisesRegex(CodeReviewError, "find-guidelines failed"),
        ):
            repo_root = Path(tmp)
            write_code_review_workflow(repo_root)
            load_guidelines(ctx, repo_root, ("pkg/foo.go",))

    def test_build_review_prompt_warns_when_prompt_file_is_deleted(self):
        ctx = FakePromptContext(
            changed_files="pkg/foo.go\nbazel/codereview_guideline.md\n",
            deleted_prompt_files="bazel/codereview_guideline.md\n",
        )

        with (
            patch(
                "tasks.libs.code_review.prompt.load_guidelines",
                return_value=(Guideline(path="codereview_guideline.md", content="root rules"),),
            ),
            patch("sys.stderr", new_callable=io.StringIO) as stderr,
        ):
            build_review_prompt(ctx=ctx, repo_root=Path("."), base="origin/main")

        self.assertIn("Warning: deleted code review prompt file(s)", stderr.getvalue())
        self.assertIn("bazel/codereview_guideline.md", stderr.getvalue())
        deleted_file_commands = [command for command in ctx.commands if "--diff-filter=D" in command]
        self.assertEqual(len(deleted_file_commands), 1)
        self.assertIn(":(glob)**/codereview_guideline.md", deleted_file_commands[0])

    def test_render_prompt_appends_extra_prompt(self):
        prompt = render_prompt(
            (Guideline(path="codereview_guideline.md", content="root rules"),),
            extra_prompt="focus on shutdown",
        )

        self.assertIn("## codereview_guideline.md\n\nroot rules", prompt)
        self.assertIn("## Extra Prompt\n\nfocus on shutdown", prompt)

    def test_build_review_prompt_uses_prompt_override(self):
        review_prompt = build_review_prompt(
            ctx=NoopContext(),
            repo_root=Path("."),
            base="origin/main",
            prompt="custom review instructions",
        )

        self.assertEqual(review_prompt.base, "origin/main")
        self.assertEqual(review_prompt.changed_files, ())
        self.assertEqual(review_prompt.guidelines, ())
        self.assertEqual(review_prompt.content, "custom review instructions\n")

    def test_build_review_prompt_rejects_prompt_and_extra_prompt(self):
        with self.assertRaises(CodeReviewError):
            build_review_prompt(
                ctx=NoopContext(),
                repo_root=Path("."),
                base="origin/main",
                prompt="custom review instructions",
                extra_prompt="additional instructions",
            )


class TestCodeReviewProviders(unittest.TestCase):
    def test_expand_providers(self):
        self.assertEqual(expand_providers("codex"), ("codex",))
        self.assertEqual(expand_providers("all"), ("codex", "claude", "gemini"))

    def test_build_codex_invocation(self):
        review_prompt = build_review_prompt(
            ctx=NoopContext(),
            repo_root=Path("."),
            base="origin/main",
            prompt="custom review instructions",
        )

        invocation = build_provider_invocation(
            provider="codex",
            review_prompt=review_prompt,
            prompt_path=Path(".tmp/code-review/prompt.md"),
            artifact_dir=Path(".tmp/code-review"),
        )

        self.assertEqual(invocation.executable, "codex")
        self.assertEqual(invocation.command, "codex exec --sandbox read-only -")
        self.assertIn("git diff --find-renames origin/main...HEAD", invocation.stdin or "")
        self.assertIn("custom review instructions", invocation.stdin or "")
        self.assertEqual(invocation.output_path, Path(".tmp/code-review/codex.md"))

    def test_build_claude_invocation_references_prompt_file(self):
        review_prompt = build_review_prompt(
            ctx=NoopContext(),
            repo_root=Path("."),
            base="origin/main",
            prompt="custom review instructions",
        )

        invocation = build_provider_invocation(
            provider="claude",
            review_prompt=review_prompt,
            prompt_path=Path(".tmp/code-review/prompt.md"),
            artifact_dir=Path(".tmp/code-review"),
        )

        self.assertEqual(invocation.executable, "claude")
        self.assertIn("claude -p ", invocation.command)
        self.assertIn("origin/main", invocation.command)
        self.assertIn(".tmp/code-review/prompt.md", invocation.command)
        self.assertIsNone(invocation.stdin)

    def test_unknown_provider_is_rejected(self):
        with self.assertRaises(CodeReviewError):
            expand_providers("unknown")

    def test_run_provider_uses_ctx(self):
        ctx = FakeContext()
        invocation = ProviderInvocation(
            provider="codex",
            executable="codex",
            command="codex exec --sandbox read-only -",
            stdin="review prompt",
            output_path=Path("codex.md"),
        )

        with (
            tempfile.TemporaryDirectory() as tmp,
            patch("sys.stdout"),
            patch("sys.stderr"),
            patch("tasks.libs.code_review.providers.is_installed", return_value=True),
        ):
            output_path = Path(tmp) / "codex.md"
            run_provider(
                ctx,
                ProviderInvocation(
                    provider=invocation.provider,
                    executable=invocation.executable,
                    command=invocation.command,
                    stdin=invocation.stdin,
                    output_path=output_path,
                ),
                cwd=Path(tmp),
            )

            self.assertEqual(output_path.read_text(encoding="utf-8"), "review output\nreview warning\n")

        self.assertIn("codex exec --sandbox read-only -", ctx.commands[0][0])
        self.assertEqual(ctx.commands[0][1]["in_stream"].read(), "review prompt")


if __name__ == "__main__":
    unittest.main()
