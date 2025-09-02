"""
Package-specific measurer for experimental static quality gates.

This module provides a convenient facade for measuring package artifacts
(DEB, RPM, etc.) with sensible defaults and error handling.
"""

from invoke import Context

from ..common.models import InPlaceArtifactReport
from ..processors.package import PackageProcessor
from .universal import UniversalArtifactMeasurer


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
        max_files: int = 20000,
        generate_checksums: bool = True,
        debug: bool = False,
    ) -> InPlaceArtifactReport:
        """
        Measure a package artifact and generate a comprehensive report.

        Args:
            ctx: Invoke context for running commands
            package_path: Path to the package file
            gate_name: Quality gate name from configuration
            build_job_name: Name of the CI job that built this package
            max_files: Maximum number of files to process in inventory
            generate_checksums: Whether to generate checksums for files
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
            max_files=max_files,
            generate_checksums=generate_checksums,
            debug=debug,
        )

    def save_report_to_yaml(self, report: InPlaceArtifactReport, output_path: str) -> None:
        """Save the measurement report to a YAML file."""
        self._measurer.save_report_to_yaml(report, output_path)
