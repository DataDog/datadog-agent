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

from tasks.static_quality_gates.experimental import (
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
        self.assertIn("size_bytes must be positive", str(cm.exception))


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
        self.assertIn("on_wire_size must be positive", str(cm.exception))


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
            _ = ArtifactMeasurement(artifact_path="/path/to/package.deb", on_wire_size=100000, on_disk_size=500000)

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

    @patch('tasks.static_quality_gates.experimental.tasks.package.InPlacePackageMeasurer')
    @patch('os.path.exists')
    @patch('builtins.print')
    def test_measure_package_local_success(self, mock_print, mock_exists, mock_measurer_class):
        """Test successful local package measurement task."""
        from tasks.static_quality_gates.experimental.tasks.package import measure_package_local

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
        from tasks.static_quality_gates.experimental.tasks.package import measure_package_local

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
        self.assertIn("size_bytes must be positive", str(cm.exception))

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
            image_ref="sha256:abc123def456",
            architecture="amd64",
            os="linux",
            layers=self.sample_layers,
            config_size=1024,
            manifest_size=512,
        )

        self.assertEqual(image_info.image_ref, "sha256:abc123def456")
        self.assertEqual(image_info.architecture, "amd64")
        self.assertEqual(image_info.os, "linux")
        self.assertEqual(len(image_info.layers), 4)
        self.assertEqual(image_info.config_size, 1024)
        self.assertEqual(image_info.manifest_size, 512)

    def test_docker_image_info_validation_empty_image_id(self):
        """Test DockerImageInfo validation with empty image_id."""
        with self.assertRaises(ValueError) as cm:
            DockerImageInfo(
                image_ref="",
                architecture="amd64",
                os="linux",
                layers=[],
                config_size=1024,
                manifest_size=512,
            )
        self.assertIn("image_ref cannot be empty", str(cm.exception))

    def test_docker_image_info_validation_negative_config_size(self):
        """Test DockerImageInfo validation with negative config_size."""
        with self.assertRaises(ValueError) as cm:
            DockerImageInfo(
                image_ref="sha256:test123",
                architecture="amd64",
                os="linux",
                layers=[],
                config_size=-1,
                manifest_size=512,
            )
        self.assertIn("config_size must be positive", str(cm.exception))

    def test_docker_image_info_validation_negative_manifest_size(self):
        """Test DockerImageInfo validation with negative manifest_size."""
        with self.assertRaises(ValueError) as cm:
            DockerImageInfo(
                image_ref="sha256:test123",
                architecture="amd64",
                os="linux",
                layers=[],
                config_size=1024,
                manifest_size=-1,
            )
        self.assertIn("manifest_size must be positive", str(cm.exception))

    def test_docker_image_info_total_layers_size(self):
        """Test calculation of total layers size."""
        image_info = DockerImageInfo(
            image_ref="sha256:test123",
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
            image_ref="sha256:test123",
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
            image_ref="sha256:test123",
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
            image_ref="sha256:test123",
            architecture="amd64",
            os="linux",
            layers=[],
            config_size=1024,
            manifest_size=512,
        )

        with self.assertRaises(AttributeError):
            image_info.image_ref = "new_id"

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

    @patch('tasks.static_quality_gates.experimental.processors.docker.DockerProcessor._ensure_image_available')
    @patch('tasks.static_quality_gates.experimental.processors.docker.DockerProcessor._get_wire_size')
    @patch('tasks.static_quality_gates.experimental.processors.docker.DockerProcessor._measure_on_disk_size')
    def test_measure_image_success(self, mock_measure_disk, mock_get_wire_size, mock_ensure_available):
        """Test successful Docker image measurement."""
        # Setup mocks
        mock_ensure_available.return_value = None
        mock_get_wire_size.return_value = 104857600  # 100 MiB

        # Mock file inventory
        mock_file_inventory = [
            FileInfo("usr/bin/app", 12345678, "sha256:abc123"),
            FileInfo("etc/config.yaml", 1024, "sha256:def456"),
        ]
        # Mock Docker metadata
        mock_docker_info = DockerImageInfo(
            image_ref="sha256:test123456789",
            architecture="amd64",
            os="linux",
            layers=[
                DockerLayerInfo("sha256:layer1", 52428800, "FROM ubuntu:20.04", False),
                DockerLayerInfo("sha256:layer2", 0, "LABEL version=1.0", True),
            ],
            config_size=1024,
            manifest_size=512,
        )

        mock_measure_disk.return_value = (
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
        self.assertEqual(report.docker_info.image_ref, "sha256:test123456789")

        # Verify mocks were called
        mock_ensure_available.assert_called_once()
        mock_get_wire_size.assert_called_once()
        mock_measure_disk.assert_called_once()

    @patch('tasks.static_quality_gates.experimental.processors.docker.DockerProcessor._ensure_image_available')
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

    @patch('tasks.static_quality_gates.experimental.processors.docker.DockerProcessor._ensure_image_available')
    @patch('tasks.static_quality_gates.experimental.processors.docker.DockerProcessor._get_wire_size')
    def test_measure_image_wire_size_failure(self, mock_get_wire_size, mock_ensure_available):
        """Test Docker image measurement when wire size measurement fails."""
        mock_ensure_available.return_value = None
        mock_get_wire_size.side_effect = RuntimeError("Docker manifest inspect failed")
        mock_ctx = Mock()

        with self.assertRaises(RuntimeError) as cm:
            self.measurer.measure_image(
                ctx=mock_ctx,
                image_ref="test:latest",
                gate_name="static_quality_gate_docker_agent_amd64",
                build_job_name="test_build",
            )

        self.assertIn("Docker manifest inspect failed", str(cm.exception))

    def test_measure_image_missing_config(self):
        """Test Docker image measurement with missing gate configuration."""
        mock_ctx = Mock()

        with self.assertRaises(ValueError) as cm:
            self.measurer.measure_image(
                ctx=mock_ctx, image_ref="test:latest", gate_name="nonexistent_gate", build_job_name="test_build"
            )

        self.assertIn("Gate configuration not found: nonexistent_gate", str(cm.exception))

    @patch('tasks.static_quality_gates.experimental.processors.docker.DockerProcessor._ensure_image_available')
    @patch('tasks.static_quality_gates.experimental.processors.docker.DockerProcessor._get_wire_size')
    @patch('tasks.static_quality_gates.experimental.processors.docker.DockerProcessor._measure_on_disk_size')
    def test_measure_image_no_layer_analysis(self, mock_measure_disk, mock_get_wire_size, mock_ensure_available):
        """Test Docker image measurement without layer analysis."""
        # Setup mocks
        mock_ensure_available.return_value = None
        mock_get_wire_size.return_value = 52428800  # 50 MiB

        mock_file_inventory = [FileInfo("app/main", 1048576, "sha256:test123")]
        # Mock minimal Docker metadata (no layers)
        mock_docker_info = DockerImageInfo(
            image_ref="sha256:minimal123",
            architecture="arm64",
            os="linux",
            layers=[],
            config_size=512,
            manifest_size=256,
        )

        mock_measure_disk.return_value = (
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
            image_ref="sha256:save_test123",
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
            self.assertEqual(saved_data['docker_info']['image_ref'], "sha256:save_test123")

        finally:
            if os.path.exists(temp_path):
                os.unlink(temp_path)


class TestInPlaceMsiMeasurer(unittest.TestCase):
    """Test cases for the InPlaceMsiMeasurer class."""

    def setUp(self):
        """Set up test data and mocks."""
        self.mock_config = {
            "static_quality_gate_agent_msi": {"max_on_wire_size": "160 MiB", "max_on_disk_size": "1000 MiB"},
        }

        # Create a temporary config file
        self.temp_config_file = tempfile.NamedTemporaryFile(mode='w', suffix='.yml', delete=False)
        yaml.dump(self.mock_config, self.temp_config_file)
        self.temp_config_file.close()

    def tearDown(self):
        """Clean up temporary files."""
        os.unlink(self.temp_config_file.name)

    @patch('tasks.static_quality_gates.experimental.processors.msi.MsiProcessor._find_msi_artifacts')
    @patch('tasks.static_quality_gates.experimental.processors.msi.MsiProcessor._extract_zip')
    @patch('tasks.static_quality_gates.experimental.common.utils.FileUtilities.calculate_directory_size')
    @patch('tasks.static_quality_gates.experimental.common.utils.FileUtilities.walk_files')
    @patch('os.path.getsize')
    def test_measure_msi_success(
        self, mock_getsize, mock_walk_files, mock_calc_size, mock_extract_zip, mock_find_artifacts
    ):
        """Test successful MSI measurement using ZIP+MSI approach."""
        from tasks.static_quality_gates.experimental import InPlaceMsiMeasurer

        # Setup mocks
        mock_find_artifacts.return_value = ("/path/to/package.zip", "/path/to/package.msi")
        mock_getsize.return_value = 150000000  # 150MB MSI file
        mock_calc_size.return_value = 800000000  # 800MB extracted size

        # Mock file inventory
        mock_file_inventory = [
            FileInfo("opt/datadog-agent/bin/agent.exe", 50000000, "sha256:abc123"),
            FileInfo("etc/datadog-agent/datadog.yaml", 2048, "sha256:def456"),
        ]
        mock_walk_files.return_value = mock_file_inventory
        mock_extract_zip.return_value = None

        measurer = InPlaceMsiMeasurer(config_path=self.temp_config_file.name)
        mock_ctx = Mock()

        with patch.dict(os.environ, {"CI_PIPELINE_ID": "12345", "CI_COMMIT_SHA": "abc123def"}):
            report = measurer.measure_msi(
                ctx=mock_ctx,
                msi_ref="/path/to/package.msi",
                gate_name="static_quality_gate_agent_msi",
                build_job_name="msi_test_job",
                max_files=10000,
                generate_checksums=True,
                debug=False,
            )

        # Verify the report
        self.assertEqual(report.artifact_path, "/path/to/package.msi")
        self.assertEqual(report.gate_name, "static_quality_gate_agent_msi")
        self.assertEqual(report.on_wire_size, 150000000)  # MSI file size
        self.assertEqual(report.on_disk_size, 800000000)  # ZIP extraction size
        self.assertEqual(report.pipeline_id, "12345")
        self.assertEqual(report.commit_sha, "abc123def")
        self.assertEqual(report.arch, "amd64")  # MSI is always amd64
        self.assertEqual(report.os, "windows")
        self.assertEqual(report.build_job_name, "msi_test_job")
        self.assertEqual(len(report.file_inventory), 2)

        # Verify mocks were called
        mock_find_artifacts.assert_called_once()
        mock_extract_zip.assert_called_once()
        mock_calc_size.assert_called_once()
        mock_walk_files.assert_called_once()

    @patch('glob.glob')
    @patch('os.path.exists')
    def test_find_msi_artifacts_directory_success(self, mock_exists, mock_glob):
        """Test finding MSI and ZIP files in a directory."""
        from tasks.static_quality_gates.experimental.processors.msi import MsiProcessor

        # Setup mocks
        mock_glob.side_effect = [
            ["/path/to/datadog-agent-7.55.0-1-x86_64.zip"],  # ZIP files
            ["/path/to/datadog-agent-7.55.0-1-x86_64.msi"],  # MSI files
        ]
        mock_exists.return_value = True  # Files exist

        processor = MsiProcessor()
        zip_path, msi_path = processor._find_msi_artifacts("/path/to/packages", Mock(), debug=False)

        self.assertEqual(zip_path, "/path/to/datadog-agent-7.55.0-1-x86_64.zip")
        self.assertEqual(msi_path, "/path/to/datadog-agent-7.55.0-1-x86_64.msi")

    def test_find_msi_artifacts_direct_path(self):
        """Test finding MSI and ZIP files using direct MSI path."""
        from tasks.static_quality_gates.experimental.processors.msi import MsiProcessor

        processor = MsiProcessor()

        with patch('os.path.exists', return_value=True):
            zip_path, msi_path = processor._find_msi_artifacts("/path/to/package.msi", Mock(), debug=False)

        self.assertEqual(zip_path, "/path/to/package.zip")
        self.assertEqual(msi_path, "/path/to/package.msi")

    @patch('glob.glob')
    def test_find_msi_artifacts_missing_zip(self, mock_glob):
        """Test error handling when ZIP file is missing."""
        from tasks.static_quality_gates.experimental.processors.msi import MsiProcessor

        # Setup mocks - no ZIP files found
        mock_glob.side_effect = [
            [],  # No ZIP files
            ["/path/to/datadog-agent-7.55.0-1-x86_64.msi"],  # MSI files
        ]

        processor = MsiProcessor()

        with self.assertRaises(ValueError) as cm:
            processor._find_msi_artifacts("/path/to/packages", Mock(), debug=False)

        self.assertIn("Could not find ZIP file", str(cm.exception))

    @patch('glob.glob')
    def test_find_msi_artifacts_missing_msi(self, mock_glob):
        """Test error handling when MSI file is missing."""
        from tasks.static_quality_gates.experimental.processors.msi import MsiProcessor

        # Setup mocks - no MSI files found
        mock_glob.side_effect = [
            ["/path/to/datadog-agent-7.55.0-1-x86_64.zip"],  # ZIP files
            [],  # No MSI files
        ]

        processor = MsiProcessor()

        with self.assertRaises(ValueError) as cm:
            processor._find_msi_artifacts("/path/to/packages", Mock(), debug=False)

        self.assertIn("Could not find MSI file", str(cm.exception))

    def test_extract_zip_success(self):
        """Test successful ZIP extraction."""
        from tasks.static_quality_gates.experimental.processors.msi import MsiProcessor

        processor = MsiProcessor()
        mock_ctx = Mock()

        # Mock successful unzip command
        mock_result = Mock()
        mock_result.exited = 0
        mock_ctx.run.return_value = mock_result

        # Mock the context manager properly
        context_manager = Mock()
        context_manager.__enter__ = Mock(return_value=context_manager)
        context_manager.__exit__ = Mock(return_value=None)
        mock_ctx.cd.return_value = context_manager

        # Should not raise an exception
        processor._extract_zip(mock_ctx, "/path/to/test.zip", "/extract/dir", debug=False)

        # Verify unzip command was called
        mock_ctx.run.assert_called_once_with("unzip -q '/path/to/test.zip'", warn=True)

    def test_extract_zip_failure(self):
        """Test ZIP extraction failure handling."""
        from tasks.static_quality_gates.experimental.processors.msi import MsiProcessor

        processor = MsiProcessor()
        mock_ctx = Mock()

        # Mock failed unzip command
        mock_result = Mock()
        mock_result.exited = 1
        mock_ctx.run.return_value = mock_result

        # Mock the context manager properly
        context_manager = Mock()
        context_manager.__enter__ = Mock(return_value=context_manager)
        context_manager.__exit__ = Mock(return_value=None)  # noqa: U100
        mock_ctx.cd.return_value = context_manager

        with self.assertRaises(RuntimeError) as cm:
            processor._extract_zip(mock_ctx, "/path/to/test.zip", "/extract/dir", debug=False)

        self.assertIn("ZIP extraction failed", str(cm.exception))


class TestMsiInvokeTask(unittest.TestCase):
    """Test cases for the MSI invoke task functionality."""

    @patch('tasks.static_quality_gates.experimental.tasks.msi.InPlaceMsiMeasurer')
    @patch('os.path.exists')
    @patch('builtins.print')
    def test_measure_msi_local_success(self, mock_print, mock_exists, mock_measurer_class):
        """Test successful local MSI measurement task."""
        from tasks.static_quality_gates.experimental.tasks.msi import measure_msi_local

        # Setup mocks
        mock_exists.return_value = True
        mock_measurer = Mock()
        mock_measurer_class.return_value = mock_measurer

        # Create mock report
        mock_report = Mock()
        mock_report.on_wire_size = 150000000  # 150MB
        mock_report.on_disk_size = 800000000  # 800MB
        mock_report.max_on_wire_size = 160000000  # 160MB
        mock_report.max_on_disk_size = 1000000000  # 1000MB

        # Create mock file info objects
        mock_file_infos = []
        for i in range(50):
            mock_file = Mock()
            mock_file.size_bytes = 5000000 * (50 - i)  # Decreasing sizes
            mock_file.relative_path = f"Program Files/Datadog/Agent/file_{i}.exe"
            mock_file.size_mb = (5000000 * (50 - i)) / (1024 * 1024)
            mock_file_infos.append(mock_file)
        mock_report.file_inventory = mock_file_infos
        mock_report.largest_files = mock_file_infos[:10]  # Top 10

        mock_measurer.measure_msi.return_value = mock_report
        mock_measurer.save_report_to_yaml.return_value = None

        # Mock context
        mock_ctx = Mock()

        # Call the function
        measure_msi_local(ctx=mock_ctx, msi_path="/test/datadog-agent.msi", gate_name="static_quality_gate_agent_msi")

        # Verify measurer was initialized and called
        mock_measurer_class.assert_called_once_with(config_path="test/static/static_quality_gates.yml")
        mock_measurer.measure_msi.assert_called_once()
        mock_measurer.save_report_to_yaml.assert_called_once()

        # Verify success messages were printed
        print_calls = [call[0][0] for call in mock_print.call_args_list if call[0]]
        success_messages = [msg for msg in print_calls if "✅" in str(msg) or "completed successfully" in str(msg)]
        self.assertTrue(len(success_messages) > 0, "Expected success messages in output")

    @patch('os.path.exists')
    @patch('builtins.print')
    def test_measure_msi_local_missing_config(self, mock_print, mock_exists):
        """Test local MSI task with missing config file."""
        from tasks.static_quality_gates.experimental.tasks.msi import measure_msi_local

        # Setup mocks - config doesn't exist
        mock_exists.return_value = False
        mock_ctx = Mock()

        # Call the function
        measure_msi_local(ctx=mock_ctx, msi_path="/test/datadog-agent.msi", gate_name="static_quality_gate_agent_msi")

        # Verify error message was printed
        print_calls = [call[0][0] for call in mock_print.call_args_list if call[0]]
        error_messages = [msg for msg in print_calls if "❌" in str(msg) and "Configuration file not found" in str(msg)]
        self.assertTrue(len(error_messages) > 0, "Expected error message for missing config")


if __name__ == '__main__':
    unittest.main()
