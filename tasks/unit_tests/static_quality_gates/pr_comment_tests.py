import unittest
from unittest.mock import MagicMock, patch

from tasks.static_quality_gates.decisions import GateEvaluationResult, GateFailureKind, GateVerdict
from tasks.static_quality_gates.gates import GateMetricHandler
from tasks.static_quality_gates.pr_comment import (
    display_pr_comment,
    get_change_metrics,
)


class TestGetChangeMetrics(unittest.TestCase):
    """Test the get_change_metrics function for change calculations."""

    def test_normal_positive_delta(self):
        """Should calculate change for a positive delta (size increased)."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "current_on_disk_size": 165 * 1024 * 1024,  # 165 MiB
            "max_on_disk_size": 200 * 1024 * 1024,  # 200 MiB
            "relative_on_disk_size": 15 * 1024 * 1024,  # +15 MiB delta
        }
        change_str, limit_bounds, is_neutral = get_change_metrics("test_gate", handler)

        # Baseline = 165 - 15 = 150 MiB
        # Change shows delta with percentage increase
        self.assertIn("+15.0 MiB", change_str)
        self.assertIn("increase", change_str)
        self.assertFalse(is_neutral)
        # Limit bounds: baseline → current (bold) → limit
        self.assertIn("150.000", limit_bounds)
        self.assertIn("**165.000**", limit_bounds)
        self.assertIn("200.000", limit_bounds)

    def test_negative_delta_reduction(self):
        """Should show reduction when size decreased (negative delta)."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "current_on_disk_size": 145 * 1024 * 1024,  # 145 MiB
            "max_on_disk_size": 200 * 1024 * 1024,  # 200 MiB
            "relative_on_disk_size": -5 * 1024 * 1024,  # -5 MiB delta (reduction)
        }
        change_str, limit_bounds, is_neutral = get_change_metrics("test_gate", handler)

        self.assertIn("reduction", change_str)
        self.assertIn("-5.0 MiB", change_str)
        self.assertFalse(is_neutral)
        # Baseline = 145 - (-5) = 150 MiB
        self.assertIn("150.000", limit_bounds)
        self.assertIn("**145.000**", limit_bounds)

    def test_zero_delta_neutral(self):
        """Should show neutral when delta is zero."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "current_on_disk_size": 150 * 1024 * 1024,  # 150 MiB
            "max_on_disk_size": 200 * 1024 * 1024,  # 200 MiB
            "relative_on_disk_size": 0,  # No change
        }
        change_str, limit_bounds, is_neutral = get_change_metrics("test_gate", handler)

        self.assertEqual("neutral", change_str)
        self.assertTrue(is_neutral)
        # Neutral shows current size and upper bound
        self.assertEqual("**150.000** MiB → 200.000", limit_bounds)

    def test_small_delta_below_threshold_neutral(self):
        """Should show neutral when delta is below 2 KiB threshold."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "current_on_disk_size": 150 * 1024 * 1024,  # 150 MiB
            "max_on_disk_size": 200 * 1024 * 1024,  # 200 MiB
            "relative_on_disk_size": 1 * 1024,  # +1 KiB delta (below 2 KiB threshold)
        }
        change_str, limit_bounds, is_neutral = get_change_metrics("test_gate", handler)

        self.assertEqual("neutral", change_str)
        self.assertTrue(is_neutral)
        # Neutral shows current size and upper bound
        self.assertEqual("**150.000** MiB → 200.000", limit_bounds)

    def test_small_delta_kib_above_threshold(self):
        """Should show delta in KiB for changes above threshold."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "current_on_disk_size": 707163 * 1024,  # ~707 MiB
            "max_on_disk_size": 708000 * 1024,  # ~708 MiB
            "relative_on_disk_size": 98 * 1024,  # +98 KiB delta (above 2 KiB threshold)
        }
        change_str, limit_bounds, is_neutral = get_change_metrics("test_gate", handler)

        self.assertIn("+98.0 KiB", change_str)
        self.assertIn("increase", change_str)
        self.assertFalse(is_neutral)
        # Should have arrows for non-neutral changes
        self.assertIn("→", limit_bounds)
        self.assertIn("**", limit_bounds)

    def test_missing_current_size(self):
        """Should return N/A when current size is missing."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "max_on_disk_size": 200 * 1024 * 1024,
        }
        change_str, limit_bounds, is_neutral = get_change_metrics("test_gate", handler)

        self.assertEqual("N/A", change_str)
        self.assertEqual("N/A", limit_bounds)
        self.assertFalse(is_neutral)

    def test_missing_relative_size(self):
        """Should handle missing relative size (no ancestor data)."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "current_on_disk_size": 165 * 1024 * 1024,
            "max_on_disk_size": 200 * 1024 * 1024,
            # No relative_on_disk_size
        }
        change_str, limit_bounds, is_neutral = get_change_metrics("test_gate", handler)

        self.assertEqual("N/A", change_str)
        self.assertFalse(is_neutral)
        # Limit bounds should show N/A for baseline but current (bold) and limit should be present
        self.assertIn("N/A", limit_bounds)
        self.assertIn("**165.000**", limit_bounds)
        self.assertIn("200.000", limit_bounds)

    def test_missing_gate(self):
        """Should return N/A when gate is not found."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics = {}

        change_str, limit_bounds, is_neutral = get_change_metrics("nonexistent_gate", handler)

        self.assertEqual("N/A", change_str)
        self.assertEqual("N/A", limit_bounds)
        self.assertFalse(is_neutral)


class TestGetWireChangeMetrics(unittest.TestCase):
    """Test the get_change_metrics function for on-wire size calculations (metric_type='wire')."""

    def test_normal_positive_delta(self):
        """Should calculate change for a positive delta (size increased)."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "current_on_wire_size": 100 * 1024 * 1024,  # 100 MiB
            "max_on_wire_size": 150 * 1024 * 1024,  # 150 MiB
            "relative_on_wire_size": 10 * 1024 * 1024,  # +10 MiB delta
        }
        change_str, limit_bounds, is_neutral = get_change_metrics("test_gate", handler, metric_type="wire")

        self.assertIn("+10.0 MiB", change_str)
        self.assertIn("increase", change_str)
        self.assertFalse(is_neutral)
        # Limit bounds: baseline → current (bold) → limit
        self.assertIn("90.000", limit_bounds)
        self.assertIn("**100.000**", limit_bounds)
        self.assertIn("150.000", limit_bounds)

    def test_neutral_change(self):
        """Should show neutral when delta is below threshold."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "current_on_wire_size": 100 * 1024 * 1024,  # 100 MiB
            "max_on_wire_size": 150 * 1024 * 1024,  # 150 MiB
            "relative_on_wire_size": 500,  # 500 bytes (below 2 KiB threshold)
        }
        change_str, limit_bounds, is_neutral = get_change_metrics("test_gate", handler, metric_type="wire")

        self.assertEqual("neutral", change_str)
        self.assertTrue(is_neutral)
        # Neutral shows current size and upper bound
        self.assertEqual("**100.000** MiB → 150.000", limit_bounds)

    def test_missing_gate(self):
        """Should return N/A when gate is not found."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics = {}

        change_str, limit_bounds, is_neutral = get_change_metrics("nonexistent_gate", handler, metric_type="wire")

        self.assertEqual("N/A", change_str)
        self.assertEqual("N/A", limit_bounds)
        self.assertFalse(is_neutral)


class TestQualityGatesPrMessage(unittest.TestCase):
    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_no_error_with_significant_changes(self, pr_commenter_mock):
        """Test PR comment with successful gates that have significant changes (>= 2 KiB)."""
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        # Add metrics with significant changes
        gate_metric_handler.metrics["gateA"] = {
            "current_on_disk_size": 100 * 1024 * 1024,
            "max_on_disk_size": 150 * 1024 * 1024,
            "relative_on_disk_size": 5 * 1024 * 1024,  # +5 MiB (significant)
            "current_on_wire_size": 50 * 1024 * 1024,
            "max_on_wire_size": 75 * 1024 * 1024,
            "relative_on_wire_size": 2 * 1024 * 1024,
        }
        gate_metric_handler.metrics["gateB"] = {
            "current_on_disk_size": 200 * 1024 * 1024,
            "max_on_disk_size": 250 * 1024 * 1024,
            "relative_on_disk_size": 10 * 1024 * 1024,  # +10 MiB (significant)
            "current_on_wire_size": 100 * 1024 * 1024,
            "max_on_wire_size": 125 * 1024 * 1024,
            "relative_on_wire_size": 5 * 1024 * 1024,
        }
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[GateVerdict(name='gateA', failure=None), GateVerdict(name='gateB', failure=None)]
            ),
            gate_metric_handler,
            "value",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Check that the table format is present with new header
        self.assertIn('Change', body)
        self.assertIn('Size (prev', body)
        self.assertIn('Successful checks', body)
        self.assertIn('gateA', body)
        self.assertIn('gateB', body)
        # Check dashboard link is present
        self.assertIn('Static Quality Gates Dashboard', body)
        # Check PR was passed to pr_commenter
        self.assertEqual(call_args[1]['pr'], mock_pr)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_neutral_changes_collapsed(self, pr_commenter_mock):
        """Test that gates with neutral changes (< 2 KiB) are collapsed."""
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        # Add metrics with neutral changes (below threshold)
        gate_metric_handler.metrics["gateA"] = {
            "current_on_disk_size": 100 * 1024 * 1024,
            "max_on_disk_size": 150 * 1024 * 1024,
            "relative_on_disk_size": 500,  # 500 bytes (neutral)
            "current_on_wire_size": 50 * 1024 * 1024,
            "max_on_wire_size": 75 * 1024 * 1024,
            "relative_on_wire_size": 200,
        }
        gate_metric_handler.metrics["gateB"] = {
            "current_on_disk_size": 200 * 1024 * 1024,
            "max_on_disk_size": 250 * 1024 * 1024,
            "relative_on_disk_size": 1000,  # 1KB (neutral)
            "current_on_wire_size": 100 * 1024 * 1024,
            "max_on_wire_size": 125 * 1024 * 1024,
            "relative_on_wire_size": 500,
        }
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[GateVerdict(name='gateA', failure=None), GateVerdict(name='gateB', failure=None)]
            ),
            gate_metric_handler,
            "value",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Check that collapsed section is present
        self.assertIn('successful checks with minimal change', body)
        self.assertIn('2 KiB', body)
        # Check that gates are in the collapsed section
        self.assertIn('gateA', body)
        self.assertIn('gateB', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_mixed_significant_and_neutral(self, pr_commenter_mock):
        """Test PR comment with both significant and neutral changes."""
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        # Gate A with significant change
        gate_metric_handler.metrics["gateA"] = {
            "current_on_disk_size": 100 * 1024 * 1024,
            "max_on_disk_size": 150 * 1024 * 1024,
            "relative_on_disk_size": 5 * 1024 * 1024,  # +5 MiB (significant)
            "current_on_wire_size": 50 * 1024 * 1024,
            "max_on_wire_size": 75 * 1024 * 1024,
            "relative_on_wire_size": 2 * 1024 * 1024,
        }
        # Gate B with neutral change
        gate_metric_handler.metrics["gateB"] = {
            "current_on_disk_size": 200 * 1024 * 1024,
            "max_on_disk_size": 250 * 1024 * 1024,
            "relative_on_disk_size": 500,  # 500 bytes (neutral)
            "current_on_wire_size": 100 * 1024 * 1024,
            "max_on_wire_size": 125 * 1024 * 1024,
            "relative_on_wire_size": 200,
        }
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[GateVerdict(name='gateA', failure=None), GateVerdict(name='gateB', failure=None)]
            ),
            gate_metric_handler,
            "value",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Check both sections are present
        self.assertIn('Successful checks', body)
        self.assertIn('successful checks with minimal change', body)
        self.assertIn('gateA', body)
        self.assertIn('gateB', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_no_info(self, pr_commenter_mock):
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[
                    GateVerdict(
                        name='gateA', failure=GateFailureKind.AbsoluteLimitExceeded, blocking=True, message='some_msg_A'
                    ),
                    GateVerdict(
                        name='gateB', failure=GateFailureKind.AbsoluteLimitExceeded, blocking=True, message='some_msg_B'
                    ),
                ],
                has_blocking_failures=True,
            ),
            gate_metric_handler,
            "value",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        # Check that the new table format is present in error section
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        self.assertIn('### Error', body)
        self.assertIn('Change', body)
        self.assertIn('Size (prev', body)
        self.assertIn('gateA', body)
        self.assertIn('gateB', body)
        self.assertIn('Gate failure full details', body)
        self.assertIn('Static quality gates prevent the PR to merge!', body)
        # Check dashboard link is present
        self.assertIn('Static Quality Gates Dashboard', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_one_of_each(self, pr_commenter_mock):
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        # Add significant change to gateB so it appears in expanded section
        gate_metric_handler.metrics["gateB"] = {
            "current_on_disk_size": 200 * 1024 * 1024,
            "max_on_disk_size": 250 * 1024 * 1024,
            "relative_on_disk_size": 5 * 1024 * 1024,  # +5 MiB
            "current_on_wire_size": 100 * 1024 * 1024,
            "max_on_wire_size": 125 * 1024 * 1024,
            "relative_on_wire_size": 2 * 1024 * 1024,
        }
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[
                    GateVerdict(
                        name='gateA', failure=GateFailureKind.AbsoluteLimitExceeded, blocking=True, message='some_msg_A'
                    ),
                    GateVerdict(name='gateB', failure=None),
                ],
                has_blocking_failures=True,
            ),
            gate_metric_handler,
            "value",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        # Check that both error and success sections are present
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        self.assertIn('### Error', body)
        self.assertIn('Successful checks', body)
        self.assertIn('gateA', body)
        self.assertIn('gateB', body)
        # Check new columns are present
        self.assertIn('Change', body)
        self.assertIn('Size (prev', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_missing_data(self, pr_commenter_mock):
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[
                    GateVerdict(
                        name='gateA', failure=GateFailureKind.AbsoluteLimitExceeded, blocking=True, message='some_msg_A'
                    )
                ],
                has_blocking_failures=True,
            ),
            gate_metric_handler,
            "value",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        # Check that N/A appears when metrics are missing
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        self.assertIn('N/A', body)


class TestNonBlockingPrComment(unittest.TestCase):
    """Test PR comment display for non-blocking failures."""

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_non_blocking_failure_shows_warning_indicator(self, pr_commenter_mock):
        """Non-blocking failures should show warning indicator and per-verdict note in the footer."""
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[
                    GateVerdict(
                        name='gateA',
                        failure=GateFailureKind.AbsoluteLimitExceeded,
                        blocking=False,
                        message='size exceeded',
                        blocking_note='non-blocking: size unchanged from ancestor',
                    ),
                ],
            ),
            gate_metric_handler,
            "ancestor123",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Should show warning indicator for non-blocking failure
        self.assertIn('⚠️', body)
        # Should NOT contain the blocking failure message
        self.assertNotIn('prevent the PR to merge', body)
        # Footer should carry the per-verdict blocking note
        self.assertIn('AbsoluteLimitExceeded (non-blocking: size unchanged from ancestor)', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_blocking_failure_shows_error_indicator(self, pr_commenter_mock):
        """Blocking failures should show error indicator and blocking message."""
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[
                    GateVerdict(
                        name='gateA',
                        failure=GateFailureKind.AbsoluteLimitExceeded,
                        blocking=True,
                        message='size exceeded',
                    ),
                ],
                has_blocking_failures=True,
            ),
            gate_metric_handler,
            "ancestor123",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Should show error indicator for blocking failure
        self.assertIn('❌', body)
        # Should contain the blocking failure message
        self.assertIn('prevent the PR to merge', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_mixed_blocking_and_non_blocking(self, pr_commenter_mock):
        """Mixed blocking and non-blocking failures should show both indicators."""
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[
                    GateVerdict(
                        name='gateA',
                        failure=GateFailureKind.AbsoluteLimitExceeded,
                        blocking=True,
                        message='size exceeded',
                    ),
                    GateVerdict(
                        name='gateB',
                        failure=GateFailureKind.AbsoluteLimitExceeded,
                        blocking=False,
                        message='size exceeded',
                    ),
                ],
                has_blocking_failures=True,
            ),
            gate_metric_handler,
            "ancestor123",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Should show both indicators
        self.assertIn('❌', body)
        self.assertIn('⚠️', body)
        # Should contain the blocking failure message (since there's a blocking failure)
        self.assertIn('prevent the PR to merge', body)


class TestExceptionBanner(unittest.TestCase):
    """Test that exception_note from GateEvaluationResult renders as the banner."""

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_exception_note_renders_as_banner(self, pr_commenter_mock):
        """exception_note on GateEvaluationResult should appear prefixed with the warning emoji."""
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[
                    GateVerdict(
                        name='gateA',
                        failure=GateFailureKind.PerPRThresholdExceeded,
                        blocking=False,
                        message='size exceeded',
                        blocking_note='non-blocking: exception granted by @granter',
                    ),
                ],
                exception_note='**Exception granted by @granter**: this PR exceeds the per-PR size threshold (600.0 KiB) but will not be blocked.\n',
            ),
            gate_metric_handler,
            "ancestor123",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        body = pr_commenter_mock.call_args[1]['body']
        self.assertIn('⚠️ **Exception granted by @granter**', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.static_quality_gates.pr_comment.pr_commenter")
    def test_no_exception_note_no_banner(self, pr_commenter_mock):
        """When exception_note is None, no exception banner should appear."""
        from invoke import MockContext

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        mock_pr = MagicMock()
        mock_pr.number = 12345
        display_pr_comment(
            c,
            GateEvaluationResult(
                verdicts=[GateVerdict(name='gateA', failure=None)],
            ),
            gate_metric_handler,
            "ancestor123",
            mock_pr,
        )
        pr_commenter_mock.assert_called_once()
        body = pr_commenter_mock.call_args[1]['body']
        self.assertNotIn('Exception granted', body)


if __name__ == '__main__':
    unittest.main()
