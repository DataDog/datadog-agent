#!/usr/bin/env python3
"""
Script to remove documentation fields from botocore JSON files.

This script:
1. Walks through botocore/data directory
2. Processes both .json and .json.gz files
3. Removes all "documentation" keys from the JSON
4. Recompresses .json.gz files with the same compression
"""

import json
import gzip
import os
import sys
from pathlib import Path
from typing import Any, Dict


def remove_documentation(obj: Any) -> Any:
    """
    Recursively remove all "documentation" keys from a JSON object.

    Args:
        obj: JSON object (dict, list, or primitive)

    Returns:
        The same object with all "documentation" keys removed
    """
    if isinstance(obj, dict):
        return {
            key: remove_documentation(value)
            for key, value in obj.items()
            if key != "documentation"
        }
    elif isinstance(obj, list):
        return [remove_documentation(item) for item in obj]
    else:
        return obj


def process_json_file(file_path: Path) -> tuple[int, int]:
    """
    Process a plain JSON file: remove documentation and write back.

    Args:
        file_path: Path to the .json file

    Returns:
        Tuple of (original_size, new_size)
    """
    try:
        with open(file_path, 'r', encoding='utf-8') as f:
            data = json.load(f)

        original_size = file_path.stat().st_size

        # Remove documentation
        cleaned_data = remove_documentation(data)

        # Write back
        with open(file_path, 'w', encoding='utf-8') as f:
            json.dump(cleaned_data, f, separators=(',', ':'))

        new_size = file_path.stat().st_size

        return original_size, new_size
    except Exception as e:
        print(f"Error processing {file_path}: {e}", file=sys.stderr)
        return 0, 0


def process_gzipped_json_file(file_path: Path) -> tuple[int, int]:
    """
    Process a gzipped JSON file: decompress, remove documentation, recompress.

    Args:
        file_path: Path to the .json.gz file

    Returns:
        Tuple of (original_size, new_size)
    """
    try:
        # Read and decompress
        with gzip.open(file_path, 'rt', encoding='utf-8') as f:
            data = json.load(f)

        original_size = file_path.stat().st_size

        # Remove documentation
        cleaned_data = remove_documentation(data)

        # Recompress and write back
        with gzip.open(file_path, 'wt', encoding='utf-8') as f:
            json.dump(cleaned_data, f, separators=(',', ':'))

        new_size = file_path.stat().st_size

        return original_size, new_size
    except Exception as e:
        print(f"Error processing {file_path}: {e}", file=sys.stderr)
        return 0, 0


def main():
    """Main function to process all botocore data files."""
    # Accept path as command-line argument, default to "botocore/data"
    if len(sys.argv) > 1:
        botocore_data_path = Path(sys.argv[1])
    else:
        botocore_data_path = Path("botocore/data")

    if not botocore_data_path.exists():
        print(f"Error: {botocore_data_path} does not exist", file=sys.stderr)
        print(f"Skipping botocore documentation removal.", file=sys.stderr)
        sys.exit(0)  # Exit successfully if botocore doesn't exist

    total_original_size = 0
    total_new_size = 0
    json_count = 0
    gz_count = 0

    print("Processing botocore data files...")
    print("-" * 60)

    # Process all .json.gz files
    for gz_file in botocore_data_path.rglob("*.json.gz"):
        original, new = process_gzipped_json_file(gz_file)
        total_original_size += original
        total_new_size += new
        gz_count += 1

        if gz_count % 100 == 0:
            print(f"Processed {gz_count} .json.gz files...")

    # Process all .json files (that are not .json.gz)
    for json_file in botocore_data_path.rglob("*.json"):
        if not str(json_file).endswith(".json.gz"):
            original, new = process_json_file(json_file)
            total_original_size += original
            total_new_size += new
            json_count += 1

            if json_count % 100 == 0:
                print(f"Processed {json_count} .json files...")

    # Print summary
    print("-" * 60)
    print(f"Processing complete!")
    print(f"  .json.gz files processed: {gz_count}")
    print(f"  .json files processed: {json_count}")
    print(f"  Total files processed: {gz_count + json_count}")
    print(f"  Original total size: {total_original_size:,} bytes ({total_original_size / 1024 / 1024:.2f} MB)")
    print(f"  New total size: {total_new_size:,} bytes ({total_new_size / 1024 / 1024:.2f} MB)")
    print(f"  Space saved: {total_original_size - total_new_size:,} bytes ({(total_original_size - total_new_size) / 1024 / 1024:.2f} MB)")

    if total_original_size > 0:
        reduction_pct = ((total_original_size - total_new_size) / total_original_size) * 100
        print(f"  Size reduction: {reduction_pct:.1f}%")


if __name__ == "__main__":
    main()
