"""
Data models and protocols for experimental static quality gates.

This module contains the core data structures used across all artifact processors:
- SizeMixin: Provides size validation and conversion utilities
- FileInfo: Information about a single file within an artifact
- DockerLayerInfo: Information about a Docker layer
- DockerImageInfo: Extended information for Docker images
- InPlaceArtifactReport: Complete measurement report for an artifact
- ArtifactProcessor: Protocol for implementing artifact processors
"""

from dataclasses import dataclass
from typing import Any, Protocol

from invoke import Context

from tasks.static_quality_gates.gates import QualityGateConfig


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
        max_files: int,
        generate_checksums: bool,
        debug: bool,
    ) -> tuple[int, int, list[FileInfo], Any]:
        """
        Measure an artifact and return wire size, disk size, file inventory, and optional metadata.

        Args:
            ctx: Invoke context for running commands
            artifact_ref: Reference to the artifact (path, image name, etc.)
            gate_config: Quality gate configuration
            max_files: Maximum number of files to process
            generate_checksums: Whether to generate file checksums
            debug: Enable debug logging

        Returns:
            Tuple of (wire_size, disk_size, file_inventory, artifact_specific_metadata)
        """
        ...
