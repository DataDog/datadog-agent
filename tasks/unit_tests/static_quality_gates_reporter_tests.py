import unittest
from unittest.mock import MagicMock, patch

from tasks.static_quality_gates.gates_reporter import QualityGateOutputFormatter


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

    @patch('builtins.print')
    def test_print_summary_table_all_passed(self, mock_print):
        """Test print_summary_table with all gates passed"""
        # Create mock gate objects with composition structure
        mock_gate1 = MagicMock()
        mock_gate1.config.gate_name = "static_quality_gate_agent_deb_amd64"
        mock_gate1.config.max_on_wire_size = 187_172_864  # ~178.6 MB
        mock_gate1.config.max_on_disk_size = 739_246_080  # ~705 MB

        mock_gate2 = MagicMock()
        mock_gate2.config.gate_name = "static_quality_gate_docker_agent_amd64"
        mock_gate2.config.max_on_wire_size = 285_212_672  # ~272.0 MB
        mock_gate2.config.max_on_disk_size = 827_064_320  # ~788.6 MB

        gate_list = [mock_gate1, mock_gate2]

        # Create gate states (all passed)
        gate_states = [
            {"name": "static_quality_gate_agent_deb_amd64", "error_type": None},
            {"name": "static_quality_gate_docker_agent_amd64", "error_type": None},
        ]

        # Mock metric handler with measurement data
        mock_metric_handler = MagicMock()
        mock_metric_handler.metrics = {
            "static_quality_gate_agent_deb_amd64": {
                "current_on_wire_size": 186_347_520,  # ~177.7 MB
                "current_on_disk_size": 734_053_376,  # ~700 MB
            },
            "static_quality_gate_docker_agent_amd64": {
                "current_on_wire_size": 282_460_160,  # ~269.3 MB
                "current_on_disk_size": 819_658_752,  # ~781.6 MB
            },
        }

        QualityGateOutputFormatter.print_summary_table(gate_list, gate_states, mock_metric_handler)

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
        self.assertIn("Comp Remain", output_text)
        self.assertIn("Uncomp Remain", output_text)

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
        # Create mock gate objects with composition structure
        mock_gate1 = MagicMock()
        mock_gate1.config.gate_name = "static_quality_gate_agent_deb_amd64"
        mock_gate1.config.max_on_wire_size = 187_172_864
        mock_gate1.config.max_on_disk_size = 739_246_080

        mock_gate2 = MagicMock()
        mock_gate2.config.gate_name = "static_quality_gate_docker_agent_amd64"
        mock_gate2.config.max_on_wire_size = 285_212_672
        mock_gate2.config.max_on_disk_size = 827_064_320

        gate_list = [mock_gate1, mock_gate2]

        # Create gate states (one failed)
        gate_states = [
            {"name": "static_quality_gate_agent_deb_amd64", "error_type": None},
            {"name": "static_quality_gate_docker_agent_amd64", "error_type": "AssertionError"},
        ]

        # Mock metric handler with measurement data (gate2 over limit)
        mock_metric_handler = MagicMock()
        mock_metric_handler.metrics = {
            "static_quality_gate_agent_deb_amd64": {
                "current_on_wire_size": 186_347_520,
                "current_on_disk_size": 734_053_376,
            },
            "static_quality_gate_docker_agent_amd64": {
                "current_on_wire_size": 300_000_000,  # Over limit
                "current_on_disk_size": 819_658_752,
            },
        }

        QualityGateOutputFormatter.print_summary_table(gate_list, gate_states, mock_metric_handler)

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
        """Test print_summary_table with gates that don't have measurement data"""
        # Create mock gate with composition structure but no measurement data
        mock_gate = MagicMock()
        mock_gate.config.gate_name = "static_quality_gate_test"
        mock_gate.config.max_on_wire_size = 1000 * 1024 * 1024  # 1000MB
        mock_gate.config.max_on_disk_size = 2000 * 1024 * 1024  # 2000MB

        gate_list = [mock_gate]
        gate_states = [{"name": "static_quality_gate_test", "error_type": None}]

        # No metric handler provided, so no measurement data
        QualityGateOutputFormatter.print_summary_table(gate_list, gate_states, None)

        calls = [str(call) for call in mock_print.call_args_list]
        output_text = ' '.join(calls)

        # Should handle missing measurement data gracefully (shows 0.0 when no data)
        self.assertIn("0.0", output_text)


if __name__ == '__main__':
    unittest.main()
