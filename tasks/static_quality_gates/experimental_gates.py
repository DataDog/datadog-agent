"""
Experimental Static Quality Gates implementation for in-place artifact measurement.

This module provides experimental functionality for measuring artifacts directly
in build jobs, generating detailed reports with file inventories for comparison.
"""

import os
import stat
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

    size_bytes: int

    def _validate_size_bytes(self, size_bytes: int) -> None:
        """Validate that size_bytes is positive"""
        if size_bytes < 0:
            raise ValueError("size_bytes must be positive")

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
    chmod: int | None = None
    owner: int | None = None
    group: int | None = None

    def __post_init__(self):
        """Validate file info data"""
        if not self.relative_path:
            raise ValueError("relative_path cannot be empty")
        self._validate_size_bytes(self.size_bytes)
        if self.is_symlink and not self.symlink_target:
            raise ValueError("symlink_target must be provided when is_symlink is True")


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

    image_ref: str
    architecture: str
    os: str
    layers: list[DockerLayerInfo]
    config_size: int  # Size of the image config JSON
    manifest_size: int  # Size of the manifest

    def __post_init__(self):
        """Validate Docker image info data"""
        if not self.image_ref:
            raise ValueError("image_ref cannot be empty")
        if not self.architecture:
            raise ValueError("architecture cannot be empty")
        if not self.os:
            raise ValueError("os cannot be empty")
        if self.config_size < 0:
            raise ValueError("config_size must be positive")
        if self.manifest_size < 0:
            raise ValueError("manifest_size must be positive")

    @property
    def total_layers_size_bytes(self) -> int:
        """Total size of all layers in bytes"""
        return sum(layer.size_bytes for layer in self.layers)

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
    Shared across package and docker image measurements.
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
    # TODO(agent-build): integrate GateMetricHandler metadata
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
            raise ValueError("on_wire_size must be positive")
        if self.on_disk_size < 0:
            raise ValueError("on_disk_size must be positive")

    @property
    def largest_files(self) -> list[FileInfo]:
        """Top 10 largest files"""
        return self.file_inventory[:10]


class ArtifactProcessor(Protocol):
    """Protocol for processing different types of artifacts (packages, docker images, etc.)"""

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_config: QualityGateConfig,
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
            with open(file_path, "rb") as f:
                sha256_hash = hashlib.file_digest(f, "sha256")
            return f"sha256:{sha256_hash.hexdigest()}"
        except Exception:
            # If checksum generation fails, return None rather than failing the whole measurement
            return None

    @staticmethod
    def walk_files(directory: str, debug: bool) -> list[FileInfo]:
        """
        Walk through files in a directory and create file inventory.

        Args:
            directory: Directory containing files to analyze
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
            # Skip directories
            if file_path.is_dir():
                continue

            try:
                relative_path = str(file_path.relative_to(directory_path))
                # Regular file - use lstat to not follow symlinks
                file_stat = file_path.lstat()
                chmod = stat.S_IMODE(file_stat.st_mode)

                if file_path.is_symlink():
                    try:
                        symlink_target = os.readlink(file_path)
                        logical_size = len(symlink_target)
                        is_broken = False

                        try:
                            resolved_target = file_path.resolve(strict=True)
                            if resolved_target.is_relative_to(directory_path):
                                symlink_target_rel = str(resolved_target.relative_to(directory_path))
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
                                chmod=chmod,
                                owner=file_stat.st_uid,
                                group=file_stat.st_gid,
                            )
                        )

                        if debug and files_processed % 1000 == 0:
                            broken_marker = " [BROKEN]" if is_broken else ""
                            print(f"🔗 Symlink: {relative_path} -> {symlink_target_rel}{broken_marker}")

                        files_processed += 1

                    except OSError as e:
                        if debug:
                            print(f"⚠️  Could not read symlink {file_path}: {e}")
                        continue

                elif file_path.is_file():
                    # Always generate checksums for regular files
                    size_bytes = file_stat.st_size
                    checksum = FileUtilities.generate_checksum(file_path)

                    file_inventory.append(
                        FileInfo(
                            relative_path=relative_path,
                            size_bytes=size_bytes,
                            checksum=checksum,
                            chmod=chmod,
                            owner=file_stat.st_uid,
                            group=file_stat.st_gid,
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

        Handles hard links correctly by tracking inodes and counting each unique file only once,
        matching the behavior of 'du --apparent-size' or 'du -sb'.

        Args:
            directory: Directory path to analyze

        Returns:
            Total size in bytes
        """
        total_size = 0
        seen_inodes = set()  # Track (device, inode) pairs to avoid counting hard links multiple times

        def walk_directory(path: str):
            """Recursively walk directory using scandir for better performance."""
            nonlocal total_size
            try:
                with os.scandir(path) as entries:
                    for entry in entries:
                        try:
                            # Use stat(follow_symlinks=False) to not follow symlinks
                            # This is equivalent to lstat but reuses cached information from scandir
                            stat_info = entry.stat(follow_symlinks=False)

                            if entry.is_dir(follow_symlinks=False):
                                # Recursively process subdirectory
                                walk_directory(entry.path)
                            else:
                                # For regular files with multiple hard links, only count once per (device, inode) pair
                                if stat.S_ISREG(stat_info.st_mode) and stat_info.st_nlink > 1:
                                    file_id = (stat_info.st_dev, stat_info.st_ino)
                                    if file_id in seen_inodes:
                                        # This is a hard link to a file we've already counted, skip it
                                        continue
                                    seen_inodes.add(file_id)

                                # Include both regular files and symlinks
                                # For symlinks: on POSIX, st_size equals len(os.readlink(path)) (length of target path)
                                #               on Windows, st_size equals 0
                                total_size += stat_info.st_size
                        except (OSError, FileNotFoundError) as e:
                            print(f"⚠️  Skipping {entry.path}: {e}")
                            continue
            except (OSError, PermissionError) as e:
                print(f"⚠️  Cannot access directory {path}: {e}")

        walk_directory(directory)
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

            # Add Docker-specific information if present
            if report.docker_info:
                report_dict["docker_info"] = {
                    "image_ref": report.docker_info.image_ref,
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
        if file_info.chmod is not None:
            result["chmod"] = file_info.chmod
        if file_info.owner is not None:
            result["owner"] = file_info.owner
        if file_info.group is not None:
            result["group"] = file_info.group

        if file_info.is_symlink:
            result["is_symlink"] = True
            result["symlink_target"] = file_info.symlink_target
            if file_info.is_broken:
                result["is_broken"] = True

        return result


class UniversalArtifactMeasurer:
    """
    Artifact measurer that can measure any type of artifact by accepting different
    ArtifactProcessor implementations.
    """

    def __init__(self, processor: ArtifactProcessor, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize the universal measurer with a specific artifact processor.

        Args:
            processor: Artifact processor implementation (package, Docker, etc.)
            config_path: Path to the quality gates configuration file
        """
        self.processor = processor
        self.config_manager = ConfigurationManager(config_path)
        self.report_builder = ReportBuilder()

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_name: str,
        build_job_name: str,
        debug: bool = False,
    ) -> InPlaceArtifactReport:
        """
        Measure an artifact using the configured processor.

        Args:
            ctx: Invoke context for running commands
            artifact_ref: Reference to the artifact (path, image name, etc.)
            gate_name: Quality gate name from configuration
            build_job_name: Name of the CI job that built this artifact
            debug: Enable debug logging

        Returns:
            InPlaceArtifactReport with complete measurement data

        Raises:
            ValueError: If configuration is invalid or artifact not found
            RuntimeError: If measurement fails
        """
        gate_config = self.config_manager.get_gate_config(gate_name)

        wire_size, disk_size, file_inventory, artifact_metadata = self.processor.measure_artifact(
            ctx, artifact_ref, gate_config, debug
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


class PackageProcessor:
    """Package artifact processor implementing the ArtifactProcessor protocol."""

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_config: QualityGateConfig,
        debug: bool,
    ) -> tuple[int, int, list[FileInfo], Any]:
        """Measure package artifact using extraction and analysis."""
        if not os.path.exists(artifact_ref):
            raise ValueError(f"Package file not found: {artifact_ref}")

        if debug:
            print(f"📦 Measuring package: {artifact_ref}")

        wire_size = file_size(artifact_ref)

        with tempfile.TemporaryDirectory() as extract_dir:
            if debug:
                print(f"📁 Extracting package to: {extract_dir}")

            extract_package(ctx, gate_config.os, artifact_ref, extract_dir)
            disk_size = FileUtilities.calculate_directory_size(extract_dir)
            file_inventory = FileUtilities.walk_files(extract_dir, debug)

            if debug:
                print("✅ Package analysis completed:")
                print(f"   • Wire size: {wire_size:,} bytes")
                print(f"   • Disk size: {disk_size:,} bytes")
                print(f"   • Files inventoried: {len(file_inventory):,}")

            return wire_size, disk_size, file_inventory, None


class DockerProcessor:
    """Docker image processor implementing the ArtifactProcessor protocol.

    Uses crane manifest for wire size calculation and crane pull for
    disk analysis and file inventory. This provides consistent wire size measurements
    regardless of image layer structure while maintaining detailed file analysis.
    """

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_config: QualityGateConfig,
        debug: bool,
    ) -> tuple[int, int, list[FileInfo], DockerImageInfo]:
        """Measure Docker image using manifest inspection for wire size and crane pull for disk analysis."""
        if debug:
            print(f"🐳 Measuring Docker image: {artifact_ref}")

        wire_size = self._get_wire_size(ctx, artifact_ref, debug)

        disk_size, file_inventory, docker_info = self._measure_on_disk_size(ctx, artifact_ref, debug)

        return wire_size, disk_size, file_inventory, docker_info

    def _measure_on_disk_size(
        self,
        ctx: Context,
        image_ref: str,
        debug: bool = False,
    ) -> tuple[int, list[FileInfo], DockerImageInfo | None]:
        """Measure disk size and generate file inventory using crane pull with OCI format."""
        try:
            if debug:
                print(f"🔍 Measuring on disk size of image {image_ref}...")

            # Use OCI format which outputs directly to a directory
            with tempfile.TemporaryDirectory() as oci_dir:
                save_result = ctx.run(f"crane pull --format=oci {image_ref} {oci_dir}", warn=True)
                if save_result.exited != 0:
                    raise RuntimeError(f"crane pull failed for {image_ref}")

                if debug:
                    print(f"📁 Pulled OCI image to: {oci_dir}")

                disk_size, file_inventory = self._analyze_extracted_docker_layers(oci_dir, debug)

                docker_info = self._extract_docker_metadata(oci_dir, image_ref, debug)

                if debug:
                    print("✅ Disk analysis completed:")
                    print(f"   • Disk size: {disk_size:,} bytes ({disk_size / 1024 / 1024:.2f} MB)")
                    print(f"   • Files inventoried: {len(file_inventory):,}")

                return disk_size, file_inventory, docker_info

        except Exception as e:
            raise RuntimeError(f"Failed to analyze image {image_ref}: {e}") from e

    def _get_wire_size(self, ctx: Context, image_ref: str, debug: bool = False) -> int:
        """Calculate Docker image compressed size using manifest inspection."""
        try:
            if debug:
                print(f"📋 Calculating wire size from manifest for {image_ref}...")

            # Use jq to properly parse JSON and sum config size + all layer sizes
            manifest_output = ctx.run(
                f"crane manifest {image_ref} | jq '[.config.size, (.layers[].size)] | add'",
                hide=True,
            )

            if manifest_output.exited != 0:
                raise RuntimeError(f"crane manifest failed for {image_ref}")

            wire_size = int(manifest_output.stdout.strip())

            if debug:
                print(f"✅ Wire size from manifest: {wire_size:,} bytes ({wire_size / 1024 / 1024:.2f} MB)")

            return wire_size

        except ValueError as e:
            raise RuntimeError(f"Failed to parse manifest size output for {image_ref}: {e}") from e
        except Exception as e:
            raise RuntimeError(f"Failed to calculate wire size from manifest for {image_ref}: {e}") from e

    def _analyze_extracted_docker_layers(
        self,
        extract_dir: str,
        debug: bool = False,
    ) -> tuple[int, list[FileInfo]]:
        """Analyze extracted crane pull tarball to get disk size and file inventory.

        Handles hard links correctly by tracking inodes within each layer to avoid
        counting the same file multiple times.
        """
        import json
        import subprocess

        total_disk_size = 0
        all_files = {}  # Use dict to handle overwrites from different layers

        try:
            # Check if this is OCI format (has index.json and oci-layout)
            index_path = os.path.join(extract_dir, "index.json")
            oci_layout_path = os.path.join(extract_dir, "oci-layout")
            manifest_path = os.path.join(extract_dir, "manifest.json")

            if os.path.exists(index_path) and os.path.exists(oci_layout_path):
                # OCI format
                if debug:
                    print("🔍 Detected OCI format")

                with open(index_path) as f:
                    index = json.load(f)

                # Get the manifest digest from the index
                manifest_digest = index["manifests"][0]["digest"]
                manifest_blob_path = os.path.join(extract_dir, "blobs", manifest_digest.replace(":", "/"))

                with open(manifest_blob_path) as f:
                    manifest = json.load(f)

                # Get layer digests from manifest
                layer_digests = [layer["digest"] for layer in manifest.get("layers", [])]

                # Convert to layer paths
                layer_files = [os.path.join("blobs", digest.replace(":", "/")) for digest in layer_digests]

            elif os.path.exists(manifest_path):
                # Old tarball format
                if debug:
                    print("🔍 Detected Docker tarball format")

                with open(manifest_path) as f:
                    manifest = json.load(f)[0]  # Typically one image per tarball

                layer_files = manifest.get("Layers", [])
            else:
                raise RuntimeError("Neither OCI format (index.json) nor tarball format (manifest.json) detected")

            if debug:
                print(f"🔍 Found {len(layer_files)} layers in manifest")

            # Process each layer by extracting and walking files
            for i, layer_file in enumerate(layer_files):
                layer_path = os.path.join(extract_dir, layer_file)

                if debug:
                    print(f"📦 Processing layer {i + 1}/{len(layer_files)}: {layer_file}")

                with tempfile.TemporaryDirectory() as layer_extract_dir:
                    # Extract layer
                    import shlex

                    extract_command = (
                        f"tar -xf {shlex.quote(layer_path)} -C {shlex.quote(layer_extract_dir)} 2>/dev/null"
                    )
                    extract_result = subprocess.run(extract_command, shell=True, capture_output=True).returncode
                    if extract_result != 0:
                        if debug:
                            print(f"⚠️  Skipping layer {layer_file} (extraction failed)")
                        continue

                    # Track (device, inode) pairs within this layer to handle hard links
                    layer_inodes = set()

                    # Walk through files in this layer, ensuring we don't follow symlinks
                    layer_files_processed = 0
                    for root, _, files in os.walk(layer_extract_dir, followlinks=False):
                        for file in files:
                            file_path = os.path.join(root, file)
                            relative_path = os.path.relpath(file_path, layer_extract_dir)

                            # Skip whiteout files (Those are marking files from lower layers that are removed in this layer)
                            if relative_path.startswith('.wh.') or '/.wh.' in relative_path:
                                continue

                            try:
                                # Use lstat to not follow symlinks, get actual file/symlink size
                                file_stat = os.lstat(file_path)

                                # For regular files with multiple hard links in this layer, only count once per (device, inode) pair
                                if stat.S_ISREG(file_stat.st_mode) and file_stat.st_nlink > 1:
                                    file_id = (file_stat.st_dev, file_stat.st_ino)
                                    if file_id in layer_inodes:
                                        # This is a hard link to a file we've already processed in this layer
                                        # Don't add to inventory but still count as processed
                                        layer_files_processed += 1
                                        continue
                                    layer_inodes.add(file_id)

                                size_bytes = file_stat.st_size

                                # Always generate checksum for regular files, not symlinks
                                checksum = None
                                if not stat.S_ISLNK(file_stat.st_mode):
                                    # Only generate checksums for regular files, not symlinks
                                    checksum = FileUtilities.generate_checksum(Path(file_path))

                                # Store file info (later layers override earlier ones)
                                all_files[relative_path] = FileInfo(
                                    relative_path=relative_path,
                                    size_bytes=size_bytes,
                                    checksum=checksum,
                                    chmod=stat.S_IMODE(file_stat.st_mode),
                                    owner=file_stat.st_uid,
                                    group=file_stat.st_gid,
                                )

                                layer_files_processed += 1

                            except (OSError, PermissionError):
                                continue

                    if debug and layer_files_processed > 0:
                        print(f"   • Processed {layer_files_processed} files from this layer")

            file_inventory = list(all_files.values())
            # Calculate total disk size from the final file inventory (without following symlinks)
            total_disk_size = sum(file_info.size_bytes for file_info in file_inventory)

            # Sort by size (descending) for easier analysis
            file_inventory.sort(key=lambda f: f.size_bytes, reverse=True)

            if debug:
                print(f"✅ Final inventory: {len(file_inventory)} unique files")
                print(f"   • Total disk size: {total_disk_size:,} bytes ({total_disk_size / 1024 / 1024:.2f} MB)")
                if len(file_inventory) > 10:
                    print(
                        f"   • Top 10 largest files consume: {sum(f.size_bytes for f in file_inventory[:10]):,} bytes"
                    )
                if len(all_files) != len(file_inventory):
                    print(
                        f"   • Note: {len(all_files)} total file entries processed, {len(file_inventory)} unique files after layer consolidation"
                    )

            return total_disk_size, file_inventory

        except Exception as e:
            raise RuntimeError(f"Failed to analyze layers: {e}") from e

    def _extract_docker_metadata(self, extract_dir: str, image_ref: str, debug: bool = False) -> DockerImageInfo | None:
        """Extract Docker metadata from the tarball contents."""
        try:
            import json

            # Check if this is OCI format
            index_path = os.path.join(extract_dir, "index.json")
            oci_layout_path = os.path.join(extract_dir, "oci-layout")

            if os.path.exists(index_path) and os.path.exists(oci_layout_path):
                # OCI format
                with open(index_path) as f:
                    index = json.load(f)

                # Get the manifest digest from the index
                manifest_digest = index["manifests"][0]["digest"]
                manifest_path = os.path.join(extract_dir, "blobs", manifest_digest.replace(":", "/"))

                with open(manifest_path) as f:
                    manifest = json.load(f)

                # Get config digest from manifest
                config_digest = manifest.get("config", {}).get("digest", "")
                config_path = os.path.join(extract_dir, "blobs", config_digest.replace(":", "/"))
                config_file = config_digest  # For OCI format, use digest as identifier

                if not os.path.exists(config_path):
                    return None

                with open(config_path) as f:
                    config_data = json.load(f)

                # Get layer digests
                layer_digests = [layer["digest"] for layer in manifest.get("layers", [])]
                layer_files = [os.path.join("blobs", digest.replace(":", "/")) for digest in layer_digests]

            else:
                # Old tarball format
                manifest_path = os.path.join(extract_dir, "manifest.json")
                if not os.path.exists(manifest_path):
                    return None

                with open(manifest_path) as f:
                    manifest = json.load(f)[0]

                config_file = manifest.get("Config", "")
                config_path = os.path.join(extract_dir, config_file)

                if not os.path.exists(config_path):
                    return None

                with open(config_path) as f:
                    config_data = json.load(f)

                layer_files = manifest.get("Layers", [])
            layers = []

            for i, layer_file in enumerate(layer_files):
                layer_path = os.path.join(extract_dir, layer_file)
                layer_size = os.path.getsize(layer_path) if os.path.exists(layer_path) else 0

                created_by = None
                if "history" in config_data and i < len(config_data["history"]):
                    history_entry = config_data["history"][i]
                    created_by = history_entry.get("created_by", "")

                layers.append(
                    DockerLayerInfo(
                        layer_id=f"layer_{i}",  # We don't have individual layer IDs from the tarball
                        size_bytes=layer_size,
                        created_by=created_by,
                        empty_layer=layer_size == 0,
                    )
                )

            architecture = config_data.get("architecture", "unknown")
            os_name = config_data.get("os", "unknown")
            if debug:
                print("📋 Extracted metadata from tarball:")
                print(f"   • Image ID: {image_ref}")
                print(f"   • Architecture: {architecture}")
                print(f"   • OS: {os_name}")
                print(f"   • Config file: {config_file}")

            return DockerImageInfo(
                image_ref=image_ref,
                architecture=architecture,
                os=os_name,
                layers=layers,
                config_size=os.path.getsize(config_path),
                manifest_size=os.path.getsize(manifest_path),
            )

        except Exception as e:
            if debug:
                print(f"⚠️  Failed to extract metadata from tarball: {e}")
            return None


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
        self._measurer = UniversalArtifactMeasurer(processor=PackageProcessor(), config_path=config_path)

    def measure_package(
        self,
        ctx: Context,
        package_path: str,
        gate_name: str,
        build_job_name: str,
        debug: bool = False,
    ) -> InPlaceArtifactReport:
        """
        Measure a package artifact and generate a comprehensive report.

        Args:
            ctx: Invoke context for running commands
            package_path: Path to the package file
            gate_name: Quality gate name from configuration
            build_job_name: Name of the CI job that built this package
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
            debug=debug,
        )

    def save_report_to_yaml(self, report: InPlaceArtifactReport, output_path: str) -> None:
        """Save the measurement report to a YAML file."""
        self._measurer.save_report_to_yaml(report, output_path)


class InPlaceDockerMeasurer:
    """
    Measures Docker image artifacts in-place and generates detailed reports.

    This class handles measurement of Docker images directly in build jobs or locally,
    using crane manifest for wire size and crane pull for disk analysis
    and comprehensive file inventory.

    Uses composition with UniversalArtifactMeasurer and DockerProcessor.
    """

    def __init__(self, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize the Docker image measurer with configuration.

        Args:
            config_path: Path to the quality gates configuration file
        """
        self._measurer = UniversalArtifactMeasurer(processor=DockerProcessor(), config_path=config_path)

    def measure_image(
        self,
        ctx: Context,
        image_ref: str,
        gate_name: str,
        build_job_name: str,
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


def measure_image_local(
    ctx,
    image_ref,
    gate_name,
    config_path="test/static/static_quality_gates.yml",
    output_path=None,
    build_job_name="local_test",
    include_layer_analysis=True,
    debug=False,
):
    """
    Run the in-place Docker image measurer locally for testing and development.

    This task allows you to test the Docker image measurement functionality on local images
    without requiring a full CI environment.

    Args:
        image_ref: Docker image reference (tag, digest, or image ID)
        gate_name: Quality gate name from the configuration file
        config_path: Path to quality gates configuration (default: test/static/static_quality_gates.yml)
        output_path: Path to save the measurement report (default: {gate_name}_report.yml)
        build_job_name: Simulated build job name (default: local_test)
        include_layer_analysis: Whether to analyze individual layers (default: true)
        debug: Enable debug logging for troubleshooting (default: false)

    Example:
        dda inv experimental-gates.measure-image-local --image-ref nginx:latest --gate-name static_quality_gate_docker_agent_amd64
    """
    from tasks.libs.common.color import color_message

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
            include_layer_analysis=include_layer_analysis,
            debug=debug,
        )

        # Save the report
        print(color_message("💾 Saving measurement report...", "cyan"))
        measurer.save_report_to_yaml(report, output_path)

        # Show size comparison with limits
        wire_limit_mb = report.max_on_wire_size / 1024 / 1024
        disk_limit_mb = report.max_on_disk_size / 1024 / 1024
        wire_usage_pct = (report.on_wire_size / report.max_on_wire_size) * 100
        disk_usage_pct = (report.on_disk_size / report.max_on_disk_size) * 100

        # Display summary
        print(color_message("✅ Measurement completed successfully!", "green"))
        print("📊 Results:")
        print(f"   • Wire size: {report.on_wire_size:,} bytes ({report.on_wire_size / 1024 / 1024:.2f} MiB)")
        print(f"   • Disk size: {report.on_disk_size:,} bytes ({report.on_disk_size / 1024 / 1024:.2f} MiB)")
        print(
            f"   • Wire limit: {(wire_limit_mb * 1024 * 1024):,} bytes ({wire_limit_mb:.2f} MiB - using {wire_usage_pct:.1f}%)"
        )
        print(
            f"   • Disk limit: {(disk_limit_mb * 1024 * 1024):,} bytes ({disk_limit_mb:.2f} MiB - using {disk_usage_pct:.1f}%)"
        )
        print("   • Note: Disk size is the uncompressed filesystem size of all files")
        print(f"   • Files inventoried: {len(report.file_inventory):,}")
        print(f"   • Report saved to: {output_path}")

        if wire_usage_pct > 100 or disk_usage_pct > 100:
            print(color_message("⚠️  WARNING: Image exceeds size limits!", "red"))
            if disk_usage_pct > 100:
                excess_bytes = report.on_disk_size - report.max_on_disk_size
                print(
                    color_message(
                        f"   • Disk size exceeds limit by {excess_bytes:.2f} bytes ({excess_bytes / 1024 / 1024:.2f} MiB)",
                        "red",
                    )
                )
            if wire_usage_pct > 100:
                excess_bytes = report.on_wire_size - report.max_on_wire_size
                print(
                    color_message(
                        f"   • Wire size exceeds limit by {excess_bytes:.2f} bytes ({excess_bytes / 1024 / 1024:.2f} MiB)",
                        "red",
                    )
                )
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
