"""
File processing utilities for experimental static quality gates.

This module provides cross-platform utilities for file operations,
directory traversal, checksum generation, and size calculations.
"""

import hashlib
import os
from pathlib import Path

from .models import FileInfo


class FileUtilities:
    """Shared file processing utilities for all artifact types."""

    @staticmethod
    def generate_checksum(file_path: Path) -> str | None:
        """
        Generate SHA256 checksum for a file.

        Args:
            file_path: Path to the file

        Returns:
            SHA256 checksum as hex string, or None if generation fails
        """
        try:
            with open(file_path, "rb") as f:
                sha256_hash = hashlib.file_digest(f, "sha256")
            return f"sha256:{sha256_hash.hexdigest()}"
        except Exception:
            # If checksum generation fails, return None rather than failing the whole measurement
            return None

    @staticmethod
    def walk_files(directory: str, max_files: int, generate_checksums: bool, debug: bool) -> list[FileInfo]:
        """
        Walk through files in a directory and create file inventory.

        Args:
            directory: Directory containing files to analyze
            max_files: Maximum number of files to process
            generate_checksums: Whether to generate checksums
            debug: Enable debug logging

        Returns:
            List of FileInfo objects for all files
        """
        directory_path = Path(directory)
        file_inventory = []
        files_processed = 0

        if debug:
            all_items = list(directory_path.rglob('*'))
            files_count = sum(1 for item in all_items if item.is_file())
            dirs_count = sum(1 for item in all_items if item.is_dir())
            print(f"📊 Found {files_count} files and {dirs_count} directories")

        for file_path in directory_path.rglob('*'):
            if file_path.is_file():
                # Respect max_files limit
                if files_processed >= max_files:
                    if debug:
                        print(f"⚠️  Reached max files limit ({max_files}), stopping inventory")
                    break

                try:
                    relative_path = str(file_path.relative_to(directory_path))
                    size_bytes = file_path.stat().st_size

                    checksum = None
                    if generate_checksums:
                        checksum = FileUtilities.generate_checksum(file_path)

                    file_inventory.append(
                        FileInfo(
                            relative_path=relative_path,
                            size_bytes=size_bytes,
                            checksum=checksum,
                        )
                    )

                    files_processed += 1

                    if debug and files_processed % 1000 == 0:
                        print(f"📋 Processed {files_processed} files...")

                except (OSError, PermissionError) as e:
                    print(f"⚠️  Skipping file {file_path}: {e}")
                    continue

        # Sort by size (descending) for easier analysis
        file_inventory.sort(key=lambda f: f.size_bytes, reverse=True)
        return file_inventory

    @staticmethod
    def calculate_directory_size(directory: str) -> int:
        """
        Calculate total size of all files in a directory.

        Args:
            directory: Directory path to analyze

        Returns:
            Total size in bytes
        """
        total_size = 0
        for dirpath, _, filenames in os.walk(directory):
            for filename in filenames:
                file_path = os.path.join(dirpath, filename)
                try:
                    total_size += os.path.getsize(file_path)
                except (OSError, FileNotFoundError) as e:
                    print(f"⚠️  Skipping file {file_path}: {e}")
                    continue
        return total_size
