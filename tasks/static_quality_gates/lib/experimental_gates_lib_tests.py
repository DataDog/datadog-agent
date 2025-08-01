import unittest
from unittest.mock import MagicMock, patch

from tasks.static_quality_gates.lib.experimental_gates_lib import (
    StaticQualityGateDocker,
    StaticQualityGateFailed,
    StaticQualityGatePackage,
)


@patch.dict(
    'os.environ',
    {
        'CI_COMMIT_REF_NAME': 'pikachu',
        'CI_COMMIT_BRANCH': 'sequoia',
        'CI_COMMIT_REF_SLUG': 'pikachu',
        'CI_COMMIT_SHORT_SHA': '668844',
        'BUCKET_BRANCH': 'main',
        'OMNIBUS_PACKAGE_DIR': '/opt/datadog-agent',
        'CI_PIPELINE_ID': '71580015',
    },
)
class TestExperimentalGatesLib(unittest.TestCase):
    @patch("tasks.static_quality_gates.lib.experimental_gates_lib.GateMetricHandler", new=MagicMock())
    def test_get_quality_gates_list(self):
        from tasks.static_quality_gates.lib.experimental_gates_lib import get_quality_gates_list

        gates = get_quality_gates_list("tasks/static_quality_gates/lib/static_quality_gates_test.yaml", MagicMock())
        assert len(gates) == 21

    def test_set_arch(self):
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.arch, "amd64")
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_arm64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.arch, "arm64")
        gate = StaticQualityGatePackage(
            "static_quality_gate_iot_agent_rpm_armhf", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.arch, "armhf")
        with self.assertRaises(ValueError):
            gate = StaticQualityGatePackage(
                "static_quality_gate_agent_deb_unknown", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
            )

    def test_set_os(self):
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "debian")
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_rpm_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "centos")
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_suse_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "suse")
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "docker")
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_2022_servercore_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(gate.os, "docker")
        with self.assertRaises(ValueError):
            gate = StaticQualityGatePackage(
                "static_quality_gate_agent_unknown_unknown",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )

    def test_max_sizes(self):
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )
        self.assertEqual(gate.max_on_wire_size, 100)
        self.assertEqual(gate.max_on_disk_size, 350)

        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": 125, "max_on_disk_size": 250}, MagicMock()
        )
        self.assertEqual(gate.max_on_wire_size, 125)
        self.assertEqual(gate.max_on_disk_size, 250)

    def test_find_package_path(self):
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
                    gate = StaticQualityGatePackage(
                        gate_name, {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
                    )
                    gate._find_package_path()

                actual_pattern = mock_glob.call_args[0][0]
                self.assertEqual(actual_pattern, expected_pattern)

    def test_check_artifact_size(self):
        # Test case where quality gate passes - sizes are within limits
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )
        gate.artifact_on_wire_size = 100
        gate.artifact_on_disk_size = 350
        gate._check_artifact_size()

        # Test case where quality gate fails - sizes exceed maximum allowed
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )
        gate.artifact_on_wire_size = 150  # Exceeds max_on_wire_size of 100
        gate.artifact_on_disk_size = 400  # Exceeds max_on_disk_size of 350

        with self.assertRaises(StaticQualityGateFailed) as context:
            gate._check_artifact_size()

        # Verify the exception message contains information about both failures
        error_message = str(context.exception)
        self.assertIn(
            "On wire size (compressed artifact size) 150 is higher than the maximum allowed 100 by the gate !",
            error_message,
        )
        self.assertIn(
            "On disk size (uncompressed artifact size) 400 is higher than the maximum allowed 350 by the gate !",
            error_message,
        )

    def test_get_image_url(self):
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-amd64")
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_2022_servercore_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(),
            "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-winltsc2022-servercore-amd64",
        )
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_1809_servercore_arm64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(),
            "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-win1809-servercore-arm64",
        )
        with patch.dict('os.environ', {"BUCKET_BRANCH": "nightly"}):
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_agent_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            self.assertEqual(
                gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent-nightly:v71580015-668844-7-amd64"
            )
