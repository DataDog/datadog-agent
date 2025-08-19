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
from typing import Any, Protocol

import yaml
from invoke import Context

from tasks.libs.package.size import extract_package, file_size
from tasks.static_quality_gates.gates import (
    QualityGateConfig,
    create_quality_gate_config,
)


class SizeMixin:
    """
    Mixin class providing size validation and conversion utilities.
    Classes using this mixin must have a size_bytes attribute.
    """

    # Type hint for the attribute that implementers must provide
    size_bytes: int

    def _validate_size_bytes(self, size_bytes: int) -> None:
        """Validate that size_bytes is non-negative"""
        if size_bytes < 0:
            raise ValueError("size_bytes must be non-negative")

    @property
    def size_mb(self) -> float:
        """Size in megabytes"""
        return self.size_bytes / (1024 * 1024)

    @property
    def size_kb(self) -> float:
        """Size in kilobytes"""
        return self.size_bytes / 1024

    @property
    def size_gb(self) -> float:
        """Size in gigabytes"""
        return self.size_bytes / (1024 * 1024 * 1024)


@dataclass(frozen=True)
class FileInfo(SizeMixin):
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
        self._validate_size_bytes(self.size_bytes)


@dataclass(frozen=True)
class DockerLayerInfo(SizeMixin):
    """
    Information about a single Docker layer within an image.
    """

    layer_id: str
    size_bytes: int
    created_by: str | None = None  # Dockerfile instruction that created this layer
    empty_layer: bool = False  # Whether this is an empty layer (metadata only)

    def __post_init__(self):
        """Validate Docker layer info data"""
        if not self.layer_id:
            raise ValueError("layer_id cannot be empty")
        self._validate_size_bytes(self.size_bytes)


@dataclass(frozen=True)
class DockerImageInfo:
    """
    Extended information specific to Docker images.
    """

    image_id: str
    image_tags: list[str]
    architecture: str
    os: str
    layers: list[DockerLayerInfo]
    config_size: int  # Size of the image config JSON
    manifest_size: int  # Size of the manifest

    def __post_init__(self):
        """Validate Docker image info data"""
        if not self.image_id:
            raise ValueError("image_id cannot be empty")
        if not self.architecture:
            raise ValueError("architecture cannot be empty")
        if not self.os:
            raise ValueError("os cannot be empty")
        if self.config_size < 0:
            raise ValueError("config_size must be non-negative")
        if self.manifest_size < 0:
            raise ValueError("manifest_size must be non-negative")

    @property
    def total_layers_size_bytes(self) -> int:
        """Total size of all layers in bytes"""
        return sum(layer.size_bytes for layer in self.layers)

    @property
    def total_layers_size_mb(self) -> float:
        """Total size of all layers in megabytes"""
        return self.total_layers_size_bytes / (1024 * 1024)

    @property
    def non_empty_layers(self) -> list[DockerLayerInfo]:
        """List of layers that actually contain file changes"""
        return [layer for layer in self.layers if not layer.empty_layer]

    @property
    def largest_layers(self) -> list[DockerLayerInfo]:
        """Top 10 largest layers by size"""
        return sorted(self.layers, key=lambda layer: layer.size_bytes, reverse=True)[:10]


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

    # Docker-specific metadata (optional)
    docker_info: DockerImageInfo | None = None

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


# Protocols for composition-based design
class ArtifactProcessor(Protocol):
    """Protocol for processing different types of artifacts (packages, Docker images, etc.)"""

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_config: QualityGateConfig,
        max_files: int,
        generate_checksums: bool,
        debug: bool,
    ) -> tuple[int, int, list[FileInfo], Any]:
        """
        Measure an artifact and return wire size, disk size, file inventory, and optional metadata.

        Returns:
            Tuple of (wire_size, disk_size, file_inventory, artifact_specific_metadata)
        """
        ...


class ConfigurationManager:
    """Shared configuration management for all artifact measurers."""

    def __init__(self, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize configuration manager.

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

    def get_gate_config(self, gate_name: str) -> QualityGateConfig:
        """Get configuration for a specific gate."""
        if gate_name not in self.config:
            raise ValueError(f"Gate configuration not found: {gate_name}")
        return create_quality_gate_config(gate_name, self.config[gate_name])


class FileUtilities:
    """Shared file processing utilities for all artifact types."""

    @staticmethod
    def generate_checksum(file_path: Path) -> str | None:
        """
        Generate SHA256 checksum for a file.

        Args:
            file_path: Path to the file

        Returns:
            SHA256 checksum as hex string, or None if generation fails
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

    @staticmethod
    def walk_files(
        directory: str,
        max_files: int,
        generate_checksums: bool,
        debug: bool
    ) -> list[FileInfo]:
        """
        Walk through files in a directory and create file inventory.

        Args:
            directory: Directory containing files to analyze
            max_files: Maximum number of files to process
            generate_checksums: Whether to generate checksums
            debug: Enable debug logging

        Returns:
            List of FileInfo objects for all files
        """
        directory_path = Path(directory)
        file_inventory = []
        files_processed = 0

        if debug:
            all_items = list(directory_path.rglob('*'))
            files_count = sum(1 for item in all_items if item.is_file())
            dirs_count = sum(1 for item in all_items if item.is_dir())
            print(f"📊 Found {files_count} files and {dirs_count} directories")

        for file_path in directory_path.rglob('*'):
            if file_path.is_file():
                # Respect max_files limit
                if files_processed >= max_files:
                    if debug:
                        print(f"⚠️  Reached max files limit ({max_files}), stopping inventory")
                    break

                try:
                    relative_path = str(file_path.relative_to(directory_path))
                    size_bytes = file_path.stat().st_size

                    checksum = None
                    if generate_checksums:
                        checksum = FileUtilities.generate_checksum(file_path)

                    file_inventory.append(
                        FileInfo(
                            relative_path=relative_path,
                            size_bytes=size_bytes,
                            checksum=checksum,
                        )
                    )

                    files_processed += 1

                    if debug and files_processed % 1000 == 0:
                        print(f"📋 Processed {files_processed} files...")

                except (OSError, PermissionError) as e:
                    if debug:
                        print(f"⚠️  Skipping file {file_path}: {e}")
                    continue

        # Sort by size (descending) for easier analysis
        file_inventory.sort(key=lambda f: f.size_bytes, reverse=True)
        return file_inventory

    @staticmethod
    def calculate_directory_size(directory: str) -> int:
        """
        Calculate total size of all files in a directory.

        Args:
            directory: Directory path to analyze

        Returns:
            Total size in bytes
        """
        total_size = 0
        for dirpath, _, filenames in os.walk(directory):
            for filename in filenames:
                file_path = os.path.join(dirpath, filename)
                try:
                    total_size += os.path.getsize(file_path)
                except (OSError, FileNotFoundError):
                    # Skip files that can't be accessed
                    continue
        return total_size


class ReportBuilder:
    """Shared report building functionality for all artifact types."""

    @staticmethod
    def create_report(
        artifact_ref: str,
        gate_name: str,
        gate_config: QualityGateConfig,
        wire_size: int,
        disk_size: int,
        file_inventory: list[FileInfo],
        build_job_name: str,
        artifact_metadata: Any = None,
    ) -> InPlaceArtifactReport:
        """
        Create a standardized artifact report.

        Args:
            artifact_ref: Reference to the artifact (path, image name, etc.)
            gate_name: Quality gate name
            gate_config: Gate configuration
            wire_size: Compressed size in bytes
            disk_size: Uncompressed size in bytes
            file_inventory: List of files in the artifact
            build_job_name: Name of the build job
            artifact_metadata: Optional artifact-specific metadata

        Returns:
            Complete InPlaceArtifactReport
        """
        return InPlaceArtifactReport(
            artifact_path=artifact_ref,
            gate_name=gate_name,
            on_wire_size=wire_size,
            on_disk_size=disk_size,
            max_on_wire_size=gate_config.max_on_wire_size,
            max_on_disk_size=gate_config.max_on_disk_size,
            file_inventory=file_inventory,
            measurement_timestamp=datetime.now().astimezone().isoformat(),
            pipeline_id=os.environ.get("CI_PIPELINE_ID", "unknown"),
            commit_sha=os.environ.get("CI_COMMIT_SHA", "unknown"),
            arch=gate_config.arch,
            os=gate_config.os,
            build_job_name=build_job_name,
            docker_info=artifact_metadata if isinstance(artifact_metadata, DockerImageInfo) else None,
        )

    @staticmethod
    def save_report_to_yaml(report: InPlaceArtifactReport, output_path: str) -> None:
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
                "file_inventory": [
                    {
                        "relative_path": file_info.relative_path,
                        "size_bytes": file_info.size_bytes,
                        "checksum": file_info.checksum,
                    }
                    for file_info in report.file_inventory
                ],
            }

            # Add Docker-specific information if present
            if report.docker_info:
                report_dict["docker_info"] = {
                    "image_id": report.docker_info.image_id,
                    "image_tags": report.docker_info.image_tags,
                    "architecture": report.docker_info.architecture,
                    "os": report.docker_info.os,
                    "config_size": report.docker_info.config_size,
                    "manifest_size": report.docker_info.manifest_size,
                    "layers": [
                        {
                            "layer_id": layer.layer_id,
                            "size_bytes": layer.size_bytes,
                            "created_by": layer.created_by,
                            "empty_layer": layer.empty_layer,
                        }
                        for layer in report.docker_info.layers
                    ],
                }

            with open(output_path, 'w') as f:
                yaml.dump(report_dict, f, default_flow_style=False, sort_keys=False)

        except Exception as e:
            raise RuntimeError(f"Failed to save report to {output_path}: {e}") from e


class UniversalArtifactMeasurer:
    """
    Universal artifact measurer using composition and protocols.

    This class can measure any type of artifact by accepting different
    ArtifactProcessor implementations.
    """

    def __init__(
        self,
        processor: ArtifactProcessor,
        config_path: str = "test/static/static_quality_gates.yml"
    ):
        """
        Initialize the universal measurer with a specific artifact processor.

        Args:
            processor: Artifact processor implementation (package, Docker, etc.)
            config_path: Path to the quality gates configuration file
        """
        self.processor = processor
        self.config_manager = ConfigurationManager(config_path)
        self.file_utils = FileUtilities()
        self.report_builder = ReportBuilder()

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_name: str,
        build_job_name: str,
        max_files: int = 20000,
        generate_checksums: bool = True,
        debug: bool = False,
    ) -> InPlaceArtifactReport:
        """
        Measure an artifact using the configured processor.

        Args:
            ctx: Invoke context for running commands
            artifact_ref: Reference to the artifact (path, image name, etc.)
            gate_name: Quality gate name from configuration
            build_job_name: Name of the CI job that built this artifact
            max_files: Maximum number of files to process in inventory
            generate_checksums: Whether to generate checksums for files
            debug: Enable debug logging

        Returns:
            InPlaceArtifactReport with complete measurement data

        Raises:
            ValueError: If configuration is invalid or artifact not found
            RuntimeError: If measurement fails
        """
        gate_config = self.config_manager.get_gate_config(gate_name)

        wire_size, disk_size, file_inventory, artifact_metadata = self.processor.measure_artifact(
            ctx, artifact_ref, gate_config, max_files, generate_checksums, debug
        )

        return self.report_builder.create_report(
            artifact_ref=artifact_ref,
            gate_name=gate_name,
            gate_config=gate_config,
            wire_size=wire_size,
            disk_size=disk_size,
            file_inventory=file_inventory,
            build_job_name=build_job_name,
            artifact_metadata=artifact_metadata,
        )

    def save_report_to_yaml(self, report: InPlaceArtifactReport, output_path: str) -> None:
        """Save the measurement report to a YAML file."""
        self.report_builder.save_report_to_yaml(report, output_path)


# Specific processor implementations
class PackageProcessor:
    """Package artifact processor implementing the ArtifactProcessor protocol."""

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_config: QualityGateConfig,
        max_files: int,
        generate_checksums: bool,
        debug: bool,
    ) -> tuple[int, int, list[FileInfo], Any]:
        """Measure package artifact using extraction and analysis."""
        # Verify package exists
        if not os.path.exists(artifact_ref):
            raise ValueError(f"Package file not found: {artifact_ref}")

        if debug:
            print(f"📦 Measuring package: {artifact_ref}")

        # Measure wire size (compressed package file)
        wire_size = file_size(artifact_ref)

        # Extract package and measure disk size + file inventory
        with tempfile.TemporaryDirectory() as extract_dir:
            if debug:
                print(f"📁 Extracting package to: {extract_dir}")

            extract_package(ctx, gate_config.os, artifact_ref, extract_dir)
            disk_size = FileUtilities.calculate_directory_size(extract_dir)
            file_inventory = FileUtilities.walk_files(extract_dir, max_files, generate_checksums, debug)

            if debug:
                print("✅ Package analysis completed:")
                print(f"   • Wire size: {wire_size:,} bytes")
                print(f"   • Disk size: {disk_size:,} bytes")
                print(f"   • Files inventoried: {len(file_inventory):,}")

            return wire_size, disk_size, file_inventory, None


class DockerProcessor:
    """Docker image processor implementing the ArtifactProcessor protocol."""

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_config: QualityGateConfig,
        max_files: int,
        generate_checksums: bool,
        debug: bool,
    ) -> tuple[int, int, list[FileInfo], DockerImageInfo]:
        """Measure Docker image using docker save and docker export."""
        if debug:
            print(f"🐳 Measuring Docker image: {artifact_ref}")

        # Ensure image is available locally
        self._ensure_image_available(ctx, artifact_ref, debug)

        # Measure wire size using docker save
        wire_size = self._measure_wire_size_with_docker_save(ctx, artifact_ref, debug)

        # Analyze image using docker export
        disk_size, file_inventory, docker_info = self._analyze_image_with_docker_export(
            ctx, artifact_ref, max_files, generate_checksums, debug
        )

        return wire_size, disk_size, file_inventory, docker_info

    def _ensure_image_available(self, ctx: Context, image_ref: str, debug: bool = False) -> None:
        """Ensure the Docker image is available locally."""
        try:
            # Check if image exists locally
            result = ctx.run(f"docker image inspect {image_ref}", hide=True, warn=True)
            if result.exited == 0:
                if debug:
                    print(f"✅ Image {image_ref} found locally")
                return

            # Try to pull the image
            if debug:
                print(f"📥 Pulling image {image_ref}...")

            pull_result = ctx.run(f"docker pull {image_ref}", warn=True)
            if pull_result.exited != 0:
                raise RuntimeError(f"Failed to pull Docker image {image_ref}")

            if debug:
                print(f"✅ Successfully pulled image {image_ref}")

        except Exception as e:
            raise RuntimeError(f"Failed to ensure image {image_ref} is available: {e}") from e

    def _measure_wire_size_with_docker_save(self, ctx: Context, image_ref: str, debug: bool = False) -> int:
        """Measure Docker image compressed size using docker save."""
        try:
            if debug:
                print(f"📏 Measuring wire size for {image_ref} using docker save...")

            with tempfile.NamedTemporaryFile(suffix='.tar', delete=True) as temp_file:
                # Save image to temporary file
                save_result = ctx.run(f"docker save {image_ref} -o {temp_file.name}", warn=True)
                if save_result.exited != 0:
                    raise RuntimeError(f"Docker save failed for {image_ref}")

                # Get file size
                wire_size = os.path.getsize(temp_file.name)

                if debug:
                    print(f"✅ Wire size measurement completed: {wire_size:,} bytes ({wire_size / 1024 / 1024:.2f} MB)")

                return wire_size

        except Exception as e:
            raise RuntimeError(f"Failed to measure wire size for {image_ref}: {e}") from e

    def _analyze_image_with_docker_export(
        self,
        ctx: Context,
        image_ref: str,
        max_files: int,
        generate_checksums: bool,
        debug: bool = False,
    ) -> tuple[int, list[FileInfo], DockerImageInfo | None]:
        """Analyze Docker image using docker export for file inventory and disk size."""
        container_id = None
        try:
            if debug:
                print(f"🔍 Analyzing image {image_ref} using docker export...")

            # Create a temporary container from the image
            create_result = ctx.run(f"docker create {image_ref}", hide=True)
            container_id = create_result.stdout.strip()

            if debug:
                print(f"📦 Created temporary container: {container_id[:12]}")

            # Export container filesystem and analyze
            with tempfile.TemporaryDirectory() as extract_dir:
                # Export container to tarball and extract
                export_cmd = f"docker export {container_id} | tar -xf - -C {extract_dir}"
                export_result = ctx.run(export_cmd, warn=True)
                if export_result.exited != 0:
                    raise RuntimeError(f"Docker export failed for container {container_id}")

                # Calculate total disk size
                disk_size = FileUtilities.calculate_directory_size(extract_dir)

                # Generate file inventory
                file_inventory = FileUtilities.walk_files(extract_dir, max_files, generate_checksums, debug)

                # Extract Docker metadata
                docker_info = self._extract_docker_metadata(ctx, image_ref, debug)

                if debug:
                    print("✅ Image analysis completed:")
                    print(f"   • Disk size: {disk_size:,} bytes ({disk_size / 1024 / 1024:.2f} MB)")
                    print(f"   • Files inventoried: {len(file_inventory):,}")

                return disk_size, file_inventory, docker_info

        except Exception as e:
            raise RuntimeError(f"Failed to analyze image {image_ref}: {e}") from e
        finally:
            # Clean up temporary container
            if container_id:
                try:
                    ctx.run(f"docker rm {container_id}", hide=True, warn=True)
                    if debug:
                        print(f"🗑️  Cleaned up temporary container: {container_id[:12]}")
                except Exception:
                    pass  # Best effort cleanup

    def _extract_docker_metadata(self, ctx: Context, image_ref: str, debug: bool = False) -> DockerImageInfo | None:
        """Extract Docker image metadata using docker inspect and docker history."""
        try:
            if debug:
                print(f"🔍 Extracting Docker metadata for {image_ref}...")

            # Get image inspection data
            inspect_result = ctx.run(f"docker inspect {image_ref}", hide=True)
            import json
            inspect_data = json.loads(inspect_result.stdout)[0]

            # Extract basic metadata
            image_id = inspect_data["Id"]
            image_tags = inspect_data.get("RepoTags", [])
            architecture = inspect_data["Architecture"]
            os_name = inspect_data["Os"]
            config_size = len(json.dumps(inspect_data["Config"]).encode('utf-8'))

            # Calculate manifest size (approximation)
            manifest_size = len(inspect_result.stdout.encode('utf-8'))

            # Get layer information
            layers = self._analyze_docker_layers(ctx, image_ref, debug)

            return DockerImageInfo(
                image_id=image_id,
                image_tags=image_tags,
                architecture=architecture,
                os=os_name,
                layers=layers,
                config_size=config_size,
                manifest_size=manifest_size,
            )

        except Exception as e:
            if debug:
                print(f"⚠️  Failed to extract Docker metadata: {e}")
            return None

    def _analyze_docker_layers(self, ctx: Context, image_ref: str, debug: bool = False) -> list[DockerLayerInfo]:
        """Analyze Docker image layers using docker history."""
        try:
            if debug:
                print(f"📊 Analyzing layers for {image_ref}...")

            # Get layer history with size information
            history_result = ctx.run(
                f'docker history {image_ref} --format "table {{{{.ID}}}}\\t{{{{.Size}}}}\\t{{{{.CreatedBy}}}}"',
                hide=True
            )

            layers = []
            lines = history_result.stdout.strip().split('\n')[1:]  # Skip header

            for line in lines:
                parts = line.split('\t', 2)
                if len(parts) >= 3:
                    layer_id = parts[0].strip()
                    size_str = parts[1].strip()
                    created_by = parts[2].strip() if len(parts) > 2 else None

                    # Parse size (handle formats like "0B", "123MB", "1.2GB")
                    size_bytes = self._parse_docker_size(size_str)
                    empty_layer = size_bytes == 0

                    layers.append(DockerLayerInfo(
                        layer_id=layer_id,
                        size_bytes=size_bytes,
                        created_by=created_by,
                        empty_layer=empty_layer,
                    ))

            if debug:
                print(f"✅ Found {len(layers)} layers")

            return layers

        except Exception as e:
            if debug:
                print(f"⚠️  Failed to analyze layers: {e}")
            return []

    def _parse_docker_size(self, size_str: str) -> int:
        """Parse Docker size string (e.g., "123MB", "1.2GB", "0B") to bytes."""
        size_str = size_str.strip()
        if size_str == "0B" or size_str == "0":
            return 0

        # Extract number and unit
        import re
        match = re.match(r'([0-9.]+)([A-Za-z]*)', size_str)
        if not match:
            return 0

        value = float(match.group(1))
        unit = match.group(2).upper()

        # Convert to bytes
        multipliers = {
            'B': 1,
            'KB': 1024,
            'MB': 1024 ** 2,
            'GB': 1024 ** 3,
            'TB': 1024 ** 4,
        }

        return int(value * multipliers.get(unit, 1))


class InPlacePackageMeasurer:
    """
    Measures package artifacts in-place and generates detailed reports.

    This class handles measurement of DEB, RPM, MSI, and other package formats
    directly in build jobs, creating comprehensive reports with file inventories.

    Uses composition with UniversalArtifactMeasurer and PackageProcessor.
    """

    def __init__(self, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize the measurer with configuration.

        Args:
            config_path: Path to the quality gates configuration file
        """
        self._measurer = UniversalArtifactMeasurer(
            processor=PackageProcessor(),
            config_path=config_path
        )

    def measure_package(
        self,
        ctx: Context,
        package_path: str,
        gate_name: str,
        build_job_name: str,
        max_files: int = 20000,
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
            max_files: Maximum number of files to process in inventory
            generate_checksums: Whether to generate checksums for files
            debug: Enable debug logging

        Returns:
            InPlaceArtifactReport with complete measurement data

        Raises:
            ValueError: If configuration is invalid or package not found
            RuntimeError: If measurement fails
        """
        return self._measurer.measure_artifact(
            ctx=ctx,
            artifact_ref=package_path,
            gate_name=gate_name,
            build_job_name=build_job_name,
            max_files=max_files,
            generate_checksums=generate_checksums,
            debug=debug,
        )

    def save_report_to_yaml(self, report: InPlaceArtifactReport, output_path: str) -> None:
        """Save the measurement report to a YAML file."""
        self._measurer.save_report_to_yaml(report, output_path)


class InPlaceDockerMeasurer:
    """
    Measures Docker image artifacts in-place and generates detailed reports.

    This class handles measurement of Docker images directly in build jobs or locally,
    using 'docker save' for wire size measurement and 'docker export' for comprehensive
    file analysis and uncompressed disk size calculation.

    Uses composition with UniversalArtifactMeasurer and DockerProcessor.
    """

    def __init__(self, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize the Docker image measurer with configuration.

        Args:
            config_path: Path to the quality gates configuration file
        """
        self._measurer = UniversalArtifactMeasurer(
            processor=DockerProcessor(),
            config_path=config_path
        )

    def measure_image(
        self,
        ctx: Context,
        image_ref: str,
        gate_name: str,
        build_job_name: str,
        max_files: int = 20000,
        generate_checksums: bool = True,
        include_layer_analysis: bool = True,
        debug: bool = False,
    ) -> InPlaceArtifactReport:
        """
        Measure a Docker image and generate a comprehensive report.

        Args:
            ctx: Invoke context for running commands
            image_ref: Docker image reference (tag, digest, or image ID)
            gate_name: Quality gate name from configuration
            build_job_name: Name of the CI job that built this image
            max_files: Maximum number of files to process in inventory
            generate_checksums: Whether to generate checksums for files
            include_layer_analysis: Whether to analyze individual layers (ignored, always included)
            debug: Enable debug logging

        Returns:
            InPlaceArtifactReport with complete measurement data

        Raises:
            ValueError: If configuration is invalid or image not found
            RuntimeError: If measurement fails
        """
        return self._measurer.measure_artifact(
            ctx=ctx,
            artifact_ref=image_ref,
            gate_name=gate_name,
            build_job_name=build_job_name,
            max_files=max_files,
            generate_checksums=generate_checksums,
            debug=debug,
        )

    def save_report_to_yaml(self, report: InPlaceArtifactReport, output_path: str) -> None:
        """Save the measurement report to a YAML file."""
        self._measurer.save_report_to_yaml(report, output_path)


def measure_package_local(
    ctx,
    package_path,
    gate_name,
    config_path="test/static/static_quality_gates.yml",
    output_path=None,
    build_job_name="local_test",
    max_files=20000,
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
        max_files: Maximum number of files to process in inventory (default: 20000)
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
        raise
