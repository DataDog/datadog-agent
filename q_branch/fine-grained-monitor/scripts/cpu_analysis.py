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
Analyze CPU usage and throttling from fine-grained-monitor data.

Key metrics analyzed:
- cgroup.v2.cpu.stat.usage_usec (total CPU time)
- cgroup.v2.cpu.stat.user_usec (user-space CPU time)
- cgroup.v2.cpu.stat.system_usec (kernel CPU time)
- cgroup.v2.cpu.stat.throttled_usec (time spent throttled)
- cgroup.v2.cpu.stat.nr_throttled (throttle event count)
- cgroup.v2.cpu.pressure.* (PSI metrics)

Usage:
    uv run scripts/cpu_analysis.py metrics.parquet
    uv run scripts/cpu_analysis.py metrics.parquet --output cpu_report.html
    uv run scripts/cpu_analysis.py metrics.parquet --throttling-only
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


def compute_cpu_deltas(df: pd.DataFrame) -> pd.DataFrame:
    """Compute CPU usage deltas between samples for counter metrics."""
    cpu_counters = [
        "cgroup.v2.cpu.stat.usage_usec",
        "cgroup.v2.cpu.stat.user_usec",
        "cgroup.v2.cpu.stat.system_usec",
        "cgroup.v2.cpu.stat.throttled_usec",
        "cgroup.v2.cpu.stat.nr_throttled",
    ]

    cpu_df = df[df["metric_name"].isin(cpu_counters)].copy()

    if cpu_df.empty:
        return pd.DataFrame()

    # Sort by time within each container/metric
    cpu_df = cpu_df.sort_values(["container_id", "metric_name", "time"])

    # Compute deltas
    cpu_df["value_delta"] = cpu_df.groupby(["container_id", "metric_name"])["value"].diff()
    cpu_df["time_delta_s"] = cpu_df.groupby(["container_id", "metric_name"])["time"].diff().dt.total_seconds()

    # Drop first row of each group (no delta possible)
    cpu_df = cpu_df.dropna(subset=["value_delta", "time_delta_s"])

    # Filter out negative deltas (counter resets) and zero time deltas
    cpu_df = cpu_df[(cpu_df["value_delta"] >= 0) & (cpu_df["time_delta_s"] > 0)]

    # Compute CPU usage percentage (usec per second = millicores / 10)
    # 1 core = 1,000,000 usec/sec, so usec/sec / 10000 = % of one core
    cpu_df["cpu_percent"] = (cpu_df["value_delta"] / cpu_df["time_delta_s"]) / 10000

    return cpu_df


def plot_cpu_usage(
    df: pd.DataFrame,
    top_n: int = 10,
    output_path: Path | None = None,
    throttling_only: bool = False,
) -> None:
    """Create CPU usage visualization."""
    cpu_df = compute_cpu_deltas(df)

    if cpu_df.empty:
        print("No CPU counter metrics found in data")
        sys.exit(1)

    # Find top CPU users
    usage_df = cpu_df[cpu_df["metric_name"] == "cgroup.v2.cpu.stat.usage_usec"]
    if not usage_df.empty:
        top_containers = usage_df.groupby("container_short")["cpu_percent"].mean().nlargest(top_n).index.tolist()
    else:
        top_containers = cpu_df["container_short"].unique()[:top_n]

    cpu_df = cpu_df[cpu_df["container_short"].isin(top_containers)]

    if throttling_only:
        # Focus on throttling metrics
        metrics_to_plot = [
            ("cgroup.v2.cpu.stat.throttled_usec", "Throttled Time (%)"),
            ("cgroup.v2.cpu.stat.nr_throttled", "Throttle Events (count/s)"),
        ]
    else:
        metrics_to_plot = [
            ("cgroup.v2.cpu.stat.usage_usec", "Total CPU (%)"),
            ("cgroup.v2.cpu.stat.user_usec", "User CPU (%)"),
            ("cgroup.v2.cpu.stat.system_usec", "System CPU (%)"),
            ("cgroup.v2.cpu.stat.throttled_usec", "Throttled Time (%)"),
        ]

    fig = make_subplots(
        rows=len(metrics_to_plot),
        cols=1,
        shared_xaxes=True,
        subplot_titles=[title for _, title in metrics_to_plot],
        vertical_spacing=0.08,
    )

    colors = px.colors.qualitative.Set2

    for row, (metric, _) in enumerate(metrics_to_plot, 1):
        metric_data = cpu_df[cpu_df["metric_name"] == metric]
        if metric_data.empty:
            continue

        for i, container in enumerate(top_containers):
            container_data = metric_data[metric_data["container_short"] == container]
            if container_data.empty:
                continue

            # Use cpu_percent for *_usec metrics, rate for count metrics
            if metric.endswith("_usec"):
                y_values = container_data["cpu_percent"]
            else:
                y_values = container_data["value_delta"] / container_data["time_delta_s"]

            fig.add_trace(
                go.Scatter(
                    x=container_data["time"],
                    y=y_values,
                    mode="lines",
                    name=container,
                    line={"color": colors[i % len(colors)]},
                    showlegend=(row == 1),
                    legendgroup=container,
                ),
                row=row,
                col=1,
            )

        ylabel = "%" if metrics_to_plot[row - 1][0].endswith("_usec") else "count/s"
        fig.update_yaxes(title_text=ylabel, row=row, col=1)

    title = "CPU Throttling Analysis" if throttling_only else "Container CPU Usage Over Time"
    fig.update_layout(
        title=title,
        height=250 * len(metrics_to_plot),
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


def print_throttling_summary(df: pd.DataFrame) -> None:
    """Print summary of throttled containers."""
    cpu_df = compute_cpu_deltas(df)

    throttle_df = cpu_df[cpu_df["metric_name"] == "cgroup.v2.cpu.stat.throttled_usec"]

    if throttle_df.empty:
        print("No throttling data found")
        return

    print("\n=== Throttling Summary ===")
    summary = (
        throttle_df.groupby(["container_short", "qos_class"])
        .agg(
            total_throttled_s=("value_delta", lambda x: x.sum() / 1_000_000),
            avg_throttle_pct=("cpu_percent", "mean"),
            max_throttle_pct=("cpu_percent", "max"),
            samples=("value_delta", "count"),
        )
        .reset_index()
        .sort_values("total_throttled_s", ascending=False)
    )

    print("\nContainers by total throttled time:")
    print("-" * 80)
    for _, row in summary.head(15).iterrows():
        if row["total_throttled_s"] > 0:
            print(
                f"  {row['container_short']} ({row['qos_class']}): "
                f"{row['total_throttled_s']:.2f}s total, "
                f"avg {row['avg_throttle_pct']:.1f}%, "
                f"max {row['max_throttle_pct']:.1f}%"
            )


def main() -> None:
    parser = argparse.ArgumentParser(description="Analyze container CPU usage and throttling")
    parser.add_argument("input", type=Path, help="Input parquet file")
    parser.add_argument(
        "-n",
        "--top",
        type=int,
        default=10,
        help="Show top N containers by CPU usage (default: 10)",
    )
    parser.add_argument(
        "-o",
        "--output",
        type=Path,
        help="Output file (HTML or image). Opens in browser if not specified.",
    )
    parser.add_argument(
        "-t",
        "--throttling-only",
        action="store_true",
        help="Focus only on throttling metrics",
    )
    parser.add_argument(
        "-s",
        "--summary",
        action="store_true",
        help="Print throttling summary to console",
    )

    args = parser.parse_args()

    if not args.input.exists():
        print(f"Error: File not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    print(f"Loading {args.input}...")
    df = load_and_prepare_data(args.input)
    print(f"Loaded {len(df):,} rows")

    if args.summary:
        print_throttling_summary(df)
    else:
        plot_cpu_usage(
            df,
            top_n=args.top,
            output_path=args.output,
            throttling_only=args.throttling_only,
        )


if __name__ == "__main__":
    main()
