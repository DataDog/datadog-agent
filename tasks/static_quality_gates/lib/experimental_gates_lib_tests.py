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
@patch("glob.glob", new=MagicMock(return_value=["/opt/datadog-agent/datadog-agent_7*amd64.deb"]))
@patch(
    "tasks.static_quality_gates.lib.experimental_gates_lib.StaticQualityGatePackage._calculate_package_size",
    new=MagicMock(return_value=(100, 350)),
)
@patch(
    "tasks.static_quality_gates.lib.experimental_gates_lib.StaticQualityGateDocker._calculate_image_on_wire_size",
    new=MagicMock(return_value=(100, 350)),
)
@patch(
    "tasks.static_quality_gates.lib.experimental_gates_lib.StaticQualityGateDocker._calculate_image_on_disk_size",
    new=MagicMock(return_value=(100, 350)),
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
        self.assertEqual(gate.arch, "armv7hl")
        with self.assertRaises(ValueError):
            gate = StaticQualityGatePackage(
                "static_quality_gate_agent_deb_unknown", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
            )

    def test_set_os(self):
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "debian")
        self.assertEqual(gate.arch, "amd64")
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_rpm_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "centos")
        self.assertEqual(gate.arch, "x86_64")
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_suse_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "suse")

        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_heroku_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate.os, "debian")
        self.assertEqual(gate.arch, "amd64")
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
            ("static_quality_gate_iot_agent_rpm_arm64", "/opt/datadog-agent/datadog-iot-agent-7*aarch64.rpm"),
            ("static_quality_gate_dogstatsd_suse_amd64", "/opt/datadog-agent/datadog-dogstatsd-7*x86_64.rpm"),
            ("static_quality_gate_agent_heroku_amd64", "/opt/datadog-agent/datadog-agent_7*amd64.deb"),
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
        gate.check_artifact_size()

        # Test case where quality gate fails - sizes exceed maximum allowed
        gate = StaticQualityGatePackage(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 350}, MagicMock()
        )
        gate.artifact_on_wire_size = 150  # Exceeds max_on_wire_size of 100
        gate.artifact_on_disk_size = 400  # Exceeds max_on_disk_size of 350

        with self.assertRaises(StaticQualityGateFailed) as context:
            gate.check_artifact_size()

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
        # Test basic agent image
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-amd64")

        # Test Windows 2022 servercore
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_2022_servercore_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(),
            "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-winltsc2022-servercore-amd64",
        )

        # Test Windows 1809 servercore arm64
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_1809_servercore_arm64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(),
            "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-win1809-servercore-arm64",
        )

        # Test nightly builds
        with patch.dict('os.environ', {"BUCKET_BRANCH": "nightly"}):
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_agent_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            self.assertEqual(
                gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent-nightly:v71580015-668844-7-amd64"
            )

        # Test cluster-agent flavor
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_cluster_amd64", {"max_on_wire_size": 100, "max_on_disk_size": 100}, MagicMock()
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/cluster-agent:v71580015-668844-amd64"
        )

        # Test dogstatsd flavor
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_dogstatsd_arm64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/dogstatsd:v71580015-668844-arm64")

        # Test cws_instrumentation flavor
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_cws_instrumentation_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/cws-instrumentation:v71580015-668844-amd64"
        )

        # Test JMX images
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_jmx_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-jmx-amd64"
        )

        # Test Windows
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_2022_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-winltsc2022-amd64"
        )

        # Test Windows 1809
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_windows_1809_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(), "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-win1809-amd64"
        )

        # Test JMX + Windows combination
        gate = StaticQualityGateDocker(
            "static_quality_gate_docker_agent_jmx_windows_2022_servercore_amd64",
            {"max_on_wire_size": 100, "max_on_disk_size": 100},
            MagicMock(),
        )
        self.assertEqual(
            gate._get_image_url(),
            "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-jmx-winltsc2022-servercore-amd64",
        )

        # Test nightly with other flavors
        with patch.dict('os.environ', {"BUCKET_BRANCH": "nightly"}):
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_cluster_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            self.assertEqual(
                gate._get_image_url(),
                "registry.ddbuild.io/ci/datadog-agent/cluster-agent-nightly:v71580015-668844-amd64",
            )

    def test_get_image_url_error_cases(self):
        # Test unknown flavor
        with self.assertRaises(ValueError) as context:
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_unknown_flavor_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            gate._get_image_url()
        self.assertIn("Unknown docker image flavor for gate", str(context.exception))

        # Test missing CI_PIPELINE_ID
        with patch.dict('os.environ', {"CI_PIPELINE_ID": ""}):
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_agent_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            with self.assertRaises(StaticQualityGateFailed) as context:
                gate._get_image_url()
            self.assertIn("Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA", str(context.exception))

        # Test missing CI_COMMIT_SHORT_SHA
        with patch.dict('os.environ', {"CI_COMMIT_SHORT_SHA": ""}):
            gate = StaticQualityGateDocker(
                "static_quality_gate_docker_agent_amd64",
                {"max_on_wire_size": 100, "max_on_disk_size": 100},
                MagicMock(),
            )
            with self.assertRaises(StaticQualityGateFailed) as context:
                gate._get_image_url()
            self.assertIn("Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA", str(context.exception))
