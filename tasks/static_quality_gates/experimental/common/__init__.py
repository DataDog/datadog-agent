"""
Common utilities and shared infrastructure for experimental static quality gates.

This module contains shared data models, configuration management, file utilities,
and report building functionality used across all artifact processors.
"""

from .config import ConfigurationManager
from .models import (
    DockerImageInfo,
    DockerLayerInfo,
    FileInfo,
    InPlaceArtifactReport,
)

__all__ = [
    # Data models
    'FileInfo',
    'DockerLayerInfo',
    'DockerImageInfo',
    'InPlaceArtifactReport',
    # Utilities
    'ConfigurationManager',
]
