"""
Docker image processor for experimental static quality gates.

This module handles measurement of Docker images using manifest inspection
for wire size calculation and docker save for disk analysis and file inventory.
"""

import json
import os
import tempfile
from pathlib import Path

from invoke import Context

from tasks.static_quality_gates.gates import QualityGateConfig

from ..common.models import DockerImageInfo, DockerLayerInfo, FileInfo
from ..common.utils import FileUtilities


class DockerProcessor:
    """Docker image processor implementing the ArtifactProcessor protocol.

    Uses docker manifest inspect for wire size calculation and docker save for
    disk analysis and file inventory. This provides consistent wire size measurements
    regardless of image layer structure while maintaining detailed file analysis.
    """

    def measure_artifact(
        self,
        ctx: Context,
        artifact_ref: str,
        gate_config: QualityGateConfig,
        max_files: int,
        generate_checksums: bool,
        debug: bool,
    ) -> tuple[int, int, list[FileInfo], DockerImageInfo]:
        """Measure Docker image using manifest inspection for wire size and docker save for disk analysis."""
        if debug:
            print(f"🐳 Measuring Docker image: {artifact_ref}")

        self._ensure_image_available(ctx, artifact_ref, debug)

        wire_size = self._get_wire_size(ctx, artifact_ref, debug)

        disk_size, file_inventory, docker_info = self._measure_on_disk_size(
            ctx, artifact_ref, max_files, generate_checksums, debug
        )

        return wire_size, disk_size, file_inventory, docker_info

    def _ensure_image_available(self, ctx: Context, image_ref: str, debug: bool = False) -> None:
        """Ensure the Docker image is available locally."""
        try:
            result = ctx.run(f"docker image inspect {image_ref}", hide=True, warn=True)
            if result.exited == 0:
                if debug:
                    print(f"✅ Image {image_ref} found locally")
                return

            if debug:
                print(f"📥 Pulling image {image_ref}...")

            pull_result = ctx.run(f"docker pull {image_ref}", warn=True)
            if pull_result.exited != 0:
                raise RuntimeError(f"Failed to pull Docker image {image_ref}")

            if debug:
                print(f"✅ Successfully pulled image {image_ref}")

        except Exception as e:
            raise RuntimeError(f"Failed to ensure image {image_ref} is available: {e}") from e

    def _measure_on_disk_size(
        self,
        ctx: Context,
        image_ref: str,
        max_files: int,
        generate_checksums: bool,
        debug: bool = False,
    ) -> tuple[int, list[FileInfo], DockerImageInfo | None]:
        """Measure disk size and generate file inventory using docker save extraction."""
        try:
            if debug:
                print(f"🔍 Measuring on disk size of image {image_ref}...")

            with tempfile.NamedTemporaryFile(suffix='.tar', delete=False) as temp_tarball:
                save_result = ctx.run(f"docker save {image_ref} -o {temp_tarball.name}", warn=True)
                if save_result.exited != 0:
                    raise RuntimeError(f"Docker save failed for {image_ref}")

                try:
                    # Extract tarball and analyze layers
                    with tempfile.TemporaryDirectory() as extract_dir:
                        extract_result = ctx.run(f"tar -xf {temp_tarball.name} -C {extract_dir}", warn=True)
                        if extract_result.exited != 0:
                            raise RuntimeError("Failed to extract Docker save tarball")

                        if debug:
                            print(f"📁 Extracted tarball to: {extract_dir}")

                        disk_size, file_inventory = self._analyze_extracted_docker_layers(
                            extract_dir, max_files, generate_checksums, debug
                        )

                        docker_info = self._extract_docker_metadata(extract_dir, image_ref, debug)

                        if debug:
                            print("✅ Disk analysis completed:")
                            print(f"   • Disk size: {disk_size:,} bytes ({disk_size / 1024 / 1024:.2f} MB)")
                            print(f"   • Files inventoried: {len(file_inventory):,}")

                        return disk_size, file_inventory, docker_info

                finally:
                    try:
                        os.unlink(temp_tarball.name)
                    except Exception:
                        pass  # Best effort cleanup

        except Exception as e:
            raise RuntimeError(f"Failed to analyze image {image_ref}: {e}") from e

    def _get_wire_size(self, ctx: Context, image_ref: str, debug: bool = False) -> int:
        """Calculate Docker image compressed size using manifest inspection."""
        try:
            if debug:
                print(f"📋 Calculating wire size from manifest for {image_ref}...")

            manifest_output = ctx.run(
                "DOCKER_CLI_EXPERIMENTAL=enabled docker manifest inspect -v "
                + image_ref
                + " | grep size | awk -F ':' '{sum+=$NF} END {printf(\"%d\",sum)}'",
                hide=True,
            )

            if manifest_output.exited != 0:
                raise RuntimeError(f"Docker manifest inspect failed for {image_ref}")

            wire_size = int(manifest_output.stdout.strip())

            if debug:
                print(f"✅ Wire size from manifest: {wire_size:,} bytes ({wire_size / 1024 / 1024:.2f} MB)")

            return wire_size

        except ValueError as e:
            raise RuntimeError(f"Failed to parse manifest size output for {image_ref}: {e}") from e
        except Exception as e:
            raise RuntimeError(f"Failed to calculate wire size from manifest for {image_ref}: {e}") from e

    def _analyze_extracted_docker_layers(
        self,
        extract_dir: str,
        max_files: int,
        generate_checksums: bool,
        debug: bool = False,
    ) -> tuple[int, list[FileInfo]]:
        """Analyze extracted Docker save tarball to get disk size and file inventory."""
        total_disk_size = 0
        all_files = {}  # Use dict to handle overwrites from different layers

        try:
            manifest_path = os.path.join(extract_dir, "manifest.json")
            if not os.path.exists(manifest_path):
                raise RuntimeError("manifest.json not found in Docker save tarball")

            with open(manifest_path) as f:
                manifest = json.load(f)[0]  # Typically one image per tarball

            layer_files = manifest.get("Layers", [])

            if debug:
                print(f"🔍 Found {len(layer_files)} layers in manifest")

            # Process each layer in order
            for i, layer_file in enumerate(layer_files):
                layer_path = os.path.join(extract_dir, layer_file)

                if debug:
                    print(f"📦 Processing layer {i + 1}/{len(layer_files)}: {layer_file}")

                with tempfile.TemporaryDirectory() as layer_extract_dir:
                    extract_result = os.system(f"tar -xf '{layer_path}' -C '{layer_extract_dir}' 2>/dev/null")
                    if extract_result != 0:
                        if debug:
                            print(f"⚠️  Skipping layer {layer_file} (extraction failed)")
                        continue

                    # Walk through files in this layer
                    layer_files_processed = 0
                    for root, _, files in os.walk(layer_extract_dir):
                        for file in files:
                            if layer_files_processed >= max_files:
                                break

                            file_path = os.path.join(root, file)
                            relative_path = os.path.relpath(file_path, layer_extract_dir)
                            # Skip whiteout files (Those are marking files from lower layers that are removed in this layer)
                            if relative_path.startswith('.wh.') or '/.wh.' in relative_path:
                                continue

                            try:
                                file_stat = os.stat(file_path)
                                size_bytes = file_stat.st_size

                                # Generate checksum if requested
                                checksum = None
                                if generate_checksums:
                                    checksum = FileUtilities.generate_checksum(Path(file_path))

                                # Store file info (later layers override earlier ones)
                                all_files[relative_path] = FileInfo(
                                    relative_path=relative_path,
                                    size_bytes=size_bytes,
                                    checksum=checksum,
                                )

                                layer_files_processed += 1

                            except (OSError, PermissionError):
                                continue
                        if layer_files_processed >= max_files:
                            break

                    if debug and layer_files_processed > 0:
                        print(f"   • Processed {layer_files_processed} files from this layer")

            file_inventory = list(all_files.values())
            total_disk_size = sum(file_info.size_bytes for file_info in file_inventory)

            # Sort by size (descending) for easier analysis
            file_inventory.sort(key=lambda f: f.size_bytes, reverse=True)

            if debug:
                print(f"✅ Final inventory: {len(file_inventory)} unique files")
                print(f"   • Total disk size: {total_disk_size:,} bytes ({total_disk_size / 1024 / 1024:.2f} MB)")
                if len(file_inventory) > 10:
                    print(
                        f"   • Top 10 largest files consume: {sum(f.size_bytes for f in file_inventory[:10]):,} bytes"
                    )
                if len(all_files) != len(file_inventory):
                    print(
                        f"   • Note: {len(all_files)} total file entries processed, {len(file_inventory)} unique files after layer consolidation"
                    )

            return total_disk_size, file_inventory

        except Exception as e:
            raise RuntimeError(f"Failed to analyze layers: {e}") from e

    def _extract_docker_metadata(self, extract_dir: str, image_ref: str, debug: bool = False) -> DockerImageInfo | None:
        """Extract Docker metadata from the tarball contents."""
        try:
            manifest_path = os.path.join(extract_dir, "manifest.json")
            if not os.path.exists(manifest_path):
                return None

            with open(manifest_path) as f:
                manifest = json.load(f)[0]

            config_file = manifest.get("Config", "")
            config_path = os.path.join(extract_dir, config_file)

            if not os.path.exists(config_path):
                return None

            with open(config_path) as f:
                config_data = json.load(f)

            layer_files = manifest.get("Layers", [])
            layers = []

            for i, layer_file in enumerate(layer_files):
                layer_path = os.path.join(extract_dir, layer_file)
                layer_size = os.path.getsize(layer_path) if os.path.exists(layer_path) else 0

                created_by = None
                if "history" in config_data and i < len(config_data["history"]):
                    history_entry = config_data["history"][i]
                    created_by = history_entry.get("created_by", "")

                layers.append(
                    DockerLayerInfo(
                        layer_id=f"layer_{i}",  # We don't have individual layer IDs from the tarball
                        size_bytes=layer_size,
                        created_by=created_by,
                        empty_layer=layer_size == 0,
                    )
                )

            architecture = config_data.get("architecture", "unknown")
            os_name = config_data.get("os", "unknown")
            if debug:
                print("📋 Extracted metadata from tarball:")
                print(f"   • Image ID: {image_ref}")
                print(f"   • Architecture: {architecture}")
                print(f"   • OS: {os_name}")
                print(f"   • Config file: {config_file}")

            return DockerImageInfo(
                image_ref=image_ref,
                architecture=architecture,
                os=os_name,
                layers=layers,
                config_size=os.path.getsize(config_path),
                manifest_size=os.path.getsize(manifest_path),
            )

        except Exception as e:
            if debug:
                print(f"⚠️  Failed to extract metadata from tarball: {e}")
            return None
