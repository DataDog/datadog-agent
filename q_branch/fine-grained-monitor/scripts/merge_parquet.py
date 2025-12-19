#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "pyarrow>=15.0.0",
#     "pandas>=2.0.0",
# ]
# ///
"""
Merge multiple fine-grained-monitor Parquet files into a single file.

Usage:
    uv run scripts/merge_parquet.py /path/to/parquet/dir -o merged.parquet
    uv run scripts/merge_parquet.py file1.parquet file2.parquet -o merged.parquet

    # Merge from kubectl cp output
    uv run scripts/merge_parquet.py /tmp/fgm-sample -o analysis.parquet
"""

import argparse
import sys
from pathlib import Path

import pyarrow as pa
import pyarrow.parquet as pq


def find_parquet_files(paths: list[Path]) -> list[Path]:
    """Find all parquet files from given paths (files or directories)."""
    files = []
    for p in paths:
        if p.is_file() and p.suffix == ".parquet":
            files.append(p)
        elif p.is_dir():
            files.extend(sorted(p.rglob("*.parquet")))
    return sorted(set(files))


def merge_parquet_files(
    input_files: list[Path],
    output_path: Path,
    compression: str = "zstd",
) -> None:
    """Merge multiple parquet files into one."""
    if not input_files:
        print("Error: No parquet files found", file=sys.stderr)
        sys.exit(1)

    print(f"Found {len(input_files)} parquet files to merge")

    # Read all tables
    tables = []
    total_rows = 0
    for i, f in enumerate(input_files):
        if f.stat().st_size == 0:
            print(f"  Skipping empty file: {f.name}")
            continue
        try:
            t = pq.read_table(f)
            tables.append(t)
            total_rows += t.num_rows
            if (i + 1) % 10 == 0:
                print(f"  Read {i + 1}/{len(input_files)} files ({total_rows:,} rows)")
        except Exception as e:
            print(f"  Warning: Failed to read {f.name}: {e}")

    if not tables:
        print("Error: No valid parquet files found", file=sys.stderr)
        sys.exit(1)

    print(f"Concatenating {len(tables)} tables with {total_rows:,} total rows...")
    merged = pa.concat_tables(tables)

    print(f"Writing merged file to {output_path}...")
    pq.write_table(merged, output_path, compression=compression)

    output_size = output_path.stat().st_size / 1024 / 1024
    print(f"Done! Output: {output_path} ({output_size:.2f} MB, {merged.num_rows:,} rows)")


def main() -> None:
    parser = argparse.ArgumentParser(description="Merge multiple Parquet files into one")
    parser.add_argument(
        "inputs",
        nargs="+",
        type=Path,
        help="Input parquet files or directories containing parquet files",
    )
    parser.add_argument(
        "-o",
        "--output",
        type=Path,
        default=Path("merged.parquet"),
        help="Output file path (default: merged.parquet)",
    )
    parser.add_argument(
        "--compression",
        default="zstd",
        choices=["zstd", "snappy", "gzip", "none"],
        help="Compression codec (default: zstd)",
    )

    args = parser.parse_args()

    input_files = find_parquet_files(args.inputs)
    merge_parquet_files(input_files, args.output, args.compression)


if __name__ == "__main__":
    main()
