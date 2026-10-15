# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

"""Regression tests for recursive dependency updates and unsupported publication."""

import io
import json
import unittest
from unittest.mock import patch

import prepare

SOURCE = "a" * 40
GENERATED = "b" * 40
TREE = "c" * 40
CONTENT_DIFF = f":100644 100644 {'1' * 40} {'2' * 40} M\0go.sum\0"


class PrepareTests(unittest.TestCase):
    def run_prepare(self, message="dependency update", diff=CONTENT_DIFF, remote=SOURCE):
        calls = []

        def git(*args):
            calls.append(args)
            if args == ("rev-parse", "HEAD"):
                return SOURCE
            if args[0] == "ls-remote":
                return f"{remote}\trefs/heads/renovate/dependency"
            if args[0] == "log":
                return message
            if args[0] == "diff":
                return diff
            if args == ("write-tree",):
                return TREE
            return ""

        output = io.StringIO()
        publication = io.StringIO()
        with (
            patch.object(prepare, "git", side_effect=git),
            patch.object(prepare.subprocess, "check_output", return_value=b"generated contents"),
        ):
            prepare.prepare(
                SOURCE, "DataDog/datadog-agent", "renovate/dependency", "Tidy dependencies", output, publication
            )
        return output.getvalue(), calls, publication.getvalue()

    def test_external_revision_gets_one_marked_commit(self):
        output, _, publication = self.run_prepare()
        self.assertIn("has_changes=true", output)
        self.assertIn(f"expected_tree={TREE}", output)
        commit = json.loads(publication)["variables"]["input"]
        self.assertEqual(commit["message"]["body"], f"Deps-Tidy-Source: {SOURCE}")
        self.assertEqual(commit["expectedHeadOid"], SOURCE)
        self.assertEqual(
            commit["fileChanges"]["additions"], [{"path": "go.sum", "contents": "Z2VuZXJhdGVkIGNvbnRlbnRz"}]
        )

    def test_own_output_cannot_publish_another_change(self):
        # Covers both generators which never converge and ones alternating output.
        for diff in (CONTENT_DIFF, CONTENT_DIFF.replace("go.sum", "Cargo.lock")):
            with self.subTest(diff=diff), self.assertRaisesRegex(RuntimeError, "own output"):
                self.run_prepare(message=f"Tidy dependencies\n\nDeps-Tidy-Source: {GENERATED}", diff=diff)

    def test_own_output_can_validate_without_changes(self):
        output, _, publication = self.run_prepare(message=f"Deps-Tidy-Source: {GENERATED}", diff="")
        self.assertEqual(output, "has_changes=false\n")
        self.assertEqual(publication, "")

    def test_new_external_revision_can_be_tidied(self):
        output, _, _ = self.run_prepare(message="Renovate rebases the dependency update")
        self.assertIn("has_changes=true", output)

    def test_rerun_or_branch_advance_does_not_publish(self):
        output, calls, publication = self.run_prepare(remote=GENERATED)
        self.assertEqual(output, "has_changes=false\n")
        self.assertFalse(any(args[0] in {"add", "commit"} for args in calls))
        self.assertEqual(publication, "")

    def test_deletions_are_included_in_the_atomic_publication(self):
        diff = f":100644 000000 {'1' * 40} {'0' * 40} D\0removed-file\0"
        _, _, publication = self.run_prepare(diff=diff)
        self.assertEqual(
            json.loads(publication)["variables"]["input"]["fileChanges"],
            {"additions": [], "deletions": [{"path": "removed-file"}]},
        )

    def test_mode_only_change_cannot_become_empty_remote_commit(self):
        diff = f":100755 100644 {'1' * 40} {'1' * 40} M\0tasks/agent_payload_version.bzl\0"
        with self.assertRaisesRegex(RuntimeError, "mode change"):
            self.run_prepare(diff=diff)

    def test_unsupported_modes_cannot_be_hidden_among_content_changes(self):
        for old, new in (("100644", "100755"), ("000000", "100755"), ("000000", "120000")):
            with self.subTest(old=old, new=new), self.assertRaisesRegex(RuntimeError, "mode change"):
                prepare.validate_changes(CONTENT_DIFF + f":{old} {new} {'1' * 40} {'2' * 40} M\0other\0")

    def test_regular_additions_modifications_and_deletions(self):
        for old, new in (("000000", "100644"), ("100755", "100755"), ("100644", "000000")):
            with self.subTest(old=old, new=new):
                prepare.validate_changes(f":{old} {new} {'1' * 40} {'2' * 40} M\0file with spaces\0")


if __name__ == "__main__":
    unittest.main()
