"""
Docker measurement tasks for experimental static quality gates.

This module provides invoke tasks for testing and using Docker image measurement
functionality locally without requiring a full CI environment.
"""

import os

from tasks.libs.common.color import color_message

from ..measurers.docker import InPlaceDockerMeasurer


def measure_image_local(
    ctx,
    image_ref,
    gate_name,
    config_path="test/static/static_quality_gates.yml",
    output_path=None,
    build_job_name="local_test",
    max_files=20000,
    no_checksums=False,
    include_layer_analysis=True,
    debug=False,
):
    """
    Run the in-place Docker image measurer locally for testing and development.

    This task allows you to test the Docker image measurement functionality on local images
    without requiring a full CI environment.

    Args:
        ctx: Invoke context for running commands
        image_ref: Docker image reference (tag, digest, or image ID)
        gate_name: Quality gate name from the configuration file
        config_path: Path to quality gates configuration (default: test/static/static_quality_gates.yml)
        output_path: Path to save the measurement report (default: {gate_name}_image_report.yml)
        build_job_name: Simulated build job name (default: local_test)
        max_files: Maximum number of files to process in inventory (default: 20000)
        no_checksums: Skip checksum generation for faster processing (default: false)
        include_layer_analysis: Whether to analyze individual layers (default: true)
        debug: Enable debug logging for troubleshooting (default: false)

    Example:
        dda inv experimental-gates.measure-image-local --image-ref nginx:latest --gate-name static_quality_gate_docker_agent_amd64
    """
    if not os.path.exists(config_path):
        print(color_message(f"❌ Configuration file not found: {config_path}", "red"))
        return

    if output_path is None:
        output_path = f"{gate_name}_image_report.yml"

    print(color_message("🔍 Starting in-place Docker image measurement...", "cyan"))
    print(f"Image: {image_ref}")
    print(f"Gate: {gate_name}")
    print(f"Config: {config_path}")
    print(f"Output: {output_path}")
    print("=" * 50)

    try:
        measurer = InPlaceDockerMeasurer(config_path=config_path)

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

        print(color_message("📏 Measuring Docker image...", "cyan"))
        report = measurer.measure_image(
            ctx=ctx,
            image_ref=image_ref,
            gate_name=gate_name,
            build_job_name=build_job_name,
            max_files=max_files,
            generate_checksums=not no_checksums,
            include_layer_analysis=include_layer_analysis,
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
        print("   • Note: Disk size is the uncompressed filesystem size of all files")

        if wire_usage_pct > 100 or disk_usage_pct > 100:
            print(color_message("⚠️  WARNING: Image exceeds size limits!", "red"))
            if disk_usage_pct > 100:
                excess_mb = (report.on_disk_size - report.max_on_disk_size) / 1024 / 1024
                print(color_message(f"   • Disk size exceeds limit by {excess_mb:.2f} MiB", "red"))
            if wire_usage_pct > 100:
                excess_mb = (report.on_wire_size - report.max_on_wire_size) / 1024 / 1024
                print(color_message(f"   • Wire size exceeds limit by {excess_mb:.2f} MiB", "red"))
        else:
            print(color_message("✅ Image within size limits", "green"))

        # Show Docker-specific information if available
        if report.docker_info:
            print("🐳 Docker Information:")
            print(f"   • Image ID: {image_ref}")
            print(f"   • Architecture: {report.docker_info.architecture}")
            print(f"   • OS: {report.docker_info.os}")
            print(f"   • Layers: {len(report.docker_info.layers)} total")
            print(f"   • Non-empty layers: {len(report.docker_info.non_empty_layers)}")

            # Show top 5 largest layers
            print("📊 Top 5 largest layers:")
            for i, layer in enumerate(report.docker_info.largest_layers[:5], 1):
                created_by = (
                    layer.created_by[:50] + "..."
                    if layer.created_by and len(layer.created_by) > 50
                    else layer.created_by
                )
                print(f"   {i}. {layer.size_mb:.2f} MiB - {created_by or 'Unknown command'}")

        # Show top 10 largest files
        print("📁 Top 10 largest files:")
        for i, file_info in enumerate(report.largest_files, 1):
            print(f"   {i:2}. {file_info.relative_path} ({file_info.size_mb:.2f} MiB)")

    except Exception as e:
        print(color_message(f"❌ Image measurement failed: {e}", "red"))
        raise
