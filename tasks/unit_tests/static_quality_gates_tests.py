"""
Comprehensive unit tests for the static quality gates implementation.

This test suite covers:
- Immutable data classes and their validation
- Architecture and OS extraction utilities
- Artifact measurement strategies (Package and Docker)
- Quality gate execution and violation handling
- Factory pattern for gate creation
- Utility functions for size conversion and formatting

The design eliminates inheritance-based issues:
- No subclassing - uses composition with Strategy pattern
- No reliance on state to pass data - uses immutable data structures
- All attributes guaranteed to be defined - enforced by dataclass validation
"""

import os
import unittest
from unittest.mock import MagicMock, mock_open, patch

from invoke import MockContext, Result
from invoke.exceptions import Exit

from tasks.libs.package.size import InfraError
from tasks.quality_gates import display_pr_comment, generate_new_quality_gate_config, parse_and_trigger_gates
from tasks.static_quality_gates.gates import (
    # Data classes
    ArtifactMeasurement,
    DockerArtifactMeasurer,
    GateMetricHandler,
    GateResult,
    # Strategy pattern classes
    PackageArtifactMeasurer,
    QualityGateConfig,
    QualityGateFactory,
    SizeViolation,
    # Main quality gate class
    StaticQualityGate,
    # Exceptions
    StaticQualityGateFailed,
    _extract_arch_from_gate_name,
    _extract_os_from_gate_name,
    byte_to_string,
    # Utility functions
    create_quality_gate_config,
    read_byte_input,
    string_to_byte,
    string_to_latex_color,
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

    def test_string_to_latex_color(self):
        """Test string_to_latex_color function"""
        # Should wrap text in $${} format
        self.assertEqual(string_to_latex_color("test"), "$${test}$$")
        self.assertEqual(string_to_latex_color("10MiB"), "$${10MiB}$$")


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

        with self.assertRaises(StaticQualityGateFailed) as cm:
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
            with self.assertRaises(StaticQualityGateFailed) as context:
                self.measurer._get_image_url(config)
            self.assertIn("Missing CI_PIPELINE_ID, CI_COMMIT_SHORT_SHA", str(context.exception))

        # Test missing CI_COMMIT_SHORT_SHA
        with patch.dict('os.environ', {"CI_COMMIT_SHORT_SHA": ""}):
            config = create_quality_gate_config(
                "static_quality_gate_docker_agent_amd64", {"max_on_wire_size": "100 MiB", "max_on_disk_size": "100 MiB"}
            )
            with self.assertRaises(StaticQualityGateFailed) as context:
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

        with self.assertRaises(StaticQualityGateFailed) as cm:
            self.gate.execute_gate(self.mock_ctx)

        error_msg = str(cm.exception)
        self.assertIn("Wire size", error_msg)
        self.assertIn("120.0 MB", error_msg)  # Current size
        self.assertIn("100.0 MB", error_msg)  # Limit
        self.assertIn("test_gate failed", error_msg)

    def test_execute_gate_on_disk_violation(self):
        # Mock measurement exceeding disk limit (250MB > 200MB limit)
        measurement = ArtifactMeasurement("/path", 80 * 1024 * 1024, 250 * 1024 * 1024)
        self.mock_measurer.measure.return_value = measurement

        with self.assertRaises(StaticQualityGateFailed) as cm:
            self.gate.execute_gate(self.mock_ctx)

        error_msg = str(cm.exception)
        self.assertIn("Disk size", error_msg)
        self.assertIn("250.0 MB", error_msg)  # Current size
        self.assertIn("200.0 MB", error_msg)  # Limit

    def test_execute_gate_both_violations(self):
        # Mock measurement exceeding both limits (120MB wire, 250MB disk)
        measurement = ArtifactMeasurement("/path", 120 * 1024 * 1024, 250 * 1024 * 1024)
        self.mock_measurer.measure.return_value = measurement

        with self.assertRaises(StaticQualityGateFailed) as cm:
            self.gate.execute_gate(self.mock_ctx)

        error_message = str(cm.exception)
        self.assertIn("Wire size", error_message)
        self.assertIn("Disk size", error_message)

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


class MockMetricHandler:
    """Mock metric handler for testing quality gate configuration updates"""

    def __init__(self, metrics):
        self.metrics = metrics
        self.total_size_saved = 0


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

    def test_exception_gate_bump(self):
        with open("tasks/unit_tests/testdata/quality_gate_config_test.yml") as f:
            new_config, saved_amount = generate_new_quality_gate_config(
                f,
                MockMetricHandler(
                    {
                        "static_quality_gate_agent_suse_amd64": {
                            "relative_on_wire_size": 424242,
                            "current_on_wire_size": 50000000,
                            "max_on_wire_size": 100000000,
                            "relative_on_disk_size": 242424,
                            "current_on_disk_size": 50000000,
                            "max_on_disk_size": 100000000,
                        },
                        "static_quality_gate_agent_deb_amd64": {
                            "relative_on_wire_size": 424242,
                            "current_on_wire_size": 4000000,
                            "max_on_wire_size": 5000000,
                            "relative_on_disk_size": 242424,
                            "current_on_disk_size": 4000000,
                            "max_on_disk_size": 5000000,
                        },
                        "static_quality_gate_docker_agent_amd64": {
                            "relative_on_wire_size": 424242,
                            "current_on_wire_size": 50000000,
                            "max_on_wire_size": 100000000,
                            "current_on_disk_size": 50000000,
                            "relative_on_disk_size": 242424,
                            "max_on_disk_size": 100000000,
                        },
                    }
                ),
                True,  # exception_gate_bump
            )
        self.assertEqual(
            new_config["static_quality_gate_agent_suse_amd64"]["max_on_wire_size"],
            "95.77 MiB",
            f"Expected 95.77 MiB got {new_config['static_quality_gate_agent_suse_amd64']['max_on_wire_size']}",
        )
        self.assertEqual(
            new_config["static_quality_gate_agent_suse_amd64"]["max_on_disk_size"],
            "95.6 MiB",
            f"Expected 95.6 MiB got {new_config['static_quality_gate_agent_suse_amd64']['max_on_disk_size']}",
        )
        self.assertEqual(
            new_config["static_quality_gate_agent_deb_amd64"]["max_on_wire_size"],
            "5.17 MiB",
            f"Expected 5.17 MiB got {new_config['static_quality_gate_agent_deb_amd64']['max_on_wire_size']}",
        )
        self.assertEqual(
            new_config["static_quality_gate_agent_deb_amd64"]["max_on_disk_size"],
            "5.0 MiB",
            f"Expected 5.0 MiB got {new_config['static_quality_gate_agent_deb_amd64']['max_on_disk_size']}",
        )


class TestQualityGatesPrMessage(unittest.TestCase):
    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.gates.GateMetricHandler.get_formatted_metric",
        new=MagicMock(return_value="10MiB"),
    )
    @patch(
        "tasks.quality_gates.get_debug_job_url",
        new=MagicMock(return_value="https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000"),
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_no_error(self, pr_commenter_mock):
        from unittest.mock import ANY

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            True,
            [
                {'name': 'gateA', 'error_type': None, 'message': None},
                {'name': 'gateB', 'error_type': None, 'message': None},
            ],
            gate_metric_handler,
            "value",
        )
        pr_commenter_mock.assert_called_once()
        pr_commenter_mock.assert_called_with(
            ANY,
            title='Static quality checks',
            body='✅ Please find below the results from static quality gates\nComparison made with [ancestor](https://github.com/DataDog/datadog-agent/commit/value) value\n\n\n<details>\n<summary>Successful checks</summary>\n\n### Info\n\n||Quality gate|Delta|On disk size (MiB)|Delta|On wire size (MiB)|\n|--|--|--|--|--|--|\n|✅|gateA|10MiB|DataNotFound|10MiB|DataNotFound|\n|✅|gateB|10MiB|DataNotFound|10MiB|DataNotFound|\n\n</details>\n',
        )

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.gates.GateMetricHandler.get_formatted_metric",
        new=MagicMock(return_value="10MiB"),
    )
    @patch(
        "tasks.quality_gates.get_debug_job_url",
        new=MagicMock(return_value="https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000"),
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_no_info(self, pr_commenter_mock):
        from unittest.mock import ANY

        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            False,
            [
                {'name': 'gateA', 'error_type': 'AssertionError', 'message': 'some_msg_A'},
                {'name': 'gateB', 'error_type': 'AssertionError', 'message': 'some_msg_B'},
            ],
            gate_metric_handler,
            "value",
        )
        pr_commenter_mock.assert_called_once()
        expected_body = '❌ Please find below the results from static quality gates\nComparison made with [ancestor](https://github.com/DataDog/datadog-agent/commit/value) value\n### Error\n\n||Quality gate|Delta|On disk size (MiB)|Delta|On wire size (MiB)|\n|--|--|--|--|--|--|\n|❌|gateA|10MiB|DataNotFound|10MiB|DataNotFound|\n|❌|gateB|10MiB|DataNotFound|10MiB|DataNotFound|\n<details>\n<summary>Gate failure full details</summary>\n\n|Quality gate|Error type|Error message|\n|----|---|--------|\n|gateA|AssertionError|some_msg_A|\n|gateB|AssertionError|some_msg_B|\n\n</details>\n\nStatic quality gates prevent the PR to merge! \nTo understand the size increase caused by this PR, feel free to use the [debug_static_quality_gates](https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000) manual gitlab job to compare what this PR introduced for a specific gate.\nUsage:\n- Run the manual job with the following Key / Value pair as CI/CD variable on the gitlab UI. Example for amd64 deb packages\nKey: `GATE_NAME`, Value: `static_quality_gate_agent_deb_amd64`\n\nYou can check the static quality gates [confluence page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4805854687/Static+Quality+Gates) for guidance. We also have a [toolbox page](https://datadoghq.atlassian.net/wiki/spaces/agent/pages/4887448722/Static+Quality+Gates+Toolbox) available to list tools useful to debug the size increase.\n\n\n'
        pr_commenter_mock.assert_called_with(ANY, title='Static quality checks', body=expected_body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.gates.GateMetricHandler.get_formatted_metric",
        new=MagicMock(return_value="10MiB"),
    )
    @patch(
        "tasks.quality_gates.get_debug_job_url",
        new=MagicMock(return_value="https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000"),
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_one_of_each(self, pr_commenter_mock):
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            False,
            [
                {'name': 'gateA', 'error_type': 'AssertionError', 'message': 'some_msg_A'},
                {'name': 'gateB', 'error_type': None, 'message': None},
            ],
            gate_metric_handler,
            "value",
        )
        pr_commenter_mock.assert_called_once()
        # Check that both error and success sections are present
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        self.assertIn('### Error', body)
        self.assertIn('Successful checks', body)
        self.assertIn('gateA', body)
        self.assertIn('gateB', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch(
        "tasks.static_quality_gates.gates.GateMetricHandler.get_formatted_metric",
        new=MagicMock(return_value="10MiB", side_effect=KeyError),
    )
    @patch(
        "tasks.quality_gates.get_debug_job_url",
        new=MagicMock(return_value="https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/00000000"),
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_missing_data(self, pr_commenter_mock):
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            False,
            [
                {'name': 'gateA', 'error_type': 'AssertionError', 'message': 'some_msg_A'},
            ],
            gate_metric_handler,
            "value",
        )
        pr_commenter_mock.assert_called_once()
        # Check that DataNotFound appears when metrics are missing
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        self.assertIn('DataNotFound', body)


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


if __name__ == '__main__':
    unittest.main()
