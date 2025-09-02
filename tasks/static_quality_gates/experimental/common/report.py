"""
Report building and persistence for experimental static quality gates.

This module handles the creation and serialization of measurement reports,
providing standardized output formats for artifact measurements.
"""

import os
from datetime import datetime
from typing import Any

import yaml

from tasks.static_quality_gates.gates import QualityGateConfig

from .models import DockerImageInfo, FileInfo, InPlaceArtifactReport


class ReportBuilder:
    """Shared report building functionality for all artifact types."""

    @staticmethod
    def create_report(
        artifact_ref: str,
        gate_name: str,
        gate_config: QualityGateConfig,
        wire_size: int,
        disk_size: int,
        file_inventory: list[FileInfo],
        build_job_name: str,
        artifact_metadata: Any = None,
    ) -> InPlaceArtifactReport:
        """
        Create a standardized artifact report.

        Args:
            artifact_ref: Reference to the artifact (path, image name, etc.)
            gate_name: Quality gate name
            gate_config: Gate configuration
            wire_size: Compressed size in bytes
            disk_size: Uncompressed size in bytes
            file_inventory: List of files in the artifact
            build_job_name: Name of the build job
            artifact_metadata: Optional artifact-specific metadata

        Returns:
            Complete InPlaceArtifactReport
        """
        return InPlaceArtifactReport(
            artifact_path=artifact_ref,
            gate_name=gate_name,
            on_wire_size=wire_size,
            on_disk_size=disk_size,
            max_on_wire_size=gate_config.max_on_wire_size,
            max_on_disk_size=gate_config.max_on_disk_size,
            file_inventory=file_inventory,
            measurement_timestamp=datetime.now().astimezone().isoformat(),
            pipeline_id=os.environ.get("CI_PIPELINE_ID", "unknown"),
            commit_sha=os.environ.get("CI_COMMIT_SHA", "unknown"),
            arch=gate_config.arch,
            os=gate_config.os,
            build_job_name=build_job_name,
            docker_info=artifact_metadata if isinstance(artifact_metadata, DockerImageInfo) else None,
        )

    @staticmethod
    def save_report_to_yaml(report: InPlaceArtifactReport, output_path: str) -> None:
        """
        Save the measurement report to a YAML file.

        Args:
            report: The measurement report to save
            output_path: Path where to save the YAML file
        """
        try:
            # Convert dataclass to dictionary
            report_dict = {
                "artifact_path": report.artifact_path,
                "gate_name": report.gate_name,
                "on_wire_size": report.on_wire_size,
                "on_disk_size": report.on_disk_size,
                "max_on_wire_size": report.max_on_wire_size,
                "max_on_disk_size": report.max_on_disk_size,
                "measurement_timestamp": report.measurement_timestamp,
                "pipeline_id": report.pipeline_id,
                "commit_sha": report.commit_sha,
                "arch": report.arch,
                "os": report.os,
                "build_job_name": report.build_job_name,
                "file_inventory": [
                    {
                        "relative_path": file_info.relative_path,
                        "size_bytes": file_info.size_bytes,
                        "checksum": file_info.checksum,
                    }
                    for file_info in report.file_inventory
                ],
            }

            # Add Docker-specific information if present
            if report.docker_info:
                report_dict["docker_info"] = {
                    "image_ref": report.docker_info.image_ref,
                    "architecture": report.docker_info.architecture,
                    "os": report.docker_info.os,
                    "config_size": report.docker_info.config_size,
                    "manifest_size": report.docker_info.manifest_size,
                    "layers": [
                        {
                            "layer_id": layer.layer_id,
                            "size_bytes": layer.size_bytes,
                            "created_by": layer.created_by,
                            "empty_layer": layer.empty_layer,
                        }
                        for layer in report.docker_info.layers
                    ],
                }

            with open(output_path, 'w') as f:
                yaml.dump(report_dict, f, default_flow_style=False, sort_keys=False)

        except Exception as e:
            raise RuntimeError(f"Failed to save report to {output_path}: {e}") from e
