"""
Docker-specific measurer for experimental static quality gates.

This module provides a convenient facade for measuring Docker image artifacts
with sensible defaults and error handling.
"""

from invoke import Context

from ..common.models import InPlaceArtifactReport
from ..processors.docker import DockerProcessor
from .universal import UniversalArtifactMeasurer


class InPlaceDockerMeasurer:
    """
    Measures Docker image artifacts in-place and generates detailed reports.

    This class handles measurement of Docker images directly in build jobs or locally,
    using docker manifest inspect for wire size and docker save for disk analysis
    and comprehensive file inventory.

    Uses composition with UniversalArtifactMeasurer and DockerProcessor.
    """

    def __init__(self, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize the Docker image measurer with configuration.

        Args:
            config_path: Path to the quality gates configuration file
        """
        self._measurer = UniversalArtifactMeasurer(processor=DockerProcessor(), config_path=config_path)

    def measure_image(
        self,
        ctx: Context,
        image_ref: str,
        gate_name: str,
        build_job_name: str,
        max_files: int = 20000,
        generate_checksums: bool = True,
        include_layer_analysis: bool = True,
        debug: bool = False,
    ) -> InPlaceArtifactReport:
        """
        Measure a Docker image and generate a comprehensive report.

        Args:
            ctx: Invoke context for running commands
            image_ref: Docker image reference (tag, digest, or image ID)
            gate_name: Quality gate name from configuration
            build_job_name: Name of the CI job that built this image
            max_files: Maximum number of files to process in inventory
            generate_checksums: Whether to generate checksums for files
            include_layer_analysis: Whether to analyze individual layers (ignored, always included)
            debug: Enable debug logging

        Returns:
            InPlaceArtifactReport with complete measurement data

        Raises:
            ValueError: If configuration is invalid or image not found
            RuntimeError: If measurement fails
        """
        return self._measurer.measure_artifact(
            ctx=ctx,
            artifact_ref=image_ref,
            gate_name=gate_name,
            build_job_name=build_job_name,
            max_files=max_files,
            generate_checksums=generate_checksums,
            debug=debug,
        )

    def save_report_to_yaml(self, report: InPlaceArtifactReport, output_path: str) -> None:
        """Save the measurement report to a YAML file."""
        self._measurer.save_report_to_yaml(report, output_path)
