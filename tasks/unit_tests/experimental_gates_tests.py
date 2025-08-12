"""
Unit tests for experimental static quality gates functionality.

These tests cover the InPlacePackageMeasurer and related classes for in-place
artifact measurement and report generation.
"""

import os
import tempfile
import unittest
import unittest.mock
from pathlib import Path
from unittest.mock import Mock, mock_open, patch

import yaml

from tasks.static_quality_gates.experimental_gates import (
    FileInfo,
    InPlaceArtifactReport,
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
            artifact_type="package",
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
            artifact_flavors=["agent"],
        )

        self.assertEqual(report.artifact_path, "/path/to/package.deb")
        self.assertEqual(report.on_wire_size, 100000)
        self.assertEqual(report.on_disk_size, 500000)
        self.assertEqual(len(report.file_inventory), 2)
        self.assertEqual(report.artifact_flavors, ["agent"])

    def test_report_validation_empty_path(self):
        """Test report validation with empty artifact_path."""
        with self.assertRaises(ValueError) as cm:
            InPlaceArtifactReport(
                artifact_path="",
                artifact_type="package",
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
                artifact_type="package",
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
        self.assertEqual(self.measurer.config_path, self.temp_config_file.name)
        self.assertIn("static_quality_gate_agent_deb_amd64", self.measurer.config)

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
        with patch.object(self.measurer, '_extract_and_analyze_package') as mock_extract_analyze:
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
            mock_extract_analyze.return_value = (mock_measurement, mock_file_inventory)

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
        self.assertEqual(report.artifact_type, "package")
        self.assertEqual(report.gate_name, "static_quality_gate_agent_deb_amd64")
        self.assertEqual(report.on_wire_size, 100000)
        self.assertEqual(report.on_disk_size, 500000)
        self.assertEqual(report.pipeline_id, "12345")
        self.assertEqual(report.commit_sha, "abc123def456")
        self.assertEqual(report.arch, "amd64")
        self.assertEqual(report.os, "debian")
        self.assertEqual(report.build_job_name, "test_job")
        self.assertEqual(len(report.file_inventory), 2)

        # Verify key methods were called
        mock_exists.assert_called_once_with("/path/to/package.deb")
        mock_extract_analyze.assert_called_once_with(
            mock_ctx, "/path/to/package.deb", unittest.mock.ANY, 10000, True, False
        )

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

    def test_extract_artifact_flavors_agent(self):
        """Test extracting artifact flavors for agent packages."""
        self.assertEqual(self.measurer._extract_artifact_flavors("static_quality_gate_agent_deb_amd64"), ["agent"])

    def test_extract_artifact_flavors_fips(self):
        """Test extracting artifact flavors for FIPS packages."""
        self.assertEqual(self.measurer._extract_artifact_flavors("static_quality_gate_agent_deb_amd64_fips"), ["fips"])

    def test_extract_artifact_flavors_iot(self):
        """Test extracting artifact flavors for IoT packages."""
        self.assertEqual(self.measurer._extract_artifact_flavors("static_quality_gate_iot_agent_deb_amd64"), ["iot"])

    def test_extract_artifact_flavors_dogstatsd(self):
        """Test extracting artifact flavors for DogStatsD packages."""
        self.assertEqual(
            self.measurer._extract_artifact_flavors("static_quality_gate_dogstatsd_deb_amd64"), ["dogstatsd"]
        )

    def test_extract_artifact_flavors_heroku(self):
        """Test extracting artifact flavors for Heroku packages."""
        self.assertEqual(self.measurer._extract_artifact_flavors("static_quality_gate_agent_heroku_amd64"), ["heroku"])

    def test_extract_artifact_flavors_unknown(self):
        """Test extracting artifact flavors for unknown packages."""
        self.assertEqual(self.measurer._extract_artifact_flavors("static_quality_gate_unknown_package"), ["unknown"])

    @patch('builtins.open', new_callable=mock_open)
    @patch('hashlib.sha256')
    def test_generate_checksum_success(self, mock_sha256, mock_file):
        """Test successful checksum generation."""
        # Setup mock hash object
        mock_hash_obj = Mock()
        mock_hash_obj.hexdigest.return_value = "abc123def456"
        mock_sha256.return_value = mock_hash_obj

        # Mock file reading
        mock_file.return_value.read.side_effect = [b"chunk1", b"chunk2", b""]

        result = self.measurer._generate_checksum(Path("/test/file"))

        self.assertEqual(result, "sha256:abc123def456")
        mock_hash_obj.update.assert_any_call(b"chunk1")
        mock_hash_obj.update.assert_any_call(b"chunk2")

    @patch('builtins.open', side_effect=OSError("File error"))
    def test_generate_checksum_failure(self, mock_file):
        """Test checksum generation failure handling."""
        result = self.measurer._generate_checksum(Path("/test/file"))
        self.assertIsNone(result)

    def test_save_report_to_yaml(self):
        """Test saving report to YAML file."""
        # Create a sample report
        report = InPlaceArtifactReport(
            artifact_path="/path/to/package.deb",
            artifact_type="package",
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
            artifact_flavors=["agent"],
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
            self.assertEqual(saved_data['artifact_flavors'], ["agent"])

        finally:
            os.unlink(output_path)

    @patch('builtins.open', side_effect=OSError("Permission denied"))
    def test_save_report_to_yaml_failure(self, mock_file):
        """Test handling of YAML save failures."""
        report = InPlaceArtifactReport(
            artifact_path="/path/to/package.deb",
            artifact_type="package",
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


if __name__ == '__main__':
    unittest.main()
