"""
Tests for static quality gates.

Tests the top-level parse_and_trigger_gates orchestration.
"""

import unittest
from unittest.mock import MagicMock, patch

from invoke import MockContext, Result
from invoke.exceptions import Exit

from tasks.libs.package.size import InfraError
from tasks.quality_gates import parse_and_trigger_gates
from tasks.static_quality_gates.gates import PackageArtifactMeasurer


class TestQualityGatesIntegration(unittest.TestCase):
    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
            'CI_COMMIT_REF_SLUG': 'pikachu',
            'CI_COMMIT_SHORT_SHA': '1234567890',
            'CI_COMMIT_SHA': '1234567890abcdef',
            'BUCKET_BRANCH': 'main',
            'OMNIBUS_PACKAGE_DIR': '/opt/datadog-agent',
            'CI_PIPELINE_ID': '71580015',
        },
    )
    @patch("tasks.static_quality_gates.gates.PackageArtifactMeasurer._find_package_paths", new=MagicMock())
    @patch("tasks.static_quality_gates.gates.PackageArtifactMeasurer._calculate_package_sizes", new=MagicMock())
    @patch("tasks.static_quality_gates.gates.DockerArtifactMeasurer._calculate_image_wire_size", new=MagicMock())
    @patch("tasks.static_quality_gates.gates.DockerArtifactMeasurer._calculate_image_disk_size", new=MagicMock())
    @patch("tasks.static_quality_gates.gates.GateMetricHandler.send_metrics_to_datadog", new=MagicMock())
    @patch(
        "tasks.static_quality_gates.gates_reporter.QualityGateOutputFormatter.print_summary_table",
        new=MagicMock(),
    )
    @patch("tasks.quality_gates.is_a_release_branch", new=MagicMock(return_value=True))
    @patch("tasks.quality_gates.get_pr_for_branch", new=MagicMock(return_value=None))
    def test_parse_and_trigger_gates_infra_error(self):
        ctx = MockContext(
            run={
                "datadog-ci tag --level job --tags static_quality_gates:\"restart\"": Result("Done"),
                "datadog-ci tag --level job --tags static_quality_gates:\"failure\"": Result("Done"),
                "datadog-ci tag --level job --tags static_quality_gates:\"success\"": Result("Done"),
            }
        )

        # Mock one gate to raise an infrastructure error
        with patch.object(PackageArtifactMeasurer, 'measure', side_effect=InfraError("Test infra error message")):
            with self.assertRaises(Exit) as cm:
                parse_and_trigger_gates(ctx, "tasks/unit_tests/testdata/quality_gate_config_test.yml")
                self.assertIn("Test infra error message", str(cm.exception))

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_BRANCH': 'mq-working-branch-12345',
            'CI_COMMIT_REF_SLUG': 'mq-working-branch-12345',
            'BUCKET_BRANCH': 'main',
            'CI_PIPELINE_ID': '71580015',
            'CI_COMMIT_SHA': '1234567890abcdef',
            'CI_COMMIT_SHORT_SHA': '1234567',
        },
    )
    @patch(
        "tasks.static_quality_gates.gates.PackageArtifactMeasurer._find_package_paths",
        return_value={'primary': '/fake/path'},
    )
    @patch("tasks.static_quality_gates.gates.PackageArtifactMeasurer._calculate_package_sizes", return_value=(0, 0))
    @patch("tasks.static_quality_gates.gates.DockerArtifactMeasurer._calculate_image_wire_size", return_value=0)
    @patch("tasks.static_quality_gates.gates.DockerArtifactMeasurer._calculate_image_disk_size", return_value=0)
    @patch("tasks.static_quality_gates.gates.GateMetricHandler.send_metrics_to_datadog", new=MagicMock())
    @patch("tasks.static_quality_gates.gates.GateMetricHandler.generate_relative_size", new=MagicMock())
    @patch("tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports", new=MagicMock())
    @patch("tasks.quality_gates.get_ancestor", return_value="ancestor-sha")
    @patch("tasks.quality_gates.get_commit_sha", return_value="current-sha")
    @patch("tasks.quality_gates.get_pr_for_branch", new=MagicMock(return_value=None))
    @patch("tasks.quality_gates.identify_gates_exceeding_pr_threshold")
    def test_per_pr_threshold_skipped_on_merge_queue(self, mock_identify, _mock_commit, _mock_ancestor, *_):
        """Per-PR threshold check is not applied on merge queue branches."""
        ctx = MockContext(
            run={
                "datadog-ci tag --level job --tags static_quality_gates:\"success\"": Result("Done"),
                "datadog-ci tag --level job --tags static_quality_gates:\"failure\"": Result("Done"),
            }
        )
        parse_and_trigger_gates(ctx, "tasks/unit_tests/testdata/quality_gate_config_test.yml")
        mock_identify.assert_not_called()


if __name__ == '__main__':
    unittest.main()
