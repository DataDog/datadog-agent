"""
High-level measurement orchestration for experimental static quality gates.

This module contains facade classes that provide convenient interfaces for
measuring different types of artifacts, abstracting away the complexity of
processor selection and configuration.
"""

from .docker import InPlaceDockerMeasurer
from .msi import InPlaceMsiMeasurer
from .package import InPlacePackageMeasurer
from .universal import UniversalArtifactMeasurer

__all__ = [
    'UniversalArtifactMeasurer',
    'InPlacePackageMeasurer',
    'InPlaceDockerMeasurer',
    'InPlaceMsiMeasurer',
]
