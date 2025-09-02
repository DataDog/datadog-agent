"""
MSI package processor for experimental static quality gates.

This module handles measurement of MSI packages using the cross-platform
ZIP + MSI approach: MSI file size for wire size, ZIP extraction for disk size.
This avoids dependency on Windows-specific tools while maintaining accuracy.
"""

import glob
import os
import tempfile
from typing import Any

from invoke import Context

from tasks.static_quality_gates.gates import QualityGateConfig

from ..common.models import FileInfo
from ..common.utils import FileUtilities


class MsiProcessor:
    """MSI package processor using cross-platform ZIP extraction approach."""

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
        Measure MSI package using the ZIP + MSI approach.

        - Wire size: MSI file size (compressed package)
        - Disk size: ZIP extraction size (installed file structure)
        - File inventory: From ZIP extraction (cross-platform)

        Args:
            ctx: Invoke context for running commands
            artifact_ref: Path to MSI file or directory containing MSI+ZIP
            gate_config: Quality gate configuration
            max_files: Maximum number of files to process
            generate_checksums: Whether to generate checksums
            debug: Enable debug logging

        Returns:
            Tuple of (wire_size, disk_size, file_inventory, None)
        """
        # Find both ZIP and MSI files
        zip_path, msi_path = self._find_msi_artifacts(artifact_ref, gate_config, debug)

        if debug:
            print("📦 Measuring MSI package:")
            print(f"   • MSI file: {msi_path}")
            print(f"   • ZIP file: {zip_path}")

        # Wire size: actual MSI file size
        wire_size = os.path.getsize(msi_path)

        # Disk size and file inventory: extract ZIP file
        with tempfile.TemporaryDirectory() as extract_dir:
            if debug:
                print(f"📁 Extracting ZIP to: {extract_dir}")

            # Use cross-platform ZIP extraction
            self._extract_zip(ctx, zip_path, extract_dir, debug)

            disk_size = FileUtilities.calculate_directory_size(extract_dir)
            file_inventory = FileUtilities.walk_files(extract_dir, max_files, generate_checksums, debug)

            if debug:
                print("✅ MSI analysis completed:")
                print(f"   • Wire size (MSI): {wire_size:,} bytes ({wire_size / 1024 / 1024:.2f} MiB)")
                print(f"   • Disk size (ZIP): {disk_size:,} bytes ({disk_size / 1024 / 1024:.2f} MiB)")
                print(f"   • Files inventoried: {len(file_inventory):,}")

            return wire_size, disk_size, file_inventory, None

    def _find_msi_artifacts(self, artifact_ref: str, gate_config: QualityGateConfig, debug: bool) -> tuple[str, str]:
        """
        Find both ZIP and MSI files using the same logic as existing gates.

        Args:
            artifact_ref: MSI path or directory containing both files
            gate_config: Gate configuration for pattern matching
            debug: Enable debug logging

        Returns:
            Tuple of (zip_path, msi_path)

        Raises:
            ValueError: If files cannot be found
        """
        if artifact_ref.endswith('.msi'):
            # Direct MSI path provided, derive ZIP path
            msi_path = artifact_ref
            zip_path = artifact_ref.replace('.msi', '.zip')
        else:
            # Directory or base name provided, find both files
            package_dir = os.path.dirname(artifact_ref) if os.path.isfile(artifact_ref) else artifact_ref

            # Use same pattern as PackageArtifactMeasurer for consistency
            # Pattern: datadog-agent-7*x86_64.{zip,msi}
            zip_pattern = f"{package_dir}/datadog-agent-7*x86_64.zip"
            msi_pattern = f"{package_dir}/datadog-agent-7*x86_64.msi"

            if debug:
                print("🔍 Looking for files:")
                print(f"   • ZIP pattern: {zip_pattern}")
                print(f"   • MSI pattern: {msi_pattern}")

            zip_files = glob.glob(zip_pattern)
            msi_files = glob.glob(msi_pattern)

            if not zip_files:
                raise ValueError(f"Could not find ZIP file matching {zip_pattern}")
            if not msi_files:
                raise ValueError(f"Could not find MSI file matching {msi_pattern}")
            if len(zip_files) > 1:
                raise ValueError(f"Found multiple ZIP files: {zip_files}")
            if len(msi_files) > 1:
                raise ValueError(f"Found multiple MSI files: {msi_files}")

            zip_path = zip_files[0]
            msi_path = msi_files[0]

        # Validate files exist
        if not os.path.exists(zip_path):
            raise ValueError(f"ZIP file not found: {zip_path}")
        if not os.path.exists(msi_path):
            raise ValueError(f"MSI file not found: {msi_path}")

        return zip_path, msi_path

    def _extract_zip(self, ctx: Context, zip_path: str, extract_dir: str, debug: bool) -> None:
        """
        Extract ZIP file using cross-platform approach.

        Args:
            ctx: Invoke context for running commands
            zip_path: Path to ZIP file
            extract_dir: Directory to extract to
            debug: Enable debug logging

        Raises:
            RuntimeError: If extraction fails
        """
        try:
            # Use the same approach as existing extract_zip_archive
            with ctx.cd(extract_dir):
                result = ctx.run(f"unzip -q '{zip_path}'", warn=True)
                if result.exited != 0:
                    raise RuntimeError(f"Failed to extract ZIP file: {zip_path}")

            if debug:
                print(f"✅ Successfully extracted ZIP to {extract_dir}")

        except Exception as e:
            raise RuntimeError(f"ZIP extraction failed: {e}") from e
