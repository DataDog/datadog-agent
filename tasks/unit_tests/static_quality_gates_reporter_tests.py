import unittest
from unittest.mock import MagicMock, patch

from tasks.static_quality_gates.lib.static_quality_gates_reporter import QualityGateOutputFormatter


class TestQualityGateOutputFormatter(unittest.TestCase):
    """
    Unit tests for the QualityGateOutputFormatter class.
    Tests all static methods for formatting and reporting quality gate results.
    """

    def test_get_display_name_docker_agent(self):
        """Test get_display_name for Docker agent gates"""
        # Standard Docker agent
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_docker_agent_amd64")
        self.assertEqual(result, "Docker Agent (AMD64)")

        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_docker_agent_arm64")
        self.assertEqual(result, "Docker Agent (ARM64)")

    def test_get_display_name_docker_cluster_agent(self):
        """Test get_display_name for Docker cluster agent gates"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_docker_cluster_agent_amd64")
        self.assertEqual(result, "Docker Cluster Agent (AMD64)")

    def test_get_display_name_docker_jmx_agent(self):
        """Test get_display_name for Docker JMX agent gates"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_docker_agent_jmx_amd64")
        self.assertEqual(result, "Docker Agent (JMX) (AMD64)")

    def test_get_display_name_docker_dogstatsd(self):
        """Test get_display_name for Docker DogStatsD gates"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_docker_dogstatsd_amd64")
        self.assertEqual(result, "Docker DogStatsD (AMD64)")

    def test_get_display_name_docker_cws_instrumentation(self):
        """Test get_display_name for Docker CWS instrumentation gates"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_docker_cws_instrumentation_amd64")
        self.assertEqual(result, "Docker CWS Instrumentation (AMD64)")

    def test_get_display_name_docker_generic_service(self):
        """Test get_display_name for generic Docker service names"""
        # Test that other multi-word Docker services work correctly
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_docker_some_service_amd64")
        self.assertEqual(result, "Docker Some Service (AMD64)")

    def test_get_display_name_package_agent_deb(self):
        """Test get_display_name for package agent DEB gates"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_agent_deb_amd64")
        self.assertEqual(result, "Agent DEB (AMD64)")

        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_agent_deb_arm64")
        self.assertEqual(result, "Agent DEB (ARM64)")

    def test_get_display_name_package_agent_rpm(self):
        """Test get_display_name for package agent RPM gates"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_agent_rpm_amd64")
        self.assertEqual(result, "Agent RPM (AMD64)")

    def test_get_display_name_package_agent_msi(self):
        """Test get_display_name for package agent MSI gates"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_agent_msi")
        self.assertEqual(result, "Agent MSI ()")

    def test_get_display_name_package_agent_fips(self):
        """Test get_display_name for FIPS packages"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_agent_deb_amd64_fips")
        self.assertEqual(result, "Agent DEB (AMD64) (FIPS)")

        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_agent_rpm_arm64_fips")
        self.assertEqual(result, "Agent RPM (ARM64) (FIPS)")

    def test_get_display_name_package_dogstatsd(self):
        """Test get_display_name for DogStatsD packages"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_dogstatsd_deb_amd64")
        self.assertEqual(result, "DogStatsD DEB (AMD64)")

        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_dogstatsd_rpm_amd64")
        self.assertEqual(result, "DogStatsD RPM (AMD64)")

    def test_get_display_name_package_iot_agent(self):
        """Test get_display_name for IoT agent packages"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_iot_agent_deb_amd64")
        self.assertEqual(result, "IoT Agent DEB (AMD64)")

        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_iot_agent_deb_armhf")
        self.assertEqual(result, "IoT Agent DEB (ARMHF)")

    def test_get_display_name_package_heroku_agent(self):
        """Test get_display_name for Heroku agent packages"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_agent_heroku_amd64")
        self.assertEqual(result, "Heroku Agent  (AMD64)")

    def test_get_display_name_package_suse(self):
        """Test get_display_name for SUSE packages"""
        result = QualityGateOutputFormatter.get_display_name("static_quality_gate_agent_suse_amd64")
        self.assertEqual(result, "Agent SUSE (AMD64)")

    def test_format_artifact_path_filename_only(self):
        """Test format_artifact_path with file paths - should return only filename or truncated if too long"""
        long_path = "/go/src/github.com/DataDog/datadog-agent/omnibus/pkg/datadog-agent_7.70.0~devel.git.405.7e4b0c4.pipeline.72709673-1_amd64.deb"
        result = QualityGateOutputFormatter.format_artifact_path(long_path)
        filename = "datadog-agent_7.70.0~devel.git.405.7e4b0c4.pipeline.72709673-1_amd64.deb"
        # Since filename is longer than 60 chars, it should be truncated
        if len(filename) > 60:
            expected = f"...{filename[-57:]}"
        else:
            expected = filename
        self.assertEqual(result, expected)

    def test_format_artifact_path_very_long_filename(self):
        """Test format_artifact_path with very long filename - should truncate"""
        # Create a filename longer than 60 characters
        long_filename = "very_long_filename_that_exceeds_sixty_characters_and_should_be_truncated_properly.deb"
        long_path = f"/some/path/{long_filename}"
        result = QualityGateOutputFormatter.format_artifact_path(long_path)
        # Should be truncated to last 57 characters with "..." prefix
        expected = f"...{long_filename[-57:]}"
        self.assertEqual(result, expected)

    def test_format_artifact_path_docker_registry(self):
        """Test format_artifact_path with Docker registry paths"""
        docker_path = "registry.ddbuild.io/ci/datadog-agent/agent:v72709673-7e4b0c4b-7-amd64"
        result = QualityGateOutputFormatter.format_artifact_path(docker_path)
        # Based on the implementation, it extracts the last part after the final "/"
        expected = "agent:v72709673-7e4b0c4b-7-amd64"
        self.assertEqual(result, expected)

    def test_format_artifact_path_unknown(self):
        """Test format_artifact_path with None or empty path"""
        result = QualityGateOutputFormatter.format_artifact_path(None)
        self.assertEqual(result, "Unknown")

        result = QualityGateOutputFormatter.format_artifact_path("")
        self.assertEqual(result, "Unknown")

    def test_format_artifact_path_no_slash(self):
        """Test format_artifact_path with path containing no slashes"""
        result = QualityGateOutputFormatter.format_artifact_path("simple_filename.deb")
        self.assertEqual(result, "simple_filename.deb")

    @patch('builtins.print')
    def test_print_enhanced_gate_result(self, mock_print):
        """Test print_enhanced_gate_result output formatting"""
        # Test with realistic size values
        gate_name = "static_quality_gate_agent_deb_amd64"
        artifact_path = "/path/to/datadog-agent.deb"
        wire_size = 186_347_520  # ~177.7 MB
        wire_limit = 187_172_864  # ~178.6 MB
        disk_size = 734_053_376  # ~700 MB
        disk_limit = 739_246_080  # ~705 MB

        QualityGateOutputFormatter.print_enhanced_gate_result(
            gate_name, artifact_path, wire_size, wire_limit, disk_size, disk_limit
        )

        # Verify print was called twice (compressed and uncompressed lines)
        self.assertEqual(mock_print.call_count, 2)

        # Check that the output contains expected elements
        calls = [str(call) for call in mock_print.call_args_list]
        output_text = ' '.join(calls)

        # Should contain size information (allowing for rounding differences)
        self.assertIn("177.7 MB", output_text)  # Current compressed size
        self.assertIn("178.5 MB", output_text)  # Max compressed size (rounded)
        self.assertIn("700.0 MB", output_text)  # Current uncompressed size
        self.assertIn("705.0 MB", output_text)  # Max uncompressed size

        # Should contain utilization percentages (allowing for rounding differences)
        self.assertIn("99.6%", output_text)  # Wire utilization
        self.assertIn("99.3%", output_text)  # Disk utilization

        # Should contain emoji indicators
        self.assertIn("📦", output_text)  # Compressed indicator
        self.assertIn("💾", output_text)  # Uncompressed indicator

    @patch('builtins.print')
    def test_print_enhanced_gate_execution(self, mock_print):
        """Test print_enhanced_gate_execution output formatting"""
        gate_name = "static_quality_gate_agent_deb_amd64"
        artifact_path = "/path/to/datadog-agent_7.70.0~devel.git.405.7e4b0c4.pipeline.72709673-1_amd64.deb"

        QualityGateOutputFormatter.print_enhanced_gate_execution(gate_name, artifact_path)

        # Verify print was called twice (gate name and artifact path)
        self.assertEqual(mock_print.call_count, 2)

        calls = [str(call) for call in mock_print.call_args_list]
        output_text = ' '.join(calls)

        # Should contain the display name
        self.assertIn("Agent DEB (AMD64)", output_text)

        # Should contain the formatted artifact name (truncated if too long)
        filename = "datadog-agent_7.70.0~devel.git.405.7e4b0c4.pipeline.72709673-1_amd64.deb"
        expected_artifact = f"...{filename[-57:]}" if len(filename) > 60 else filename
        self.assertIn(expected_artifact, output_text)

        # Should contain emoji indicators
        self.assertIn("🔍", output_text)  # Checking indicator
        self.assertIn("📄", output_text)  # Artifact indicator

    @patch('builtins.print')
    def test_print_enhanced_gate_success(self, mock_print):
        """Test print_enhanced_gate_success output formatting"""
        gate_name = "static_quality_gate_docker_agent_amd64"

        QualityGateOutputFormatter.print_enhanced_gate_success(gate_name)

        # Verify print was called once
        self.assertEqual(mock_print.call_count, 1)

        call_str = str(mock_print.call_args_list[0])

        # Should contain success message with display name
        self.assertIn("Docker Agent (AMD64)", call_str)
        self.assertIn("PASSED", call_str)
        self.assertIn("✅", call_str)  # Success emoji

    @patch('builtins.print')
    def test_print_startup_message(self, mock_print):
        """Test print_startup_message output formatting"""
        gates_count = 25
        config_path = "test/static/static_quality_gates.yml"

        QualityGateOutputFormatter.print_startup_message(gates_count, config_path)

        # Verify print was called twice
        self.assertEqual(mock_print.call_count, 2)

        calls = [str(call) for call in mock_print.call_args_list]
        output_text = ' '.join(calls)

        # Should contain config path
        self.assertIn(config_path, output_text)
        self.assertIn("correctly parsed", output_text)

        # Should contain gates count
        self.assertIn("25", output_text)
        self.assertIn("quality gates", output_text)

        # Should contain emoji indicator
        self.assertIn("🚀", output_text)

    @patch('builtins.print')
    def test_print_summary_table_all_passed(self, mock_print):
        """Test print_summary_table with all gates passed"""
        # Create mock gate objects
        mock_gate1 = MagicMock()
        mock_gate1.gate_name = "static_quality_gate_agent_deb_amd64"
        mock_gate1.artifact_on_wire_size = 186_347_520  # ~177.7 MB
        mock_gate1.max_on_wire_size = 187_172_864  # ~178.6 MB
        mock_gate1.artifact_on_disk_size = 734_053_376  # ~700 MB
        mock_gate1.max_on_disk_size = 739_246_080  # ~705 MB

        mock_gate2 = MagicMock()
        mock_gate2.gate_name = "static_quality_gate_docker_agent_amd64"
        mock_gate2.artifact_on_wire_size = 282_460_160  # ~269.3 MB
        mock_gate2.max_on_wire_size = 285_212_672  # ~272.0 MB
        mock_gate2.artifact_on_disk_size = 819_658_752  # ~781.6 MB
        mock_gate2.max_on_disk_size = 827_064_320  # ~788.6 MB

        gate_list = [mock_gate1, mock_gate2]

        # Create gate states (all passed)
        gate_states = [
            {"name": "static_quality_gate_agent_deb_amd64", "error_type": None},
            {"name": "static_quality_gate_docker_agent_amd64", "error_type": None},
        ]

        QualityGateOutputFormatter.print_summary_table(gate_list, gate_states)

        # Verify print was called multiple times for the table
        self.assertGreater(mock_print.call_count, 10)

        calls = [str(call) for call in mock_print.call_args_list]
        output_text = ' '.join(calls)

        # Should contain table headers
        self.assertIn("STATIC QUALITY GATES SUMMARY", output_text)
        self.assertIn("Gate Name", output_text)
        self.assertIn("Status", output_text)
        self.assertIn("Compressed", output_text)
        self.assertIn("Uncompressed", output_text)
        self.assertIn("Comp Util", output_text)
        self.assertIn("Uncomp Util", output_text)

        # Should contain gate display names
        self.assertIn("Agent DEB (AMD64)", output_text)
        self.assertIn("Docker Agent (AMD64)", output_text)

        # Should show PASS status
        self.assertIn("PASS", output_text)

        # Should contain summary statistics
        self.assertIn("2/2 gates passed", output_text)
        self.assertIn("All gates passed successfully", output_text)

        # Should contain emoji indicators
        self.assertIn("🛡️", output_text)  # Shield emoji
        self.assertIn("📊", output_text)  # Chart emoji
        self.assertIn("✅", output_text)  # Success emoji

    @patch('builtins.print')
    def test_print_summary_table_with_failures(self, mock_print):
        """Test print_summary_table with some gates failed"""
        # Create mock gate objects
        mock_gate1 = MagicMock()
        mock_gate1.gate_name = "static_quality_gate_agent_deb_amd64"
        mock_gate1.artifact_on_wire_size = 186_347_520
        mock_gate1.max_on_wire_size = 187_172_864
        mock_gate1.artifact_on_disk_size = 734_053_376
        mock_gate1.max_on_disk_size = 739_246_080

        mock_gate2 = MagicMock()
        mock_gate2.gate_name = "static_quality_gate_docker_agent_amd64"
        mock_gate2.artifact_on_wire_size = 300_000_000  # Over limit
        mock_gate2.max_on_wire_size = 285_212_672
        mock_gate2.artifact_on_disk_size = 819_658_752
        mock_gate2.max_on_disk_size = 827_064_320

        gate_list = [mock_gate1, mock_gate2]

        # Create gate states (one failed)
        gate_states = [
            {"name": "static_quality_gate_agent_deb_amd64", "error_type": None},
            {"name": "static_quality_gate_docker_agent_amd64", "error_type": "AssertionError"},
        ]

        QualityGateOutputFormatter.print_summary_table(gate_list, gate_states)

        calls = [str(call) for call in mock_print.call_args_list]
        output_text = ' '.join(calls)

        # Should show mixed status
        self.assertIn("PASS", output_text)
        self.assertIn("FAIL", output_text)

        # Should contain failure summary
        self.assertIn("1/2 gates passed", output_text)
        self.assertIn("1 gate(s) failed", output_text)

        # Should contain failure emoji
        self.assertIn("❌", output_text)

    @patch('builtins.print')
    def test_print_summary_table_gates_without_sizes(self, mock_print):
        """Test print_summary_table with gates that don't have size attributes"""
        # Create mock gate without size attributes
        mock_gate = MagicMock()
        mock_gate.gate_name = "static_quality_gate_test"
        # Don't set artifact_on_wire_size or artifact_on_disk_size
        del mock_gate.artifact_on_wire_size
        del mock_gate.artifact_on_disk_size

        gate_list = [mock_gate]
        gate_states = [{"name": "static_quality_gate_test", "error_type": None}]

        QualityGateOutputFormatter.print_summary_table(gate_list, gate_states)

        calls = [str(call) for call in mock_print.call_args_list]
        output_text = ' '.join(calls)

        # Should handle missing size attributes gracefully
        self.assertIn("N/A", output_text)


if __name__ == '__main__':
    unittest.main()
