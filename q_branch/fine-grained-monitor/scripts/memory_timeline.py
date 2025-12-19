#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "pyarrow>=15.0.0",
#     "pandas>=2.0.0",
#     "plotly>=5.0.0",
#     "kaleido>=0.2.1",
# ]
# ///
"""
Plot memory usage over time for containers.

Visualizes key memory metrics:
- cgroup.v2.memory.current (total memory usage)
- cgroup.v2.memory.stat.anon (anonymous memory)
- cgroup.v2.memory.stat.file (file-backed memory)
- process.smaps_rollup.pss (Proportional Set Size per process)

Usage:
    uv run scripts/memory_timeline.py metrics.parquet
    uv run scripts/memory_timeline.py metrics.parquet --metric memory.current
    uv run scripts/memory_timeline.py metrics.parquet --top 10 --output memory.html
    uv run scripts/memory_timeline.py metrics.parquet --container-id abc123
"""

import argparse
import sys
from pathlib import Path

import pandas as pd
import plotly.express as px
import plotly.graph_objects as go
import pyarrow.parquet as pq
from plotly.subplots import make_subplots


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

    # Extract labels into separate columns
    df["container_id"] = df["labels"].apply(lambda x: extract_label(x, "container_id"))
    df["pod_uid"] = df["labels"].apply(lambda x: extract_label(x, "pod_uid"))
    df["qos_class"] = df["labels"].apply(lambda x: extract_label(x, "qos_class"))
    df["node_name"] = df["labels"].apply(lambda x: extract_label(x, "node_name"))
    df["pid"] = df["labels"].apply(lambda x: extract_label(x, "pid"))

    # Create short container ID for display
    df["container_short"] = df["container_id"].apply(lambda x: x[:12] if x else "unknown")

    # Combine value columns
    df["value"] = df["value_float"].combine_first(df["value_int"])

    return df


def plot_memory_timeline(
    df: pd.DataFrame,
    metric_filter: str | None = None,
    container_filter: str | None = None,
    top_n: int = 10,
    output_path: Path | None = None,
) -> None:
    """Create memory timeline visualization."""

    # Default memory metrics to show
    memory_metrics = [
        "cgroup.v2.memory.current",
        "cgroup.v2.memory.stat.anon",
        "cgroup.v2.memory.stat.file",
        "cgroup.v2.memory.swap.current",
    ]

    if metric_filter:
        memory_metrics = [m for m in df["metric_name"].unique() if metric_filter in m]
        if not memory_metrics:
            print(f"No metrics matching '{metric_filter}' found")
            print("Available memory metrics:")
            for m in sorted(df["metric_name"].unique()):
                if "memory" in m.lower():
                    print(f"  - {m}")
            sys.exit(1)

    # Filter to memory metrics
    mem_df = df[df["metric_name"].isin(memory_metrics)].copy()

    if mem_df.empty:
        print("No memory metrics found in data")
        sys.exit(1)

    # Filter by container if specified
    if container_filter:
        mem_df = mem_df[mem_df["container_id"].str.contains(container_filter, na=False)]
        if mem_df.empty:
            print(f"No data for container matching '{container_filter}'")
            sys.exit(1)

    # Find top containers by average memory usage
    if "cgroup.v2.memory.current" in mem_df["metric_name"].values:
        current_mem = mem_df[mem_df["metric_name"] == "cgroup.v2.memory.current"]
        top_containers = current_mem.groupby("container_short")["value"].mean().nlargest(top_n).index.tolist()
    else:
        # Fallback: use first metric
        top_containers = mem_df["container_short"].unique()[:top_n]

    mem_df = mem_df[mem_df["container_short"].isin(top_containers)]

    # Create figure with subplots for each metric
    fig = make_subplots(
        rows=len(memory_metrics),
        cols=1,
        shared_xaxes=True,
        subplot_titles=memory_metrics,
        vertical_spacing=0.05,
    )

    colors = px.colors.qualitative.Set2

    for row, metric in enumerate(memory_metrics, 1):
        metric_data = mem_df[mem_df["metric_name"] == metric]
        if metric_data.empty:
            continue

        for i, container in enumerate(top_containers):
            container_data = metric_data[metric_data["container_short"] == container]
            if container_data.empty:
                continue

            # Aggregate by time (in case of multiple samples)
            agg_data = container_data.groupby("time")["value"].mean().reset_index().sort_values("time")

            # Convert to MiB for readability
            agg_data["value_mib"] = agg_data["value"] / (1024 * 1024)

            fig.add_trace(
                go.Scatter(
                    x=agg_data["time"],
                    y=agg_data["value_mib"],
                    mode="lines",
                    name=container,
                    line={"color": colors[i % len(colors)]},
                    showlegend=(row == 1),  # Only show legend for first subplot
                    legendgroup=container,
                ),
                row=row,
                col=1,
            )

        fig.update_yaxes(title_text="MiB", row=row, col=1)

    fig.update_layout(
        title="Container Memory Usage Over Time",
        height=300 * len(memory_metrics),
        hovermode="x unified",
    )

    if output_path:
        if output_path.suffix == ".html":
            fig.write_html(output_path)
        else:
            fig.write_image(output_path)
        print(f"Saved to {output_path}")
    else:
        fig.show()


def main() -> None:
    parser = argparse.ArgumentParser(description="Plot container memory usage over time")
    parser.add_argument("input", type=Path, help="Input parquet file")
    parser.add_argument(
        "-m",
        "--metric",
        help="Filter to metrics containing this string (e.g., 'memory.stat')",
    )
    parser.add_argument(
        "-c",
        "--container-id",
        help="Filter to specific container ID (partial match)",
    )
    parser.add_argument(
        "-n",
        "--top",
        type=int,
        default=10,
        help="Show top N containers by memory usage (default: 10)",
    )
    parser.add_argument(
        "-o",
        "--output",
        type=Path,
        help="Output file (HTML or image format). Opens in browser if not specified.",
    )

    args = parser.parse_args()

    if not args.input.exists():
        print(f"Error: File not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    print(f"Loading {args.input}...")
    df = load_and_prepare_data(args.input)
    print(f"Loaded {len(df):,} rows")

    plot_memory_timeline(
        df,
        metric_filter=args.metric,
        container_filter=args.container_id,
        top_n=args.top,
        output_path=args.output,
    )


if __name__ == "__main__":
    main()
