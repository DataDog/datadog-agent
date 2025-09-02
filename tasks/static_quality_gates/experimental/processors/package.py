"""
Package artifact processor for experimental static quality gates.

This module handles measurement of standard package formats (DEB, RPM, etc.)
by extracting package contents and analyzing the resulting file structure.
"""

import os
import tempfile
from typing import Any

from invoke import Context

from tasks.libs.package.size import extract_package, file_size
from tasks.static_quality_gates.gates import QualityGateConfig

from ..common.models import FileInfo
from ..common.utils import FileUtilities


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
            file_inventory = FileUtilities.walk_files(extract_dir, max_files, generate_checksums, debug)

            if debug:
                print("✅ Package analysis completed:")
                print(f"   • Wire size: {wire_size:,} bytes")
                print(f"   • Disk size: {disk_size:,} bytes")
                print(f"   • Files inventoried: {len(file_inventory):,}")

            return wire_size, disk_size, file_inventory, None
