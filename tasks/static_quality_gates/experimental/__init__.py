"""
Experimental Static Quality Gates - Clean modular implementation

This package provides experimental functionality for measuring artifacts directly
in build jobs, generating detailed reports with file inventories for comparison.

The implementation is organized into focused modules:
- common/: Shared data models, utilities, and configuration
- processors/: Artifact-specific measurement logic
- measurers/: High-level measurement orchestration
- tasks/: Invoke task definitions

Example usage:
    from tasks.static_quality_gates.experimental import InPlacePackageMeasurer

    measurer = InPlacePackageMeasurer()
    report = measurer.measure_package(ctx, "/path/to/package.deb", "static_quality_gate_agent_deb_amd64", "test_job")
"""

# Data models
from .common.models import DockerImageInfo, DockerLayerInfo, FileInfo, InPlaceArtifactReport

# Measurers - Main API
from .measurers.docker import InPlaceDockerMeasurer
from .measurers.msi import InPlaceMsiMeasurer
from .measurers.package import InPlacePackageMeasurer
from .measurers.universal import UniversalArtifactMeasurer

# Task functions
from .tasks.docker import measure_image_local
from .tasks.msi import measure_msi_local
from .tasks.package import measure_package_local

__all__ = [
    # Measurers - Main API
    'UniversalArtifactMeasurer',
    'InPlacePackageMeasurer',
    'InPlaceDockerMeasurer',
    'InPlaceMsiMeasurer',
    # Data Models
    'InPlaceArtifactReport',
    'FileInfo',
    'DockerImageInfo',
    'DockerLayerInfo',
    # Tasks
    'measure_package_local',
    'measure_image_local',
    'measure_msi_local',
]
