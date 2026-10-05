from __future__ import annotations

import io
import json
import os
import shlex
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch

from tasks.libs.code_review.prompt import (
    CodeReviewError,
    Guideline,
    build_review_prompt,
    load_guidelines,
    render_prompt,
)
from tasks.libs.code_review.providers import (
    ProviderInvocation,
    build_provider_invocation,
    collect_review_diff,
    expand_providers,
    run_provider,
)
from tasks.libs.common.utils import join_command


class NoopContext:
    def run(self, *_args, **_kwargs):
        raise AssertionError("No command should be run in this test")


class FakeContext:
    def __init__(self):
        self.commands = []
        self.cwds = []

    def run(self, command, **kwargs):
        self.commands.append((command, kwargs))
        self.cwds.append(Path.cwd())
        return type("Result", (), {"exited": 0, "stdout": "review output\n", "stderr": "review warning\n"})()


class FakeNonAsciiContext:
    def run(self, _command, **_kwargs):
        return type("Result", (), {"exited": 0, "stdout": "verdict: ✅\n", "stderr": ""})()


class Cp1252Stream(io.StringIO):
    """Stand in for a redirected Windows console, whose encoding cannot represent most of UTF-8."""

    encoding = "cp1252"


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


class FakeDiffContext:
    def __init__(self):
        self.commands = []

    def run(self, command, **_kwargs):
        self.commands.append(command)
        stdout = {
            "--stat": " tasks/foo.py | 2 ++\n",
            "--name-only": "tasks/foo.py\n",
        }
        for marker, output in stdout.items():
            if marker in command:
                return type("Result", (), {"exited": 0, "stdout": output, "stderr": ""})()
        return type("Result", (), {"exited": 0, "stdout": "diff --git a/tasks/foo.py b/tasks/foo.py\n", "stderr": ""})()


def write_code_review_workflow(
    repo_root: Path,
    ref: str = "test-action-ref",
    prompt_file_pattern: str = "**/codereview_guideline.md",
) -> None:
    workflow_dir = repo_root / ".github" / "workflows"
    workflow_dir.mkdir(parents=True)
    (workflow_dir / "code-review.yml").write_text(
        f"""
jobs:
  review:
    uses: DataDog/code-review-action/.github/workflows/code-review.yml@{ref} # v1.1.0
    with:
      prompt_file_pattern: "{prompt_file_pattern}"
""".lstrip(),
        encoding="utf-8",
    )


class TestCodeReviewPrompt(unittest.TestCase):
    def test_load_guidelines_uses_code_review_action_helper(self):
        ctx = FakeGuidelineContext()

        with (
            tempfile.TemporaryDirectory() as tmp,
            patch("tasks.libs.code_review.prompt.is_installed", return_value=True),
            patch("tasks.libs.common.utils.is_windows", return_value=False),
        ):
            repo_root = Path(tmp)
            write_code_review_workflow(repo_root, prompt_file_pattern="**/custom_guideline.md")
            guidelines = load_guidelines(ctx, repo_root, ("bazel/BUILD.bazel", "pkg/foo.go"))

        self.assertEqual(
            guidelines,
            (
                Guideline(path="codereview_guideline.md", content="root rules"),
                Guideline(path="bazel/codereview_guideline.md", content="bazel rules"),
            ),
        )
        self.assertEqual(
            shlex.split(ctx.commands[0][0]),
            [
                "npm",
                "exec",
                "--yes",
                "--package",
                "github:DataDog/code-review-action#test-action-ref",
                "--",
                "find-guidelines",
                "--repo-root",
                str(repo_root),
                "--pattern",
                "**/custom_guideline.md",
                "--changed-files",
                "-",
            ],
        )
        self.assertEqual(ctx.commands[0][1]["encoding"], "utf-8")
        self.assertEqual(ctx.stdin, "bazel/BUILD.bazel\npkg/foo.go")

    def test_load_guidelines_quotes_arguments_for_cmd_exe(self):
        ctx = FakeGuidelineContext()

        with (
            tempfile.TemporaryDirectory() as tmp,
            patch("tasks.libs.code_review.prompt.is_installed", return_value=True),
            patch("tasks.libs.common.utils.is_windows", return_value=True),
        ):
            repo_root = Path(tmp)
            write_code_review_workflow(repo_root, prompt_file_pattern="**/custom guideline.md")
            load_guidelines(ctx, repo_root, ("pkg/foo.go",))

        # cmd.exe treats the single quotes shlex produces as literal characters.
        self.assertNotIn("'", ctx.commands[0][0])
        self.assertIn('--pattern "**/custom guideline.md"', ctx.commands[0][0])

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
            changed_files="pkg/foo.go\nbazel/custom_guideline.md\n",
            deleted_prompt_files="bazel/custom_guideline.md\n",
        )

        with (
            tempfile.TemporaryDirectory() as tmp,
            patch(
                "tasks.libs.code_review.prompt.load_guidelines",
                return_value=(Guideline(path="codereview_guideline.md", content="root rules"),),
            ),
            patch("sys.stderr", new_callable=io.StringIO) as stderr,
        ):
            repo_root = Path(tmp)
            write_code_review_workflow(repo_root, prompt_file_pattern="**/custom_guideline.md")
            build_review_prompt(ctx=ctx, repo_root=repo_root, base="origin/main")

        self.assertIn("Warning: deleted code review prompt file(s)", stderr.getvalue())
        self.assertIn("**/custom_guideline.md", stderr.getvalue())
        self.assertIn("bazel/custom_guideline.md", stderr.getvalue())
        deleted_file_commands = [command for command in ctx.commands if "--diff-filter=D" in command]
        self.assertEqual(len(deleted_file_commands), 1)
        self.assertIn(":(glob)**/custom_guideline.md", deleted_file_commands[0])
        # `-C` keeps the pathspec anchored to the repository root, whatever the invocation directory.
        self.assertIn(f"git -C {join_command([str(repo_root)])} diff", deleted_file_commands[0])

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
            review_diff="--- DIFF STAT ---\ntasks/foo.py | 2 ++\n\n--- PATCH ---\ndiff --git a/tasks/foo.py b/tasks/foo.py\n",
        )

        self.assertEqual(invocation.executable, "codex")
        self.assertEqual(invocation.args, ("codex", "exec", "--sandbox", "read-only", "-"))
        self.assertIn("--- DIFF STAT ---", invocation.stdin or "")
        self.assertIn("diff --git a/tasks/foo.py b/tasks/foo.py", invocation.stdin or "")
        self.assertIn("custom review instructions", invocation.stdin or "")
        self.assertEqual(invocation.output_path, Path(".tmp/code-review/codex.md"))

    def test_collect_review_diff(self):
        ctx = FakeDiffContext()

        review_diff = collect_review_diff(ctx, Path("/repo"), "origin/main")

        self.assertIn("--- DIFF STAT ---\ntasks/foo.py | 2 ++", review_diff)
        self.assertNotIn("--- CHANGED FILES ---", review_diff)
        self.assertIn("--- PATCH ---\ndiff --git a/tasks/foo.py b/tasks/foo.py", review_diff)
        self.assertEqual(len(ctx.commands), 2)
        self.assertTrue(all("origin/main...HEAD" in command for command in ctx.commands))
        # The diff is anchored with `-C` rather than a shell `cd`, which cmd.exe cannot chain.
        self.assertTrue(all(command.startswith("git -C ") for command in ctx.commands))
        self.assertFalse(any(command.startswith("cd ") for command in ctx.commands))

    def test_build_claude_invocation_references_prompt_file(self):
        review_prompt = build_review_prompt(
            ctx=NoopContext(),
            repo_root=Path("."),
            base="origin/main",
            prompt="custom review instructions",
        )

        prompt_path = Path(".tmp/code-review/prompt.md")
        invocation = build_provider_invocation(
            provider="claude",
            review_prompt=review_prompt,
            prompt_path=prompt_path,
            artifact_dir=Path(".tmp/code-review"),
        )

        self.assertEqual(invocation.executable, "claude")
        self.assertEqual(len(invocation.args), 3)
        self.assertEqual(invocation.args[:2], ("claude", "-p"))
        self.assertIn("origin/main", invocation.args[2])
        self.assertIn(str(prompt_path), invocation.args[2])
        self.assertIsNone(invocation.stdin)

    def test_provider_instruction_is_passed_as_a_single_cmd_exe_argument(self):
        review_prompt = build_review_prompt(
            ctx=NoopContext(),
            repo_root=Path("."),
            base="origin/main",
            prompt="custom review instructions",
        )

        invocation = build_provider_invocation(
            provider="gemini",
            review_prompt=review_prompt,
            prompt_path=Path(".tmp/code-review/prompt.md"),
            artifact_dir=Path(".tmp/code-review"),
        )

        with patch("tasks.libs.common.utils.is_windows", return_value=True):
            command = join_command(invocation.args)

        # cmd.exe would word-split the instruction if it were wrapped in the single quotes shlex produces.
        self.assertTrue(command.startswith('gemini -p "Review the current git changes'))
        self.assertTrue(command.endswith('references."'))
        self.assertNotIn("'", command)

    def test_unknown_provider_is_rejected(self):
        with self.assertRaises(CodeReviewError):
            expand_providers("unknown")

    def test_run_provider_uses_ctx(self):
        ctx = FakeContext()
        original_cwd = Path.cwd()

        with (
            tempfile.TemporaryDirectory() as tmp,
            patch("sys.stdout", new_callable=io.StringIO),
            patch("sys.stderr", new_callable=io.StringIO),
            patch("tasks.libs.code_review.providers.is_installed", return_value=True),
        ):
            output_path = Path(tmp) / "codex.md"
            run_provider(
                ctx,
                ProviderInvocation(
                    provider="codex",
                    executable="codex",
                    args=("codex", "exec", "--sandbox", "read-only", "-"),
                    stdin="review prompt",
                    output_path=output_path,
                ),
                cwd=Path(tmp),
            )

            self.assertEqual(output_path.read_text(encoding="utf-8"), "review output\nreview warning\n")
            # The provider runs in the repository root rather than behind a shell `cd` prefix.
            self.assertEqual(ctx.cwds[0], Path(os.path.realpath(tmp)))

        self.assertEqual(Path.cwd(), original_cwd)
        self.assertEqual(ctx.commands[0][0], "codex exec --sandbox read-only -")
        self.assertEqual(ctx.commands[0][1]["encoding"], "utf-8")
        self.assertEqual(ctx.commands[0][1]["in_stream"].read(), "review prompt")

    def test_run_provider_echoes_output_the_console_cannot_encode(self):
        ctx = FakeNonAsciiContext()
        stdout = Cp1252Stream()

        with (
            tempfile.TemporaryDirectory() as tmp,
            patch("sys.stdout", stdout),
            patch("sys.stderr", new_callable=io.StringIO),
            patch("tasks.libs.code_review.providers.is_installed", return_value=True),
        ):
            output_path = Path(tmp) / "codex.md"
            run_provider(
                ctx,
                ProviderInvocation(
                    provider="codex",
                    executable="codex",
                    args=("codex", "exec", "-"),
                    stdin=None,
                    output_path=output_path,
                ),
                cwd=Path(tmp),
            )

            # The console loses what it cannot represent; the artifact keeps the original text.
            self.assertEqual(output_path.read_text(encoding="utf-8"), "verdict: ✅\n")

        self.assertEqual(stdout.getvalue(), "verdict: ?\n")


if __name__ == "__main__":
    unittest.main()
