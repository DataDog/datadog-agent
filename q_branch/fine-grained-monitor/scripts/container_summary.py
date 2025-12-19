#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "pyarrow>=15.0.0",
#     "pandas>=2.0.0",
#     "tabulate>=0.9.0",
# ]
# ///
"""
Generate summary statistics for each container from fine-grained-monitor data.

Provides per-container aggregates for:
- Memory: current, anon, file, swap (min/avg/max/p95)
- CPU: usage rate, throttling
- PSI: pressure indicators

Usage:
    uv run scripts/container_summary.py metrics.parquet
    uv run scripts/container_summary.py metrics.parquet --format csv > summary.csv
    uv run scripts/container_summary.py metrics.parquet --sort-by memory_avg
"""

import argparse
import sys
from pathlib import Path

import pandas as pd
import pyarrow.parquet as pq
from tabulate import tabulate


def extract_label(labels: list[tuple], key: str) -> str | None:
    """Extract a label value from the labels list."""
    for k, v in labels:
        if k == key:
            return v
    return None


def load_and_prepare_data(filepath: Path) -> pd.DataFrame:
    """Load parquet file and extract labels into columns."""
    table = pq.read_table(filepath)
    df = table.to_pandas()

    df["container_id"] = df["labels"].apply(lambda x: extract_label(x, "container_id"))
    df["pod_uid"] = df["labels"].apply(lambda x: extract_label(x, "pod_uid"))
    df["qos_class"] = df["labels"].apply(lambda x: extract_label(x, "qos_class"))
    df["node_name"] = df["labels"].apply(lambda x: extract_label(x, "node_name"))

    df["container_short"] = df["container_id"].apply(lambda x: x[:12] if x else "unknown")

    df["value"] = df["value_float"].combine_first(df["value_int"])

    return df


def compute_container_summary(df: pd.DataFrame) -> pd.DataFrame:
    """Compute summary statistics for each container."""
    summaries = []

    containers = df.groupby(["container_short", "container_id", "qos_class", "node_name"])

    for (short_id, _full_id, qos, node), group in containers:
        summary = {
            "container": short_id,
            "qos_class": qos,
            "node": node,
        }

        # Time range
        summary["start_time"] = group["time"].min()
        summary["end_time"] = group["time"].max()
        summary["duration_s"] = (summary["end_time"] - summary["start_time"]).total_seconds()
        summary["samples"] = group["fetch_index"].nunique()

        # Memory current (gauge, in MiB)
        mem_current = group[group["metric_name"] == "cgroup.v2.memory.current"]["value"]
        if not mem_current.empty:
            summary["memory_min_mib"] = mem_current.min() / (1024 * 1024)
            summary["memory_avg_mib"] = mem_current.mean() / (1024 * 1024)
            summary["memory_max_mib"] = mem_current.max() / (1024 * 1024)
            summary["memory_p95_mib"] = mem_current.quantile(0.95) / (1024 * 1024)
        else:
            summary["memory_min_mib"] = None
            summary["memory_avg_mib"] = None
            summary["memory_max_mib"] = None
            summary["memory_p95_mib"] = None

        # Memory anon (gauge, in MiB)
        mem_anon = group[group["metric_name"] == "cgroup.v2.memory.stat.anon"]["value"]
        if not mem_anon.empty:
            summary["anon_avg_mib"] = mem_anon.mean() / (1024 * 1024)

        # Memory file (gauge, in MiB)
        mem_file = group[group["metric_name"] == "cgroup.v2.memory.stat.file"]["value"]
        if not mem_file.empty:
            summary["file_avg_mib"] = mem_file.mean() / (1024 * 1024)

        # Swap current
        swap = group[group["metric_name"] == "cgroup.v2.memory.swap.current"]["value"]
        if not swap.empty:
            summary["swap_max_mib"] = swap.max() / (1024 * 1024)

        # CPU usage (counter - need to compute rate)
        cpu_usage = group[group["metric_name"] == "cgroup.v2.cpu.stat.usage_usec"].sort_values("time")
        if len(cpu_usage) > 1:
            total_cpu_usec = cpu_usage["value"].iloc[-1] - cpu_usage["value"].iloc[0]
            time_span = (cpu_usage["time"].iloc[-1] - cpu_usage["time"].iloc[0]).total_seconds()
            if time_span > 0:
                # Average CPU % (1 core = 100%)
                summary["cpu_avg_pct"] = (total_cpu_usec / time_span) / 10000

        # Throttling
        throttled = group[group["metric_name"] == "cgroup.v2.cpu.stat.throttled_usec"].sort_values("time")
        if len(throttled) > 1:
            total_throttle_usec = throttled["value"].iloc[-1] - throttled["value"].iloc[0]
            summary["throttled_s"] = total_throttle_usec / 1_000_000

        nr_throttled = group[group["metric_name"] == "cgroup.v2.cpu.stat.nr_throttled"].sort_values("time")
        if len(nr_throttled) > 1:
            total_events = nr_throttled["value"].iloc[-1] - nr_throttled["value"].iloc[0]
            summary["throttle_events"] = int(total_events)

        # PSI pressure (some.avg60 is a good indicator)
        cpu_pressure = group[group["metric_name"] == "cgroup.v2.cpu.pressure.some.avg60"]["value"]
        if not cpu_pressure.empty:
            summary["cpu_pressure_avg"] = cpu_pressure.mean()

        mem_pressure = group[group["metric_name"] == "cgroup.v2.memory.pressure.some.avg60"]["value"]
        if not mem_pressure.empty:
            summary["mem_pressure_avg"] = mem_pressure.mean()

        summaries.append(summary)

    return pd.DataFrame(summaries)


def format_summary(
    df: pd.DataFrame,
    sort_by: str | None = None,
    format: str = "table",
) -> str:
    """Format summary dataframe for output."""

    # Select and rename columns for display
    display_cols = [
        ("container", "Container"),
        ("qos_class", "QoS"),
        ("node", "Node"),
        ("duration_s", "Duration(s)"),
        ("samples", "Samples"),
        ("memory_avg_mib", "Mem Avg(MiB)"),
        ("memory_max_mib", "Mem Max(MiB)"),
        ("memory_p95_mib", "Mem P95(MiB)"),
        ("cpu_avg_pct", "CPU Avg(%)"),
        ("throttled_s", "Throttled(s)"),
        ("throttle_events", "Throttle#"),
        ("cpu_pressure_avg", "CPU PSI"),
        ("mem_pressure_avg", "Mem PSI"),
    ]

    available_cols = [(col, name) for col, name in display_cols if col in df.columns]
    display_df = df[[col for col, _ in available_cols]].copy()
    display_df.columns = [name for _, name in available_cols]

    # Sort
    if sort_by:
        # Map display name back to original
        name_map = {name: col for col, name in display_cols}
        sort_col = name_map.get(sort_by, sort_by)
        if sort_col in df.columns:
            display_df = display_df.sort_values(
                [name for _, name in available_cols if _ == sort_col][0]
                if sort_col in [c for c, _ in available_cols]
                else sort_by,
                ascending=False,
            )

    # Format numeric columns
    for col in display_df.columns:
        if display_df[col].dtype in ["float64", "float32"]:
            display_df[col] = display_df[col].apply(lambda x: f"{x:.1f}" if pd.notna(x) else "-")
        elif display_df[col].dtype in ["int64", "int32"]:
            display_df[col] = display_df[col].apply(lambda x: f"{x:,}" if pd.notna(x) else "-")

    display_df = display_df.fillna("-")

    if format == "csv":
        return display_df.to_csv(index=False)
    elif format == "json":
        return display_df.to_json(orient="records", indent=2)
    else:
        return tabulate(display_df, headers="keys", tablefmt="simple", showindex=False)


def main() -> None:
    parser = argparse.ArgumentParser(description="Generate per-container summary statistics")
    parser.add_argument("input", type=Path, help="Input parquet file")
    parser.add_argument(
        "-s",
        "--sort-by",
        help="Column to sort by (e.g., memory_avg_mib, cpu_avg_pct, throttled_s)",
    )
    parser.add_argument(
        "-f",
        "--format",
        choices=["table", "csv", "json"],
        default="table",
        help="Output format (default: table)",
    )
    parser.add_argument(
        "-n",
        "--top",
        type=int,
        help="Show only top N containers",
    )

    args = parser.parse_args()

    if not args.input.exists():
        print(f"Error: File not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    print(f"Loading {args.input}...", file=sys.stderr)
    df = load_and_prepare_data(args.input)
    print(f"Loaded {len(df):,} rows", file=sys.stderr)

    print("Computing summary statistics...", file=sys.stderr)
    summary_df = compute_container_summary(df)

    # Sort before limiting
    if args.sort_by and args.sort_by in summary_df.columns:
        summary_df = summary_df.sort_values(args.sort_by, ascending=False)
    elif "memory_avg_mib" in summary_df.columns:
        summary_df = summary_df.sort_values("memory_avg_mib", ascending=False)

    if args.top:
        summary_df = summary_df.head(args.top)

    print(f"\n=== Container Summary ({len(summary_df)} containers) ===\n", file=sys.stderr)
    output = format_summary(summary_df, sort_by=args.sort_by, format=args.format)
    print(output)


if __name__ == "__main__":
    main()
