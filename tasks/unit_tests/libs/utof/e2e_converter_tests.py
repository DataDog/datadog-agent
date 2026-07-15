"""Tests for tasks.libs.testing.utof.go.e2e.converter.

Uses a slice of a real e2e test2json output that includes a Pulumi
``Diagnostics:`` block to verify that infrastructure failures are detected
and surfaced via the ``pulumi_extractor``.
"""

from __future__ import annotations

import unittest
from pathlib import Path
from unittest.mock import MagicMock

from tasks.libs.testing.result_json import ResultJson
from tasks.libs.testing.utof.go.converter import convert_go_test_results
from tasks.libs.testing.utof.go.e2e.extractors import _extract_pulumi_errors, pulumi_extractor

TESTDATA = Path(__file__).parents[2] / "testdata"


def _mock_ctx():
    ctx = MagicMock()
    result = MagicMock()
    result.ok = True
    result.stdout = ""
    ctx.run.return_value = result
    return ctx


class TestE2EConverterPulumiFailure(unittest.TestCase):
    """End-to-end conversion of a Pulumi-failing e2e run."""

    @classmethod
    def setUpClass(cls):
        result = ResultJson.from_file(str(TESTDATA / "test_output_e2e_pulumi_failure.json"))
        cls.doc = convert_go_test_results(_mock_ctx(), result, test_type="e2e", custom_extractors=[pulumi_extractor])

    def test_test_marked_e2e(self):
        leaves = [t for t in self.doc.tests if not t.subtests]
        self.assertEqual(len(leaves), 1)
        self.assertEqual(leaves[0].type, "e2e")

    def test_failure_detected(self):
        test = self.doc.tests[0]
        self.assertEqual(test.status, "fail")
        self.assertEqual(self.doc.summary.failed, 1)

    def test_failure_classified_as_infrastructure(self):
        test = self.doc.tests[0]
        failure = next((a.failure for a in test.attempts if a.failure), None)
        self.assertIsNotNone(failure, "Expected a failure on the failing attempt")
        self.assertEqual(failure.type, "infrastructure")

    def test_failure_message_includes_continuation_context(self):
        test = self.doc.tests[0]
        failure = next((a.failure for a in test.attempts if a.failure), None)
        self.assertIsNotNone(failure)
        # Both resource headers and the full indented block under each one
        # must be in the message — the continuation lines (apt lock errors,
        # package list output) explain *why* the resource failed.
        self.assertIn("command:remote:Command (remote-aws-kind-cmd-apt-update):", failure.message)
        self.assertIn("error: Process exited with status 100", failure.message)
        self.assertIn("Reading package lists...", failure.message)
        self.assertIn("E: Could not get lock /var/lib/apt/lists/lock", failure.message)
        self.assertIn("E: Unable to lock directory /var/lib/apt/lists/", failure.message)
        self.assertIn(
            "pulumi:pulumi:Stack (e2eci-ci-1653849066-4670-e2e-gpuk8ssuite-a7372dbd9f7adc2f):", failure.message
        )
        self.assertIn("error: update failed", failure.message)


class TestPulumiExtractorUnit(unittest.TestCase):
    """Direct tests for the pulumi_extractor regex behaviour."""

    def test_returns_none_when_no_pulumi_diagnostics(self):
        self.assertIsNone(pulumi_extractor(["some unrelated test failure", "    error: not in a section"]))

    def test_extract_single_resource_keeps_full_block(self):
        lines = [
            "        Diagnostics:",
            "          command:remote:Command (do-the-thing):",
            "            error: Process exited with status 1",
            "            stderr: oops",
        ]
        out = pulumi_extractor(lines)
        self.assertEqual(
            out,
            (
                "infrastructure",
                "command:remote:Command (do-the-thing):\n" "  error: Process exited with status 1\n" "  stderr: oops",
            ),
        )

    def test_extract_multiple_resources_preserves_order(self):
        lines = [
            "        Diagnostics:",
            "          command:remote:Command (a):",
            "            error: first failure",
            "",
            "          pulumi:pulumi:Stack (b):",
            "            error: stack failed",
        ]
        out = pulumi_extractor(lines)
        self.assertIsNotNone(out)
        _, msg = out
        # Resources are separated by a blank line for readability.
        self.assertEqual(
            msg,
            "command:remote:Command (a):\n  error: first failure\n\n" "pulumi:pulumi:Stack (b):\n  error: stack failed",
        )

    def test_extract_includes_continuation_context(self):
        # Continuation lines under a resource (package lists, apt lock errors,
        # warnings) are part of the failure context and must be kept.
        lines = [
            "        Diagnostics:",
            "          command:remote:Command (apt):",
            "            error: Process exited with status 100",
            "            Reading package lists...",
            "            E: Could not get lock /var/lib/apt/lists/lock",
            "            W: Target Packages is configured multiple times",
        ]
        errors = _extract_pulumi_errors(lines)
        self.assertEqual(
            errors,
            [
                "command:remote:Command (apt):\n"
                "  error: Process exited with status 100\n"
                "  Reading package lists...\n"
                "  E: Could not get lock /var/lib/apt/lists/lock\n"
                "  W: Target Packages is configured multiple times"
            ],
        )

    def test_block_ends_at_blank_line(self):
        # A blank line terminates the current resource block; any following
        # text (e.g. the "Resources:" summary) must not be attributed to it.
        lines = [
            "          pulumi:pulumi:Stack (s):",
            "            error: update failed",
            "        ",
            "        Resources:",
            "            2 errored",
        ]
        errors = _extract_pulumi_errors(lines)
        self.assertEqual(errors, ["pulumi:pulumi:Stack (s):\n  error: update failed"])

    def test_legacy_bullet_format_kept_with_summary_line(self):
        # The legacy ``\\t*`` bullet format is preserved by the same
        # block-capture rule: every indented line under the resource is
        # included, including both the "N error(s) occurred:" summary and
        # each bullet.
        lines = [
            "          kubernetes:helm.sh/v3:Release (dda-linux):",
            "            error: 2 error(s) occurred:",
            "        \t* Helm release failed: timeout",
            "        \t* connection refused",
        ]
        out = pulumi_extractor(lines)
        self.assertEqual(
            out,
            (
                "infrastructure",
                "kubernetes:helm.sh/v3:Release (dda-linux):\n"
                "  error: 2 error(s) occurred:\n"
                "  * Helm release failed: timeout\n"
                "  * connection refused",
            ),
        )


if __name__ == "__main__":
    unittest.main()
