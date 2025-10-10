"""
Experimental Static Quality Gates implementation for in-place artifact measurement.

This module provides experimental functionality for measuring artifacts directly
in build jobs, generating detailed reports with file inventories for comparison.
"""

import os
import tempfile
from dataclasses import dataclass
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

    For regular files, only relative_path, size_bytes, and optionally checksum are set.
    For symlinks, is_symlink is True and symlink_target contains the link target.
    For broken symlinks, is_broken is True.
    """

    relative_path: str
    size_bytes: int
    checksum: str | None = None
    is_symlink: bool | None = None
    symlink_target: str | None = None
    is_broken: bool | None = None

    def __post_init__(self):
        """Validate file info data"""
        if not self.relative_path:
            raise ValueError("relative_path cannot be empty")
        if self.size_bytes < 0:
            raise ValueError("size_bytes must be non-negative")
        if self.is_symlink and not self.symlink_target:
            raise ValueError("symlink_target must be provided when is_symlink is True")

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
        if not os.path.exists(package_path):
            raise ValueError(f"Package file not found: {package_path}")

        if gate_name not in self.config:
            raise ValueError(f"Gate configuration not found: {gate_name}")

        gate_config = create_quality_gate_config(gate_name, self.config[gate_name])

        measurement, file_inventory = self._extract_and_analyze_package(
            ctx, package_path, gate_config, generate_checksums, debug
        )

        return InPlaceArtifactReport(
            artifact_path=package_path,
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
        )

    def _extract_and_analyze_package(
        self,
        ctx: Context,
        package_path: str,
        config: QualityGateConfig,
        generate_checksums: bool = True,
        debug: bool = False,
    ) -> tuple[ArtifactMeasurement, list[FileInfo]]:
        """
        Extract package and perform size measurement and file inventory.

        Args:
            ctx: Invoke context for running commands
            package_path: Path to the package file
            config: Quality gate configuration
            generate_checksums: Whether to generate checksums for files
            debug: Enable debug logging

        Returns:
            Tuple of (ArtifactMeasurement, list[FileInfo])

        Raises:
            RuntimeError: If extraction or analysis fails
        """
        try:
            wire_size = file_size(package_path)

            with tempfile.TemporaryDirectory() as extract_dir:
                if debug:
                    print(f"📁 Extracting package to: {extract_dir}")

                extract_package(ctx, config.os, package_path, extract_dir)

                disk_size = directory_size(extract_dir)

                measurement = ArtifactMeasurement(
                    artifact_path=package_path, on_wire_size=wire_size, on_disk_size=disk_size
                )

                file_inventory = self._walk_extracted_files(extract_dir, generate_checksums, debug)

                if debug:
                    print("✅ Single extraction completed:")
                    print(f"   • Wire size: {wire_size:,} bytes")
                    print(f"   • Disk size: {disk_size:,} bytes")
                    print(f"   • Files inventoried: {len(file_inventory):,}")

                return measurement, file_inventory

        except Exception as e:
            raise RuntimeError(f"Failed to extract and analyze package {package_path}: {e}") from e

    def _walk_extracted_files(self, extract_dir: str, generate_checksums: bool, debug: bool) -> list[FileInfo]:
        """
        Walk through extracted files and create file inventory.

        Args:
            extract_dir: Directory containing extracted package files
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

        for file_path in extract_path.rglob('*'):
            # Skip directories
            if file_path.is_dir():
                continue

            try:
                relative_path = str(file_path.relative_to(extract_path))

                if file_path.is_symlink():
                    try:
                        symlink_target = os.readlink(file_path)
                        logical_size = len(symlink_target)
                        is_broken = False

                        try:
                            resolved_target = file_path.resolve(strict=True)
                            if resolved_target.is_relative_to(extract_path):
                                symlink_target_rel = str(resolved_target.relative_to(extract_path))
                            else:
                                symlink_target_rel = symlink_target
                        except (OSError, RuntimeError):
                            symlink_target_rel = symlink_target
                            is_broken = True

                        file_inventory.append(
                            FileInfo(
                                relative_path=relative_path,
                                size_bytes=logical_size,
                                checksum=None,
                                is_symlink=True,
                                symlink_target=symlink_target_rel,
                                is_broken=is_broken if is_broken else None,
                            )
                        )

                        if debug and files_processed % 1000 == 0:
                            broken_marker = " [BROKEN]" if is_broken else ""
                            print(f"🔗 Symlink: {relative_path} -> {symlink_target_rel}{broken_marker}")

                    except OSError as e:
                        if debug:
                            print(f"⚠️  Could not read symlink {file_path}: {e}")
                        continue

                elif file_path.is_file():
                    # Regular file - use lstat to not follow symlinks
                    file_stat = file_path.lstat()
                    size_bytes = file_stat.st_size

                    checksum = self._generate_checksum(file_path) if generate_checksums else None

                    file_inventory.append(
                        FileInfo(
                            relative_path=relative_path,
                            size_bytes=size_bytes,
                            checksum=checksum,
                        )
                    )

                    total_size += size_bytes

                    if debug and files_processed % 1000 == 0:
                        print(f"📋 Processed {files_processed} files...")

                files_processed += 1

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
                "file_inventory": [self._serialize_file_info(file_info) for file_info in report.file_inventory],
            }

            with open(output_path, 'w') as f:
                yaml.dump(report_dict, f, default_flow_style=False, sort_keys=False)

        except Exception as e:
            raise RuntimeError(f"Failed to save report to {output_path}: {e}") from e

    def _serialize_file_info(self, file_info: FileInfo) -> dict[str, Any]:
        """
        Serialize a FileInfo object to a dictionary, excluding None/False fields for regular files.

        Args:
            file_info: The FileInfo object to serialize

        Returns:
            Dictionary with only relevant fields
        """
        result = {
            "relative_path": file_info.relative_path,
            "size_bytes": file_info.size_bytes,
        }

        if file_info.checksum is not None:
            result["checksum"] = file_info.checksum

        if file_info.is_symlink:
            result["is_symlink"] = True
            result["symlink_target"] = file_info.symlink_target
            if file_info.is_broken:
                result["is_broken"] = True

        return result


def measure_package_local(
    ctx,
    package_path,
    gate_name,
    config_path="test/static/static_quality_gates.yml",
    output_path=None,
    build_job_name="local_test",
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
        no_checksums: Skip checksum generation for faster processing (default: false)
        debug: Enable debug logging for troubleshooting (default: false)

    Example:
        dda inv experimental-gates.measure-package-local --package-path /path/to/package.deb --gate-name static_quality_gate_agent_deb_amd64
    """
    from tasks.libs.common.color import color_message

    if not os.path.exists(package_path):
        print(color_message(f"❌ Package file not found: {package_path}", "red"))
        return

    if not os.path.exists(config_path):
        print(color_message(f"❌ Configuration file not found: {config_path}", "red"))
        return

    if output_path is None:
        output_path = f"{gate_name}_report.yml"

    print(color_message("🔍 Starting in-place package measurement...", "cyan"))
    print(f"Package: {package_path}")
    print(f"Gate: {gate_name}")
    print(f"Config: {config_path}")
    print(f"Output: {output_path}")
    print("=" * 50)

    try:
        measurer = InPlacePackageMeasurer(config_path=config_path)

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

        print(color_message("📏 Measuring package...", "cyan"))
        report = measurer.measure_package(
            ctx=ctx,
            package_path=package_path,
            gate_name=gate_name,
            build_job_name=build_job_name,
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
        raise
