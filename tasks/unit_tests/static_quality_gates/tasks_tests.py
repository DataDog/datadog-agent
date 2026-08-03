"""
Tests for static quality gates.

Tests the top-level parse_and_trigger_gates orchestration.
"""

import os
import re
import tempfile
import unittest
from contextlib import contextmanager
from dataclasses import dataclass
from types import SimpleNamespace
from unittest.mock import MagicMock, patch

import yaml
from invoke import MockContext, Result
from invoke.exceptions import Exit

from tasks.libs.package.size import InfraError
from tasks.quality_gates import parse_and_trigger_gates
from tasks.static_quality_gates.gates import ArtifactMeasurement, PackageArtifactMeasurer


def gitlab_ref_slug(git_ref):
    """
    CI_COMMIT_REF_NAME in lowercase, shortened to 63 bytes, and with everything
    except 0-9 and a-z replaced with -. No leading / trailing -. Use in URLs, host names and domain names.
    (https://docs.gitlab.com/ci/variables/predefined_variables/)
    """
    return re.sub(r"[^0-9a-z]", "-", git_ref.lower())[:63]


_PR_BRANCH_NAME = "feature/branch"
_PR_BUCKET_BRANCH = 'dev'
_CI_PIPELINE_ID = '999999999'
_CI_COMMIT_SHA = '1234567890abcdef'
_ANCESTOR_SHA = 'ancestor-sha'


_PR_ENV_VARS = {
    'BUCKET_BRANCH': _PR_BUCKET_BRANCH,
    'CI_COMMIT_BRANCH': _PR_BRANCH_NAME,
    'CI_COMMIT_REF_SLUG': gitlab_ref_slug(_PR_BRANCH_NAME),
    'CI_PIPELINE_ID': _CI_PIPELINE_ID,
    'CI_COMMIT_SHA': _CI_COMMIT_SHA,
}


class FakeGithubAPI:
    PR = SimpleNamespace(
        number=12345,
        title="fix(somewhere): did something",
        user=SimpleNamespace(login="some-author"),
        get_reviews=lambda: [],
    )

    def get_pr_for_branch(self, branch):
        return [self.PR] if branch == _PR_BRANCH_NAME else []

    def get_pr(self, pr_id):
        return self.PR if pr_id == self.PR.number else None


_DEB_GATE = "static_quality_gate_agent_deb_amd64"
_DOCKER_GATE = "static_quality_gate_docker_agent_amd64"

_KiB = 1024
_MiB = 1024 * _KiB


@dataclass
class GateScenario:
    """Represents the desired state of a gate, for use with the _gate_scenarios_fixture fixture."""

    name: str
    current_disk: int
    current_wire: int
    ancestor_disk: int
    ancestor_wire: int
    max_disk: int
    max_wire: int


@contextmanager
def _gate_scenarios_fixture(*scenarios: GateScenario, ancestor_sha: str):
    """
    Context manager that wires up gate scenarios for integration tests.

    Patches PackageArtifactMeasurer.measure, DockerArtifactMeasurer.measure, and
    query_gate_metrics_for_commit for the duration of the test.

    Yields the path to a temp YAML file with the gate limits.
    """
    by_name = {s.name: s for s in scenarios}
    by_ancestor_sha = {
        ancestor_sha: {
            s.name: {"current_on_disk_size": s.ancestor_disk, "current_on_wire_size": s.ancestor_wire}
            for s in scenarios
        }
    }

    config = {s.name: {"max_on_disk_size": s.max_disk, "max_on_wire_size": s.max_wire} for s in scenarios}

    def _query_metrics(sha):
        return by_ancestor_sha[sha]

    def _package_measure(ctx, config):
        s = by_name[config.gate_name]
        return ArtifactMeasurement(artifact_path='/fake/path', on_wire_size=s.current_wire, on_disk_size=s.current_disk)

    def _docker_measure(ctx, config):
        s = by_name[config.gate_name]
        return ArtifactMeasurement(
            artifact_path='fake-docker-image', on_wire_size=s.current_wire, on_disk_size=s.current_disk
        )

    with tempfile.TemporaryDirectory() as tmpdir:
        config_path = os.path.join(tmpdir, 'gates.yml')
        with open(config_path, 'w') as f:
            yaml.dump(config, f)
        with (
            patch(
                "tasks.libs.common.datadog_api.query_gate_metrics_for_commit",
                side_effect=_query_metrics,
            ),
            patch(
                "tasks.static_quality_gates.gates.PackageArtifactMeasurer.measure",
                side_effect=_package_measure,
            ),
            patch(
                "tasks.static_quality_gates.gates.DockerArtifactMeasurer.measure",
                side_effect=_docker_measure,
            ),
        ):
            yield config_path


class TestQualityGatesIntegration(unittest.TestCase):
    def _assert_gate_metrics(self, sent, gate_name, *, on_disk, on_wire, max_disk, max_wire, rel_disk, rel_wire):
        metric_prefix = "datadog.agent.static_quality_gate."
        self.assertEqual(sent[(metric_prefix + "on_disk_size", gate_name)], on_disk)
        self.assertEqual(sent[(metric_prefix + "on_wire_size", gate_name)], on_wire)
        self.assertEqual(sent[(metric_prefix + "max_allowed_on_disk_size", gate_name)], max_disk)
        self.assertEqual(sent[(metric_prefix + "max_allowed_on_wire_size", gate_name)], max_wire)
        self.assertEqual(sent[(metric_prefix + "relative_on_disk_size", gate_name)], rel_disk)
        self.assertEqual(sent[(metric_prefix + "relative_on_wire_size", gate_name)], rel_wire)

    @patch.dict('os.environ', _PR_ENV_VARS, clear=True)
    def test_pr_all_gates_pass_and_send_metrics(self):
        gate_scenarios = [
            GateScenario(
                name=_DEB_GATE,
                ancestor_disk=100 * _MiB,
                ancestor_wire=50 * _MiB,
                current_disk=100 * _MiB + 20 * _KiB,
                current_wire=50 * _MiB,
                max_disk=105 * _MiB,
                max_wire=55 * _MiB,
            ),
            GateScenario(
                name=_DOCKER_GATE,
                ancestor_disk=600 * _MiB,
                ancestor_wire=240 * _MiB,
                current_disk=600 * _MiB + 20 * _KiB,
                current_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
            ),
        ]
        with (
            _gate_scenarios_fixture(*gate_scenarios, ancestor_sha=_ANCESTOR_SHA) as config_path,
            patch("tasks.quality_gates.get_ancestor", return_value=_ANCESTOR_SHA),
            patch("tasks.quality_gates.get_commit_sha", return_value=_CI_COMMIT_SHA),
            patch("tasks.static_quality_gates.github.GithubAPI", new=FakeGithubAPI),
            patch("tasks.static_quality_gates.gates.send_metrics") as mock_send_metrics,
            patch("tasks.static_quality_gates.pr_comment.pr_commenter") as mock_pr_commenter,
            patch(
                "tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports"
            ) as mock_generate_reports,
        ):
            ctx = MockContext(
                run={
                    "datadog-ci tag --level job --tags static_quality_gates:\"success\"": Result("Done"),
                }
            )
            parse_and_trigger_gates(ctx, config_path)

            # Assert on metrics sent
            mock_send_metrics.assert_called_once()
            series = mock_send_metrics.call_args.kwargs["series"]
            sent = {}
            for m in series:
                self.assertIn(f"git_ref:{gitlab_ref_slug(_PR_BRANCH_NAME)}", m.tags)
                self.assertIn(f"bucket_branch:{_PR_BUCKET_BRANCH}", m.tags)
                self.assertIn(f"pipeline_id:{_CI_PIPELINE_ID}", m.tags)
                self.assertIn(f"ci_commit_sha:{_CI_COMMIT_SHA}", m.tags)
                gate_name = next(t.split(":", 1)[1] for t in m.tags if t.startswith("gate_name:"))
                sent[(m.metric, gate_name)] = m.points[0].value

            self._assert_gate_metrics(
                sent,
                _DEB_GATE,
                on_disk=100 * _MiB + 20 * _KiB,
                on_wire=50 * _MiB,
                max_disk=105 * _MiB,
                max_wire=55 * _MiB,
                rel_disk=20 * _KiB,
                rel_wire=0,
            )
            self._assert_gate_metrics(
                sent,
                _DOCKER_GATE,
                on_disk=600 * _MiB + 20 * _KiB,
                on_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
                rel_disk=20 * _KiB,
                rel_wire=0,
            )

            mock_generate_reports.assert_called_once()

            # Assert PR comment was posted for the correct PR
            mock_pr_commenter.assert_called_once()
            self.assertIs(mock_pr_commenter.call_args.kwargs["pr"], FakeGithubAPI.PR)

    @patch.dict('os.environ', _PR_ENV_VARS, clear=True)
    def test_gate_fails_absolute_disk_limit(self):
        gate_scenarios = [
            GateScenario(
                name=_DEB_GATE,
                ancestor_disk=100 * _MiB,
                ancestor_wire=50 * _MiB,
                current_disk=110 * _MiB,  # exceeds max_disk
                current_wire=50 * _MiB,
                max_disk=105 * _MiB,
                max_wire=55 * _MiB,
            ),
            GateScenario(
                name=_DOCKER_GATE,
                ancestor_disk=600 * _MiB,
                ancestor_wire=240 * _MiB,
                current_disk=600 * _MiB + 20 * _KiB,
                current_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
            ),
        ]
        with (
            _gate_scenarios_fixture(*gate_scenarios, ancestor_sha=_ANCESTOR_SHA) as config_path,
            patch("tasks.quality_gates.get_ancestor", return_value=_ANCESTOR_SHA),
            patch("tasks.quality_gates.get_commit_sha", return_value=_CI_COMMIT_SHA),
            patch("tasks.static_quality_gates.github.GithubAPI", new=FakeGithubAPI),
            patch("tasks.static_quality_gates.gates.GateMetricHandler.generate_relative_size"),
            patch(
                "tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports"
            ) as mock_generate_reports,
            patch("tasks.static_quality_gates.gates.send_metrics") as mock_send_metrics,
            patch("tasks.static_quality_gates.pr_comment.pr_commenter") as mock_pr_commenter,
        ):
            ctx = MockContext(
                run={"datadog-ci tag --level job --tags static_quality_gates:\"failure\"": Result("Done")}
            )
            with self.assertRaises(Exit):
                parse_and_trigger_gates(ctx, config_path)
            mock_send_metrics.assert_called_once()
            mock_generate_reports.assert_called_once()
            mock_pr_commenter.assert_called_once()

    @patch.dict('os.environ', _PR_ENV_VARS, clear=True)
    def test_gate_absolute_wire_limit_does_not_block(self):
        gate_scenarios = [
            GateScenario(
                name=_DEB_GATE,
                ancestor_disk=100 * _MiB,
                ancestor_wire=50 * _MiB,
                current_disk=100 * _MiB + 20 * _KiB,
                current_wire=60 * _MiB,  # exceeds max_wire
                max_disk=105 * _MiB,
                max_wire=55 * _MiB,
            ),
            GateScenario(
                name=_DOCKER_GATE,
                ancestor_disk=600 * _MiB,
                ancestor_wire=240 * _MiB,
                current_disk=600 * _MiB + 20 * _KiB,
                current_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
            ),
        ]
        with (
            _gate_scenarios_fixture(*gate_scenarios, ancestor_sha=_ANCESTOR_SHA) as config_path,
            patch("tasks.quality_gates.get_ancestor", return_value=_ANCESTOR_SHA),
            patch("tasks.quality_gates.get_commit_sha", return_value=_CI_COMMIT_SHA),
            patch("tasks.static_quality_gates.github.GithubAPI", new=FakeGithubAPI),
            patch("tasks.static_quality_gates.gates.GateMetricHandler.generate_relative_size"),
            patch(
                "tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports"
            ) as mock_generate_reports,
            patch("tasks.static_quality_gates.gates.send_metrics") as mock_send_metrics,
            patch("tasks.static_quality_gates.pr_comment.pr_commenter") as mock_pr_commenter,
        ):
            ctx = MockContext(
                run={"datadog-ci tag --level job --tags static_quality_gates:\"success\"": Result("Done")}
            )
            parse_and_trigger_gates(ctx, config_path)
            mock_send_metrics.assert_called_once()
            mock_generate_reports.assert_called_once()
            mock_pr_commenter.assert_called_once()

    @patch.dict('os.environ', _PR_ENV_VARS, clear=True)
    def test_gate_fails_per_pr_disk_threshold(self):
        """Gate passes absolute limits but on-disk delta exceeds per-PR threshold."""
        gate_scenarios = [
            GateScenario(
                name=_DEB_GATE,
                ancestor_disk=100 * _MiB,
                ancestor_wire=50 * _MiB,
                current_disk=100 * _MiB + 700 * _KiB,  # > PER_PR_THRESHOLD (600 KiB)
                current_wire=50 * _MiB,
                max_disk=105 * _MiB,
                max_wire=55 * _MiB,
            ),
            GateScenario(
                name=_DOCKER_GATE,
                ancestor_disk=600 * _MiB,
                ancestor_wire=240 * _MiB,
                current_disk=600 * _MiB + 20 * _KiB,
                current_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
            ),
        ]
        with (
            _gate_scenarios_fixture(*gate_scenarios, ancestor_sha=_ANCESTOR_SHA) as config_path,
            patch("tasks.quality_gates.get_ancestor", return_value=_ANCESTOR_SHA),
            patch("tasks.quality_gates.get_commit_sha", return_value=_CI_COMMIT_SHA),
            patch("tasks.static_quality_gates.github.GithubAPI", new=FakeGithubAPI),
            patch("tasks.static_quality_gates.gates.send_metrics"),
            patch("tasks.static_quality_gates.pr_comment.pr_commenter") as mock_pr_commenter,
            patch(
                "tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports"
            ) as mock_generate_reports,
        ):
            ctx = MockContext(
                run={"datadog-ci tag --level job --tags static_quality_gates:\"failure\"": Result("Done")}
            )
            with self.assertRaises(Exit):
                parse_and_trigger_gates(ctx, config_path)
            mock_generate_reports.assert_called_once()
            mock_pr_commenter.assert_called_once()
            self.assertIn("per-PR threshold", mock_pr_commenter.call_args.kwargs["body"])

    @patch.dict('os.environ', _PR_ENV_VARS, clear=True)
    def test_gate_fails_per_pr_disk_threshold_exception_approval(self):
        """Gate exceeds per-PR threshold but an exception approver has approved, making it pass"""
        approved_pr = SimpleNamespace(
            number=12345,
            title="fix(somewhere): did something",
            user=SimpleNamespace(login="some-author"),
            get_reviews=lambda: [SimpleNamespace(state="APPROVED", user=SimpleNamespace(login="cmourot"))],
        )
        gate_scenarios = [
            GateScenario(
                name=_DEB_GATE,
                ancestor_disk=100 * _MiB,
                ancestor_wire=50 * _MiB,
                current_disk=100 * _MiB + 700 * _KiB,
                current_wire=50 * _MiB,
                max_disk=105 * _MiB,
                max_wire=55 * _MiB,
            ),
            GateScenario(
                name=_DOCKER_GATE,
                ancestor_disk=600 * _MiB,
                ancestor_wire=240 * _MiB,
                current_disk=600 * _MiB + 20 * _KiB,
                current_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
            ),
        ]
        with (
            _gate_scenarios_fixture(*gate_scenarios, ancestor_sha=_ANCESTOR_SHA) as config_path,
            patch("tasks.quality_gates.get_ancestor", return_value=_ANCESTOR_SHA),
            patch("tasks.quality_gates.get_commit_sha", return_value=_CI_COMMIT_SHA),
            patch("tasks.quality_gates.get_pr_for_branch", return_value=approved_pr),
            patch("tasks.static_quality_gates.gates.send_metrics"),
            patch("tasks.static_quality_gates.pr_comment.pr_commenter") as mock_pr_commenter,
            patch(
                "tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports"
            ) as mock_generate_reports,
        ):
            ctx = MockContext(
                run={"datadog-ci tag --level job --tags static_quality_gates:\"failure\"": Result("Done")}
            )
            parse_and_trigger_gates(ctx, config_path)
            mock_generate_reports.assert_called_once()
            mock_pr_commenter.assert_called_once()
            self.assertIs(mock_pr_commenter.call_args.kwargs["pr"], approved_pr)
            body = mock_pr_commenter.call_args.kwargs["body"]
            self.assertIn("per-PR threshold", body)
            self.assertIn("Exception granted by @cmourot", body)

    @patch.dict('os.environ', _PR_ENV_VARS, clear=True)
    def test_gate_fails_per_pr_wire_threshold(self):
        """Gate passes absolute limits but on-wire delta exceeds per-PR threshold."""
        gate_scenarios = [
            GateScenario(
                name=_DEB_GATE,
                ancestor_disk=100 * _MiB,
                ancestor_wire=50 * _MiB,
                current_disk=100 * _MiB,
                current_wire=50 * _MiB + 5.1 * _MiB,  # > 5 MiB wire threshold
                max_disk=105 * _MiB,
                max_wire=60 * _MiB,
            ),
            GateScenario(
                name=_DOCKER_GATE,
                ancestor_disk=600 * _MiB,
                ancestor_wire=240 * _MiB,
                current_disk=600 * _MiB + 20 * _KiB,
                current_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
            ),
        ]
        with (
            _gate_scenarios_fixture(*gate_scenarios, ancestor_sha=_ANCESTOR_SHA) as config_path,
            patch("tasks.quality_gates.get_ancestor", return_value=_ANCESTOR_SHA),
            patch("tasks.quality_gates.get_commit_sha", return_value=_CI_COMMIT_SHA),
            patch("tasks.static_quality_gates.github.GithubAPI", new=FakeGithubAPI),
            patch("tasks.static_quality_gates.gates.send_metrics"),
            patch("tasks.static_quality_gates.pr_comment.pr_commenter") as mock_pr_commenter,
            patch(
                "tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports"
            ) as mock_generate_reports,
        ):
            ctx = MockContext(
                run={"datadog-ci tag --level job --tags static_quality_gates:\"failure\"": Result("Done")}
            )
            with self.assertRaises(Exit):
                parse_and_trigger_gates(ctx, config_path)
            mock_generate_reports.assert_called_once()
            mock_pr_commenter.assert_called_once()
            self.assertIn("per-PR wire threshold", mock_pr_commenter.call_args.kwargs["body"])

    @patch.dict('os.environ', _PR_ENV_VARS, clear=True)
    def test_gate_fails_per_pr_wire_threshold_exception_approval(self):
        """Gate exceeds per-PR wire threshold but an exception approver has approved, making it pass."""
        approved_pr = SimpleNamespace(
            number=12345,
            title="fix(somewhere): did something",
            user=SimpleNamespace(login="some-author"),
            get_reviews=lambda: [SimpleNamespace(state="APPROVED", user=SimpleNamespace(login="cmourot"))],
        )
        gate_scenarios = [
            GateScenario(
                name=_DEB_GATE,
                ancestor_disk=100 * _MiB,
                ancestor_wire=50 * _MiB,
                current_disk=100 * _MiB,
                current_wire=50 * _MiB + 5.1 * _MiB,  # > 5 MiB wire threshold
                max_disk=105 * _MiB,
                max_wire=60 * _MiB,
            ),
            GateScenario(
                name=_DOCKER_GATE,
                ancestor_disk=600 * _MiB,
                ancestor_wire=240 * _MiB,
                current_disk=600 * _MiB + 20 * _KiB,
                current_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
            ),
        ]
        with (
            _gate_scenarios_fixture(*gate_scenarios, ancestor_sha=_ANCESTOR_SHA) as config_path,
            patch("tasks.quality_gates.get_ancestor", return_value=_ANCESTOR_SHA),
            patch("tasks.quality_gates.get_commit_sha", return_value=_CI_COMMIT_SHA),
            patch("tasks.quality_gates.get_pr_for_branch", return_value=approved_pr),
            patch("tasks.static_quality_gates.gates.send_metrics"),
            patch("tasks.static_quality_gates.pr_comment.pr_commenter") as mock_pr_commenter,
            patch(
                "tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports"
            ) as mock_generate_reports,
        ):
            ctx = MockContext(
                run={"datadog-ci tag --level job --tags static_quality_gates:\"failure\"": Result("Done")}
            )
            parse_and_trigger_gates(ctx, config_path)
            mock_generate_reports.assert_called_once()
            mock_pr_commenter.assert_called_once()
            self.assertIs(mock_pr_commenter.call_args.kwargs["pr"], approved_pr)
            body = mock_pr_commenter.call_args.kwargs["body"]
            self.assertIn("per-PR wire threshold", body)
            self.assertIn("Exception granted by @cmourot", body)

    @patch.dict('os.environ', _PR_ENV_VARS, clear=True)
    def test_gate_fails_absolute_limit_non_blocking_unchanged_from_ancestor(self):
        """Gate exceeds absolute disk limit but size is unchanged from ancestor — failure is non-blocking."""
        gate_scenarios = [
            GateScenario(
                name=_DEB_GATE,
                ancestor_disk=110 * _MiB,  # ancestor was already over the 105 MiB limit
                ancestor_wire=50 * _MiB,
                current_disk=110 * _MiB,  # no increase from ancestor (delta = 0)
                current_wire=50 * _MiB,
                max_disk=105 * _MiB,
                max_wire=55 * _MiB,
            ),
            GateScenario(
                name=_DOCKER_GATE,
                ancestor_disk=600 * _MiB,
                ancestor_wire=240 * _MiB,
                current_disk=600 * _MiB + 20 * _KiB,
                current_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
            ),
        ]
        with (
            _gate_scenarios_fixture(*gate_scenarios, ancestor_sha=_ANCESTOR_SHA) as config_path,
            patch("tasks.quality_gates.get_ancestor", return_value=_ANCESTOR_SHA),
            patch("tasks.quality_gates.get_commit_sha", return_value=_CI_COMMIT_SHA),
            patch("tasks.static_quality_gates.github.GithubAPI", new=FakeGithubAPI),
            patch("tasks.static_quality_gates.gates.send_metrics"),
            patch("tasks.static_quality_gates.pr_comment.pr_commenter") as mock_pr_commenter,
            patch(
                "tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports"
            ) as mock_generate_reports,
        ):
            ctx = MockContext(
                run={"datadog-ci tag --level job --tags static_quality_gates:\"failure\"": Result("Done")}
            )
            # Should NOT raise Exit — failure is non-blocking because size is unchanged from ancestor
            parse_and_trigger_gates(ctx, config_path)
            mock_generate_reports.assert_called_once()
            mock_pr_commenter.assert_called_once()

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
            'BUCKET_BRANCH': 'main',
            'CI_COMMIT_BRANCH': 'mq-working-branch-12345',
            'CI_COMMIT_REF_SLUG': 'mq-working-branch-12345',
            'CI_PIPELINE_ID': _CI_PIPELINE_ID,
            'CI_COMMIT_SHA': _CI_COMMIT_SHA,
        },
        clear=True,
    )
    def test_per_pr_threshold_skipped_on_merge_queue(self):
        """Per-PR threshold check is not applied on merge queue branches."""
        gate_scenarios = [
            GateScenario(
                name=_DEB_GATE,
                ancestor_disk=100 * _MiB,
                ancestor_wire=50 * _MiB,
                current_disk=100 * _MiB + 700 * _KiB,  # delta > PER_PR_THRESHOLD, within absolute limit
                current_wire=50 * _MiB,
                max_disk=105 * _MiB,
                max_wire=55 * _MiB,
            ),
            GateScenario(
                name=_DOCKER_GATE,
                ancestor_disk=600 * _MiB,
                ancestor_wire=240 * _MiB,
                current_disk=600 * _MiB + 20 * _KiB,
                current_wire=240 * _MiB,
                max_disk=700 * _MiB,
                max_wire=250 * _MiB,
            ),
        ]
        with (
            _gate_scenarios_fixture(*gate_scenarios, ancestor_sha=_ANCESTOR_SHA) as config_path,
            patch("tasks.quality_gates.get_ancestor", return_value=_ANCESTOR_SHA),
            patch("tasks.quality_gates.get_commit_sha", return_value=_CI_COMMIT_SHA),
            patch("tasks.quality_gates.get_pr_for_branch", return_value=None),
            patch("tasks.quality_gates.get_pr_number_from_commit", return_value=None),
            patch("tasks.static_quality_gates.gates.send_metrics"),
            patch("tasks.static_quality_gates.gates.GateMetricHandler.generate_metric_reports"),
        ):
            ctx = MockContext(
                run={"datadog-ci tag --level job --tags static_quality_gates:\"success\"": Result("Done")}
            )
            # Should NOT raise Exit — per-PR threshold is skipped on merge queue branches
            parse_and_trigger_gates(ctx, config_path)


if __name__ == '__main__':
    unittest.main()
