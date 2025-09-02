"""
Universal artifact measurer for experimental static quality gates.

This module provides the core orchestration class that can measure any type
of artifact by accepting different ArtifactProcessor implementations.
"""

from invoke import Context

from ..common.config import ConfigurationManager
from ..common.models import ArtifactProcessor, InPlaceArtifactReport
from ..common.report import ReportBuilder


class UniversalArtifactMeasurer:
    """
    Artifact measurer that can measure any type of artifact by accepting different
    ArtifactProcessor implementations.
    """

    def __init__(self, processor: ArtifactProcessor, config_path: str = "test/static/static_quality_gates.yml"):
        """
        Initialize the universal measurer with a specific artifact processor.

        Args:
            processor: Artifact processor implementation (package, Docker, MSI, etc.)
            config_path: Path to the quality gates configuration file
        """
        self.processor = processor
        self.config_manager = ConfigurationManager(config_path)
        self.report_builder = ReportBuilder()

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_name: str,
        build_job_name: str,
        max_files: int = 20000,
        generate_checksums: bool = True,
        debug: bool = False,
    ) -> InPlaceArtifactReport:
        """
        Measure an artifact using the configured processor.

        Args:
            ctx: Invoke context for running commands
            artifact_ref: Reference to the artifact (path, image name, etc.)
            gate_name: Quality gate name from configuration
            build_job_name: Name of the CI job that built this artifact
            max_files: Maximum number of files to process in inventory
            generate_checksums: Whether to generate checksums for files
            debug: Enable debug logging

        Returns:
            InPlaceArtifactReport with complete measurement data

        Raises:
            ValueError: If configuration is invalid or artifact not found
            RuntimeError: If measurement fails
        """
        gate_config = self.config_manager.get_gate_config(gate_name)

        wire_size, disk_size, file_inventory, artifact_metadata = self.processor.measure_artifact(
            ctx, artifact_ref, gate_config, max_files, generate_checksums, debug
        )

        return self.report_builder.create_report(
            artifact_ref=artifact_ref,
            gate_name=gate_name,
            gate_config=gate_config,
            wire_size=wire_size,
            disk_size=disk_size,
            file_inventory=file_inventory,
            build_job_name=build_job_name,
            artifact_metadata=artifact_metadata,
        )

    def save_report_to_yaml(self, report: InPlaceArtifactReport, output_path: str) -> None:
        """Save the measurement report to a YAML file."""
        self.report_builder.save_report_to_yaml(report, output_path)
