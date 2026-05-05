import os
import tempfile
import unittest
from unittest.mock import MagicMock, mock_open, patch

from invoke import MockContext, Result

from tasks.static_quality_gates.gates import (
    ArtifactMeasurement,
    DockerArtifactMeasurer,
    GateMetricHandler,
    GateResult,
    PackageArtifactMeasurer,
    QualityGateConfig,
    QualityGateFactory,
    SizeViolation,
    StaticQualityGate,
    StaticQualityGateError,
    _extract_arch_from_gate_name,
    _extract_os_from_gate_name,
    byte_to_string,
    create_quality_gate_config,
    read_byte_input,
    string_to_byte,
)


class TestDataClasses(unittest.TestCase):
    def test_artifact_measurement_validation(self):
        # Valid measurement should work
        measurement = ArtifactMeasurement(artifact_path="/path/to/artifact", on_wire_size=1000, on_disk_size=2000)
        self.assertEqual(measurement.artifact_path, "/path/to/artifact")

        # Empty path should fail
        with self.assertRaises(ValueError):
            ArtifactMeasurement(artifact_path="", on_wire_size=1000, on_disk_size=2000)

        # Negative sizes should fail
        with self.assertRaises(ValueError):
            ArtifactMeasurement(artifact_path="/path", on_wire_size=-1, on_disk_size=2000)

        with self.assertRaises(ValueError):
            ArtifactMeasurement(artifact_path="/path", on_wire_size=1000, on_disk_size=-1)

    def test_quality_gate_config_validation(self):
        # Valid config should work
        config = QualityGateConfig(
            gate_name="test_gate", max_on_wire_size=1000, max_on_disk_size=2000, arch="amd64", os="debian"
        )
        self.assertEqual(config.gate_name, "test_gate")

        # Empty gate name should fail
        with self.assertRaises(ValueError):
            QualityGateConfig("", 1000, 2000, "amd64", "debian")

        # Non-positive sizes should fail
        with self.assertRaises(ValueError):
            QualityGateConfig("test", 0, 2000, "amd64", "debian")

        with self.assertRaises(ValueError):
            QualityGateConfig("test", 1000, -1, "amd64", "debian")

    def test_gate_result(self):
        config = QualityGateConfig("test", 1000, 2000, "amd64", "debian")
        measurement = ArtifactMeasurement("/path", 800, 1500)
        violations = []

        result = GateResult(config, measurement, violations, True)

        # Test remaining capacity calculations
        self.assertEqual(result.wire_remaining_bytes, 200)  # 1000 - 800
        self.assertEqual(result.disk_remaining_bytes, 500)  # 2000 - 1500

    def test_size_violation(self):
        violation = SizeViolation("wire", 1200, 1000)

        self.assertEqual(violation.excess_bytes, 200)
        self.assertAlmostEqual(violation.excess_mb, 200 / (1024 * 1024), places=6)


class TestUtilityFunctions(unittest.TestCase):
    """Test utility functions for size conversion and validation"""

    def test_byte_to_string_basic(self):
        """Test basic byte_to_string functionality"""
        # Test zero
        self.assertEqual(byte_to_string(0), "0 B")
        self.assertEqual(byte_to_string(0, with_unit=False), "0")

        # Test bytes
        self.assertEqual(byte_to_string(500), "500.0 B")
        self.assertEqual(byte_to_string(1023), "1023.0 B")

        # Test KiB
        self.assertEqual(byte_to_string(1024), "1.0 KiB")
        self.assertEqual(byte_to_string(1536), "1.5 KiB")

        # Test MiB
        self.assertEqual(byte_to_string(1024 * 1024), "1.0 MiB")
        self.assertEqual(byte_to_string(int(1024 * 1024 * 1.5)), "1.5 MiB")

        # Test GiB
        self.assertEqual(byte_to_string(1024 * 1024 * 1024), "1.0 GiB")

    def test_byte_to_string_negative(self):
        """Test byte_to_string with negative values"""
        self.assertEqual(byte_to_string(-1024), "-1.0 KiB")
        self.assertEqual(byte_to_string(-1536), "-1.5 KiB")

    def test_byte_to_string_unit_power(self):
        """Test byte_to_string with specific unit power"""
        # Force MiB unit (power 2)
        self.assertEqual(byte_to_string(1024, unit_power=2), "0 MiB")
        self.assertEqual(byte_to_string(1024 * 1024, unit_power=2), "1.0 MiB")

        # Force KiB unit (power 1)
        self.assertEqual(byte_to_string(1024 * 1024, unit_power=1), "1024.0 KiB")

    def test_byte_to_string_without_unit(self):
        """Test byte_to_string without unit suffix"""
        self.assertEqual(byte_to_string(1024, with_unit=False), "1.0")
        self.assertEqual(byte_to_string(1536, with_unit=False), "1.5")

    def test_string_to_byte_basic(self):
        """Test basic string_to_byte functionality"""
        # Test empty/None
        self.assertEqual(string_to_byte(""), 0)
        self.assertEqual(string_to_byte(None), 0)

        # Test bytes
        self.assertEqual(string_to_byte("500B"), 500)
        self.assertEqual(string_to_byte("1023B"), 1023)

        # Test KiB
        self.assertEqual(string_to_byte("1KiB"), 1024)
        self.assertEqual(string_to_byte("1.5KiB"), 1536)

        # Test MiB
        self.assertEqual(string_to_byte("1MiB"), 1024 * 1024)
        self.assertEqual(string_to_byte("2.5MiB"), int(2.5 * 1024 * 1024))

        # Test GiB
        self.assertEqual(string_to_byte("1GiB"), 1024 * 1024 * 1024)

    def test_string_to_byte_numeric_only(self):
        """Test string_to_byte with numeric-only strings"""
        self.assertEqual(string_to_byte("1024"), 1024)
        self.assertEqual(string_to_byte("500"), 500)

    def test_read_byte_input_string(self):
        """Test read_byte_input with string input"""
        self.assertEqual(read_byte_input("1MiB"), 1024 * 1024)
        self.assertEqual(read_byte_input("500B"), 500)

    def test_read_byte_input_numeric(self):
        """Test read_byte_input with numeric input"""
        self.assertEqual(read_byte_input(1024), 1024)
        self.assertEqual(read_byte_input(500), 500)


class TestArchitectureAndOSExtraction(unittest.TestCase):
    """Test extraction of architecture and OS from gate names"""

    def test_extract_arch_from_gate_name(self):
        """Test architecture extraction"""
        self.assertEqual(_extract_arch_from_gate_name("static_quality_gate_agent_deb_amd64"), "amd64")
        self.assertEqual(_extract_arch_from_gate_name("static_quality_gate_agent_deb_arm64"), "arm64")
        self.assertEqual(_extract_arch_from_gate_name("static_quality_gate_agent_deb_armhf"), "armhf")
        self.assertEqual(_extract_arch_from_gate_name("static_quality_gate_agent_msi"), "amd64")  # MSI always amd64

        with self.assertRaises(ValueError):
            _extract_arch_from_gate_name("static_quality_gate_unknown")

    def test_extract_os_from_gate_name(self):
        """Test OS extraction"""
        self.assertEqual(_extract_os_from_gate_name("static_quality_gate_docker_agent_amd64"), "docker")
        self.assertEqual(_extract_os_from_gate_name("static_quality_gate_agent_deb_amd64"), "debian")
        self.assertEqual(_extract_os_from_gate_name("static_quality_gate_agent_rpm_amd64"), "centos")
        self.assertEqual(_extract_os_from_gate_name("static_quality_gate_agent_suse_amd64"), "suse")
        self.assertEqual(_extract_os_from_gate_name("static_quality_gate_agent_msi"), "windows")

        with self.assertRaises(ValueError):
            _extract_os_from_gate_name("static_quality_gate_unknown")


class TestPackageArtifactMeasurer(unittest.TestCase):
    def setUp(self):
        self.measurer = PackageArtifactMeasurer()
        self.mock_ctx = MagicMock()

    def test_extract_package_flavor(self):
        self.assertEqual(self.measurer._extract_package_flavor("agent_deb_amd64"), "datadog-agent")
        self.assertEqual(self.measurer._extract_package_flavor("dogstatsd_deb_amd64"), "datadog-dogstatsd")
        self.assertEqual(self.measurer._extract_package_flavor("iot_agent_deb_amd64"), "datadog-iot-agent")
        self.assertEqual(self.measurer._extract_package_flavor("agent_deb_amd64_fips"), "datadog-fips-agent")
        self.assertEqual(self.measurer._extract_package_flavor("agent_heroku_amd64"), "datadog-heroku-agent")

    @patch.dict('os.environ', {'OMNIBUS_PACKAGE_DIR': '/test/pkg'})
    @patch('tasks.static_quality_gates.gates.glob.glob')
    def test_find_package_by_pattern_success(self, mock_glob):
        mock_glob.return_value = ['/test/pkg/datadog-agent_7.0.0-1_amd64.deb']

        config = QualityGateConfig("test", 1000, 2000, "amd64", "debian")
        result = self.measurer._find_package_by_pattern("datadog-agent", "deb", config, "_")

        self.assertEqual(result, '/test/pkg/datadog-agent_7.0.0-1_amd64.deb')
        mock_glob.assert_called_once_with('/test/pkg/datadog-agent_7*amd64.deb')

    @patch.dict('os.environ', {'OMNIBUS_PACKAGE_DIR': '/test/pkg'})
    @patch('tasks.static_quality_gates.gates.glob.glob')
    def test_find_package_by_pattern_multiple_files(self, mock_glob):
        mock_glob.return_value = [
            '/test/pkg/datadog-agent_7.0.0-1_amd64.deb',
            '/test/pkg/datadog-agent_7.0.1-1_amd64.deb',
        ]

        config = QualityGateConfig("test", 1000, 2000, "amd64", "debian")

        with self.assertRaises(ValueError) as cm:
            self.measurer._find_package_by_pattern("datadog-agent", "deb", config, "_")

        self.assertIn("Too many DEB files", str(cm.exception))

    @patch.dict('os.environ', {'OMNIBUS_PACKAGE_DIR': '/test/pkg'})
    @patch('tasks.static_quality_gates.gates.glob.glob')
    def test_find_package_by_pattern_no_files(self, mock_glob):
        mock_glob.return_value = []

        config = QualityGateConfig("test", 1000, 2000, "amd64", "debian")

        with self.assertRaises(ValueError) as cm:
            self.measurer._find_package_by_pattern("datadog-agent", "deb", config, "_")

        self.assertIn("Couldn't find any DEB file", str(cm.exception))

    @patch.dict('os.environ', {'OMNIBUS_PACKAGE_DIR': '/test/pkg'})
    def test_find_package_path_patterns(self):
        test_cases = [
            ("static_quality_gate_agent_deb_amd64", "/test/pkg/datadog-agent_7*amd64.deb"),
            ("static_quality_gate_agent_deb_amd64_fips", "/test/pkg/datadog-fips-agent_7*amd64.deb"),
            ("static_quality_gate_iot_agent_rpm_arm64", "/test/pkg/datadog-iot-agent-7*aarch64.rpm"),
            ("static_quality_gate_dogstatsd_suse_amd64", "/test/pkg/datadog-dogstatsd-7*x86_64.rpm"),
            ("static_quality_gate_agent_heroku_amd64", "/test/pkg/datadog-heroku-agent_7*amd64.deb"),
        ]

        for gate_name, expected_pattern in test_cases:
            with self.subTest(gate_name=gate_name):
                config = create_quality_gate_config(
                    gate_name, {"max_on_wire_size": "100 MiB", "max_on_disk_size": "350 MiB"}
                )

                mock_glob = MagicMock(return_value=["/test/pkg/some_package.ext"])
                with patch('tasks.static_quality_gates.gates.glob.glob', mock_glob):
                    with patch.object(self.measurer, '_calculate_package_sizes', return_value=(100, 350)):
                        self.measurer.measure(self.mock_ctx, config)

                actual_pattern = mock_glob.call_args_list[0][0][0]  # First call's first argument
                self.assertEqual(actual_pattern, expected_pattern)

    @patch.dict('os.environ', {'OMNIBUS_PACKAGE_DIR': '/test/pkg', 'CI_PIPELINE_ID': '123'})
    @patch('tasks.static_quality_gates.gates.glob.glob')
    def test_find_package_path_msi_dual_file(self, mock_glob):
        # First call returns ZIP file, second call returns MSI file
        mock_glob.side_effect = [
            ["/test/pkg/pipeline-123/datadog-agent-7.50.0-x86_64.zip"],
            ["/test/pkg/pipeline-123/datadog-agent-7.50.0-x86_64.msi"],
        ]

        config = create_quality_gate_config(
            "static_quality_gate_agent_msi", {"max_on_wire_size": "100 MiB", "max_on_disk_size": "350 MiB"}
        )

        with patch.object(self.measurer, '_calculate_package_sizes', return_value=(100, 350)):
            measurement = self.measurer.measure(self.mock_ctx, config)

        # Verify MSI file is used as the primary artifact path
        self.assertEqual(measurement.artifact_path, "/test/pkg/pipeline-123/datadog-agent-7.50.0-x86_64.msi")


class TestDockerArtifactMeasurer(unittest.TestCase):
    def setUp(self):
        self.measurer = DockerArtifactMeasurer()
        self.mock_ctx = MagicMock()

    @patch.dict('os.environ', {'BUCKET_BRANCH': 'main', 'CI_PIPELINE_ID': '12345', 'CI_COMMIT_SHORT_SHA': 'abc123'})
    def test_get_image_url_agent(self):
        config = QualityGateConfig("docker_agent_amd64", 1000, 2000, "amd64", "docker")

        url = self.measurer._get_image_url(config)

        expected = "registry.ddbuild.io/ci/datadog-agent/agent:v12345-abc123-7-amd64"
        self.assertEqual(url, expected)

    @patch.dict('os.environ', {'BUCKET_BRANCH': 'main', 'CI_PIPELINE_ID': '12345', 'CI_COMMIT_SHORT_SHA': 'abc123'})
    def test_get_image_url_jmx(self):
        config = QualityGateConfig("docker_agent_jmx_amd64", 1000, 2000, "amd64", "docker")

        url = self.measurer._get_image_url(config)

        expected = "registry.ddbuild.io/ci/datadog-agent/agent:v12345-abc123-7-jmx-amd64"
        self.assertEqual(url, expected)

    @patch.dict('os.environ', {'BUCKET_BRANCH': 'nightly', 'CI_PIPELINE_ID': '12345', 'CI_COMMIT_SHORT_SHA': 'abc123'})
    def test_get_image_url_nightly(self):
        """Test Docker image URL generation for nightly"""
        config = QualityGateConfig("docker_agent_amd64", 1000, 2000, "amd64", "docker")

        url = self.measurer._get_image_url(config)

        expected = "registry.ddbuild.io/ci/datadog-agent/agent-nightly:v12345-abc123-7-amd64"
        self.assertEqual(url, expected)

    @patch.dict('os.environ', {}, clear=True)
    def test_get_image_url_missing_ci_vars(self):
        """Test Docker image URL generation with missing CI variables"""
        config = QualityGateConfig("docker_agent_amd64", 1000, 2000, "amd64", "docker")

        with self.assertRaises(StaticQualityGateError) as cm:
            self.measurer._get_image_url(config)

        self.assertIn("CI environment", str(cm.exception))

    @patch.dict('os.environ', {'BUCKET_BRANCH': 'main', 'CI_PIPELINE_ID': '71580015', 'CI_COMMIT_SHORT_SHA': '668844'})
    def test_get_image_url(self):
        test_cases = [
            (
                "static_quality_gate_docker_agent_amd64",
                "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-amd64",
            ),
            (
                "static_quality_gate_docker_cluster_amd64",
                "registry.ddbuild.io/ci/datadog-agent/cluster-agent:v71580015-668844-amd64",
            ),
            (
                "static_quality_gate_docker_dogstatsd_arm64",
                "registry.ddbuild.io/ci/datadog-agent/dogstatsd:v71580015-668844-arm64",
            ),
            (
                "static_quality_gate_docker_cws_instrumentation_amd64",
                "registry.ddbuild.io/ci/datadog-agent/cws-instrumentation:v71580015-668844-amd64",
            ),
            (
                "static_quality_gate_docker_agent_jmx_amd64",
                "registry.ddbuild.io/ci/datadog-agent/agent:v71580015-668844-7-jmx-amd64",
            ),
            (
                "static_quality_gate_docker_host_profiler_amd64",
                "registry.ddbuild.io/ci/datadog-agent/ddot-ebpf:v71580015-668844-7-amd64",
            ),
            (
                "static_quality_gate_docker_host_profiler_arm64",
                "registry.ddbuild.io/ci/datadog-agent/ddot-ebpf:v71580015-668844-7-arm64",
            ),
        ]

        for gate_name, expected_url in test_cases:
            with self.subTest(gate_name=gate_name):
                config = create_quality_gate_config(
                    gate_name, {"max_on_wire_size": "100 MiB", "max_on_disk_size": "100 MiB"}
                )
                actual_url = self.measurer._get_image_url(config)
                self.assertEqual(actual_url, expected_url)

    @patch.dict(
        'os.environ', {'BUCKET_BRANCH': 'nightly', 'CI_PIPELINE_ID': '71580015', 'CI_COMMIT_SHORT_SHA': '668844'}
    )
    def test_get_image_url_nightly_flavors(self):
        test_cases = [
            (
                "static_quality_gate_docker_agent_amd64",
                "registry.ddbuild.io/ci/datadog-agent/agent-nightly:v71580015-668844-7-amd64",
            ),
            (
                "static_quality_gate_docker_cluster_amd64",
                "registry.ddbuild.io/ci/datadog-agent/cluster-agent-nightly:v71580015-668844-amd64",
            ),
            (
                "static_quality_gate_docker_host_profiler_amd64",
                "registry.ddbuild.io/ci/datadog-agent/ddot-ebpf-nightly:v71580015-668844-7-amd64",
            ),
        ]

        for gate_name, expected_url in test_cases:
            with self.subTest(gate_name=gate_name):
                config = create_quality_gate_config(
                    gate_name, {"max_on_wire_size": "100 MiB", "max_on_disk_size": "100 MiB"}
                )
                actual_url = self.measurer._get_image_url(config)
                self.assertEqual(actual_url, expected_url)

    def test_get_image_url_error_cases(self):
        """Test Docker image URL generation error cases"""
        # Test unknown flavor
        with self.assertRaises(ValueError) as context:
            config = create_quality_gate_config(
                "static_quality_gate_docker_unknown_flavor_amd64",
                {"max_on_wire_size": "100 MiB", "max_on_disk_size": "100 MiB"},
            )
            self.measurer._get_image_url(config)
        self.assertIn("Unknown docker image flavor for gate", str(context.exception))

        # Test missing CI_PIPELINE_ID
        with patch.dict('os.environ', {"CI_PIPELINE_ID": ""}):
            config = create_quality_gate_config(
                "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": "100 MiB", "max_on_disk_size": "100 MiB"}
            )
            with self.assertRaises(StaticQualityGateError) as context:
                self.measurer._get_image_url(config)
            self.assertIn("Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA", str(context.exception))

        # Test missing CI_COMMIT_SHORT_SHA
        with patch.dict('os.environ', {"CI_COMMIT_SHORT_SHA": ""}):
            config = create_quality_gate_config(
                "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": "100 MiB", "max_on_disk_size": "100 MiB"}
            )
            with self.assertRaises(StaticQualityGateError) as context:
                self.measurer._get_image_url(config)
            self.assertIn("Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA", str(context.exception))


class TestStaticQualityGate(unittest.TestCase):
    def setUp(self):
        # Use realistic MB sizes for testing
        self.config = QualityGateConfig(
            "test_gate", 100 * 1024 * 1024, 200 * 1024 * 1024, "amd64", "debian"
        )  # 100MB wire, 200MB disk
        self.mock_measurer = MagicMock()
        self.mock_ctx = MagicMock()
        self.gate = StaticQualityGate(self.config, self.mock_measurer)

    def test_execute_gate_success(self):
        # Mock measurement within limits (80MB wire, 150MB disk)
        measurement = ArtifactMeasurement("/path", 80 * 1024 * 1024, 150 * 1024 * 1024)
        self.mock_measurer.measure.return_value = measurement

        result = self.gate.execute_gate(self.mock_ctx)

        self.assertTrue(result.success)
        self.assertEqual(len(result.violations), 0)
        self.assertEqual(result.measurement, measurement)
        self.assertEqual(result.config, self.config)

    def test_execute_gate_on_wire_violation(self):
        # Mock measurement exceeding wire limit (120MB > 100MB limit)
        measurement = ArtifactMeasurement("/path", 120 * 1024 * 1024, 150 * 1024 * 1024)
        self.mock_measurer.measure.return_value = measurement

        self.assertFalse(self.gate.execute_gate(self.mock_ctx).success)

    def test_execute_gate_on_disk_violation(self):
        # Mock measurement exceeding disk limit (250MB > 200MB limit)
        measurement = ArtifactMeasurement("/path", 80 * 1024 * 1024, 250 * 1024 * 1024)
        self.mock_measurer.measure.return_value = measurement

        self.assertFalse(self.gate.execute_gate(self.mock_ctx).success)

    def test_execute_gate_both_violations(self):
        # Mock measurement exceeding both limits (120MB wire, 250MB disk)
        measurement = ArtifactMeasurement("/path", 120 * 1024 * 1024, 250 * 1024 * 1024)
        self.mock_measurer.measure.return_value = measurement

        self.assertFalse(self.gate.execute_gate(self.mock_ctx).success)

    def test_check_size_limits_success(self):
        measurement = ArtifactMeasurement("/path", 80 * 1024 * 1024, 150 * 1024 * 1024)
        violations = self.gate._check_size_limits(measurement)
        self.assertEqual(len(violations), 0)

    def test_check_size_limits_on_wire_violation(self):
        measurement = ArtifactMeasurement("/path", 120 * 1024 * 1024, 150 * 1024 * 1024)
        violations = self.gate._check_size_limits(measurement)

        self.assertEqual(len(violations), 1)
        self.assertEqual(violations[0].measurement_type, "wire")
        self.assertEqual(violations[0].current_size, 120 * 1024 * 1024)
        self.assertEqual(violations[0].max_size, 100 * 1024 * 1024)

    def test_check_size_limits_on_disk_violation(self):
        measurement = ArtifactMeasurement("/path", 80 * 1024 * 1024, 250 * 1024 * 1024)
        violations = self.gate._check_size_limits(measurement)

        self.assertEqual(len(violations), 1)
        self.assertEqual(violations[0].measurement_type, "disk")
        self.assertEqual(violations[0].current_size, 250 * 1024 * 1024)
        self.assertEqual(violations[0].max_size, 200 * 1024 * 1024)

    def test_measurement_validation_at_creation(self):
        # Valid measurement should work
        measurement = ArtifactMeasurement("/path", 1000, 2000)
        self.assertEqual(measurement.artifact_path, "/path")

        # Empty path should fail at creation
        with self.assertRaises(ValueError):
            ArtifactMeasurement("", 1000, 2000)

        # Negative sizes should fail at creation
        with self.assertRaises(ValueError):
            ArtifactMeasurement("/path", -1, 2000)


class TestQualityGateFactory(unittest.TestCase):
    def test_create_gate_docker(self):
        gate = QualityGateFactory.create_gate(
            "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": "1000 MiB", "max_on_disk_size": "2000 MiB"}
        )

        self.assertIsInstance(gate, StaticQualityGate)
        self.assertEqual(gate.config.gate_name, "static_quality_gate_docker_agent_amd64")
        self.assertEqual(gate.config.arch, "amd64")
        self.assertEqual(gate.config.os, "docker")
        self.assertIsInstance(gate.measurer, DockerArtifactMeasurer)

    def test_create_gate_package(self):
        gate = QualityGateFactory.create_gate(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": "1000 MiB", "max_on_disk_size": "2000 MiB"}
        )

        self.assertIsInstance(gate, StaticQualityGate)
        self.assertEqual(gate.config.gate_name, "static_quality_gate_agent_deb_amd64")
        self.assertEqual(gate.config.arch, "amd64")
        self.assertEqual(gate.config.os, "debian")
        self.assertIsInstance(gate.measurer, PackageArtifactMeasurer)

    def test_create_gate_unknown_type(self):
        with self.assertRaises(ValueError):
            QualityGateFactory.create_gate(
                "static_quality_gate_unknown_type", {"max_on_wire_size": "1000 MiB", "max_on_disk_size": "2000 MiB"}
            )

    @patch(
        'builtins.open',
        mock_open(
            read_data="""
static_quality_gate_agent_deb_amd64:
  max_on_wire_size: "1000 MiB"
  max_on_disk_size: "2000 MiB"
static_quality_gate_docker_agent_amd64:
  max_on_wire_size: "1500 MiB"
  max_on_disk_size: "3000 MiB"
"""
        ),
    )
    def test_create_gates_from_config(self):
        gates = QualityGateFactory.create_gates_from_config("test_config.yml")

        self.assertEqual(len(gates), 2)

        # Verify gates are created correctly
        gate_names = [gate.config.gate_name for gate in gates]
        self.assertIn("static_quality_gate_agent_deb_amd64", gate_names)
        self.assertIn("static_quality_gate_docker_agent_amd64", gate_names)


class TestGateListGeneration(unittest.TestCase):
    def test_create_quality_gate_config(self):
        gate_config = create_quality_gate_config(
            "static_quality_gate_agent_deb_amd64", {"max_on_wire_size": "100 MiB", "max_on_disk_size": "200 MiB"}
        )

        self.assertEqual(gate_config.gate_name, "static_quality_gate_agent_deb_amd64")
        self.assertEqual(gate_config.arch, "amd64")
        self.assertEqual(gate_config.os, "debian")
        self.assertEqual(gate_config.max_on_wire_size, 104857600)  # 100 MiB in bytes
        self.assertEqual(gate_config.max_on_disk_size, 209715200)  # 200 MiB in bytes


class TestArchitectureMapping(unittest.TestCase):
    def test_arch_mapping_package_gates(self):
        test_cases = [
            ("static_quality_gate_agent_deb_amd64", "amd64"),
            ("static_quality_gate_agent_deb_arm64", "arm64"),
            ("static_quality_gate_iot_agent_rpm_armhf", "armhf"),
            ("static_quality_gate_agent_msi", "amd64"),  # MSI defaults to amd64
        ]

        for gate_name, expected_arch in test_cases:
            with self.subTest(gate_name=gate_name):
                config = create_quality_gate_config(
                    gate_name, {"max_on_wire_size": "100 MiB", "max_on_disk_size": "200 MiB"}
                )
                self.assertEqual(config.arch, expected_arch)

    def test_os_mapping_package_gates(self):
        test_cases = [
            ("static_quality_gate_agent_deb_amd64", "debian"),
            ("static_quality_gate_agent_rpm_amd64", "centos"),
            ("static_quality_gate_agent_suse_amd64", "suse"),
            ("static_quality_gate_agent_msi", "windows"),
            ("static_quality_gate_agent_heroku_amd64", "debian"),
            ("static_quality_gate_docker_agent_amd64", "docker"),
        ]

        for gate_name, expected_os in test_cases:
            with self.subTest(gate_name=gate_name):
                config = create_quality_gate_config(
                    gate_name, {"max_on_wire_size": "100 MiB", "max_on_disk_size": "200 MiB"}
                )
                self.assertEqual(config.os, expected_os)

    def test_invalid_gate_names(self):
        invalid_names = [
            "static_quality_gate_agent_unknown_unknown",
            "static_quality_gate_unknown",
            "not_a_quality_gate",
        ]

        for invalid_name in invalid_names:
            with self.subTest(gate_name=invalid_name):
                with self.assertRaises(ValueError):
                    create_quality_gate_config(
                        invalid_name, {"max_on_wire_size": "100 MiB", "max_on_disk_size": "200 MiB"}
                    )


class TestOnDiskImageSizeCalculation(unittest.TestCase):
    def tearDown(self):
        try:
            os.remove('./tasks/unit_tests/testdata/fake_agent_image/with_tar_gz_archive/some_archive.tar.gz')
            os.remove('./tasks/unit_tests/testdata/fake_agent_image/with_tar_gz_archive/some_metadata.json')
        except OSError:
            pass
        try:
            os.remove('./tasks/unit_tests/testdata/fake_agent_image/without_tar_gz_archive/some_metadata.json')
        except OSError:
            pass


class TestMetricsDictDelta(unittest.TestCase):
    """Test that METRICS_DICT includes relative (delta) metrics."""

    def test_relative_metrics_in_dict(self):
        """Verify relative metrics are included in METRICS_DICT values."""
        self.assertIn("relative_on_wire_size", GateMetricHandler.METRICS_DICT.values())
        self.assertIn("relative_on_disk_size", GateMetricHandler.METRICS_DICT.values())

    def test_relative_metrics_naming(self):
        """Verify relative metric names follow naming convention."""
        self.assertIn(
            "datadog.agent.static_quality_gate.relative_on_wire_size",
            GateMetricHandler.METRICS_DICT.keys(),
        )
        self.assertIn(
            "datadog.agent.static_quality_gate.relative_on_disk_size",
            GateMetricHandler.METRICS_DICT.keys(),
        )

    def test_metrics_dict_has_six_entries(self):
        """Verify METRICS_DICT has all 6 expected metrics (4 original + 2 new delta metrics)."""
        self.assertEqual(len(GateMetricHandler.METRICS_DICT), 6)


class TestGenerateMetricReports(unittest.TestCase):
    """Test the generate_metric_reports function for S3 upload behavior."""

    def setUp(self):
        """Create a temporary directory for test files."""
        self.temp_dir = tempfile.mkdtemp()
        self.temp_report_file = os.path.join(self.temp_dir, "static_gate_report.json")

    def tearDown(self):
        """Clean up temporary files."""
        if os.path.exists(self.temp_report_file):
            os.remove(self.temp_report_file)
        if os.path.exists(self.temp_dir):
            os.rmdir(self.temp_dir)

    @patch.dict('os.environ', {'CI_COMMIT_SHA': 'abc123def456'})
    @patch('tasks.static_quality_gates.gates.is_a_release_branch')
    def test_uploads_to_s3_for_main_branch(self, mock_is_release):
        """Should upload report to S3 when on main branch."""
        mock_is_release.return_value = False
        handler = GateMetricHandler("main", "dev")
        handler.metrics = {"test_gate": {"current_on_disk_size": 100}}
        ctx = MockContext(
            run={
                f'aws s3 cp --only-show-errors --region us-east-1 --sse AES256 {self.temp_report_file} s3://dd-ci-artefacts-build-stable/datadog-agent/static_quality_gates/abc123def456/{self.temp_report_file}': Result(
                    "Done"
                ),
            }
        )

        handler.generate_metric_reports(ctx, filename=self.temp_report_file, branch="main", is_nightly=False)

        # Verify S3 upload was called
        self.assertEqual(len(ctx.run.call_args_list), 1)

    @patch.dict('os.environ', {'CI_COMMIT_SHA': 'abc123def456'})
    @patch('tasks.static_quality_gates.gates.is_a_release_branch')
    def test_uploads_to_s3_for_release_branch(self, mock_is_release):
        """Should upload report to S3 when on a release branch (e.g., 7.54.x)."""
        mock_is_release.return_value = True
        handler = GateMetricHandler("7.54.x", "dev")
        handler.metrics = {"test_gate": {"current_on_disk_size": 100}}
        ctx = MockContext(
            run={
                f'aws s3 cp --only-show-errors --region us-east-1 --sse AES256 {self.temp_report_file} s3://dd-ci-artefacts-build-stable/datadog-agent/static_quality_gates/abc123def456/{self.temp_report_file}': Result(
                    "Done"
                ),
            }
        )

        handler.generate_metric_reports(ctx, filename=self.temp_report_file, branch="7.54.x", is_nightly=False)

        # Verify S3 upload was called
        self.assertEqual(len(ctx.run.call_args_list), 1)

    @patch.dict('os.environ', {'CI_COMMIT_SHA': 'abc123def456'})
    @patch('tasks.static_quality_gates.gates.is_a_release_branch')
    def test_no_upload_for_feature_branch(self, mock_is_release):
        """Should NOT upload report to S3 when on a feature branch."""
        mock_is_release.return_value = False
        handler = GateMetricHandler("feature/my-branch", "dev")
        handler.metrics = {"test_gate": {"current_on_disk_size": 100}}
        ctx = MockContext(run={})

        handler.generate_metric_reports(
            ctx, filename=self.temp_report_file, branch="feature/my-branch", is_nightly=False
        )

        # Verify S3 upload was NOT called
        self.assertEqual(len(ctx.run.call_args_list), 0)

    @patch.dict('os.environ', {'CI_COMMIT_SHA': 'abc123def456'})
    @patch('tasks.static_quality_gates.gates.is_a_release_branch')
    def test_no_upload_for_nightly_main(self, mock_is_release):
        """Should NOT upload report to S3 for nightly builds even on main."""
        mock_is_release.return_value = False
        handler = GateMetricHandler("main", "nightly")
        handler.metrics = {"test_gate": {"current_on_disk_size": 100}}
        ctx = MockContext(run={})

        handler.generate_metric_reports(ctx, filename=self.temp_report_file, branch="main", is_nightly=True)

        # Verify S3 upload was NOT called
        self.assertEqual(len(ctx.run.call_args_list), 0)

    @patch.dict('os.environ', {'CI_COMMIT_SHA': 'abc123def456'})
    @patch('tasks.static_quality_gates.gates.is_a_release_branch')
    def test_no_upload_for_nightly_release(self, mock_is_release):
        """Should NOT upload report to S3 for nightly builds on release branches."""
        mock_is_release.return_value = True
        handler = GateMetricHandler("7.54.x", "nightly")
        handler.metrics = {"test_gate": {"current_on_disk_size": 100}}
        ctx = MockContext(run={})

        handler.generate_metric_reports(ctx, filename=self.temp_report_file, branch="7.54.x", is_nightly=True)

        # Verify S3 upload was NOT called
        self.assertEqual(len(ctx.run.call_args_list), 0)

    @patch.dict('os.environ', {}, clear=True)
    @patch('tasks.static_quality_gates.gates.is_a_release_branch')
    def test_no_upload_without_commit_sha(self, mock_is_release):
        """Should NOT upload report to S3 when CI_COMMIT_SHA is not set."""
        mock_is_release.return_value = False
        handler = GateMetricHandler("main", "dev")
        handler.metrics = {"test_gate": {"current_on_disk_size": 100}}
        ctx = MockContext(run={})

        handler.generate_metric_reports(ctx, filename=self.temp_report_file, branch="main", is_nightly=False)

        # Verify S3 upload was NOT called
        self.assertEqual(len(ctx.run.call_args_list), 0)


class TestShouldSkipSendMetrics(unittest.TestCase):
    """Test the _should_skip_send_metrics method for pipeline source filtering."""

    def setUp(self):
        """Create a GateMetricHandler instance for testing."""
        self.handler = GateMetricHandler("main", "dev")

    # Should SKIP metrics (return True) - main branch + non-push pipelines

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'main', 'CI_PIPELINE_SOURCE': 'web'})
    def test_skip_main_branch_web_trigger(self):
        """Should skip metrics on main branch with manual web trigger."""
        self.assertTrue(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'main', 'CI_PIPELINE_SOURCE': 'trigger'})
    def test_skip_main_branch_trigger(self):
        """Should skip metrics on main branch with downstream trigger."""
        self.assertTrue(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'main', 'CI_PIPELINE_SOURCE': 'pipeline'})
    def test_skip_main_branch_pipeline(self):
        """Should skip metrics on main branch with multi-project downstream pipeline."""
        self.assertTrue(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'main', 'CI_PIPELINE_SOURCE': 'schedule'})
    def test_skip_main_branch_schedule(self):
        """Should skip metrics on main branch with scheduled pipeline."""
        self.assertTrue(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'main', 'CI_PIPELINE_SOURCE': 'api'})
    def test_skip_main_branch_api(self):
        """Should skip metrics on main branch with API-triggered pipeline."""
        self.assertTrue(self.handler._should_skip_send_metrics())

    # Should NOT skip metrics (return False) - main branch + push pipeline

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'main', 'CI_PIPELINE_SOURCE': 'push'})
    def test_no_skip_main_branch_push(self):
        """Should NOT skip metrics on main branch with push pipeline."""
        self.assertFalse(self.handler._should_skip_send_metrics())

    # Should NOT skip metrics (return False) - non-main branches

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'feature/my-branch', 'CI_PIPELINE_SOURCE': 'push'})
    def test_no_skip_feature_branch_push(self):
        """Should NOT skip metrics on feature branch with push pipeline."""
        self.assertFalse(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'feature/my-branch', 'CI_PIPELINE_SOURCE': 'web'})
    def test_no_skip_feature_branch_web(self):
        """Should NOT skip metrics on feature branch with web trigger."""
        self.assertFalse(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'feature/my-branch', 'CI_PIPELINE_SOURCE': 'trigger'})
    def test_no_skip_feature_branch_trigger(self):
        """Should NOT skip metrics on feature branch with trigger."""
        self.assertFalse(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': '7.55.x', 'CI_PIPELINE_SOURCE': 'push'})
    def test_no_skip_release_branch_push(self):
        """Should NOT skip metrics on release branch with push pipeline."""
        self.assertFalse(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': '7.55.x', 'CI_PIPELINE_SOURCE': 'web'})
    def test_no_skip_release_branch_web(self):
        """Should NOT skip metrics on release branch with web trigger."""
        self.assertFalse(self.handler._should_skip_send_metrics())

    # Edge cases - missing/empty environment variables

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': '', 'CI_PIPELINE_SOURCE': ''}, clear=True)
    def test_no_skip_empty_env_vars(self):
        """Should NOT skip metrics when both env vars are empty."""
        self.assertFalse(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_COMMIT_BRANCH': 'main', 'CI_PIPELINE_SOURCE': ''}, clear=True)
    def test_skip_main_empty_source(self):
        """Should skip metrics on main branch when pipeline source is empty (not 'push')."""
        self.assertTrue(self.handler._should_skip_send_metrics())

    @patch.dict('os.environ', {'CI_PIPELINE_SOURCE': 'push'}, clear=True)
    def test_no_skip_empty_branch(self):
        """Should NOT skip metrics when branch is empty (not 'main')."""
        self.assertFalse(self.handler._should_skip_send_metrics())
