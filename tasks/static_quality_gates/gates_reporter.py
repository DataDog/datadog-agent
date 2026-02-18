"""
Static Quality Gates Reporter.
Provides clear, customer-friendly reporting and output formatting for external users.
"""

import typing

from tasks.libs.common.color import color_message


class QualityGateOutputFormatter:
    """
    Handles formatting and display of quality gate results in a customer-friendly way.
    """

    @staticmethod
    def get_display_name(gate_name: str) -> str:
        """
        Convert gate name to a user-friendly display name

        Args:
            gate_name: The technical gate name (e.g., "static_quality_gate_agent_deb_amd64")

        Returns:
            Human-readable display name (e.g., "Agent DEB (AMD64)")
        """
        # Remove the "static_quality_gate_" prefix
        name = gate_name.replace("static_quality_gate_", "")

        # Add spaces and capitalize appropriately
        if "docker" in name:
            parts = name.split("_")
            if "agent" in parts:
                agent_type = "Agent"
                if "cluster" in parts:
                    agent_type = "Cluster Agent"
                elif "jmx" in parts:
                    agent_type = "Agent (JMX)"
            elif "dogstatsd" in parts:
                agent_type = "DogStatsD"
            elif "cws" in parts and "instrumentation" in parts:
                agent_type = "CWS Instrumentation"
            else:
                # For other cases, extract the service name (skip "docker" and arch)
                service_parts = [part for part in parts if part not in ["docker", "amd64", "arm64"]]
                agent_type = " ".join(service_parts).title()

            arch = parts[-1] if parts[-1] in ["amd64", "arm64"] else "unknown"
            return f"Docker {agent_type} ({arch.upper()})"
        else:
            # Package-based gates
            parts = name.split("_")
            agent_type = "Agent"
            if "dogstatsd" in parts:
                agent_type = "DogStatsD"
            elif "iot" in parts:
                agent_type = "IoT Agent"
            elif "heroku" in parts:
                agent_type = "Heroku Agent"

            # Extract package type and architecture
            package_type = ""
            arch = ""
            fips = ""

            for part in parts:
                if part in ["deb", "rpm", "msi", "suse"]:
                    package_type = part.upper()
                elif part in ["amd64", "arm64", "armhf"]:
                    arch = part.upper()
                elif part == "fips":
                    fips = " (FIPS)"

            return f"{agent_type} {package_type} ({arch}){fips}"

    @staticmethod
    def print_summary_table(gate_list, gate_states: list[dict[str, typing.Any]], metric_handler=None) -> None:
        """
        Print a comprehensive summary table of all quality gates with their metrics

        Args:
            gate_list: List of StaticQualityGate objects (composition-based)
            gate_states: List of gate state dictionaries with execution results
            metric_handler: Optional GateMetricHandler for getting measurement data
        """
        print(color_message("\n" + "=" * 132, "magenta"))
        print(color_message("🛡️  STATIC QUALITY GATES SUMMARY", "magenta"))
        print(color_message("=" * 132, "magenta"))

        # Create a lookup for gate states
        state_lookup = {state["name"]: state for state in gate_states}

        # Table header
        header = f"{'Gate Name':<40} {'Status':<8} {'Compressed':<20} {'Uncompressed':<20} {'Comp Remain':<12} {'Uncomp Remain':<14}"
        print(color_message(header, "cyan"))
        print(color_message("-" * 132, "cyan"))

        passed_count = 0
        failed_count = 0

        for gate in sorted(gate_list, key=lambda x: x.config.gate_name):
            state = state_lookup.get(gate.config.gate_name, {})

            # Get display name
            display_name = QualityGateOutputFormatter.get_display_name(gate.config.gate_name)
            if len(display_name) > 38:
                display_name = display_name[:35] + "..."

            # Status
            if state.get("error_type") is None:
                status = color_message("PASS", "green")
                passed_count += 1
            else:
                status = color_message("FAIL", "red")
                failed_count += 1

            # Get measurement data from metric handler if available
            wire_current_bytes = 0
            disk_current_bytes = 0

            if metric_handler and gate.config.gate_name in metric_handler.metrics:
                gate_metrics = metric_handler.metrics[gate.config.gate_name]
                wire_current_bytes = gate_metrics.get('current_on_wire_size', 0)
                disk_current_bytes = gate_metrics.get('current_on_disk_size', 0)

            # Convert to MB for display
            wire_current = wire_current_bytes / (1024 * 1024)
            wire_limit = gate.config.max_on_wire_size / (1024 * 1024)
            disk_current = disk_current_bytes / (1024 * 1024)
            disk_limit = gate.config.max_on_disk_size / (1024 * 1024)

            wire_remaining = wire_limit - wire_current
            disk_remaining = disk_limit - disk_current

            # Format remaining space with color based on how much is left
            def get_remaining_color(remaining_mb, limit_mb):
                if limit_mb == 0:
                    return "gray"
                remaining_percent = (remaining_mb / limit_mb) * 100
                if remaining_percent < 5:  # Less than 5% remaining
                    return "red"
                elif remaining_percent < 15:  # Less than 15% remaining
                    return "orange"
                elif remaining_percent < 25:  # Less than 25% remaining
                    return "yellow"
                else:
                    return "green"

            wire_remaining_color = get_remaining_color(wire_remaining, wire_limit)
            disk_remaining_color = get_remaining_color(disk_remaining, disk_limit)

            compressed_info = f"{wire_current:6.1f}/{wire_limit:6.1f} MB"
            uncompressed_info = f"{disk_current:6.1f}/{disk_limit:6.1f} MB"

            # Format remaining space text without color first for proper alignment calculation
            compressed_remaining_text = f"{wire_remaining:6.1f} MB"
            uncompressed_remaining_text = f"{disk_remaining:6.1f} MB"

            # Apply color and calculate proper padding to maintain alignment
            compressed_remaining_info = color_message(compressed_remaining_text, wire_remaining_color)
            uncompressed_remaining_info = color_message(uncompressed_remaining_text, disk_remaining_color)

            # Calculate padding needed (color codes don't count toward visual width)
            comp_remain_padding = 12 - len(compressed_remaining_text)
            uncomp_remain_padding = 14 - len(uncompressed_remaining_text)

            print(
                f"{display_name:<40} {status:<8} {compressed_info:<20} {uncompressed_info:<20} {compressed_remaining_info}{' ' * comp_remain_padding} {uncompressed_remaining_info}{' ' * uncomp_remain_padding}"
            )

        # Summary footer
        print(color_message("-" * 132, "cyan"))

        # Overall statistics
        total_gates = len(gate_list)

        print(color_message(f"📊 SUMMARY: {passed_count}/{total_gates} gates passed", "cyan"))

        if failed_count > 0:
            print(color_message(f"❌ {failed_count} gate(s) failed - check details above", "red"))
        else:
            print(color_message("✅ All gates passed successfully!", "green"))

        print(
            color_message("📊 Dashboard: https://app.datadoghq.com/dashboard/5np-man-vak/static-quality-gates", "cyan")
        )
        print(color_message("=" * 132, "magenta"))
