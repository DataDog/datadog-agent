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
from tasks.quality_gates import (
    display_pr_comment,
    generate_new_quality_gate_config,
    get_change_metrics,
    get_wire_change_metrics,
    parse_and_trigger_gates,
)
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
    StaticQualityGateError,
    _extract_arch_from_gate_name,
    _extract_os_from_gate_name,
    byte_to_string,
    # Utility functions
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
    @patch("tasks.quality_gates.pr_commenter")
    def test_no_error_with_significant_changes(self, pr_commenter_mock):
        """Test PR comment with successful gates that have significant changes (>= 2 KiB)."""
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
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Check that the table format is present
        self.assertIn('Change', body)
        self.assertIn('Limit Bounds', body)
        self.assertIn('Successful checks', body)
        self.assertIn('gateA', body)
        self.assertIn('gateB', body)
        # Check on-wire section is present
        self.assertIn('On-wire sizes (compressed)', body)
        # Check dashboard link is present
        self.assertIn('Static Quality Gates Dashboard', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_neutral_changes_collapsed(self, pr_commenter_mock):
        """Test that gates with neutral changes (< 2 KiB) are collapsed."""
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
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Check that collapsed section is present
        self.assertIn('successful checks with minimal change', body)
        self.assertIn('2 KiB', body)
        # Check that gates are in the collapsed section
        self.assertIn('gateA', body)
        self.assertIn('gateB', body)
        # Check on-wire section is present
        self.assertIn('On-wire sizes (compressed)', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_mixed_significant_and_neutral(self, pr_commenter_mock):
        """Test PR comment with both significant and neutral changes."""
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
    @patch("tasks.quality_gates.pr_commenter")
    def test_no_info(self, pr_commenter_mock):
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
        # Check that the new table format is present in error section
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        self.assertIn('### Error', body)
        self.assertIn('Change', body)
        self.assertIn('Limit Bounds', body)
        self.assertIn('gateA', body)
        self.assertIn('gateB', body)
        self.assertIn('Gate failure full details', body)
        self.assertIn('Static quality gates prevent the PR to merge!', body)
        # Check on-wire section is present
        self.assertIn('On-wire sizes (compressed)', body)
        # Check dashboard link is present
        self.assertIn('Static Quality Gates Dashboard', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_one_of_each(self, pr_commenter_mock):
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
        # Check new columns are present
        self.assertIn('Change', body)
        self.assertIn('Limit Bounds', body)
        # Check on-wire section is present
        self.assertIn('On-wire sizes (compressed)', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
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
        # Check that N/A appears when metrics are missing
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        self.assertIn('N/A', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_wire_table_separate(self, pr_commenter_mock):
        """Test that on-wire sizes appear in a separate collapsed section."""
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        gate_metric_handler.metrics["gateA"] = {
            "current_on_disk_size": 100 * 1024 * 1024,
            "max_on_disk_size": 150 * 1024 * 1024,
            "relative_on_disk_size": 5 * 1024 * 1024,
            "current_on_wire_size": 50 * 1024 * 1024,
            "max_on_wire_size": 75 * 1024 * 1024,
            "relative_on_wire_size": 2 * 1024 * 1024,
        }
        display_pr_comment(
            c,
            True,
            [
                {'name': 'gateA', 'error_type': None, 'message': None},
            ],
            gate_metric_handler,
            "value",
        )
        pr_commenter_mock.assert_called_once()
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Check on-wire section is present and collapsed
        self.assertIn('On-wire sizes (compressed)', body)
        self.assertIn('<details>', body)
        # Check gateA appears in wire section
        wire_section_start = body.find('On-wire sizes (compressed)')
        self.assertIn('gateA', body[wire_section_start:])


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


class TestShouldBypassFailure(unittest.TestCase):
    """Test the should_bypass_failure function for delta-based non-blocking failures."""

    def test_bypass_when_disk_delta_zero(self):
        """Should bypass when on-disk size delta is exactly 0."""
        from tasks.quality_gates import should_bypass_failure

        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": 0,
        }
        self.assertTrue(should_bypass_failure("test_gate", handler))

    def test_bypass_when_disk_delta_negative(self):
        """Should bypass when on-disk size delta is negative (size decreased)."""
        from tasks.quality_gates import should_bypass_failure

        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": -500000,  # -500KB
        }
        self.assertTrue(should_bypass_failure("test_gate", handler))

    def test_bypass_when_disk_delta_within_threshold(self):
        """Should bypass when on-disk size delta is positive but within threshold (~2KiB)."""
        from tasks.quality_gates import should_bypass_failure

        handler = GateMetricHandler("main", "dev")
        # Small positive delta (1KB) should be treated as 0
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": 1024,  # 1KB - within 2KiB threshold
        }
        self.assertTrue(should_bypass_failure("test_gate", handler))

    def test_bypass_ignores_wire_delta(self):
        """Should bypass based only on disk delta, ignoring wire delta."""
        from tasks.quality_gates import should_bypass_failure

        handler = GateMetricHandler("main", "dev")
        # Even with positive wire delta, should bypass if disk delta is within threshold
        handler.metrics["test_gate"] = {
            "relative_on_wire_size": 1000000,  # Positive wire delta (1MB)
            "relative_on_disk_size": 0,  # Zero disk delta
        }
        self.assertTrue(should_bypass_failure("test_gate", handler))

    def test_no_bypass_when_disk_delta_exceeds_threshold(self):
        """Should NOT bypass when on-disk size delta exceeds threshold."""
        from tasks.quality_gates import should_bypass_failure

        handler = GateMetricHandler("main", "dev")
        # Delta of 5KB exceeds threshold of 2KiB
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": 5000,  # 5KB - exceeds 2KiB threshold
        }
        self.assertFalse(should_bypass_failure("test_gate", handler))

    def test_no_bypass_when_disk_delta_significantly_positive(self):
        """Should NOT bypass when on-disk size delta is significantly positive."""
        from tasks.quality_gates import should_bypass_failure

        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "relative_on_disk_size": 1000000,  # 1MB - way over threshold
        }
        self.assertFalse(should_bypass_failure("test_gate", handler))

    def test_no_bypass_when_missing_disk_delta(self):
        """Should NOT bypass when disk delta is missing."""
        from tasks.quality_gates import should_bypass_failure

        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "relative_on_wire_size": 0,
        }
        self.assertFalse(should_bypass_failure("test_gate", handler))

    def test_no_bypass_when_gate_not_found(self):
        """Should NOT bypass when gate doesn't exist in metrics."""
        from tasks.quality_gates import should_bypass_failure

        handler = GateMetricHandler("main", "dev")
        handler.metrics = {}
        self.assertFalse(should_bypass_failure("nonexistent_gate", handler))


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


class TestNonBlockingPrComment(unittest.TestCase):
    """Test PR comment display for non-blocking failures."""

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_non_blocking_failure_shows_warning_indicator(self, pr_commenter_mock):
        """Non-blocking failures should show warning indicator, not error."""
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            True,  # final_state is success (no blocking failures)
            [
                {
                    'name': 'gateA',
                    'error_type': 'StaticQualityGateFailed',
                    'message': 'size exceeded',
                    'blocking': False,
                },
            ],
            gate_metric_handler,
            "ancestor123",
        )
        pr_commenter_mock.assert_called_once()
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Should show warning indicator for non-blocking failure
        self.assertIn('⚠️', body)
        # Should NOT contain the blocking failure message
        self.assertNotIn('prevent the PR to merge', body)
        # Should contain the non-blocking note
        self.assertIn('non-blocking', body)

    @patch.dict(
        'os.environ',
        {
            'CI_COMMIT_REF_NAME': 'pikachu',
            'CI_COMMIT_BRANCH': 'sequoia',
        },
    )
    @patch("tasks.quality_gates.pr_commenter")
    def test_blocking_failure_shows_error_indicator(self, pr_commenter_mock):
        """Blocking failures should show error indicator and blocking message."""
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            False,  # final_state is failure (has blocking failures)
            [
                {
                    'name': 'gateA',
                    'error_type': 'StaticQualityGateFailed',
                    'message': 'size exceeded',
                    'blocking': True,
                },
            ],
            gate_metric_handler,
            "ancestor123",
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
    @patch("tasks.quality_gates.pr_commenter")
    def test_mixed_blocking_and_non_blocking(self, pr_commenter_mock):
        """Mixed blocking and non-blocking failures should show both indicators."""
        c = MockContext()
        gate_metric_handler = GateMetricHandler("main", "dev")
        display_pr_comment(
            c,
            False,  # final_state is failure (has blocking failures)
            [
                {
                    'name': 'gateA',
                    'error_type': 'StaticQualityGateFailed',
                    'message': 'size exceeded',
                    'blocking': True,
                },
                {
                    'name': 'gateB',
                    'error_type': 'StaticQualityGateFailed',
                    'message': 'size exceeded',
                    'blocking': False,
                },
            ],
            gate_metric_handler,
            "ancestor123",
        )
        pr_commenter_mock.assert_called_once()
        call_args = pr_commenter_mock.call_args
        body = call_args[1]['body']
        # Should show both indicators
        self.assertIn('❌', body)
        self.assertIn('⚠️', body)
        # Should contain the blocking failure message (since there's a blocking failure)
        self.assertIn('prevent the PR to merge', body)


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
        # Neutral shows only current size (bolded), no arrows
        self.assertEqual("**150.000** MiB", limit_bounds)

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
        # Neutral shows only current size (bolded), no arrows
        self.assertIn("**", limit_bounds)
        self.assertIn("MiB", limit_bounds)
        self.assertNotIn("→", limit_bounds)

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
    """Test the get_wire_change_metrics function for on-wire size calculations."""

    def test_normal_positive_delta(self):
        """Should calculate change for a positive delta (size increased)."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics["test_gate"] = {
            "current_on_wire_size": 100 * 1024 * 1024,  # 100 MiB
            "max_on_wire_size": 150 * 1024 * 1024,  # 150 MiB
            "relative_on_wire_size": 10 * 1024 * 1024,  # +10 MiB delta
        }
        change_str, limit_bounds, is_neutral = get_wire_change_metrics("test_gate", handler)

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
        change_str, limit_bounds, is_neutral = get_wire_change_metrics("test_gate", handler)

        self.assertEqual("neutral", change_str)
        self.assertTrue(is_neutral)
        self.assertIn("**", limit_bounds)
        self.assertNotIn("→", limit_bounds)

    def test_missing_gate(self):
        """Should return N/A when gate is not found."""
        handler = GateMetricHandler("main", "dev")
        handler.metrics = {}

        change_str, limit_bounds, is_neutral = get_wire_change_metrics("nonexistent_gate", handler)

        self.assertEqual("N/A", change_str)
        self.assertEqual("N/A", limit_bounds)
        self.assertFalse(is_neutral)


class TestGetPrForBranch(unittest.TestCase):
    """Test the get_pr_for_branch helper function."""

    @patch("tasks.quality_gates.GithubAPI")
    def test_returns_pr_when_found(self, mock_github_class):
        """Should return PR object when a PR exists for the branch."""
        from tasks.quality_gates import get_pr_for_branch

        mock_pr = MagicMock()
        mock_pr.number = 12345
        mock_pr.title = "Test PR"
        mock_github = MagicMock()
        mock_github.get_pr_for_branch.return_value = [mock_pr]
        mock_github_class.return_value = mock_github

        result = get_pr_for_branch("test-branch")

        self.assertEqual(result, mock_pr)
        self.assertEqual(result.number, 12345)
        mock_github.get_pr_for_branch.assert_called_once_with("test-branch")

    @patch("tasks.quality_gates.GithubAPI")
    def test_returns_none_when_no_pr(self, mock_github_class):
        """Should return None when no PR exists for the branch."""
        from tasks.quality_gates import get_pr_for_branch

        mock_github = MagicMock()
        mock_github.get_pr_for_branch.return_value = []
        mock_github_class.return_value = mock_github

        result = get_pr_for_branch("test-branch")

        self.assertIsNone(result)

    @patch("tasks.quality_gates.GithubAPI")
    def test_returns_none_on_exception(self, mock_github_class):
        """Should return None and not raise when GitHub API fails."""
        from tasks.quality_gates import get_pr_for_branch

        mock_github_class.side_effect = Exception("API error")

        result = get_pr_for_branch("test-branch")

        self.assertIsNone(result)

    @patch("tasks.quality_gates.GithubAPI")
    def test_returns_first_pr_when_multiple(self, mock_github_class):
        """Should return first PR when multiple PRs exist for branch."""
        from tasks.quality_gates import get_pr_for_branch

        mock_pr1 = MagicMock()
        mock_pr1.number = 111
        mock_pr2 = MagicMock()
        mock_pr2.number = 222
        mock_github = MagicMock()
        mock_github.get_pr_for_branch.return_value = [mock_pr1, mock_pr2]
        mock_github_class.return_value = mock_github

        result = get_pr_for_branch("test-branch")

        self.assertEqual(result.number, 111)


class TestPrNumberInMetricTags(unittest.TestCase):
    """Test that PR number is included in metric tags when available."""

    def test_pr_number_added_to_gate_tags(self):
        """Verify pr_number is included when building gate tags with a PR."""
        # Simulate the gate tags dict building from parse_and_trigger_gates
        mock_pr = MagicMock()
        mock_pr.number = 42

        gate_tags = {
            "gate_name": "test_gate",
            "arch": "amd64",
            "os": "debian",
            "pipeline_id": "12345",
            "ci_commit_ref_slug": "test-branch",
            "ci_commit_sha": "abc123",
        }
        if mock_pr:
            gate_tags["pr_number"] = str(mock_pr.number)

        self.assertIn("pr_number", gate_tags)
        self.assertEqual(gate_tags["pr_number"], "42")

    def test_pr_number_not_added_when_no_pr(self):
        """Verify pr_number is not included when there's no PR."""
        pr = None

        gate_tags = {
            "gate_name": "test_gate",
            "arch": "amd64",
            "os": "debian",
            "pipeline_id": "12345",
            "ci_commit_ref_slug": "test-branch",
            "ci_commit_sha": "abc123",
        }
        if pr:
            gate_tags["pr_number"] = str(pr.number)

        self.assertNotIn("pr_number", gate_tags)


if __name__ == '__main__':
    unittest.main()
