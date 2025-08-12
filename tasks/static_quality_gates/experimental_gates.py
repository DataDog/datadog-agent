"""
Experimental Static Quality Gates implementation for in-place artifact measurement.

This module provides experimental functionality for measuring artifacts directly
in build jobs, generating detailed reports with file inventories for comparison.
"""

import os
import tempfile
from dataclasses import dataclass, field
from datetime import datetime
from pathlib import Path
from typing import Any

import yaml
from invoke import Context

from tasks.libs.package.size import directory_size, extract_package, file_size
from tasks.static_quality_gates.gates import (
    ArtifactMeasurement,
    QualityGateConfig,
    create_quality_gate_config,
)


@dataclass(frozen=True)
class FileInfo:
    """
    Information about a single file within an artifact.
    """

    relative_path: str
    size_bytes: int
    checksum: str | None = None

    def __post_init__(self):
        """Validate file info data"""
        if not self.relative_path:
            raise ValueError("relative_path cannot be empty")
        if self.size_bytes < 0:
            raise ValueError("size_bytes must be non-negative")

    @property
    def size_mb(self) -> float:
        """Size in megabytes"""
        return self.size_bytes / (1024 * 1024)


@dataclass(frozen=True)
class InPlaceArtifactReport:
    """
    Complete measurement report for a single artifact, generated in-place.
    """

    # Core identification
    artifact_path: str
    artifact_type: str
    gate_name: str

    # Size measurements
    on_wire_size: int
    on_disk_size: int
    max_on_wire_size: int
    max_on_disk_size: int

    # File inventory
    file_inventory: list[FileInfo]

    # Metadata
    measurement_timestamp: str
    pipeline_id: str
    commit_sha: str
    arch: str
    os: str
    build_job_name: str
    artifact_flavors: list[str] = field(default_factory=list)

    def __post_init__(self):
        """Validate report data"""
        if not self.artifact_path:
            raise ValueError("artifact_path cannot be empty")
        if not self.gate_name:
            raise ValueError("gate_name cannot be empty")
        if self.on_wire_size < 0:
            raise ValueError("on_wire_size must be non-negative")
        if self.on_disk_size < 0:
            raise ValueError("on_disk_size must be non-negative")

    @property
    def largest_files(self) -> list[FileInfo]:
        """Top 10 largest files"""
        return self.file_inventory[:10]


class InPlacePackageMeasurer:
    """
    Measures package artifacts in-place and generates detailed reports.

    This class handles measurement of DEB, RPM, MSI, and other package formats
    directly in build jobs, creating comprehensive reports with file inventories.
    """

    def __init__(self, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize the measurer with configuration.

        Args:
            config_path: Path to the quality gates configuration file
        """
        self.config_path = config_path
        self.config = self._load_config()

    def _load_config(self) -> dict[str, Any]:
        """Load quality gates configuration from YAML file."""
        try:
            with open(self.config_path) as f:
                return yaml.safe_load(f)
        except FileNotFoundError:
            raise ValueError(f"Configuration file not found: {self.config_path}") from None
        except yaml.YAMLError as e:
            raise ValueError(f"Invalid YAML configuration: {e}") from e

    def measure_package(
        self,
        ctx: Context,
        package_path: str,
        gate_name: str,
        build_job_name: str,
        max_files: int = 10000,
        generate_checksums: bool = True,
        debug: bool = False,
    ) -> InPlaceArtifactReport:
        """
        Measure a package artifact and generate a comprehensive report.

        Args:
            ctx: Invoke context for running commands
            package_path: Path to the package file
            gate_name: Quality gate name from configuration
            build_job_name: Name of the CI job that built this package

        Returns:
            InPlaceArtifactReport with complete measurement data

        Raises:
            ValueError: If configuration is invalid or package not found
            RuntimeError: If measurement fails
        """
        # Validate inputs
        if not os.path.exists(package_path):
            raise ValueError(f"Package file not found: {package_path}")

        if gate_name not in self.config:
            raise ValueError(f"Gate configuration not found: {gate_name}")

        # Create quality gate config
        gate_config = create_quality_gate_config(gate_name, self.config[gate_name])

        # Single extraction and analysis (optimization 1A)
        measurement, file_inventory = self._extract_and_analyze_package(
            ctx, package_path, gate_config, max_files, generate_checksums, debug
        )

        # Extract artifact flavors from gate name
        artifact_flavors = self._extract_artifact_flavors(gate_name)

        # Create report
        return InPlaceArtifactReport(
            artifact_path=package_path,
            artifact_type="package",
            gate_name=gate_name,
            on_wire_size=measurement.on_wire_size,
            on_disk_size=measurement.on_disk_size,
            max_on_wire_size=gate_config.max_on_wire_size,
            max_on_disk_size=gate_config.max_on_disk_size,
            file_inventory=file_inventory,
            measurement_timestamp=datetime.now().astimezone().isoformat(),
            pipeline_id=os.environ.get("CI_PIPELINE_ID", "unknown"),
            commit_sha=os.environ.get("CI_COMMIT_SHA", "unknown"),
            arch=gate_config.arch,
            os=gate_config.os,
            build_job_name=build_job_name,
            artifact_flavors=artifact_flavors,
        )

    def _extract_and_analyze_package(
        self,
        ctx: Context,
        package_path: str,
        config: QualityGateConfig,
        max_files: int = 10000,
        generate_checksums: bool = True,
        debug: bool = False,
    ) -> tuple[ArtifactMeasurement, list[FileInfo]]:
        """
        Extract package once and perform both size measurement and file inventory.

        This optimization (1A) eliminates duplicate package extraction by combining
        size measurement and file inventory generation into a single extraction operation.

        Args:
            ctx: Invoke context for running commands
            package_path: Path to the package file
            config: Quality gate configuration
            max_files: Maximum number of files to process in inventory
            generate_checksums: Whether to generate checksums for files
            debug: Enable debug logging

        Returns:
            Tuple of (ArtifactMeasurement, list[FileInfo])

        Raises:
            RuntimeError: If extraction or analysis fails
        """
        try:
            # Measure wire size (compressed package file size)
            wire_size = file_size(package_path)

            with tempfile.TemporaryDirectory() as extract_dir:
                if debug:
                    print(f"📁 Extracting package to: {extract_dir}")

                # Extract package once for both measurements
                extract_package(ctx, config.os, package_path, extract_dir)

                # Measure disk size from extracted content
                disk_size = directory_size(extract_dir)

                # Create measurement object
                measurement = ArtifactMeasurement(
                    artifact_path=package_path, on_wire_size=wire_size, on_disk_size=disk_size
                )

                # Generate file inventory from the same extracted content
                file_inventory = self._walk_extracted_files(extract_dir, max_files, generate_checksums, debug)

                if debug:
                    print("✅ Single extraction completed:")
                    print(f"   • Wire size: {wire_size:,} bytes")
                    print(f"   • Disk size: {disk_size:,} bytes")
                    print(f"   • Files inventoried: {len(file_inventory):,}")

                return measurement, file_inventory

        except Exception as e:
            raise RuntimeError(f"Failed to extract and analyze package {package_path}: {e}") from e

    def _walk_extracted_files(
        self, extract_dir: str, max_files: int, generate_checksums: bool, debug: bool
    ) -> list[FileInfo]:
        """
        Walk through extracted files and create file inventory.

        This method is extracted from _generate_file_inventory to be reused
        by the optimized _extract_and_analyze_package method.

        Args:
            extract_dir: Directory containing extracted package files
            max_files: Maximum number of files to process
            generate_checksums: Whether to generate checksums for files
            debug: Enable debug logging

        Returns:
            List of FileInfo objects for all files in the package
        """
        extract_path = Path(extract_dir)

        # Verify extraction worked
        if not any(extract_path.iterdir()):
            if debug:
                print("⚠️  Warning: No files found after extraction")
            return []

        if debug:
            all_items = list(extract_path.rglob('*'))
            files_count = sum(1 for item in all_items if item.is_file())
            dirs_count = sum(1 for item in all_items if item.is_dir())
            print(f"📊 Found {files_count} files and {dirs_count} directories")

        file_inventory = []
        files_processed = 0
        total_size = 0

        # Walk through all files in the extracted package
        for file_path in extract_path.rglob('*'):
            if file_path.is_file():
                # Respect max_files limit
                if files_processed >= max_files:
                    if debug:
                        print(f"⚠️  Reached max files limit ({max_files}), stopping inventory")
                    break

                try:
                    relative_path = str(file_path.relative_to(extract_path))
                    file_stat = file_path.stat()
                    size_bytes = file_stat.st_size

                    # Generate checksum for larger files to help track changes
                    checksum = None
                    if generate_checksums and size_bytes > 1024:
                        checksum = self._generate_checksum(file_path)

                    file_inventory.append(
                        FileInfo(
                            relative_path=relative_path,
                            size_bytes=size_bytes,
                            checksum=checksum,
                        )
                    )

                    files_processed += 1
                    total_size += size_bytes

                    if debug and files_processed % 1000 == 0:
                        print(f"📋 Processed {files_processed} files...")

                except (OSError, PermissionError) as e:
                    if debug:
                        print(f"⚠️  Skipping file {file_path}: {e}")
                    continue

        # Sort by size (descending) for easier analysis
        file_inventory.sort(key=lambda f: f.size_bytes, reverse=True)

        if debug:
            print("✅ File inventory completed:")
            print(f"   • Total files processed: {files_processed}")
            print(f"   • Total size: {total_size:,} bytes ({total_size / 1024 / 1024:.2f} MB)")

        return file_inventory

    def _measure_package_sizes(self, ctx: Context, package_path: str, config: QualityGateConfig) -> ArtifactMeasurement:  # noqa: vulture
        """
        Measure package wire and disk sizes.

        DEPRECATED: This method is kept for backward compatibility but is no longer used
        by the main flow. Use _extract_and_analyze_package instead for better performance.

        Args:
            ctx: Invoke context
            package_path: Path to package file
            config: Quality gate configuration

        Returns:
            ArtifactMeasurement with size data
        """
        try:
            # Wire size is the compressed package file size
            wire_size = file_size(package_path)

            # Disk size requires extracting the package
            with tempfile.TemporaryDirectory() as extract_dir:
                extract_package(ctx, config.os, package_path, extract_dir)
                disk_size = directory_size(extract_dir)

            return ArtifactMeasurement(artifact_path=package_path, on_wire_size=wire_size, on_disk_size=disk_size)
        except Exception as e:
            raise RuntimeError(f"Failed to measure package sizes for {package_path}: {e}") from e

    def _generate_file_inventory(  # noqa: vulture
        self,
        ctx: Context,
        package_path: str,
        config: QualityGateConfig,
        max_files: int = 10000,
        generate_checksums: bool = True,
        debug: bool = False,
    ) -> list[FileInfo]:
        """
        Generate detailed file inventory for the package.

        DEPRECATED: This method is kept for backward compatibility but is no longer used
        by the main flow. Use _extract_and_analyze_package instead for better performance.

        Args:
            ctx: Invoke context
            package_path: Path to package file
            config: Quality gate configuration
            max_files: Maximum number of files to process (default: 10000)
            generate_checksums: Whether to generate checksums for binary files (default: True)
            debug: Enable debug logging (default: False)

        Returns:
            List of FileInfo objects for all files in the package
        """
        if debug:
            print(f"🔍 Starting file inventory generation for {package_path}")
            print(f"Package OS: {config.os}, Max files: {max_files}, Checksums: {generate_checksums}")

        try:
            with tempfile.TemporaryDirectory() as extract_dir:
                if debug:
                    print(f"📁 Extracting package to: {extract_dir}")

                # Extract package with error handling
                try:
                    extract_package(ctx, config.os, package_path, extract_dir)
                except Exception as e:
                    if debug:
                        print(f"❌ Package extraction failed: {e}")
                    raise RuntimeError(f"Package extraction failed: {e}") from e

                # Use the optimized file walking method
                return self._walk_extracted_files(extract_dir, max_files, generate_checksums, debug)

        except Exception as e:
            error_msg = f"Failed to generate file inventory for {package_path}: {e}"
            if debug:
                print(f"❌ {error_msg}")
                import traceback

                traceback.print_exc()
            raise RuntimeError(error_msg) from e

    def _generate_checksum(self, file_path: Path) -> str:
        """
        Generate SHA256 checksum for a file.

        Args:
            file_path: Path to the file

        Returns:
            SHA256 checksum as hex string
        """
        import hashlib

        try:
            sha256_hash = hashlib.sha256()
            with open(file_path, "rb") as f:
                # Read in chunks to handle large files
                for chunk in iter(lambda: f.read(4096), b""):
                    sha256_hash.update(chunk)
            return f"sha256:{sha256_hash.hexdigest()}"
        except Exception:
            # If checksum generation fails, return None rather than failing the whole measurement
            return None

    def _extract_artifact_flavors(self, gate_name: str) -> list[str]:
        """
        Extract artifact flavors from gate name.

        Args:
            gate_name: Quality gate name

        Returns:
            List of flavor strings
        """
        flavors = []

        # Check for specific flavors first (most specific to least specific)
        if "fips" in gate_name:
            flavors.append("fips")
            # Don't add "agent" if "fips" is already present
        elif "iot" in gate_name:
            flavors.append("iot")
        elif "dogstatsd" in gate_name:
            flavors.append("dogstatsd")
        elif "heroku" in gate_name:
            flavors.append("heroku")
        elif "agent" in gate_name:
            flavors.append("agent")

        return flavors if flavors else ["unknown"]

    def save_report_to_yaml(self, report: InPlaceArtifactReport, output_path: str) -> None:
        """
        Save the measurement report to a YAML file.

        Args:
            report: The measurement report to save
            output_path: Path where to save the YAML file
        """
        try:
            # Convert dataclass to dictionary
            report_dict = {
                "artifact_path": report.artifact_path,
                "artifact_type": report.artifact_type,
                "gate_name": report.gate_name,
                "on_wire_size": report.on_wire_size,
                "on_disk_size": report.on_disk_size,
                "max_on_wire_size": report.max_on_wire_size,
                "max_on_disk_size": report.max_on_disk_size,
                "measurement_timestamp": report.measurement_timestamp,
                "pipeline_id": report.pipeline_id,
                "commit_sha": report.commit_sha,
                "arch": report.arch,
                "os": report.os,
                "build_job_name": report.build_job_name,
                "artifact_flavors": report.artifact_flavors,
                "file_inventory": [
                    {
                        "relative_path": file_info.relative_path,
                        "size_bytes": file_info.size_bytes,
                        "checksum": file_info.checksum,
                    }
                    for file_info in report.file_inventory
                ],
            }

            with open(output_path, 'w') as f:
                yaml.dump(report_dict, f, default_flow_style=False, sort_keys=False)

        except Exception as e:
            raise RuntimeError(f"Failed to save report to {output_path}: {e}") from e


def measure_package_local(
    ctx,
    package_path,
    gate_name,
    config_path="test/static/static_quality_gates.yml",
    output_path=None,
    build_job_name="local_test",
    max_files=10000,
    no_checksums=False,
    debug=False,
):
    """
    Run the in-place package measurer locally for testing and development.

    This task allows you to test the measurement functionality on local packages
    without requiring a full CI environment.

    Args:
        package_path: Path to the package file to measure
        gate_name: Quality gate name from the configuration file
        config_path: Path to quality gates configuration (default: test/static/static_quality_gates.yml)
        output_path: Path to save the measurement report (default: {gate_name}_report.yml)
        build_job_name: Simulated build job name (default: local_test)
        max_files: Maximum number of files to process in inventory (default: 10000)
        no_checksums: Skip checksum generation for faster processing (default: false)
        debug: Enable debug logging for troubleshooting (default: false)

    Example:
        dda inv experimental-gates.measure-package-local --package-path /path/to/package.deb --gate-name static_quality_gate_agent_deb_amd64
    """
    from tasks.libs.common.color import color_message

    # Validate inputs
    if not os.path.exists(package_path):
        print(color_message(f"❌ Package file not found: {package_path}", "red"))
        return

    if not os.path.exists(config_path):
        print(color_message(f"❌ Configuration file not found: {config_path}", "red"))
        return

    # Set default output path if not provided
    if output_path is None:
        output_path = f"{gate_name}_report.yml"

    print(color_message("🔍 Starting in-place package measurement...", "cyan"))
    print(f"Package: {package_path}")
    print(f"Gate: {gate_name}")
    print(f"Config: {config_path}")
    print(f"Output: {output_path}")
    print("=" * 50)

    try:
        # Initialize the measurer
        measurer = InPlacePackageMeasurer(config_path=config_path)

        # Set mock environment variables if not present
        original_env = {}
        mock_env_vars = {
            "CI_PIPELINE_ID": "local_12345",
            "CI_COMMIT_SHA": "local_abc123def456789",
        }

        for var, value in mock_env_vars.items():
            if var not in os.environ:
                original_env[var] = os.environ.get(var, None)
                os.environ[var] = value
                print(color_message(f"🏷️  Set mock env var: {var}={value}", "yellow"))

        # Measure the package
        print(color_message("📏 Measuring package...", "cyan"))
        report = measurer.measure_package(
            ctx=ctx,
            package_path=package_path,
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

        if wire_usage_pct > 100 or disk_usage_pct > 100:
            print(color_message("⚠️  WARNING: Package exceeds size limits!", "red"))
        else:
            print(color_message("✅ Package within size limits", "green"))

        # Show top 10 largest files
        print("📁 Top 10 largest files:")
        for i, file_info in enumerate(report.largest_files, 1):
            print(f"   {i:2}. {file_info.relative_path} ({file_info.size_mb:.2f} MiB)")

    except Exception as e:
        print(color_message(f"❌ Measurement failed: {e}", "red"))
        # Restore original environment variables
        for var, value in original_env.items():
            if value is None:
                os.environ.pop(var, None)
            else:
                os.environ[var] = value
        raise

    finally:
        # Restore original environment variables
        for var, value in original_env.items():
            if value is None:
                os.environ.pop(var, None)
            else:
                os.environ[var] = value
