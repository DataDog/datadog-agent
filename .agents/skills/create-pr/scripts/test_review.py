#!/usr/bin/env python3
"""Unit tests for review.py's reviewer selection. Runs no reviewer: commands are recorded instead.

Stdlib only, so this runs the same with `python3 test_review.py` or `python3 -m pytest test_review.py`.
"""

from __future__ import annotations

import unittest

from review import EXIT_FAILED, EXIT_OK, EXIT_UNAVAILABLE, EXIT_USAGE, Reviewer, detect_self, main, parse_args, review


def fake(provider: str, available: bool = True) -> Reviewer:
    label = f"{provider}-{'up' if available else 'down'}"
    return Reviewer(label, provider, lambda args: [label, *args] if available else None)


class ParseArgsTest(unittest.TestCase):
    def test_self_anywhere(self):
        self.assertEqual(
            parse_args(["--base", "main", "--self", "codex", "check x"]), ("codex", ["--base", "main", "check x"])
        )

    def test_self_equals(self):
        self.assertEqual(parse_args(["--self=claude"]), ("claude", []))

    def test_trailing_self_without_value(self):
        self.assertEqual(parse_args(["--base", "main", "--self"]), ("", ["--base", "main"]))

    def test_no_self(self):
        self.assertEqual(parse_args(["prompt"]), ("", ["prompt"]))


class DetectSelfTest(unittest.TestCase):
    def test_claude(self):
        self.assertEqual(detect_self({"CLAUDECODE": "1"}), "claude")

    def test_codex(self):
        self.assertEqual(detect_self({"CODEX_SANDBOX": "seatbelt"}), "codex")

    def test_both_is_unknown(self):
        self.assertEqual(detect_self({"CLAUDECODE": "1", "CODEX_THREAD_ID": "t"}), "")

    def test_neither(self):
        self.assertEqual(detect_self({"CLAUDECODE": ""}), "")


class ReviewTest(unittest.TestCase):
    def run_review(self, reviewers, self_provider, results=()):
        ran = []
        results = list(results)

        def run(cmd):
            ran.append(cmd[0])
            return results.pop(0) if results else 0

        return review(reviewers, self_provider, ["--base", "main"], run), ran

    def test_claude_gets_codex(self):
        code, ran = self.run_review([fake("codex"), fake("claude"), fake("claude")], "claude")
        self.assertEqual((code, ran), (EXIT_OK, ["codex-up"]))

    def test_codex_gets_claude(self):
        code, ran = self.run_review([fake("codex"), fake("claude")], "codex")
        self.assertEqual((code, ran), (EXIT_OK, ["claude-up"]))

    def test_claude_never_falls_back_to_claude(self):
        code, ran = self.run_review([fake("codex"), fake("claude")], "claude", results=[1])
        self.assertEqual((code, ran), (EXIT_FAILED, ["codex-up"]))

    def test_claude_without_codex_is_unavailable(self):
        code, ran = self.run_review([fake("codex", available=False), fake("claude")], "claude")
        self.assertEqual((code, ran), (EXIT_UNAVAILABLE, []))

    def test_unknown_agent_tries_in_order(self):
        reviewers = [fake("codex"), fake("claude", available=False), fake("claude")]
        code, ran = self.run_review(reviewers, "", results=[1, 0])
        self.assertEqual((code, ran), (EXIT_OK, ["codex-up", "claude-up"]))

    def test_args_reach_the_reviewer(self):
        seen = []
        review([fake("codex")], "", ["--base", "main"], lambda cmd: seen.append(cmd) or 0)
        self.assertEqual(seen, [["codex-up", "--base", "main"]])


class MainTest(unittest.TestCase):
    def test_invalid_self(self):
        self.assertEqual(main(["--self", "Claude"]), EXIT_USAGE)


if __name__ == "__main__":
    unittest.main()
