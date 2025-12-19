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
Analyze PSI (Pressure Stall Information) metrics from fine-grained-monitor data.

PSI metrics indicate resource pressure:
- cpu.pressure.some/full: Some/all tasks stalled waiting for CPU
- memory.pressure.some/full: Some/all tasks stalled due to memory reclaim
- io.pressure.some/full: Some/all tasks stalled waiting for I/O

avg10/avg60/avg300: Exponentially weighted moving averages (10s/60s/300s windows)
total: Total stall time in microseconds (cumulative counter)

Usage:
    uv run scripts/pressure_analysis.py metrics.parquet
    uv run scripts/pressure_analysis.py metrics.parquet --type memory
    uv run scripts/pressure_analysis.py metrics.parquet --output psi.html
    uv run scripts/pressure_analysis.py metrics.parquet --heatmap
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

    df["container_id"] = df["labels"].apply(lambda x: extract_label(x, "container_id"))
    df["pod_uid"] = df["labels"].apply(lambda x: extract_label(x, "pod_uid"))
    df["qos_class"] = df["labels"].apply(lambda x: extract_label(x, "qos_class"))
    df["node_name"] = df["labels"].apply(lambda x: extract_label(x, "node_name"))

    df["container_short"] = df["container_id"].apply(lambda x: x[:12] if x else "unknown")

    df["value"] = df["value_float"].combine_first(df["value_int"])

    return df


def plot_pressure_timeline(
    df: pd.DataFrame,
    pressure_type: str = "all",
    top_n: int = 10,
    output_path: Path | None = None,
) -> None:
    """Plot PSI metrics over time."""

    # Define PSI metric groups
    psi_metrics = {
        "cpu": [
            "cgroup.v2.cpu.pressure.some.avg60",
            "cgroup.v2.cpu.pressure.full.avg60",
        ],
        "memory": [
            "cgroup.v2.memory.pressure.some.avg60",
            "cgroup.v2.memory.pressure.full.avg60",
        ],
        "io": [
            "cgroup.v2.io.pressure.some.avg60",
            "cgroup.v2.io.pressure.full.avg60",
        ],
    }

    if pressure_type == "all":
        selected_metrics = psi_metrics["cpu"] + psi_metrics["memory"] + psi_metrics["io"]
    else:
        selected_metrics = psi_metrics.get(pressure_type, [])

    if not selected_metrics:
        print(f"Unknown pressure type: {pressure_type}")
        print("Valid types: cpu, memory, io, all")
        sys.exit(1)

    psi_df = df[df["metric_name"].isin(selected_metrics)].copy()

    if psi_df.empty:
        print("No PSI metrics found in data")
        print("Available metrics containing 'pressure':")
        for m in sorted(df["metric_name"].unique()):
            if "pressure" in m:
                print(f"  - {m}")
        sys.exit(1)

    # Find containers with highest pressure
    avg_pressure = psi_df.groupby("container_short")["value"].mean().nlargest(top_n).index.tolist()

    psi_df = psi_df[psi_df["container_short"].isin(avg_pressure)]

    # Create subplots for each metric type
    metric_groups = []
    if pressure_type == "all":
        metric_groups = [
            ("CPU Pressure", psi_metrics["cpu"]),
            ("Memory Pressure", psi_metrics["memory"]),
            ("I/O Pressure", psi_metrics["io"]),
        ]
    else:
        metric_groups = [(f"{pressure_type.upper()} Pressure", selected_metrics)]

    fig = make_subplots(
        rows=len(metric_groups),
        cols=1,
        shared_xaxes=True,
        subplot_titles=[title for title, _ in metric_groups],
        vertical_spacing=0.1,
    )

    colors = px.colors.qualitative.Set2

    for row, (_, metrics) in enumerate(metric_groups, 1):
        for i, container in enumerate(avg_pressure):
            for metric in metrics:
                container_metric_data = psi_df[
                    (psi_df["container_short"] == container) & (psi_df["metric_name"] == metric)
                ].sort_values("time")

                if container_metric_data.empty:
                    continue

                # Determine line style based on some vs full
                dash = "solid" if ".some." in metric else "dash"
                name_suffix = " (some)" if ".some." in metric else " (full)"

                fig.add_trace(
                    go.Scatter(
                        x=container_metric_data["time"],
                        y=container_metric_data["value"],
                        mode="lines",
                        name=f"{container}{name_suffix}",
                        line={"color": colors[i % len(colors)], "dash": dash},
                        showlegend=(row == 1),
                        legendgroup=container,
                    ),
                    row=row,
                    col=1,
                )

        fig.update_yaxes(title_text="% stalled", row=row, col=1)

    fig.update_layout(
        title="Container Pressure Stall Information (PSI)",
        height=300 * len(metric_groups),
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


def plot_pressure_heatmap(
    df: pd.DataFrame,
    pressure_type: str = "memory",
    output_path: Path | None = None,
) -> None:
    """Create a heatmap showing pressure across containers over time."""

    metric_map = {
        "cpu": "cgroup.v2.cpu.pressure.some.avg60",
        "memory": "cgroup.v2.memory.pressure.some.avg60",
        "io": "cgroup.v2.io.pressure.some.avg60",
    }

    metric = metric_map.get(pressure_type)
    if not metric:
        print(f"Unknown pressure type: {pressure_type}")
        sys.exit(1)

    psi_df = df[df["metric_name"] == metric].copy()

    if psi_df.empty:
        print(f"No data for metric: {metric}")
        sys.exit(1)

    # Create pivot table: time x container
    # First, bucket time into intervals for cleaner visualization
    psi_df["time_bucket"] = psi_df["time"].dt.round("30s")

    pivot = psi_df.pivot_table(
        values="value",
        index="container_short",
        columns="time_bucket",
        aggfunc="mean",
    )

    # Sort by average pressure
    pivot = pivot.loc[pivot.mean(axis=1).sort_values(ascending=False).index]

    # Take top 20 containers
    pivot = pivot.head(20)

    fig = px.imshow(
        pivot,
        labels={"x": "Time", "y": "Container", "color": "% stalled"},
        title=f"{pressure_type.upper()} Pressure Heatmap (some.avg60)",
        color_continuous_scale="YlOrRd",
        aspect="auto",
    )

    fig.update_layout(
        height=max(400, 25 * len(pivot)),
    )

    if output_path:
        if output_path.suffix == ".html":
            fig.write_html(output_path)
        else:
            fig.write_image(output_path)
        print(f"Saved to {output_path}")
    else:
        fig.show()


def print_pressure_summary(df: pd.DataFrame) -> None:
    """Print summary of containers with highest pressure."""

    psi_metrics = [
        ("cgroup.v2.cpu.pressure.some.avg60", "CPU (some)"),
        ("cgroup.v2.memory.pressure.some.avg60", "Memory (some)"),
        ("cgroup.v2.io.pressure.some.avg60", "I/O (some)"),
    ]

    print("\n=== Pressure Summary (avg60) ===\n")

    for metric, label in psi_metrics:
        metric_df = df[df["metric_name"] == metric]
        if metric_df.empty:
            continue

        print(f"\n{label} - Top containers by average pressure:")
        print("-" * 60)

        summary = (
            metric_df.groupby(["container_short", "qos_class"])
            .agg(
                avg=("value", "mean"),
                max=("value", "max"),
                p95=("value", lambda x: x.quantile(0.95)),
            )
            .reset_index()
            .sort_values("avg", ascending=False)
            .head(10)
        )

        for _, row in summary.iterrows():
            if row["avg"] > 0.1:  # Only show containers with measurable pressure
                print(
                    f"  {row['container_short']} ({row['qos_class']}): "
                    f"avg={row['avg']:.1f}%, max={row['max']:.1f}%, p95={row['p95']:.1f}%"
                )


def main() -> None:
    parser = argparse.ArgumentParser(description="Analyze PSI (Pressure Stall Information) metrics")
    parser.add_argument("input", type=Path, help="Input parquet file")
    parser.add_argument(
        "-t",
        "--type",
        choices=["cpu", "memory", "io", "all"],
        default="all",
        help="Pressure type to analyze (default: all)",
    )
    parser.add_argument(
        "-n",
        "--top",
        type=int,
        default=10,
        help="Show top N containers by pressure (default: 10)",
    )
    parser.add_argument(
        "-o",
        "--output",
        type=Path,
        help="Output file (HTML or image). Opens in browser if not specified.",
    )
    parser.add_argument(
        "--heatmap",
        action="store_true",
        help="Generate heatmap instead of timeline",
    )
    parser.add_argument(
        "-s",
        "--summary",
        action="store_true",
        help="Print pressure summary to console",
    )

    args = parser.parse_args()

    if not args.input.exists():
        print(f"Error: File not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    print(f"Loading {args.input}...")
    df = load_and_prepare_data(args.input)
    print(f"Loaded {len(df):,} rows")

    if args.summary:
        print_pressure_summary(df)
    elif args.heatmap:
        plot_pressure_heatmap(df, pressure_type=args.type, output_path=args.output)
    else:
        plot_pressure_timeline(
            df,
            pressure_type=args.type,
            top_n=args.top,
            output_path=args.output,
        )


if __name__ == "__main__":
    main()
