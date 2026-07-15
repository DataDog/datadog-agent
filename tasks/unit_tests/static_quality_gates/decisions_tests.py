import unittest
from unittest.mock import MagicMock, patch

from tasks.static_quality_gates.decisions import (
    EXCEPTION_APPROVERS,
    ExceptionApprovalChecker,
    should_bypass_failure,
)
from tasks.static_quality_gates.gates import GateMetricHandler


class TestShouldBypassFailure(unittest.TestCase):
    """Test the should_bypass_failure function for delta-based non-blocking failures."""

    def test_bypass_when_disk_delta_zero(self):
        """Should bypass when on-disk size delta is exactly 0."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": 0,
        }
        self.assertTrue(should_bypass_failure("test_gate", handler))

    def test_bypass_when_disk_delta_negative(self):
        """Should bypass when on-disk size delta is negative (size decreased)."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": -500000,  # -500KB
        }
        self.assertTrue(should_bypass_failure("test_gate", handler))

    def test_bypass_when_disk_delta_within_threshold(self):
        """Should bypass when on-disk size delta is positive but within threshold (~2KiB)."""
        handler = GateMetricHandler("main", "dev")
        # Small positive delta (1KB) should be treated as 0
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": 1024,  # 1KB - within 2KiB threshold
        }
        self.assertTrue(should_bypass_failure("test_gate", handler))

    def test_bypass_ignores_wire_delta(self):
        """Should bypass based only on disk delta, ignoring wire delta."""
        handler = GateMetricHandler("main", "dev")
        # Even with positive wire delta, should bypass if disk delta is within threshold
        handler.metrics["test_gate"] = {
            "relative_on_wire_size": 1000000,  # Positive wire delta (1MB)
            "relative_on_disk_size": 0,  # Zero disk delta
        }
        self.assertTrue(should_bypass_failure("test_gate", handler))

    def test_no_bypass_when_disk_delta_exceeds_threshold(self):
        """Should NOT bypass when on-disk size delta exceeds threshold."""
        handler = GateMetricHandler("main", "dev")
        # Delta of 5KB exceeds threshold of 2KiB
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": 5000,  # 5KB - exceeds 2KiB threshold
        }
        self.assertFalse(should_bypass_failure("test_gate", handler))

    def test_no_bypass_when_disk_delta_significantly_positive(self):
        """Should NOT bypass when on-disk size delta is significantly positive."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": 1000000,  # 1MB - way over threshold
        }
        self.assertFalse(should_bypass_failure("test_gate", handler))

    def test_no_bypass_when_missing_disk_delta(self):
        """Should NOT bypass when disk delta is missing."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "relative_on_wire_size": 0,
        }
        self.assertFalse(should_bypass_failure("test_gate", handler))

    def test_no_bypass_when_gate_not_found(self):
        """Should NOT bypass when gate doesn't exist in metrics."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics = {}
        self.assertFalse(should_bypass_failure("nonexistent_gate", handler))


class TestBypassOnlyAppliesToPRs(unittest.TestCase):
    """
    Test that the bypass tolerance (delta <= 2KiB) only applies to PRs, not to main branch.

    On main branch, all gate failures should be blocking unconditionally.
    On PRs, failures with delta <= 2KiB threshold can be marked non-blocking.

    Note: The actual integration of this behavior is in parse_and_trigger_gates()
    where the bypass loop is wrapped with `if not is_on_main_branch:`.
    These tests document the expected behavior at the integration level.
    """

    def test_main_branch_detection_logic(self):
        """
        Document: On main branch, ancestor == current_commit, so is_on_main_branch = True.

        When is_on_main_branch is True, the bypass loop in parse_and_trigger_gates
        is skipped entirely, meaning all failures remain blocking regardless of delta.
        """
        # This test documents the detection logic:
        # ancestor = get_common_ancestor(ctx, "HEAD", base_branch)
        # is_on_main_branch = ancestor == current_commit
        # On main, merge-base of HEAD and origin/main is HEAD itself

        # Simulate: on main branch, ancestor equals current commit
        ancestor = "abc123"
        current_commit = "abc123"
        is_on_main_branch = ancestor == current_commit
        self.assertTrue(is_on_main_branch)

    def test_pr_branch_detection_logic(self):
        """
        Document: On PR branches, ancestor != current_commit, so is_on_main_branch = False.

        When is_on_main_branch is False, the bypass loop runs and failures with
        delta <= 2KiB threshold can be marked non-blocking.
        """
        # Simulate: on PR branch, ancestor is different from current commit
        ancestor = "abc123"  # Common ancestor with main
        current_commit = "def456"  # PR's HEAD
        is_on_main_branch = ancestor == current_commit
        self.assertFalse(is_on_main_branch)

    def test_bypass_logic_skipped_on_main_conceptually(self):
        """
        Document: The bypass logic should NOT run on main branch.

        This ensures that even if delta <= 2KiB, failures on main remain blocking.
        The actual implementation wraps the bypass loop with:
        `if not is_on_main_branch:`
        """
        handler = GateMetricHandler("main", "dev")
        # Even with zero delta (which would normally allow bypass)
        handler.metrics["test_gate"] = {"relative_on_disk_size": 0}

        # The function itself still returns True for eligible bypass
        self.assertTrue(should_bypass_failure("test_gate", handler))

        # But on main branch (is_on_main_branch=True), the calling code
        # in parse_and_trigger_gates skips the bypass loop entirely,
        # so gate_state["blocking"] stays True regardless


class TestBlockingFailureDetection(unittest.TestCase):
    """Test the blocking failure detection logic."""

    def test_has_blocking_failures_true(self):
        """Should detect blocking failures."""
        gate_states = [
            {'name': 'gateA', 'state': False, 'blocking': True},
            {'name': 'gateB', 'state': True, 'blocking': True},
        ]
        has_blocking = any(gs["state"] is False and gs.get("blocking", True) for gs in gate_states)
        self.assertTrue(has_blocking)

    def test_has_blocking_failures_false_all_non_blocking(self):
        """Should not detect blocking failures when all failures are non-blocking."""
        gate_states = [
            {'name': 'gateA', 'state': False, 'blocking': False},
            {'name': 'gateB', 'state': True, 'blocking': True},
        ]
        has_blocking = any(gs["state"] is False and gs.get("blocking", True) for gs in gate_states)
        self.assertFalse(has_blocking)

    def test_has_blocking_failures_default_blocking_true(self):
        """Should default to blocking=True when field is missing."""
        gate_states = [
            {'name': 'gateA', 'state': False},  # No blocking field
        ]
        has_blocking = any(gs["state"] is False and gs.get("blocking", True) for gs in gate_states)
        self.assertTrue(has_blocking)

    def test_has_blocking_failures_false_all_success(self):
        """Should not detect blocking failures when all gates succeeded."""
        gate_states = [
            {'name': 'gateA', 'state': True, 'blocking': True},
            {'name': 'gateB', 'state': True, 'blocking': True},
        ]
        has_blocking = any(gs["state"] is False and gs.get("blocking", True) for gs in gate_states)
        self.assertFalse(has_blocking)

    def test_multiple_non_blocking_failures(self):
        """Should not detect blocking failures when multiple failures are all non-blocking."""
        gate_states = [
            {'name': 'gateA', 'state': False, 'blocking': False},
            {'name': 'gateB', 'state': False, 'blocking': False},
            {'name': 'gateC', 'state': True, 'blocking': True},
        ]
        has_blocking = any(gs["state"] is False and gs.get("blocking", True) for gs in gate_states)
        self.assertFalse(has_blocking)


class TestExceptionApprovalChecker(unittest.TestCase):
    """Test ExceptionApprovalChecker lazy fetch and caching behaviour."""

    def test_returns_approver_login_when_authorized(self):
        """Returns the login of the first authorized approver."""
        mock_review = MagicMock()
        mock_review.state = "APPROVED"
        mock_review.user.login = "cmourot"
        mock_pr = MagicMock()
        mock_pr.get_reviews.return_value = [mock_review]

        self.assertEqual(ExceptionApprovalChecker(mock_pr).get(), "cmourot")

    def test_returns_none_when_no_authorized_approver(self):
        """Returns None when approvals exist but none from authorized reviewers."""
        mock_review = MagicMock()
        mock_review.state = "APPROVED"
        mock_review.user.login = "someone_else"
        mock_pr = MagicMock()
        mock_pr.get_reviews.return_value = [mock_review]

        self.assertIsNone(ExceptionApprovalChecker(mock_pr).get())

    def test_returns_none_when_authorized_reviewer_did_not_approve(self):
        """Returns None when an authorized reviewer left a non-approval review."""
        mock_review = MagicMock()
        mock_review.state = "CHANGES_REQUESTED"
        mock_review.user.login = "cmourot"
        mock_pr = MagicMock()
        mock_pr.get_reviews.return_value = [mock_review]

        self.assertIsNone(ExceptionApprovalChecker(mock_pr).get())

    def test_returns_none_when_pr_is_none(self):
        """Returns None when no PR object is provided."""
        self.assertIsNone(ExceptionApprovalChecker(None).get())

    def test_fetches_reviews_only_once(self):
        """get_reviews is called exactly once regardless of how many times get() is called."""
        mock_review = MagicMock()
        mock_review.state = "APPROVED"
        mock_review.user.login = "cmourot"
        mock_pr = MagicMock()
        mock_pr.get_reviews.return_value = [mock_review]

        checker = ExceptionApprovalChecker(mock_pr)
        checker.get()
        checker.get()
        checker.get()

        mock_pr.get_reviews.assert_called_once()

    def test_returns_none_on_github_api_error(self):
        """Returns None and does not raise when get_reviews() throws."""
        mock_pr = MagicMock()
        mock_pr.get_reviews.side_effect = Exception("API error")

        with patch('builtins.print'):
            self.assertIsNone(ExceptionApprovalChecker(mock_pr).get())

    def test_accepts_any_authorized_approver(self):
        """Both authorized approvers (cmourot and dd-ddamien) grant an exception."""
        for approver in EXCEPTION_APPROVERS:
            with self.subTest(approver=approver):
                mock_review = MagicMock()
                mock_review.state = "APPROVED"
                mock_review.user.login = approver
                mock_pr = MagicMock()
                mock_pr.get_reviews.return_value = [mock_review]

                self.assertEqual(ExceptionApprovalChecker(mock_pr).get(), approver)


if __name__ == '__main__':
    unittest.main()
