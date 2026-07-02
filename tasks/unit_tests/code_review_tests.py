from __future__ import annotations

import tempfile
import unittest
from pathlib import Path

from tasks.libs.code_review.prompt import (
    CodeReviewError,
    Guideline,
    build_review_prompt,
    get_prompt_files,
    load_guidelines,
    render_prompt,
    select_guideline_paths,
)
from tasks.libs.code_review.providers import build_provider_invocation, expand_providers


class TestCodeReviewPrompt(unittest.TestCase):
    def test_select_guideline_paths_includes_root_for_any_change(self):
        self.assertEqual(
            select_guideline_paths(("pkg/foo.go",)),
            ("codereview_guideline.md",),
        )

    def test_select_guideline_paths_includes_matching_scoped_guidelines(self):
        self.assertEqual(
            select_guideline_paths(("bazel/rules/foo.bzl", ".claude/skills/my-skill/SKILL.md")),
            (
                "codereview_guideline.md",
                "bazel/codereview_guideline.md",
                ".claude/skills/codereview_guideline.md",
            ),
        )

    def test_load_guidelines_reads_selected_files(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo_root = Path(tmp)
            (repo_root / "codereview_guideline.md").write_text("root rules\n", encoding="utf-8")
            (repo_root / "bazel").mkdir()
            (repo_root / "bazel" / "codereview_guideline.md").write_text("bazel rules\n", encoding="utf-8")
            (repo_root / ".claude" / "skills").mkdir(parents=True)
            (repo_root / ".claude" / "skills" / "codereview_guideline.md").write_text(
                "skill rules\n",
                encoding="utf-8",
            )

            guidelines = load_guidelines(repo_root, ("bazel/BUILD.bazel",))

        self.assertEqual(
            guidelines,
            (
                Guideline(path="codereview_guideline.md", content="root rules"),
                Guideline(path="bazel/codereview_guideline.md", content="bazel rules"),
            ),
        )

    def test_get_prompt_files_reads_workflow_prompt_file_block(self):
        with tempfile.TemporaryDirectory() as tmp:
            repo_root = Path(tmp)
            (repo_root / ".github" / "workflows").mkdir(parents=True)
            (repo_root / ".github" / "workflows" / "code-review.yml").write_text(
                """
name: Code review
jobs:
  review:
    with:
      prompt_file: |
        codereview_guideline.md
        bazel/codereview_guideline.md
""".lstrip(),
                encoding="utf-8",
            )

            self.assertEqual(
                get_prompt_files(repo_root),
                ("codereview_guideline.md", "bazel/codereview_guideline.md"),
            )

    def test_render_prompt_appends_extra_prompt(self):
        prompt = render_prompt(
            (Guideline(path="codereview_guideline.md", content="root rules"),),
            extra_prompt="focus on shutdown",
        )

        self.assertIn("## codereview_guideline.md\n\nroot rules", prompt)
        self.assertIn("## Extra Prompt\n\nfocus on shutdown", prompt)

    def test_build_review_prompt_uses_prompt_override(self):
        review_prompt = build_review_prompt(
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

        self.assertEqual(invocation.command, ("codex", "exec", "--sandbox", "read-only", "-"))
        self.assertIn("git diff --find-renames origin/main...HEAD", invocation.stdin or "")
        self.assertIn("custom review instructions", invocation.stdin or "")
        self.assertEqual(invocation.output_path, Path(".tmp/code-review/codex.md"))

    def test_build_claude_invocation_references_prompt_file(self):
        review_prompt = build_review_prompt(
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

        self.assertEqual(invocation.command[0:2], ("claude", "-p"))
        self.assertIn("origin/main", invocation.command[2])
        self.assertIn(".tmp/code-review/prompt.md", invocation.command[2])
        self.assertIsNone(invocation.stdin)

    def test_unknown_provider_is_rejected(self):
        with self.assertRaises(CodeReviewError):
            expand_providers("unknown")


if __name__ == "__main__":
    unittest.main()
