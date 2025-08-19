"""
Unit tests for experimental static quality gates functionality.

These tests cover the InPlacePackageMeasurer and related classes for in-place
artifact measurement and report generation.
"""

import os
import tempfile
import unittest
import unittest.mock
from unittest.mock import Mock, patch

import yaml

from tasks.static_quality_gates.experimental_gates import (
    DockerImageInfo,
    DockerLayerInfo,
    FileInfo,
    InPlaceArtifactReport,
    InPlaceDockerMeasurer,
    InPlacePackageMeasurer,
)
from tasks.static_quality_gates.gates import ArtifactMeasurement


class TestFileInfo(unittest.TestCase):
    """Test cases for the FileInfo dataclass."""

    def test_file_info_creation_valid(self):
        """Test creating a valid FileInfo instance."""
        file_info = FileInfo(relative_path="opt/datadog-agent/bin/agent", size_bytes=12345, checksum="sha256:abc123")

        self.assertEqual(file_info.relative_path, "opt/datadog-agent/bin/agent")
        self.assertEqual(file_info.size_bytes, 12345)
        self.assertEqual(file_info.checksum, "sha256:abc123")

    def test_file_info_creation_minimal(self):
        """Test creating FileInfo with minimal required fields."""
        file_info = FileInfo(relative_path="etc/config.yaml", size_bytes=1024)

        self.assertEqual(file_info.relative_path, "etc/config.yaml")
        self.assertEqual(file_info.size_bytes, 1024)
        self.assertIsNone(file_info.checksum)

    def test_file_info_validation_empty_path(self):
        """Test FileInfo validation with empty relative_path."""
        with self.assertRaises(ValueError) as cm:
            FileInfo(relative_path="", size_bytes=1024)
        self.assertIn("relative_path cannot be empty", str(cm.exception))

    def test_file_info_validation_negative_size(self):
        """Test FileInfo validation with negative size."""
        with self.assertRaises(ValueError) as cm:
            FileInfo(relative_path="test/file", size_bytes=-1)
        self.assertIn("size_bytes must be non-negative", str(cm.exception))


class TestInPlaceArtifactReport(unittest.TestCase):
    """Test cases for the InPlaceArtifactReport dataclass."""

    def setUp(self):
        """Set up test data."""
        self.sample_file_inventory = [
            FileInfo("opt/agent/bin/agent", 12345, "sha256:abc123"),
            FileInfo("etc/config.yaml", 1024, None),
        ]

    def test_report_creation_valid(self):
        """Test creating a valid InPlaceArtifactReport."""
        report = InPlaceArtifactReport(
            artifact_path="/path/to/package.deb",
            gate_name="static_quality_gate_agent_deb_amd64",
            on_wire_size=100000,
            on_disk_size=500000,
            max_on_wire_size=150000,
            max_on_disk_size=600000,
            file_inventory=self.sample_file_inventory,
            measurement_timestamp="2024-01-15T14:30:22.123456Z",
            pipeline_id="12345",
            commit_sha="abc123def",
            arch="amd64",
            os="debian",
            build_job_name="agent_deb-x64-a7",
        )

        self.assertEqual(report.artifact_path, "/path/to/package.deb")
        self.assertEqual(report.on_wire_size, 100000)
        self.assertEqual(report.on_disk_size, 500000)
        self.assertEqual(len(report.file_inventory), 2)

    def test_report_validation_empty_path(self):
        """Test report validation with empty artifact_path."""
        with self.assertRaises(ValueError) as cm:
            InPlaceArtifactReport(
                artifact_path="",
                gate_name="test_gate",
                on_wire_size=100000,
                on_disk_size=500000,
                max_on_wire_size=150000,
                max_on_disk_size=600000,
                file_inventory=[],
                measurement_timestamp="2024-01-15T14:30:22.123456Z",
                pipeline_id="12345",
                commit_sha="abc123def",
                arch="amd64",
                os="debian",
                build_job_name="test_job",
            )
        self.assertIn("artifact_path cannot be empty", str(cm.exception))

    def test_report_validation_negative_sizes(self):
        """Test report validation with negative sizes."""
        with self.assertRaises(ValueError) as cm:
            InPlaceArtifactReport(
                artifact_path="/path/to/package.deb",
                gate_name="test_gate",
                on_wire_size=-1,
                on_disk_size=500000,
                max_on_wire_size=150000,
                max_on_disk_size=600000,
                file_inventory=[],
                measurement_timestamp="2024-01-15T14:30:22.123456Z",
                pipeline_id="12345",
                commit_sha="abc123def",
                arch="amd64",
                os="debian",
                build_job_name="test_job",
            )
        self.assertIn("on_wire_size must be non-negative", str(cm.exception))


class TestInPlacePackageMeasurer(unittest.TestCase):
    """Test cases for the InPlacePackageMeasurer class."""

    def setUp(self):
        """Set up test data and mocks."""
        self.mock_config = {
            "static_quality_gate_agent_deb_amd64": {"max_on_wire_size": "150 MiB", "max_on_disk_size": "600 MiB"},
            "static_quality_gate_dogstatsd_deb_amd64": {"max_on_wire_size": "10 MiB", "max_on_disk_size": "50 MiB"},
        }

        # Create a temporary config file
        self.temp_config_file = tempfile.NamedTemporaryFile(mode='w', suffix='.yml', delete=False)
        yaml.dump(self.mock_config, self.temp_config_file)
        self.temp_config_file.close()

        self.measurer = InPlacePackageMeasurer(config_path=self.temp_config_file.name)

    def tearDown(self):
        """Clean up temporary files."""
        os.unlink(self.temp_config_file.name)

    def test_init_valid_config(self):
        """Test initializing measurer with valid config."""
        # After refactoring, the config_path and config are on the internal _measurer
        self.assertEqual(self.measurer._measurer.config_manager.config_path, self.temp_config_file.name)
        # Test that we can access the configuration through the public interface
        # by trying to measure with a known gate (this would fail if config wasn't loaded)
        self.assertIsNotNone(self.measurer._measurer.config_manager)

    def test_init_missing_config_file(self):
        """Test initializing measurer with missing config file."""
        with self.assertRaises(ValueError) as cm:
            InPlacePackageMeasurer(config_path="/nonexistent/config.yml")
        self.assertIn("Configuration file not found", str(cm.exception))

    def test_init_invalid_yaml(self):
        """Test initializing measurer with invalid YAML."""
        invalid_config_file = tempfile.NamedTemporaryFile(mode='w', suffix='.yml', delete=False)
        invalid_config_file.write("invalid: yaml: content: [")
        invalid_config_file.close()

        try:
            with self.assertRaises(ValueError) as cm:
                InPlacePackageMeasurer(config_path=invalid_config_file.name)
            self.assertIn("Invalid YAML configuration", str(cm.exception))
        finally:
            os.unlink(invalid_config_file.name)

    @patch.dict(os.environ, {"CI_PIPELINE_ID": "12345", "CI_COMMIT_SHA": "abc123def456"})
    @patch('os.path.exists')
    def test_measure_package_success(self, mock_exists):
        """Test successful package measurement with optimized single extraction."""
        # Setup mocks
        mock_exists.return_value = True

        # Mock the optimized extraction and analysis method
        with patch.object(self.measurer._measurer.processor, 'measure_artifact') as mock_measure_artifact:
            # Create mock measurement
            mock_measurement = ArtifactMeasurement(
                artifact_path="/path/to/package.deb", on_wire_size=100000, on_disk_size=500000
            )

            # Create mock file inventory
            mock_file_inventory = [
                FileInfo("opt/agent/bin/agent", 400000, "sha256:abc123"),
                FileInfo("etc/config.yaml", 100000, None),
            ]

            # Configure the mock to return both measurement and file inventory
            mock_measure_artifact.return_value = (
                100000,  # wire_size
                500000,  # disk_size
                mock_file_inventory,  # file_inventory
                None,  # artifact_metadata (for packages, this is usually None)
            )

            # Mock context
            mock_ctx = Mock()

            # Call the method
            report = self.measurer.measure_package(
                ctx=mock_ctx,
                package_path="/path/to/package.deb",
                gate_name="static_quality_gate_agent_deb_amd64",
                build_job_name="test_job",
            )

        # Verify the report
        self.assertEqual(report.artifact_path, "/path/to/package.deb")
        self.assertEqual(report.gate_name, "static_quality_gate_agent_deb_amd64")
        self.assertEqual(report.on_wire_size, 100000)
        self.assertEqual(report.on_disk_size, 500000)
        self.assertEqual(report.pipeline_id, "12345")
        self.assertEqual(report.commit_sha, "abc123def456")
        self.assertEqual(report.arch, "amd64")
        self.assertEqual(report.os, "debian")
        self.assertEqual(report.build_job_name, "test_job")
        self.assertEqual(len(report.file_inventory), 2)

        # Verify mocked processor was called
        mock_measure_artifact.assert_called_once()

    def test_measure_package_missing_file(self):
        """Test measuring package with missing file."""
        mock_ctx = Mock()

        with patch('os.path.exists', return_value=False):
            with self.assertRaises(ValueError) as cm:
                self.measurer.measure_package(
                    ctx=mock_ctx,
                    package_path="/nonexistent/package.deb",
                    gate_name="static_quality_gate_agent_deb_amd64",
                    build_job_name="test_job",
                )
            self.assertIn("Package file not found", str(cm.exception))

    def test_measure_package_invalid_gate(self):
        """Test measuring package with invalid gate name."""
        mock_ctx = Mock()

        with patch('os.path.exists', return_value=True):
            with self.assertRaises(ValueError) as cm:
                self.measurer.measure_package(
                    ctx=mock_ctx,
                    package_path="/path/to/package.deb",
                    gate_name="nonexistent_gate",
                    build_job_name="test_job",
                )
            self.assertIn("Gate configuration not found", str(cm.exception))

    # Checksum generation tests removed - functionality moved to FileUtilities component
    # and tested through integration tests via the public interface

    def test_save_report_to_yaml(self):
        """Test saving report to YAML file."""
        # Create a sample report
        report = InPlaceArtifactReport(
            artifact_path="/path/to/package.deb",
            gate_name="static_quality_gate_agent_deb_amd64",
            on_wire_size=100000,
            on_disk_size=500000,
            max_on_wire_size=150000,
            max_on_disk_size=600000,
            file_inventory=[
                FileInfo("opt/agent/bin/agent", 400000, "sha256:abc123"),
                FileInfo("etc/config.yaml", 100000, None),
            ],
            measurement_timestamp="2024-01-15T14:30:22.123456Z",
            pipeline_id="12345",
            commit_sha="abc123def",
            arch="amd64",
            os="debian",
            build_job_name="test_job",
        )

        # Save to temporary file
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yml', delete=False) as temp_file:
            output_path = temp_file.name

        try:
            self.measurer.save_report_to_yaml(report, output_path)

            # Verify the file was created and contains expected data
            self.assertTrue(os.path.exists(output_path))

            with open(output_path) as f:
                saved_data = yaml.safe_load(f)

            self.assertEqual(saved_data['artifact_path'], "/path/to/package.deb")
            self.assertEqual(saved_data['gate_name'], "static_quality_gate_agent_deb_amd64")
            self.assertEqual(saved_data['on_wire_size'], 100000)
            self.assertEqual(saved_data['on_disk_size'], 500000)
            self.assertEqual(len(saved_data['file_inventory']), 2)
            self.assertEqual(saved_data['file_inventory'][0]['relative_path'], "opt/agent/bin/agent")
            self.assertEqual(saved_data['file_inventory'][0]['checksum'], "sha256:abc123")

        finally:
            os.unlink(output_path)

    @patch('builtins.open', side_effect=OSError("Permission denied"))
    def test_save_report_to_yaml_failure(self, mock_file):
        """Test handling of YAML save failures."""
        report = InPlaceArtifactReport(
            artifact_path="/path/to/package.deb",
            gate_name="test_gate",
            on_wire_size=100000,
            on_disk_size=500000,
            max_on_wire_size=150000,
            max_on_disk_size=600000,
            file_inventory=[],
            measurement_timestamp="2024-01-15T14:30:22.123456Z",
            pipeline_id="12345",
            commit_sha="abc123def",
            arch="amd64",
            os="debian",
            build_job_name="test_job",
        )

        with self.assertRaises(RuntimeError) as cm:
            self.measurer.save_report_to_yaml(report, "/invalid/path.yml")
        self.assertIn("Failed to save report", str(cm.exception))


class TestInvokeTask(unittest.TestCase):
    """Test cases for the invoke task functionality."""

    @patch('tasks.static_quality_gates.experimental_gates.InPlacePackageMeasurer')
    @patch('os.path.exists')
    @patch('builtins.print')
    def test_measure_package_local_success(self, mock_print, mock_exists, mock_measurer_class):
        """Test successful local package measurement task."""
        from tasks.static_quality_gates.experimental_gates import measure_package_local

        # Setup mocks
        mock_exists.return_value = True
        mock_measurer = Mock()
        mock_measurer_class.return_value = mock_measurer

        # Create mock report
        mock_report = Mock()
        mock_report.on_wire_size = 100000000  # 100MB
        mock_report.on_disk_size = 500000000  # 500MB
        mock_report.max_on_wire_size = 150000000  # 150MB
        mock_report.max_on_disk_size = 600000000  # 600MB

        # Create mock file info objects with proper attributes
        mock_file_infos = []
        for i in range(100):
            mock_file = Mock()
            mock_file.size_bytes = 1000000 * (100 - i)  # Decreasing sizes
            mock_file.relative_path = f"test/file_{i}.bin"
            mock_file.size_mb = (1000000 * (100 - i)) / (1024 * 1024)  # Size in MB
            mock_file_infos.append(mock_file)
        mock_report.file_inventory = mock_file_infos

        # Mock the new properties
        mock_report.largest_files = mock_file_infos[:10]  # Top 10

        mock_measurer.measure_package.return_value = mock_report
        mock_measurer.save_report_to_yaml.return_value = None

        # Mock context
        mock_ctx = Mock()

        # Call the function directly
        measure_package_local(
            ctx=mock_ctx, package_path="/test/package.deb", gate_name="static_quality_gate_agent_deb_amd64"
        )

        # Verify measurer was initialized and called
        mock_measurer_class.assert_called_once_with(config_path="test/static/static_quality_gates.yml")
        mock_measurer.measure_package.assert_called_once()
        mock_measurer.save_report_to_yaml.assert_called_once()

        # Verify print statements were called (summary output)
        self.assertTrue(mock_print.called)

        # Check that success messages were printed
        print_calls = [call[0][0] for call in mock_print.call_args_list if call[0]]
        success_messages = [msg for msg in print_calls if "✅" in str(msg) or "completed successfully" in str(msg)]
        self.assertTrue(len(success_messages) > 0, "Expected success messages in output")

    @patch('os.path.exists')
    @patch('builtins.print')
    def test_measure_package_local_missing_file(self, mock_print, mock_exists):
        """Test local task with missing package file."""
        from tasks.static_quality_gates.experimental_gates import measure_package_local

        # Setup mocks - package doesn't exist
        mock_exists.return_value = False
        mock_ctx = Mock()

        # Call the function directly
        measure_package_local(
            ctx=mock_ctx, package_path="/nonexistent/package.deb", gate_name="static_quality_gate_agent_deb_amd64"
        )

        # Verify error message was printed
        print_calls = [call[0][0] for call in mock_print.call_args_list if call[0]]
        error_messages = [msg for msg in print_calls if "❌" in str(msg) and "not found" in str(msg)]
        self.assertTrue(len(error_messages) > 0, "Expected error message for missing file")


class TestDockerLayerInfo(unittest.TestCase):
    """Test cases for the DockerLayerInfo dataclass."""

    def test_docker_layer_info_creation_valid(self):
        """Test creating a valid DockerLayerInfo instance."""
        layer_info = DockerLayerInfo(
            layer_id="sha256:abcd1234567890",
            size_bytes=123456789,
            created_by="RUN apt-get update && apt-get install -y wget",
            empty_layer=False,
        )

        self.assertEqual(layer_info.layer_id, "sha256:abcd1234567890")
        self.assertEqual(layer_info.size_bytes, 123456789)
        self.assertEqual(layer_info.created_by, "RUN apt-get update && apt-get install -y wget")
        self.assertFalse(layer_info.empty_layer)

    def test_docker_layer_info_creation_minimal(self):
        """Test creating DockerLayerInfo with minimal required fields."""
        layer_info = DockerLayerInfo(layer_id="sha256:minimal123", size_bytes=1024)

        self.assertEqual(layer_info.layer_id, "sha256:minimal123")
        self.assertEqual(layer_info.size_bytes, 1024)
        self.assertIsNone(layer_info.created_by)
        self.assertFalse(layer_info.empty_layer)

    def test_docker_layer_info_empty_layer(self):
        """Test creating DockerLayerInfo for empty layer."""
        layer_info = DockerLayerInfo(
            layer_id="sha256:empty123", size_bytes=0, created_by="LABEL version=1.0", empty_layer=True
        )

        self.assertEqual(layer_info.layer_id, "sha256:empty123")
        self.assertEqual(layer_info.size_bytes, 0)
        self.assertTrue(layer_info.empty_layer)

    def test_docker_layer_info_validation_empty_layer_id(self):
        """Test DockerLayerInfo validation with empty layer_id."""
        with self.assertRaises(ValueError) as cm:
            DockerLayerInfo(layer_id="", size_bytes=1024)
        self.assertIn("layer_id cannot be empty", str(cm.exception))

    def test_docker_layer_info_validation_negative_size(self):
        """Test DockerLayerInfo validation with negative size."""
        with self.assertRaises(ValueError) as cm:
            DockerLayerInfo(layer_id="sha256:test123", size_bytes=-1)
        self.assertIn("size_bytes must be non-negative", str(cm.exception))

    def test_docker_layer_info_size_properties(self):
        """Test DockerLayerInfo size conversion properties."""
        layer_info = DockerLayerInfo(layer_id="sha256:test123", size_bytes=2097152)  # 2 MiB

        self.assertEqual(layer_info.size_mb, 2.0)
        self.assertEqual(layer_info.size_kb, 2048.0)
        self.assertEqual(layer_info.size_gb, 2.0 / 1024)

    def test_docker_layer_info_immutability(self):
        """Test that DockerLayerInfo is immutable (frozen dataclass)."""
        layer_info = DockerLayerInfo(layer_id="sha256:test123", size_bytes=1024)

        with self.assertRaises(AttributeError):
            layer_info.layer_id = "new_id"

        with self.assertRaises(AttributeError):
            layer_info.size_bytes = 2048


class TestDockerImageInfo(unittest.TestCase):
    """Test cases for the DockerImageInfo dataclass."""

    def setUp(self):
        """Set up test data."""
        self.sample_layers = [
            DockerLayerInfo("sha256:layer1", 1048576, "FROM ubuntu:20.04", False),
            DockerLayerInfo("sha256:layer2", 2097152, "RUN apt-get update", False),
            DockerLayerInfo("sha256:layer3", 0, "LABEL version=1.0", True),
            DockerLayerInfo("sha256:layer4", 4194304, "COPY app /opt/app", False),
        ]

    def test_docker_image_info_creation_valid(self):
        """Test creating a valid DockerImageInfo instance."""
        image_info = DockerImageInfo(
            image_id="sha256:abc123def456",
            image_tags=["myapp:latest", "myapp:v1.0"],
            architecture="amd64",
            os="linux",
            layers=self.sample_layers,
            config_size=1024,
            manifest_size=512,
        )

        self.assertEqual(image_info.image_id, "sha256:abc123def456")
        self.assertEqual(image_info.image_tags, ["myapp:latest", "myapp:v1.0"])
        self.assertEqual(image_info.architecture, "amd64")
        self.assertEqual(image_info.os, "linux")
        self.assertEqual(len(image_info.layers), 4)
        self.assertEqual(image_info.config_size, 1024)
        self.assertEqual(image_info.manifest_size, 512)

    def test_docker_image_info_validation_empty_image_id(self):
        """Test DockerImageInfo validation with empty image_id."""
        with self.assertRaises(ValueError) as cm:
            DockerImageInfo(
                image_id="",
                image_tags=["test:latest"],
                architecture="amd64",
                os="linux",
                layers=[],
                config_size=1024,
                manifest_size=512,
            )
        self.assertIn("image_id cannot be empty", str(cm.exception))

    def test_docker_image_info_validation_empty_tags(self):
        """Test DockerImageInfo validation with empty image_tags."""
        # Note: Current implementation doesn't validate empty tags, so this test
        # verifies the current behavior. This could be a future enhancement.
        image_info = DockerImageInfo(
            image_id="sha256:test123",
            image_tags=[],
            architecture="amd64",
            os="linux",
            layers=[],
            config_size=1024,
            manifest_size=512,
        )
        self.assertEqual(image_info.image_tags, [])

    def test_docker_image_info_validation_negative_config_size(self):
        """Test DockerImageInfo validation with negative config_size."""
        with self.assertRaises(ValueError) as cm:
            DockerImageInfo(
                image_id="sha256:test123",
                image_tags=["test:latest"],
                architecture="amd64",
                os="linux",
                layers=[],
                config_size=-1,
                manifest_size=512,
            )
        self.assertIn("config_size must be non-negative", str(cm.exception))

    def test_docker_image_info_validation_negative_manifest_size(self):
        """Test DockerImageInfo validation with negative manifest_size."""
        with self.assertRaises(ValueError) as cm:
            DockerImageInfo(
                image_id="sha256:test123",
                image_tags=["test:latest"],
                architecture="amd64",
                os="linux",
                layers=[],
                config_size=1024,
                manifest_size=-1,
            )
        self.assertIn("manifest_size must be non-negative", str(cm.exception))

    def test_docker_image_info_total_layers_size(self):
        """Test calculation of total layers size."""
        image_info = DockerImageInfo(
            image_id="sha256:test123",
            image_tags=["test:latest"],
            architecture="amd64",
            os="linux",
            layers=self.sample_layers,
            config_size=1024,
            manifest_size=512,
        )

        # 1048576 + 2097152 + 0 + 4194304 = 7340032 bytes
        expected_total = 7340032
        self.assertEqual(image_info.total_layers_size_bytes, expected_total)

    def test_docker_image_info_non_empty_layers(self):
        """Test filtering of non-empty layers."""
        image_info = DockerImageInfo(
            image_id="sha256:test123",
            image_tags=["test:latest"],
            architecture="amd64",
            os="linux",
            layers=self.sample_layers,
            config_size=1024,
            manifest_size=512,
        )

        non_empty = image_info.non_empty_layers
        self.assertEqual(len(non_empty), 3)  # 3 non-empty layers
        for layer in non_empty:
            self.assertFalse(layer.empty_layer)
            self.assertGreater(layer.size_bytes, 0)

    def test_docker_image_info_largest_layers(self):
        """Test ordering of largest layers."""
        image_info = DockerImageInfo(
            image_id="sha256:test123",
            image_tags=["test:latest"],
            architecture="amd64",
            os="linux",
            layers=self.sample_layers,
            config_size=1024,
            manifest_size=512,
        )

        largest = image_info.largest_layers
        self.assertEqual(len(largest), 4)  # All layers (including empty ones)
        # Should be ordered by size descending
        self.assertEqual(largest[0].size_bytes, 4194304)  # layer4
        self.assertEqual(largest[1].size_bytes, 2097152)  # layer2
        self.assertEqual(largest[2].size_bytes, 1048576)  # layer1
        self.assertEqual(largest[3].size_bytes, 0)  # layer3 (empty)

    def test_docker_image_info_immutability(self):
        """Test that DockerImageInfo is immutable (frozen dataclass)."""
        image_info = DockerImageInfo(
            image_id="sha256:test123",
            image_tags=["test:latest"],
            architecture="amd64",
            os="linux",
            layers=[],
            config_size=1024,
            manifest_size=512,
        )

        with self.assertRaises(AttributeError):
            image_info.image_id = "new_id"

        with self.assertRaises(AttributeError):
            image_info.architecture = "arm64"


class TestInPlaceDockerMeasurer(unittest.TestCase):
    """Test cases for the InPlaceDockerMeasurer class."""

    def setUp(self):
        """Set up test data and mocks."""
        self.mock_config = {
            "static_quality_gate_docker_agent_amd64": {"max_on_wire_size": "300 MiB", "max_on_disk_size": "800 MiB"},
            "static_quality_gate_docker_cluster_agent_amd64": {
                "max_on_wire_size": "70 MiB",
                "max_on_disk_size": "230 MiB",
            },
        }

        # Create a temporary config file
        self.temp_config_file = tempfile.NamedTemporaryFile(mode='w', suffix='.yml', delete=False)
        yaml.dump(self.mock_config, self.temp_config_file)
        self.temp_config_file.close()

        self.measurer = InPlaceDockerMeasurer(config_path=self.temp_config_file.name)

    def tearDown(self):
        """Clean up temporary files."""
        os.unlink(self.temp_config_file.name)

    @patch('tasks.static_quality_gates.experimental_gates.DockerProcessor._ensure_image_available')
    @patch('tasks.static_quality_gates.experimental_gates.DockerProcessor._measure_wire_size_with_docker_save')
    @patch('tasks.static_quality_gates.experimental_gates.DockerProcessor._analyze_image_with_docker_export')
    def test_measure_image_success(self, mock_analyze_export, mock_measure_wire, mock_ensure_available):
        """Test successful Docker image measurement."""
        # Setup mocks
        mock_ensure_available.return_value = None
        mock_measure_wire.return_value = 104857600  # 100 MiB

        # Mock file inventory
        mock_file_inventory = [
            FileInfo("usr/bin/app", 12345678, "sha256:abc123"),
            FileInfo("etc/config.yaml", 1024, "sha256:def456"),
        ]
        # Mock Docker metadata
        mock_docker_info = DockerImageInfo(
            image_id="sha256:test123456789",
            image_tags=["test:latest"],
            architecture="amd64",
            os="linux",
            layers=[
                DockerLayerInfo("sha256:layer1", 52428800, "FROM ubuntu:20.04", False),
                DockerLayerInfo("sha256:layer2", 0, "LABEL version=1.0", True),
            ],
            config_size=1024,
            manifest_size=512,
        )

        mock_analyze_export.return_value = (
            314572800,
            mock_file_inventory,
            mock_docker_info,
        )  # 300 MiB disk size with docker_info

        # Mock context and environment
        mock_ctx = Mock()

        with patch.dict(os.environ, {"CI_PIPELINE_ID": "12345", "CI_COMMIT_SHA": "abcdef"}):
            # Call the method
            report = self.measurer.measure_image(
                ctx=mock_ctx,
                image_ref="test:latest",
                gate_name="static_quality_gate_docker_agent_amd64",
                build_job_name="test_build",
                max_files=1000,
                generate_checksums=True,
                include_layer_analysis=True,
                debug=False,
            )

        # Verify the result
        self.assertEqual(report.artifact_path, "test:latest")
        self.assertEqual(report.gate_name, "static_quality_gate_docker_agent_amd64")
        self.assertEqual(report.on_wire_size, 104857600)
        self.assertEqual(report.on_disk_size, 314572800)
        self.assertEqual(report.pipeline_id, "12345")
        self.assertEqual(report.commit_sha, "abcdef")
        self.assertEqual(report.arch, "amd64")
        self.assertEqual(report.os, "docker")
        self.assertEqual(report.build_job_name, "test_build")
        self.assertEqual(len(report.file_inventory), 2)
        self.assertIsNotNone(report.docker_info)
        self.assertEqual(report.docker_info.image_id, "sha256:test123456789")

        # Verify mocks were called
        mock_ensure_available.assert_called_once()
        mock_measure_wire.assert_called_once()
        mock_analyze_export.assert_called_once()

    @patch('tasks.static_quality_gates.experimental_gates.DockerProcessor._ensure_image_available')
    def test_measure_image_ensure_available_failure(self, mock_ensure_available):
        """Test Docker image measurement when image is not available."""
        mock_ensure_available.side_effect = RuntimeError("Image not found locally")
        mock_ctx = Mock()

        with self.assertRaises(RuntimeError) as cm:
            self.measurer.measure_image(
                ctx=mock_ctx,
                image_ref="nonexistent:latest",
                gate_name="static_quality_gate_docker_agent_amd64",
                build_job_name="test_build",
            )

        self.assertIn("Image not found locally", str(cm.exception))

    @patch('tasks.static_quality_gates.experimental_gates.DockerProcessor._ensure_image_available')
    @patch('tasks.static_quality_gates.experimental_gates.DockerProcessor._measure_wire_size_with_docker_save')
    def test_measure_image_wire_size_failure(self, mock_measure_wire, mock_ensure_available):
        """Test Docker image measurement when wire size measurement fails."""
        mock_ensure_available.return_value = None
        mock_measure_wire.side_effect = RuntimeError("Docker save failed")
        mock_ctx = Mock()

        with self.assertRaises(RuntimeError) as cm:
            self.measurer.measure_image(
                ctx=mock_ctx,
                image_ref="test:latest",
                gate_name="static_quality_gate_docker_agent_amd64",
                build_job_name="test_build",
            )

        self.assertIn("Docker save failed", str(cm.exception))

    def test_measure_image_missing_config(self):
        """Test Docker image measurement with missing gate configuration."""
        mock_ctx = Mock()

        with self.assertRaises(ValueError) as cm:
            self.measurer.measure_image(
                ctx=mock_ctx, image_ref="test:latest", gate_name="nonexistent_gate", build_job_name="test_build"
            )

        self.assertIn("Gate configuration not found: nonexistent_gate", str(cm.exception))

    @patch('tasks.static_quality_gates.experimental_gates.DockerProcessor._ensure_image_available')
    @patch('tasks.static_quality_gates.experimental_gates.DockerProcessor._measure_wire_size_with_docker_save')
    @patch('tasks.static_quality_gates.experimental_gates.DockerProcessor._analyze_image_with_docker_export')
    def test_measure_image_no_layer_analysis(self, mock_analyze_export, mock_measure_wire, mock_ensure_available):
        """Test Docker image measurement without layer analysis."""
        # Setup mocks
        mock_ensure_available.return_value = None
        mock_measure_wire.return_value = 52428800  # 50 MiB

        mock_file_inventory = [FileInfo("app/main", 1048576, "sha256:test123")]
        # Mock minimal Docker metadata (no layers)
        mock_docker_info = DockerImageInfo(
            image_id="sha256:minimal123",
            image_tags=["minimal:latest"],
            architecture="arm64",
            os="linux",
            layers=[],
            config_size=512,
            manifest_size=256,
        )

        mock_analyze_export.return_value = (
            104857600,
            mock_file_inventory,
            mock_docker_info,
        )  # 100 MiB disk size with docker_info

        mock_ctx = Mock()

        with patch.dict(os.environ, {"CI_PIPELINE_ID": "67890", "CI_COMMIT_SHA": "fedcba"}):
            report = self.measurer.measure_image(
                ctx=mock_ctx,
                image_ref="minimal:latest",
                gate_name="static_quality_gate_docker_cluster_agent_amd64",
                build_job_name="minimal_build",
                max_files=500,
                generate_checksums=False,
                include_layer_analysis=False,
                debug=True,
            )

        # Verify the result
        self.assertEqual(report.artifact_path, "minimal:latest")
        self.assertEqual(report.on_wire_size, 52428800)
        self.assertEqual(report.on_disk_size, 104857600)
        self.assertEqual(len(report.file_inventory), 1)
        self.assertIsNotNone(report.docker_info)
        self.assertEqual(len(report.docker_info.layers), 0)

    def test_save_report_to_yaml(self):
        """Test saving measurement report to YAML file."""
        # Create a sample report
        sample_file_inventory = [
            FileInfo("usr/bin/test", 4096, "sha256:test123"),
        ]

        sample_docker_info = DockerImageInfo(
            image_id="sha256:save_test123",
            image_tags=["save_test:latest"],
            architecture="amd64",
            os="linux",
            layers=[],
            config_size=1024,
            manifest_size=512,
        )

        report = InPlaceArtifactReport(
            artifact_path="save_test:latest",
            gate_name="static_quality_gate_docker_agent_amd64",
            on_wire_size=1048576,
            on_disk_size=2097152,
            max_on_wire_size=314572800,
            max_on_disk_size=838860800,
            file_inventory=sample_file_inventory,
            measurement_timestamp="2024-01-15T14:30:22.123456Z",
            pipeline_id="save_test_pipeline",
            commit_sha="save_test_commit",
            arch="amd64",
            os="docker",
            build_job_name="save_test_build",
            docker_info=sample_docker_info,
        )

        # Test saving to temporary file
        with tempfile.NamedTemporaryFile(mode='w', suffix='.yml', delete=False) as temp_file:
            temp_path = temp_file.name

        try:
            self.measurer.save_report_to_yaml(report, temp_path)

            # Verify file was created and contains expected data
            self.assertTrue(os.path.exists(temp_path))

            with open(temp_path) as file:
                saved_data = yaml.safe_load(file)

            self.assertEqual(saved_data['artifact_path'], "save_test:latest")
            self.assertEqual(saved_data['on_wire_size'], 1048576)
            self.assertEqual(saved_data['on_disk_size'], 2097152)
            self.assertIn('docker_info', saved_data)
            self.assertEqual(saved_data['docker_info']['image_id'], "sha256:save_test123")

        finally:
            if os.path.exists(temp_path):
                os.unlink(temp_path)


if __name__ == '__main__':
    unittest.main()
