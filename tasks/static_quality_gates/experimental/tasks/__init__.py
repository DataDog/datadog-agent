"""
Invoke task definitions for experimental static quality gates.

This module contains task functions that can be called via invoke to test
and use the experimental measurement functionality locally.
"""

from .docker import measure_image_local
from .msi import measure_msi_local
from .package import measure_package_local

__all__ = [
    'measure_package_local',
    'measure_image_local',
    'measure_msi_local',
]
