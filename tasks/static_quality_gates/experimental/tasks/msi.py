"""
MSI measurement tasks for experimental static quality gates.

This module provides invoke tasks for testing and using MSI measurement
functionality locally without requiring a full CI environment.
"""

import os

from tasks.libs.common.color import color_message

from ..measurers.msi import InPlaceMsiMeasurer


def measure_msi_local(
    ctx,
    msi_path,
    gate_name,
    config_path="test/static/static_quality_gates.yml",
    output_path=None,
    build_job_name="local_test",
    max_files=20000,
    no_checksums=False,
    debug=False,
):
    """
    Run the in-place MSI measurer locally for testing and development.

    This task allows you to test the MSI measurement functionality on local packages
    without requiring Windows-specific tools (uses cross-platform ZIP extraction).

    Args:
        ctx: Invoke context for running commands
        msi_path: Path to MSI file or directory containing both MSI and ZIP files
        gate_name: Quality gate name from the configuration file
        config_path: Path to quality gates configuration (default: test/static/static_quality_gates.yml)
        output_path: Path to save the measurement report (default: {gate_name}_msi_report.yml)
        build_job_name: Simulated build job name (default: local_test)
        max_files: Maximum number of files to process in inventory (default: 20000)
        no_checksums: Skip checksum generation for faster processing (default: false)
        debug: Enable debug logging for troubleshooting (default: false)

    Example:
        dda inv experimental-gates.measure-msi-local --msi-path /path/to/datadog-agent.msi --gate-name static_quality_gate_agent_msi
        dda inv experimental-gates.measure-msi-local --msi-path /path/to/packages/ --gate-name static_quality_gate_agent_msi
    """
    if not os.path.exists(config_path):
        print(color_message(f"❌ Configuration file not found: {config_path}", "red"))
        return

    if output_path is None:
        output_path = f"{gate_name}_msi_report.yml"

    print(color_message("🔍 Starting in-place MSI measurement...", "cyan"))
    print(f"MSI/Directory: {msi_path}")
    print(f"Gate: {gate_name}")
    print(f"Config: {config_path}")
    print(f"Output: {output_path}")
    print("=" * 50)

    try:
        measurer = InPlaceMsiMeasurer(config_path=config_path)

        # Set dummy values in case of local execution
        os.environ["CI_PIPELINE_ID"] = os.environ.get("CI_PIPELINE_ID", "LOCAL")
        os.environ["CI_COMMIT_SHA"] = os.environ.get("CI_COMMIT_SHA", "LOCAL")

        if os.environ.get("CI_PIPELINE_ID") == "LOCAL" or os.environ.get("CI_COMMIT_SHA") == "LOCAL":
            print(
                color_message(
                    "🏷️  Warning! Running in local mode, using dummy values for CI_PIPELINE_ID and CI_COMMIT_SHA",
                    "yellow",
                )
            )

        print(color_message("📏 Measuring MSI package...", "cyan"))
        report = measurer.measure_msi(
            ctx=ctx,
            msi_ref=msi_path,
            gate_name=gate_name,
            build_job_name=build_job_name,
            max_files=max_files,
            generate_checksums=not no_checksums,
            debug=debug,
        )

        # Save the report
        print(color_message("💾 Saving measurement report...", "cyan"))
        measurer.save_report_to_yaml(report, output_path)

        # Display summary
        print(color_message("✅ Measurement completed successfully!", "green"))
        print("📊 Results:")
        print(f"   • Wire size: {report.on_wire_size:,} bytes ({report.on_wire_size / 1024 / 1024:.2f} MiB)")
        print(f"   • Disk size: {report.on_disk_size:,} bytes ({report.on_disk_size / 1024 / 1024:.2f} MiB)")
        print(f"   • Files inventoried: {len(report.file_inventory):,}")
        print(f"   • Report saved to: {output_path}")

        # Show size comparison with limits
        wire_limit_mb = report.max_on_wire_size / 1024 / 1024
        disk_limit_mb = report.max_on_disk_size / 1024 / 1024
        wire_usage_pct = (report.on_wire_size / report.max_on_wire_size) * 100
        disk_usage_pct = (report.on_disk_size / report.max_on_disk_size) * 100

        print("📏 Size Limits:")
        print(f"   • Wire limit: {wire_limit_mb:.2f} MiB (using {wire_usage_pct:.1f}%)")
        print(f"   • Disk limit: {disk_limit_mb:.2f} MiB (using {disk_usage_pct:.1f}%)")
        print("   • Note: Wire size = MSI file, Disk size = ZIP extraction (cross-platform)")

        if wire_usage_pct > 100 or disk_usage_pct > 100:
            print(color_message("⚠️  WARNING: MSI package exceeds size limits!", "red"))
            if disk_usage_pct > 100:
                excess_mb = (report.on_disk_size - report.max_on_disk_size) / 1024 / 1024
                print(color_message(f"   • Disk size exceeds limit by {excess_mb:.2f} MiB", "red"))
            if wire_usage_pct > 100:
                excess_mb = (report.on_wire_size - report.max_on_wire_size) / 1024 / 1024
                print(color_message(f"   • Wire size exceeds limit by {excess_mb:.2f} MiB", "red"))
        else:
            print(color_message("✅ MSI package within size limits", "green"))

        # Show top 10 largest files
        print("📁 Top 10 largest files:")
        for i, file_info in enumerate(report.largest_files, 1):
            print(f"   {i:2}. {file_info.relative_path} ({file_info.size_mb:.2f} MiB)")

        # Show MSI-specific notes
        print("\n📝 MSI Measurement Notes:")
        print("   • Wire size measured from actual MSI file")
        print("   • Disk size measured from ZIP file extraction (cross-platform)")
        print("   • File inventory generated from ZIP contents")
        print("   • Both MSI and ZIP files are automatically located")

    except Exception as e:
        print(color_message(f"❌ MSI measurement failed: {e}", "red"))
        raise
