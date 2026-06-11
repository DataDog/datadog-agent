import unittest
from unittest.mock import patch

from tasks.static_quality_gates.metrics import (
    GateMetricsData,
    _extract_gate_name_from_scope,
    _get_latest_value_from_pointlist,
    fetch_main_headroom,
    fetch_pr_metrics,
)


class MockPoint:
    """Mock Point object matching datadog_api_client.v1.model.point.Point structure."""

    def __init__(self, timestamp, metric_value):
        self.value = [timestamp, metric_value]


def make_pointlist(points: list) -> list:
    """Convert [[timestamp, value], ...] to [MockPoint, ...] for tests."""
    return [MockPoint(p[0], p[1]) for p in points]


class TestExceptionThresholdBumpHelpers(unittest.TestCase):
    """Test helper functions for the exception_threshold_bump implementation."""

    def test_extract_gate_name_from_scope_valid(self):
        """Should extract gate name from scope string."""
        scope = "gate_name:static_quality_gate_agent_deb_amd64,pr_number:12345"
        result = _extract_gate_name_from_scope(scope)
        self.assertEqual(result, "static_quality_gate_agent_deb_amd64")

    def test_extract_gate_name_from_scope_single_tag(self):
        """Should extract gate name when it's the only tag."""
        scope = "gate_name:static_quality_gate_docker_agent_arm64"
        result = _extract_gate_name_from_scope(scope)
        self.assertEqual(result, "static_quality_gate_docker_agent_arm64")

    def test_extract_gate_name_from_scope_missing(self):
        """Should return None when gate_name is not in scope."""
        scope = "pr_number:12345,arch:amd64"
        result = _extract_gate_name_from_scope(scope)
        self.assertIsNone(result)

    def test_extract_gate_name_from_scope_empty(self):
        """Should return None for empty scope."""
        result = _extract_gate_name_from_scope("")
        self.assertIsNone(result)

    def test_get_latest_value_from_pointlist_valid(self):
        """Should get the latest non-null value from pointlist."""
        pointlist = make_pointlist([[1704067200, 100.0], [1704153600, 150.0], [1704240000, 200.0]])
        result = _get_latest_value_from_pointlist(pointlist)
        self.assertEqual(result, 200.0)

    def test_get_latest_value_from_pointlist_with_nulls(self):
        """Should skip null values and get the latest non-null value."""
        pointlist = make_pointlist([[1704067200, 100.0], [1704153600, 150.0], [1704240000, None]])
        result = _get_latest_value_from_pointlist(pointlist)
        self.assertEqual(result, 150.0)

    def test_get_latest_value_from_pointlist_all_nulls(self):
        """Should return None if all values are null."""
        pointlist = make_pointlist([[1704067200, None], [1704153600, None]])
        result = _get_latest_value_from_pointlist(pointlist)
        self.assertIsNone(result)

    def test_get_latest_value_from_pointlist_empty(self):
        """Should return None for empty pointlist."""
        result = _get_latest_value_from_pointlist([])
        self.assertIsNone(result)


class TestFetchPrMetrics(unittest.TestCase):
    """Test the fetch_pr_metrics function."""

    @patch("tasks.static_quality_gates.metrics.query_metrics")
    def test_fetches_and_parses_metrics(self, mock_query):
        """Should fetch metrics and parse them correctly with single API call."""
        # Single API call returns all 4 metrics
        mock_query.return_value = [
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 100 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 50 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.max_allowed_on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 150 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.max_allowed_on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 75 * 1024 * 1024]]),
            },
        ]

        result = fetch_pr_metrics(12345)

        # Should make exactly 1 API call
        mock_query.assert_called_once()
        self.assertEqual(len(result), 1)
        self.assertIn("static_quality_gate_agent_deb_amd64", result)
        gate = result["static_quality_gate_agent_deb_amd64"]
        self.assertEqual(gate.current_on_disk_size, 100 * 1024 * 1024)
        self.assertEqual(gate.current_on_wire_size, 50 * 1024 * 1024)
        self.assertEqual(gate.max_on_disk_size, 150 * 1024 * 1024)
        self.assertEqual(gate.max_on_wire_size, 75 * 1024 * 1024)

    @patch("tasks.static_quality_gates.metrics.query_metrics")
    def test_returns_empty_when_no_metrics(self, mock_query):
        """Should return empty dict when no metrics found."""
        mock_query.return_value = []

        result = fetch_pr_metrics(12345)

        self.assertEqual(len(result), 0)

    @patch("tasks.static_quality_gates.metrics.query_metrics")
    def test_handles_multiple_gates(self, mock_query):
        """Should handle metrics for multiple gates in single API call."""
        # Single API call returns metrics for multiple gates
        mock_query.return_value = [
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 100 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_docker_agent_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 200 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 50 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_docker_agent_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 80 * 1024 * 1024]]),
            },
        ]

        result = fetch_pr_metrics(12345)

        # Should make exactly 1 API call
        mock_query.assert_called_once()
        self.assertEqual(len(result), 2)
        self.assertIn("static_quality_gate_agent_deb_amd64", result)
        self.assertIn("static_quality_gate_docker_agent_amd64", result)

    @patch("tasks.static_quality_gates.metrics.query_metrics")
    def test_fetches_relative_size_metrics(self, mock_query):
        """Should fetch and parse relative_on_disk_size and relative_on_wire_size."""
        # API call includes all 6 metrics including relative sizes
        mock_query.return_value = [
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 100 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 50 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.max_allowed_on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 150 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.max_allowed_on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 75 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.relative_on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 5 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.relative_on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 2 * 1024 * 1024]]),
            },
        ]

        result = fetch_pr_metrics(12345)

        self.assertEqual(len(result), 1)
        gate = result["static_quality_gate_agent_deb_amd64"]
        self.assertEqual(gate.current_on_disk_size, 100 * 1024 * 1024)
        self.assertEqual(gate.current_on_wire_size, 50 * 1024 * 1024)
        self.assertEqual(gate.max_on_disk_size, 150 * 1024 * 1024)
        self.assertEqual(gate.max_on_wire_size, 75 * 1024 * 1024)
        self.assertEqual(gate.relative_on_disk_size, 5 * 1024 * 1024)
        self.assertEqual(gate.relative_on_wire_size, 2 * 1024 * 1024)


class TestFetchMainHeadroom(unittest.TestCase):
    """Test the fetch_main_headroom function."""

    @patch("tasks.static_quality_gates.metrics.query_metrics")
    def test_calculates_headroom_correctly(self, mock_query):
        """Should calculate headroom as max - current."""
        # Single API call returns all 4 metrics for the gate
        mock_query.return_value = [
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 100 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 50 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.max_allowed_on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 150 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.max_allowed_on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 75 * 1024 * 1024]]),
            },
        ]

        result = fetch_main_headroom(["static_quality_gate_agent_deb_amd64"])

        self.assertEqual(len(result), 1)
        self.assertIn("static_quality_gate_agent_deb_amd64", result)
        headroom = result["static_quality_gate_agent_deb_amd64"]
        # disk_headroom = 150 - 100 = 50 MiB
        self.assertEqual(headroom["disk_headroom"], 50 * 1024 * 1024)
        # wire_headroom = 75 - 50 = 25 MiB
        self.assertEqual(headroom["wire_headroom"], 25 * 1024 * 1024)

    @patch("tasks.static_quality_gates.metrics.query_metrics")
    def test_headroom_never_negative(self, mock_query):
        """Headroom should never be negative (clamped to 0)."""
        # Single API call with current > max
        mock_query.return_value = [
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 200 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 100 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.max_allowed_on_disk_size{...}",
                "pointlist": make_pointlist([[1704240000, 150 * 1024 * 1024]]),
            },
            {
                "scope": "gate_name:static_quality_gate_agent_deb_amd64",
                "expression": "avg:datadog.agent.static_quality_gate.max_allowed_on_wire_size{...}",
                "pointlist": make_pointlist([[1704240000, 75 * 1024 * 1024]]),
            },
        ]

        result = fetch_main_headroom(["static_quality_gate_agent_deb_amd64"])

        headroom = result["static_quality_gate_agent_deb_amd64"]
        # disk_headroom = max(0, 150 - 200) = 0
        self.assertEqual(headroom["disk_headroom"], 0)

    def test_returns_empty_for_no_gates(self):
        """Should return empty dict when no gates provided."""
        result = fetch_main_headroom([])
        self.assertEqual(result, {})


class TestGateMetricsData(unittest.TestCase):
    """Test the GateMetricsData dataclass."""

    def test_default_values(self):
        """Should have None as default for all fields."""
        metrics = GateMetricsData()
        self.assertIsNone(metrics.current_on_disk_size)
        self.assertIsNone(metrics.current_on_wire_size)
        self.assertIsNone(metrics.max_on_disk_size)
        self.assertIsNone(metrics.max_on_wire_size)
        self.assertIsNone(metrics.relative_on_disk_size)
        self.assertIsNone(metrics.relative_on_wire_size)

    def test_with_values(self):
        """Should store provided values."""
        metrics = GateMetricsData(
            current_on_disk_size=100,
            current_on_wire_size=50,
            max_on_disk_size=150,
            max_on_wire_size=75,
            relative_on_disk_size=10,
            relative_on_wire_size=5,
        )
        self.assertEqual(metrics.current_on_disk_size, 100)
        self.assertEqual(metrics.current_on_wire_size, 50)
        self.assertEqual(metrics.max_on_disk_size, 150)
        self.assertEqual(metrics.max_on_wire_size, 75)
        self.assertEqual(metrics.relative_on_disk_size, 10)
        self.assertEqual(metrics.relative_on_wire_size, 5)


if __name__ == '__main__':
    unittest.main()
