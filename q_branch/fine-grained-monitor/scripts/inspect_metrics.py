#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "pyarrow>=15.0.0",
#     "pandas>=2.0.0",
# ]
# ///
"""
Inspect fine-grained-monitor Parquet metrics files.

Usage:
    uv run scripts/inspect_metrics.py [PARQUET_FILE]

If no file is specified, defaults to ../../out/metrics.parquet
"""

import sys
from pathlib import Path

import pyarrow.parquet as pq


def inspect_parquet(filepath: Path) -> None:
    """Inspect a Parquet file and print summary statistics."""
    print(f"Inspecting: {filepath}")
    print("=" * 60)

    # Read the parquet file
    table = pq.read_table(filepath)

    print("\n=== Schema ===")
    print(table.schema)

    print(f"\n=== Row count: {table.num_rows:,} ===")

    print("\n=== Column names ===")
    for col in table.column_names:
        print(f"  - {col}")

    # Convert to pandas for easier analysis
    df = table.to_pandas()

    print("\n=== Sample data (first 10 rows) ===")
    print(df.head(10).to_string())

    # Look for metric name column
    name_col = None
    for candidate in ["metric_name", "name", "metric"]:
        if candidate in df.columns:
            name_col = candidate
            break

    if name_col:
        print(f"\n=== Unique metrics (column: {name_col}) ===")
        unique_metrics = df[name_col].unique()
        print(f"Total unique metrics: {len(unique_metrics)}")
        for metric in sorted(unique_metrics)[:30]:
            print(f"  - {metric}")
        if len(unique_metrics) > 30:
            print(f"  ... and {len(unique_metrics) - 30} more")

    # Check for timestamp column
    time_col = None
    for candidate in ["timestamp", "time", "ts"]:
        if candidate in df.columns:
            time_col = candidate
            break

    if time_col:
        print(f"\n=== Time range (column: {time_col}) ===")
        print(f"  Start: {df[time_col].min()}")
        print(f"  End:   {df[time_col].max()}")

    # Check for label columns
    label_cols = [c for c in df.columns if c not in [name_col, time_col, "value"]]
    if label_cols:
        print("\n=== Label columns ===")
        for col in label_cols[:10]:
            unique_vals = df[col].nunique()
            print(f"  - {col}: {unique_vals} unique values")

    print("\n=== Data types ===")
    print(df.dtypes.to_string())

    print("\n=== Memory usage ===")
    print(f"  DataFrame: {df.memory_usage(deep=True).sum() / 1024 / 1024:.2f} MB")
    print(f"  File size: {filepath.stat().st_size / 1024 / 1024:.2f} MB")


def main() -> None:
    if len(sys.argv) > 1:
        filepath = Path(sys.argv[1])
    else:
        # Default to out/metrics.parquet relative to q_branch
        script_dir = Path(__file__).parent
        filepath = script_dir.parent.parent / "out" / "metrics.parquet"

    if not filepath.exists():
        print(f"Error: File not found: {filepath}", file=sys.stderr)
        print(f"Usage: {sys.argv[0]} [PARQUET_FILE]", file=sys.stderr)
        sys.exit(1)

    inspect_parquet(filepath)


if __name__ == "__main__":
    main()
