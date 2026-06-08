import unittest

from tasks.static_quality_gates.metrics import GateMetricsData
from tasks.static_quality_gates.thresholds import (
    SIZE_INCREASE_THRESHOLD_BYTES,
    generate_new_quality_gate_config,
    identify_failing_gates,
    identify_gates_with_size_increase,
)


class MockMetricHandler:
    """Mock metric handler for testing quality gate configuration updates."""

    def __init__(self, metrics):
        self.metrics = metrics
        self.total_size_saved = 0


class TestQualityGatesConfigUpdate(unittest.TestCase):
    def test_one_gate_update(self):
        with open("tasks/unit_tests/testdata/quality_gate_config_test.yml") as f:
            new_config, saved_amount = generate_new_quality_gate_config(
                f,
                MockMetricHandler(
                    {
                        "static_quality_gate_agent_suse_amd64": {
                            "current_on_wire_size": 50000000,
                            "max_on_wire_size": 100000000,
                            "current_on_disk_size": 50000000,
                            "max_on_disk_size": 100000000,
                        },
                        "static_quality_gate_agent_deb_amd64": {
                            "current_on_wire_size": 4000000,
                            "max_on_wire_size": 5000000,
                            "current_on_disk_size": 4000000,
                            "max_on_disk_size": 5000000,
                        },
                        "static_quality_gate_docker_agent_amd64": {
                            "current_on_wire_size": 50000000,
                            "max_on_wire_size": 100000000,
                            "current_on_disk_size": 50000000,
                            "max_on_disk_size": 100000000,
                        },
                    }
                ),
            )
        self.assertEqual(
            new_config["static_quality_gate_agent_suse_amd64"]["max_on_wire_size"],
            "48.64 MiB",
            f"Expected 48.64 MiB got {new_config['static_quality_gate_agent_suse_amd64']['max_on_wire_size']}",
        )
        self.assertEqual(
            new_config["static_quality_gate_agent_suse_amd64"]["max_on_disk_size"],
            "48.64 MiB",
            f"Expected 48.64 MiB got {new_config['static_quality_gate_agent_suse_amd64']['max_on_disk_size']}",
        )
        self.assertEqual(
            new_config["static_quality_gate_agent_deb_amd64"]["max_on_wire_size"],
            "4.77 MiB",
            f"Expected 4.77 MiB got {new_config['static_quality_gate_agent_deb_amd64']['max_on_wire_size']}",
        )
        self.assertEqual(
            new_config["static_quality_gate_agent_deb_amd64"]["max_on_disk_size"],
            "4.77 MiB",
            f"Expected 4.77 MiB got {new_config['static_quality_gate_agent_deb_amd64']['max_on_disk_size']}",
        )


class TestIdentifyFailingGates(unittest.TestCase):
    """Test the identify_failing_gates function."""

    def test_identifies_disk_failure(self):
        """Should identify gate failing on disk size."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,  # 200 MiB
                max_on_disk_size=150 * 1024 * 1024,  # 150 MiB limit
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
            )
        }
        failing = identify_failing_gates(pr_metrics)
        self.assertEqual(len(failing), 1)
        self.assertIn("static_quality_gate_agent_deb_amd64", failing)

    def test_identifies_wire_failure(self):
        """Should identify gate failing on wire size."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=100 * 1024 * 1024,
                max_on_disk_size=150 * 1024 * 1024,
                current_on_wire_size=120 * 1024 * 1024,  # 120 MiB
                max_on_wire_size=100 * 1024 * 1024,  # 100 MiB limit
            )
        }
        failing = identify_failing_gates(pr_metrics)
        self.assertEqual(len(failing), 1)
        self.assertIn("static_quality_gate_agent_deb_amd64", failing)

    def test_identifies_both_failures(self):
        """Should identify gate failing on both disk and wire size."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,
                max_on_disk_size=150 * 1024 * 1024,
                current_on_wire_size=120 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
            )
        }
        failing = identify_failing_gates(pr_metrics)
        self.assertEqual(len(failing), 1)

    def test_excludes_passing_gates(self):
        """Should not include gates that are passing."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=100 * 1024 * 1024,
                max_on_disk_size=150 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
            )
        }
        failing = identify_failing_gates(pr_metrics)
        self.assertEqual(len(failing), 0)

    def test_handles_missing_values(self):
        """Should handle gates with missing metric values."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=None,
                max_on_disk_size=150 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
            )
        }
        failing = identify_failing_gates(pr_metrics)
        self.assertEqual(len(failing), 0)

    def test_multiple_gates_mixed(self):
        """Should correctly identify failing gates among multiple."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,  # Failing
                max_on_disk_size=150 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
            ),
            "static_quality_gate_docker_agent_amd64": GateMetricsData(
                current_on_disk_size=100 * 1024 * 1024,  # Passing
                max_on_disk_size=150 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
            ),
            "static_quality_gate_agent_rpm_amd64": GateMetricsData(
                current_on_disk_size=160 * 1024 * 1024,  # Failing
                max_on_disk_size=150 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
            ),
        }
        failing = identify_failing_gates(pr_metrics)
        self.assertEqual(len(failing), 2)
        self.assertIn("static_quality_gate_agent_deb_amd64", failing)
        self.assertIn("static_quality_gate_agent_rpm_amd64", failing)
        self.assertNotIn("static_quality_gate_docker_agent_amd64", failing)


class TestIdentifyGatesWithSizeIncrease(unittest.TestCase):
    """Test the identify_gates_with_size_increase function."""

    def test_identifies_gate_with_size_increase(self):
        """Should identify gate with positive relative_on_disk_size above threshold."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,
                max_on_disk_size=250 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
                relative_on_disk_size=5 * 1024 * 1024,  # +5 MiB (above threshold)
                relative_on_wire_size=1 * 1024 * 1024,
            )
        }
        gates_to_bump = identify_gates_with_size_increase(pr_metrics)
        self.assertEqual(len(gates_to_bump), 1)
        self.assertIn("static_quality_gate_agent_deb_amd64", gates_to_bump)

    def test_excludes_gate_with_no_size_increase(self):
        """Should exclude gate with zero or negative relative_on_disk_size."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,
                max_on_disk_size=250 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
                relative_on_disk_size=0,  # No change
                relative_on_wire_size=0,
            )
        }
        gates_to_bump = identify_gates_with_size_increase(pr_metrics)
        self.assertEqual(len(gates_to_bump), 0)

    def test_excludes_gate_with_size_decrease(self):
        """Should exclude gate with negative relative_on_disk_size (size decreased)."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,
                max_on_disk_size=250 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
                relative_on_disk_size=-5 * 1024 * 1024,  # -5 MiB (decreased)
                relative_on_wire_size=-1 * 1024 * 1024,
            )
        }
        gates_to_bump = identify_gates_with_size_increase(pr_metrics)
        self.assertEqual(len(gates_to_bump), 0)

    def test_excludes_gate_with_size_below_threshold(self):
        """Should exclude gate with size increase below 2 KiB threshold."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,
                max_on_disk_size=250 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
                relative_on_disk_size=1024,  # +1 KiB (below 2 KiB threshold)
                relative_on_wire_size=512,
            )
        }
        gates_to_bump = identify_gates_with_size_increase(pr_metrics)
        self.assertEqual(len(gates_to_bump), 0)

    def test_includes_gate_with_size_at_threshold(self):
        """Should include gate with size increase exactly at threshold."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,
                max_on_disk_size=250 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
                relative_on_disk_size=SIZE_INCREASE_THRESHOLD_BYTES + 1,  # Just above threshold
                relative_on_wire_size=0,
            )
        }
        gates_to_bump = identify_gates_with_size_increase(pr_metrics)
        self.assertEqual(len(gates_to_bump), 1)

    def test_handles_missing_relative_size(self):
        """Should exclude gate when relative_on_disk_size is None."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,
                max_on_disk_size=250 * 1024 * 1024,
                current_on_wire_size=50 * 1024 * 1024,
                max_on_wire_size=100 * 1024 * 1024,
                relative_on_disk_size=None,  # Missing
                relative_on_wire_size=None,
            )
        }
        gates_to_bump = identify_gates_with_size_increase(pr_metrics)
        self.assertEqual(len(gates_to_bump), 0)

    def test_multiple_gates_mixed(self):
        """Should correctly identify gates with size increase among multiple."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=200 * 1024 * 1024,
                max_on_disk_size=250 * 1024 * 1024,
                relative_on_disk_size=10 * 1024 * 1024,  # +10 MiB (include)
            ),
            "static_quality_gate_docker_agent_amd64": GateMetricsData(
                current_on_disk_size=100 * 1024 * 1024,
                max_on_disk_size=150 * 1024 * 1024,
                relative_on_disk_size=0,  # No change (exclude)
            ),
            "static_quality_gate_agent_rpm_amd64": GateMetricsData(
                current_on_disk_size=160 * 1024 * 1024,
                max_on_disk_size=150 * 1024 * 1024,  # Failing but no increase
                relative_on_disk_size=-5 * 1024 * 1024,  # Size decreased (exclude)
            ),
            "static_quality_gate_agent_suse_amd64": GateMetricsData(
                current_on_disk_size=180 * 1024 * 1024,
                max_on_disk_size=200 * 1024 * 1024,
                relative_on_disk_size=3 * 1024 * 1024,  # +3 MiB (include)
            ),
        }
        gates_to_bump = identify_gates_with_size_increase(pr_metrics)
        self.assertEqual(len(gates_to_bump), 2)
        self.assertIn("static_quality_gate_agent_deb_amd64", gates_to_bump)
        self.assertIn("static_quality_gate_agent_suse_amd64", gates_to_bump)
        self.assertNotIn("static_quality_gate_docker_agent_amd64", gates_to_bump)
        self.assertNotIn("static_quality_gate_agent_rpm_amd64", gates_to_bump)

    def test_includes_non_failing_gate_with_increase(self):
        """Should include gate with size increase even if not failing (current < max)."""
        pr_metrics = {
            "static_quality_gate_agent_deb_amd64": GateMetricsData(
                current_on_disk_size=100 * 1024 * 1024,  # 100 MiB (not failing)
                max_on_disk_size=150 * 1024 * 1024,  # 150 MiB limit
                relative_on_disk_size=5 * 1024 * 1024,  # +5 MiB increase
            )
        }
        gates_to_bump = identify_gates_with_size_increase(pr_metrics)
        self.assertEqual(len(gates_to_bump), 1)
        self.assertIn("static_quality_gate_agent_deb_amd64", gates_to_bump)


if __name__ == '__main__':
    unittest.main()
