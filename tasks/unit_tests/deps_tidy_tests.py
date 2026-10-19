# Unless explicitly stated otherwise all files in this repository are licensed
# under the Apache License Version 2.0.
# This product includes software developed at Datadog (https://www.datadoghq.com/).
# Copyright 2026-present Datadog, Inc.

"""Regression tests for recursive dependency updates and unsupported publication."""

import shlex
import unittest
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

from invoke.exceptions import Exit

from tasks.libs.common import deps_tidy
from tasks.libs.common.deps_tidy import TidyUpdate
from tasks.renovate import publish_tidy

SOURCE = "a" * 40
GENERATED = "b" * 40
TREE = "c" * 40
CONTENT_DIFF = f":100644 100644 {'1' * 40} {'2' * 40} M\0go.sum\0"


class TestPrepare(unittest.TestCase):
    def run_prepare(self, message="dependency update", diff=CONTENT_DIFF, remote=SOURCE, head=SOURCE):
        calls = []

        def run(command, **kwargs):
            args = tuple(shlex.split(command)[1:])
            calls.append(args)
            if args == ("rev-parse", "HEAD"):
                stdout = head
            elif args[0] == "ls-remote":
                stdout = f"{remote}\trefs/heads/renovate/dependency"
            elif args[0] == "log":
                stdout = message
            elif args[0] == "diff":
                stdout = diff
            elif args == ("write-tree",):
                stdout = TREE
            else:
                stdout = ""
            return SimpleNamespace(stdout=stdout)

        ctx = MagicMock()
        ctx.run.side_effect = run
        with patch.object(deps_tidy.subprocess, "check_output", return_value=b"generated contents"):
            update = deps_tidy.prepare_tidy_update(ctx, SOURCE, "renovate/dependency")
        return update, calls

    def test_external_revision_prepares_generated_contents(self):
        update, _ = self.run_prepare()
        self.assertEqual(update.expected_tree, TREE)
        self.assertEqual(update.file_changes["additions"], [{"path": "go.sum", "contents": "Z2VuZXJhdGVkIGNvbnRlbnRz"}])

    def test_own_output_cannot_publish_another_change(self):
        # Covers both generators which never converge and ones alternating output.
        for diff in (CONTENT_DIFF, CONTENT_DIFF.replace("go.sum", "Cargo.lock")):
            with self.subTest(diff=diff), self.assertRaisesRegex(RuntimeError, "own output"):
                self.run_prepare(message=f"Tidy dependencies\n\nDeps-Tidy-Source: {GENERATED}", diff=diff)

    def test_own_output_can_validate_without_changes(self):
        update, _ = self.run_prepare(message=f"Deps-Tidy-Source: {GENERATED}", diff="")
        self.assertIsNone(update)

    def test_new_external_revision_can_be_tidied(self):
        update, _ = self.run_prepare(message="Renovate rebases the dependency update")
        self.assertIsNotNone(update)

    def test_rerun_or_branch_advance_does_not_publish(self):
        update, calls = self.run_prepare(remote=GENERATED)
        self.assertIsNone(update)
        self.assertFalse(any(args[0] == "add" for args in calls))

    def test_checkout_must_match_event_revision(self):
        with self.assertRaisesRegex(RuntimeError, "event revision"):
            self.run_prepare(head=GENERATED)

    def test_deletions_are_included_in_the_atomic_publication(self):
        diff = f":100644 000000 {'1' * 40} {'0' * 40} D\0removed-file\0"
        update, _ = self.run_prepare(diff=diff)
        self.assertEqual(update.file_changes, {"additions": [], "deletions": [{"path": "removed-file"}]})

    def test_mode_only_change_cannot_become_empty_remote_commit(self):
        diff = f":100755 100644 {'1' * 40} {'1' * 40} M\0tasks/agent_payload_version.bzl\0"
        with self.assertRaisesRegex(RuntimeError, "mode change"):
            self.run_prepare(diff=diff)

    def test_unsupported_modes_cannot_be_hidden_among_content_changes(self):
        for old, new in (("100644", "100755"), ("000000", "100755"), ("000000", "120000")):
            with self.subTest(old=old, new=new), self.assertRaisesRegex(RuntimeError, "mode change"):
                deps_tidy.staged_changes(CONTENT_DIFF + f":{old} {new} {'1' * 40} {'2' * 40} M\0other\0")

    def test_regular_additions_modifications_and_deletions(self):
        for old, new in (("000000", "100644"), ("100755", "100755"), ("100644", "000000")):
            with self.subTest(old=old, new=new):
                deps_tidy.staged_changes(f":{old} {new} {'1' * 40} {'2' * 40} M\0file with spaces\0")

    def test_binary_contents_are_not_decoded_by_invoke(self):
        with patch.object(deps_tidy.subprocess, "check_output", return_value=b"\xff\0"):
            changes = deps_tidy.file_changes(CONTENT_DIFF)
        self.assertEqual(changes["additions"], [{"path": "go.sum", "contents": "/wA="}])


class TestPublishTidy(unittest.TestCase):
    def run_publish(self, update, tree=TREE):
        with (
            patch("tasks.renovate.prepare_tidy_update", return_value=update),
            patch("tasks.renovate.GithubAPI") as github,
        ):
            github.return_value.create_signed_commit.return_value = {"oid": GENERATED, "tree": {"oid": tree}}
            publish_tidy.body(
                MagicMock(), source_sha=SOURCE, branch="renovate/dependency", repository="DataDog/datadog-agent"
            )
            return github

    def test_publishes_with_source_marker_and_expected_head(self):
        changes = {"additions": [], "deletions": [{"path": "removed"}]}
        github = self.run_publish(TidyUpdate(changes, TREE))
        github.assert_called_once_with(repository="DataDog/datadog-agent")
        github.return_value.create_signed_commit.assert_called_once_with(
            "renovate/dependency",
            SOURCE,
            "[renovate skip] Tidy generated dependency files",
            f"Deps-Tidy-Source: {SOURCE}",
            changes,
        )

    def test_no_api_authentication_or_write_when_nothing_to_publish(self):
        github = self.run_publish(None)
        github.assert_not_called()

    def test_rejects_mismatched_publication_tree(self):
        with self.assertRaisesRegex(Exit, "differs from the generated tree"):
            self.run_publish(TidyUpdate({}, TREE), tree=GENERATED)

    def test_loop_detection_fails_the_task_before_publication(self):
        with (
            patch("tasks.renovate.prepare_tidy_update", side_effect=RuntimeError("own output")),
            patch("tasks.renovate.GithubAPI") as github,
            self.assertRaisesRegex(Exit, "own output"),
        ):
            publish_tidy.body(MagicMock(), source_sha=SOURCE, branch="renovate/dependency")
        github.assert_not_called()


if __name__ == "__main__":
    unittest.main()
