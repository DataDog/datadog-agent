"""
MSI-specific measurer for experimental static quality gates.

This module provides a convenient facade for measuring MSI package artifacts
using the cross-platform ZIP+MSI approach with sensible defaults and error handling.
"""

from invoke import Context

from ..common.models import InPlaceArtifactReport
from ..processors.msi import MsiProcessor
from .universal import UniversalArtifactMeasurer


class InPlaceMsiMeasurer:
    """
    Measures MSI package artifacts in-place and generates detailed reports.

    This class handles measurement of MSI packages using the cross-platform
    ZIP + MSI approach: MSI file size for wire size, ZIP extraction for disk size.
    This avoids dependency on Windows-specific tools while maintaining accuracy.

    Uses composition with UniversalArtifactMeasurer and MsiProcessor.
    """

    def __init__(self, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize the MSI measurer with configuration.

        Args:
            config_path: Path to the quality gates configuration file
        """
        self._measurer = UniversalArtifactMeasurer(processor=MsiProcessor(), config_path=config_path)

    def measure_msi(
        self,
        ctx: Context,
        msi_ref: str,
        gate_name: str,
        build_job_name: str,
        max_files: int = 20000,
        generate_checksums: bool = True,
        debug: bool = False,
    ) -> InPlaceArtifactReport:
        """
        Measure an MSI package and generate a comprehensive report.

        Args:
            ctx: Invoke context for running commands
            msi_ref: Path to MSI file or directory containing MSI+ZIP files
            gate_name: Quality gate name from configuration
            build_job_name: Name of the CI job that built this package
            max_files: Maximum number of files to process in inventory
            generate_checksums: Whether to generate checksums for files
            debug: Enable debug logging

        Returns:
            InPlaceArtifactReport with complete measurement data

        Raises:
            ValueError: If configuration is invalid or MSI/ZIP files not found
            RuntimeError: If measurement fails
        """
        return self._measurer.measure_artifact(
            ctx=ctx,
            artifact_ref=msi_ref,
            gate_name=gate_name,
            build_job_name=build_job_name,
            max_files=max_files,
            generate_checksums=generate_checksums,
            debug=debug,
        )

    def save_report_to_yaml(self, report: InPlaceArtifactReport, output_path: str) -> None:
        """Save the measurement report to a YAML file."""
        self._measurer.save_report_to_yaml(report, output_path)
