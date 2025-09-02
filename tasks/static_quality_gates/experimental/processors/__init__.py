"""
Artifact-specific processors for experimental static quality gates.

This module contains implementations for measuring different types of artifacts:
- package: Standard packages (DEB, RPM, etc.)
- docker: Docker images
- msi: MSI packages using ZIP+MSI approach
"""

from .docker import DockerProcessor
from .msi import MsiProcessor
from .package import PackageProcessor

__all__ = [
    'PackageProcessor',
    'DockerProcessor',
    'MsiProcessor',
]
