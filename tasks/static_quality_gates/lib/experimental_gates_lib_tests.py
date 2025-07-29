import unittest
from unittest.mock import MagicMock, patch


@patch.dict(
    'os.environ',
    {
        'CI_COMMIT_REF_NAME': 'pikachu',
        'CI_COMMIT_BRANCH': 'sequoia',
        'CI_COMMIT_REF_SLUG': 'pikachu',
        'BUCKET_BRANCH': 'main',
        'OMNIBUS_PACKAGE_DIR': '/opt/datadog-agent',
    },
)
class TestExperimentalGatesLib(unittest.TestCase):
    @patch("tasks.static_quality_gates.lib.experimental_gates_lib.GateMetricHandler", new=MagicMock())
    def test_get_quality_gates_list(self):
        from tasks.static_quality_gates.lib.experimental_gates_lib import get_quality_gates_list

        gates = get_quality_gates_list("tasks/static_quality_gates/lib/static_quality_gates_test.yaml", MagicMock())
        assert len(gates) == 21

    def test_set_arch(self):
        from tasks.static_quality_gates.lib.experimental_gates_lib import StaticQualityGate

        gate = StaticQualityGate(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.arch, "amd64")
        gate = StaticQualityGate(
            "static_quality_gate_agent_deb_arm64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.arch, "arm64")
        gate = StaticQualityGate(
            "static_quality_gate_iot_agent_rpm_armhf", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.arch, "armhf")
        with self.assertRaises(ValueError):
            gate = StaticQualityGate(
                "static_quality_gate_agent_deb_unknown", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
            )

    def test_set_os(self):
        from tasks.static_quality_gates.lib.experimental_gates_lib import StaticQualityGate

        gate = StaticQualityGate(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "debian")
        gate = StaticQualityGate(
            "static_quality_gate_agent_rpm_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "centos")
        gate = StaticQualityGate(
            "static_quality_gate_agent_suse_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "suse")
        with self.assertRaises(ValueError):
            gate = StaticQualityGate(
                "static_quality_gate_agent_unknown_unknown",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )

    def test_max_sizes(self):
        from tasks.static_quality_gates.lib.experimental_gates_lib import StaticQualityGate

        gate = StaticQualityGate(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )
        self.assertEqual(gate.max_on_wire_size, 100)
        self.assertEqual(gate.max_on_disk_size, 350)

    def test_find_package_path(self):
        from tasks.static_quality_gates.lib.experimental_gates_lib import StaticQualityGate

        test_cases = [
            ("static_quality_gate_agent_deb_amd64", "/opt/datadog-agent/datadog-agent_7*amd64.deb"),
            ("static_quality_gate_agent_deb_amd64_fips", "/opt/datadog-agent/datadog-fips-agent_7*amd64.deb"),
            ("static_quality_gate_iot_agent_rpm_arm64", "/opt/datadog-agent/datadog-iot-agent-7*arm64.rpm"),
            ("static_quality_gate_dogstatsd_suse_amd64", "/opt/datadog-agent/datadog-dogstatsd-7*amd64.rpm"),
        ]

        for gate_name, expected_pattern in test_cases:
            with self.subTest(gate_name=gate_name):
                mock_glob = MagicMock(return_value=["/opt/datadog-agent/some_package.ext"])

                with patch("glob.glob", mock_glob):
                    gate = StaticQualityGate(gate_name, {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock())
                    gate._find_package_path()

                actual_pattern = mock_glob.call_args[0][0]
                self.assertEqual(actual_pattern, expected_pattern)

    def test_check_package_size(self):
        from tasks.static_quality_gates.lib.experimental_gates_lib import StaticQualityGate

        gate = StaticQualityGate(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )
        gate.artifact_on_wire_size = 100
        gate.artifact_on_disk_size = 350
        gate._check_package_size()
