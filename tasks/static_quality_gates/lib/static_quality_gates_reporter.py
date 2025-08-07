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
    def format_artifact_path(artifact_path: str) -> str:
        """
        Format artifact path to be more readable

        Args:
            artifact_path: Full path to the artifact

        Returns:
            Shortened, readable artifact identifier
        """
        if not artifact_path:
            return "Unknown"

        # For file paths, show just the filename
        if "/" in artifact_path:
            filename = artifact_path.split("/")[-1]
            if len(filename) > 60:
                # Truncate very long filenames
                return f"...{filename[-57:]}"
            return filename

        # For Docker images, make them more readable
        if "registry.ddbuild.io" in artifact_path:
            parts = artifact_path.split("/")
            if len(parts) >= 3:
                image_name = "/".join(parts[-2:])
                return f"Docker: {image_name}"

        return artifact_path

    @staticmethod
    def print_enhanced_gate_result(
        gate_name: str,
        artifact_path: str,
        artifact_on_wire_size: float,
        max_on_wire_size: float,
        artifact_on_disk_size: float,
        max_on_disk_size: float,
    ) -> None:
        """
        Print enhanced results for a single gate

        Args:
            gate_name: Technical gate name
            artifact_path: Path to the artifact
            artifact_on_wire_size: Current compressed size in bytes
            max_on_wire_size: Maximum allowed compressed size in bytes
            artifact_on_disk_size: Current uncompressed size in bytes
            max_on_disk_size: Maximum allowed uncompressed size in bytes
        """
        # Calculate remaining space
        wire_remaining = max_on_wire_size - artifact_on_wire_size
        disk_remaining = max_on_disk_size - artifact_on_disk_size

        # Format sizes for readability
        wire_current = f"{artifact_on_wire_size / 1024 / 1024:.1f} MB"
        wire_limit = f"{max_on_wire_size / 1024 / 1024:.1f} MB"
        wire_remaining_formatted = f"{wire_remaining / 1024 / 1024:.1f} MB"
        disk_current = f"{artifact_on_disk_size / 1024 / 1024:.1f} MB"
        disk_limit = f"{max_on_disk_size / 1024 / 1024:.1f} MB"
        disk_remaining_formatted = f"{disk_remaining / 1024 / 1024:.1f} MB"

        print(
            color_message(
                f"📦 Compressed Size:   {wire_current:>10} / {wire_limit:>10} ({wire_remaining_formatted:>10} remaining)",
                "green",
            )
        )
        print(
            color_message(
                f"💾 Uncompressed Size: {disk_current:>10} / {disk_limit:>10} ({disk_remaining_formatted:>10} remaining)",
                "green",
            )
        )

    @staticmethod
    def print_enhanced_gate_execution(gate_name: str, artifact_path: str) -> None:
        """
        Print enhanced gate execution header

        Args:
            gate_name: Technical gate name
            artifact_path: Path to the artifact
        """
        display_name = QualityGateOutputFormatter.get_display_name(gate_name)
        artifact_display = QualityGateOutputFormatter.format_artifact_path(artifact_path)

        print(f"\n🔍 Checking {display_name}")
        print(f"📄 Artifact: {artifact_display}")

    @staticmethod
    def print_enhanced_gate_success(gate_name: str) -> None:
        """
        Print enhanced gate success message

        Args:
            gate_name: Technical gate name
        """
        display_name = QualityGateOutputFormatter.get_display_name(gate_name)
        print(color_message(f"✅ {display_name} PASSED", "green"))

    @staticmethod
    def print_summary_table(gate_list, gate_states: list[dict[str, typing.Any]]) -> None:
        """
        Print a comprehensive summary table of all quality gates with their metrics

        Args:
            gate_list: List of StaticQualityGate objects
            gate_states: List of gate state dictionaries with execution results
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

        total_compressed = 0
        total_uncompressed = 0
        total_compressed_limit = 0
        total_uncompressed_limit = 0
        passed_count = 0
        failed_count = 0

        for gate in sorted(gate_list, key=lambda x: x.gate_name):
            state = state_lookup.get(gate.gate_name, {})

            # Get display name
            display_name = QualityGateOutputFormatter.get_display_name(gate.gate_name)
            if len(display_name) > 38:
                display_name = display_name[:35] + "..."

            # Status
            if state.get("error_type") is None:
                status = color_message("PASS", "green")
                passed_count += 1
            else:
                status = color_message("FAIL", "red")
                failed_count += 1

            # Sizes and remaining space
            if hasattr(gate, 'artifact_on_wire_size') and hasattr(gate, 'artifact_on_disk_size'):
                wire_current = gate.artifact_on_wire_size / (1024 * 1024)
                wire_limit = gate.max_on_wire_size / (1024 * 1024)
                disk_current = gate.artifact_on_disk_size / (1024 * 1024)
                disk_limit = gate.max_on_disk_size / (1024 * 1024)

                wire_remaining = wire_limit - wire_current
                disk_remaining = disk_limit - disk_current

                # Accumulate totals
                total_compressed += wire_current
                total_uncompressed += disk_current
                total_compressed_limit += wire_limit
                total_uncompressed_limit += disk_limit

                # Format remaining space with color based on how much is left
                def get_remaining_color(remaining_mb, limit_mb):
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
                compressed_remaining_info = color_message(f"{wire_remaining:6.1f} MB", wire_remaining_color)
                uncompressed_remaining_info = color_message(f"{disk_remaining:6.1f} MB", disk_remaining_color)
            else:
                compressed_info = "N/A"
                uncompressed_info = "N/A"
                compressed_remaining_info = "N/A"
                uncompressed_remaining_info = "N/A"

            print(
                f"{display_name:<40} {status:<8} {compressed_info:<20} {uncompressed_info:<20} {compressed_remaining_info:<12} {uncompressed_remaining_info:<14}"
            )

        # Summary footer
        print(color_message("-" * 132, "cyan"))

        # Overall statistics
        total_gates = len(gate_list)
        overall_compressed_remaining = total_compressed_limit - total_compressed if total_compressed_limit > 0 else 0
        overall_uncompressed_remaining = (
            total_uncompressed_limit - total_uncompressed if total_uncompressed_limit > 0 else 0
        )

        print(color_message(f"📊 SUMMARY: {passed_count}/{total_gates} gates passed", "cyan"))
        print(
            color_message(
                f"📦 Total Compressed:   {total_compressed:7.1f} MB / {total_compressed_limit:7.1f} MB ({overall_compressed_remaining:7.1f} MB remaining)",
                "cyan",
            )
        )
        print(
            color_message(
                f"💾 Total Uncompressed: {total_uncompressed:7.1f} MB / {total_uncompressed_limit:7.1f} MB ({overall_uncompressed_remaining:7.1f} MB remaining)",
                "cyan",
            )
        )

        if failed_count > 0:
            print(color_message(f"❌ {failed_count} gate(s) failed - check details above", "red"))
        else:
            print(color_message("✅ All gates passed successfully!", "green"))

        print(color_message("=" * 132, "magenta"))

    @staticmethod
    def print_startup_message(gates_count: int, config_path: str) -> None:
        """
        Print enhanced startup message

        Args:
            gates_count: Number of gates to be executed
            config_path: Path to the configuration file
        """
        print(f"{config_path} correctly parsed!")
        print(color_message(f"\n🚀 Starting {gates_count} quality gates...", "cyan"))
