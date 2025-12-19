#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "pyarrow>=15.0.0",
#     "pandas>=2.0.0",
#     "numpy>=1.24.0",
#     "plotly>=5.0.0",
#     "kaleido>=0.2.1",
# ]
# ///
"""
Detect CPU oscillation patterns in container metrics using autocorrelation.

This is a standalone implementation of the algorithm from the cpu_oscillation
check in the Datadog Agent (sopell/q-branch-cpu-oscillation-per-container branch).

The algorithm detects periodic CPU usage patterns by:
1. Computing CPU usage deltas from cgroup counter metrics
2. Running autocorrelation analysis to find periodicity
3. Detecting oscillation when both periodicity score and amplitude exceed thresholds

Key parameters (matching Go implementation):
- window_size: 60 samples (60 seconds at 1Hz)
- min_periodicity_score: 0.6 (autocorrelation threshold)
- min_amplitude: 10.0 (% CPU swing threshold)
- min_period: 2 (minimum period in seconds, Nyquist limit)
- max_period: 30 (maximum period in seconds)

Usage:
    uv run scripts/oscillation_detector.py metrics.parquet
    uv run scripts/oscillation_detector.py metrics.parquet --plot
    uv run scripts/oscillation_detector.py metrics.parquet --output results.html
    uv run scripts/oscillation_detector.py metrics.parquet --threshold 0.5 --min-amplitude 5.0

Reference:
    pkg/collector/corechecks/containers/cpu_oscillation/detector.go
"""

import argparse
import sys
from dataclasses import dataclass, field
from pathlib import Path

import numpy as np
import pandas as pd
import plotly.graph_objects as go
import pyarrow.parquet as pq
from plotly.subplots import make_subplots


@dataclass
class OscillationConfig:
    """Configuration for oscillation detection (mirrors Go OscillationConfig)."""

    window_size: int = 60  # Number of samples in analysis window
    min_periodicity_score: float = 0.6  # Minimum autocorrelation peak
    min_amplitude: float = 10.0  # Minimum peak-to-trough % CPU
    min_period: int = 2  # Minimum period in samples (Nyquist limit)
    max_period: int = 30  # Maximum period in samples


@dataclass
class OscillationResult:
    """Results of oscillation analysis (mirrors Go OscillationResult)."""

    detected: bool = False
    periodicity_score: float = 0.0  # Peak autocorrelation value (0.0-1.0)
    period: float = 0.0  # Detected period in seconds
    frequency: float = 0.0  # Cycles per second (Hz = 1/Period)
    amplitude: float = 0.0  # Peak-to-trough percentage
    stddev: float = 0.0  # Standard deviation of samples


@dataclass
class ContainerTimeseries:
    """CPU usage timeseries for a single container."""

    container_id: str
    container_short: str
    timestamps: list = field(default_factory=list)
    cpu_percent: list = field(default_factory=list)
    pod_uid: str | None = None
    qos_class: str | None = None


def autocorrelation(samples: np.ndarray, mean: float, variance: float, lag: int) -> float:
    """
    Compute normalized autocorrelation at a given lag.

    Returns a value in [-1, 1] where:
    - 1.0 means perfect positive correlation (signal repeats exactly)
    - 0.0 means no correlation (random noise)
    - -1.0 means perfect negative correlation (signal inverts)

    Matches Go implementation in detector.go.
    """
    if variance == 0 or lag >= len(samples):
        return 0.0

    n = len(samples)
    count = n - lag

    if count == 0:
        return 0.0

    # Sum of (x[i] - mean) * (x[i+lag] - mean)
    sum_val = np.sum((samples[:count] - mean) * (samples[lag:] - mean))

    # Normalize by variance to get correlation coefficient in [-1, 1]
    return sum_val / (count * variance)


def analyze_oscillation(samples: np.ndarray, config: OscillationConfig) -> OscillationResult:
    """
    Perform autocorrelation-based oscillation detection on a sample window.

    Mirrors the Analyze() method in detector.go.
    """
    result = OscillationResult()

    if len(samples) < config.window_size:
        return result

    # Use the most recent window_size samples
    window = samples[-config.window_size :]

    # Compute statistics
    mean = np.mean(window)
    variance = np.var(window)
    result.stddev = np.sqrt(variance)

    # Calculate amplitude (peak-to-trough)
    result.amplitude = float(np.max(window) - np.min(window))

    # Early exit if amplitude is below threshold
    if config.min_amplitude > 0 and result.amplitude < config.min_amplitude:
        return result

    # Compute autocorrelation for lags in [min_period, max_period]
    # Find the lag with the highest autocorrelation (strongest periodicity)
    best_lag = 0
    best_corr = 0.0

    for lag in range(config.min_period, config.max_period + 1):
        corr = autocorrelation(window, mean, variance, lag)
        if corr > best_corr:
            best_corr = corr
            best_lag = lag

    result.periodicity_score = best_corr

    # Convert lag (in samples) to period (in seconds, assuming 1Hz sampling)
    if best_lag > 0:
        result.period = float(best_lag)  # 1 sample = 1 second
        result.frequency = 1.0 / result.period

    # Detection criteria (all must be met):
    # 1. Periodicity score exceeds threshold
    # 2. Amplitude exceeds minimum threshold
    meets_periodicity = best_corr >= config.min_periodicity_score
    meets_amplitude = config.min_amplitude == 0 or result.amplitude >= config.min_amplitude

    if meets_periodicity and meets_amplitude:
        result.detected = True

    return result


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

    df["container_short"] = df["container_id"].apply(lambda x: x[:12] if x else "unknown")
    df["value"] = df["value_float"].combine_first(df["value_int"])

    return df


def compute_cpu_timeseries(df: pd.DataFrame) -> dict[str, ContainerTimeseries]:
    """
    Compute CPU usage timeseries for each container.

    Converts cumulative cpu.stat.usage_usec to CPU percentage.
    """
    # Filter to CPU usage metric
    cpu_metric = "cgroup.v2.cpu.stat.usage_usec"
    cpu_df = df[df["metric_name"] == cpu_metric].copy()

    if cpu_df.empty:
        print(f"Warning: No {cpu_metric} data found", file=sys.stderr)
        return {}

    # Sort by time within each container
    cpu_df = cpu_df.sort_values(["container_id", "time"])

    # Compute deltas
    cpu_df["value_delta"] = cpu_df.groupby("container_id")["value"].diff()
    cpu_df["time_delta_s"] = cpu_df.groupby("container_id")["time"].diff().dt.total_seconds()

    # Drop first row of each group and invalid deltas
    cpu_df = cpu_df.dropna(subset=["value_delta", "time_delta_s"])
    cpu_df = cpu_df[(cpu_df["value_delta"] >= 0) & (cpu_df["time_delta_s"] > 0)]

    # Compute CPU percentage (usec used / elapsed usec * 100)
    # 1 second of 1 core = 1,000,000 usec, so usec/sec / 10000 = % of one core
    cpu_df["cpu_percent"] = (cpu_df["value_delta"] / cpu_df["time_delta_s"]) / 10000

    # Build timeseries for each container
    timeseries = {}
    for container_id, group in cpu_df.groupby("container_id"):
        if pd.isna(container_id):
            continue

        ts = ContainerTimeseries(
            container_id=container_id,
            container_short=container_id[:12] if container_id else "unknown",
            timestamps=group["time"].tolist(),
            cpu_percent=group["cpu_percent"].tolist(),
            pod_uid=group["pod_uid"].iloc[0] if "pod_uid" in group.columns else None,
            qos_class=group["qos_class"].iloc[0] if "qos_class" in group.columns else None,
        )
        timeseries[container_id] = ts

    return timeseries


def analyze_containers(
    timeseries: dict[str, ContainerTimeseries],
    config: OscillationConfig,
) -> list[tuple[ContainerTimeseries, OscillationResult]]:
    """Analyze all containers for oscillation patterns."""
    results = []

    for container_id, ts in timeseries.items():
        samples = np.array(ts.cpu_percent)

        if len(samples) < config.window_size:
            # Not enough data
            result = OscillationResult()
            result.amplitude = float(np.max(samples) - np.min(samples)) if len(samples) > 1 else 0.0
            result.stddev = float(np.std(samples)) if len(samples) > 1 else 0.0
        else:
            result = analyze_oscillation(samples, config)

        results.append((ts, result))

    # Sort by periodicity score descending
    results.sort(key=lambda x: x[1].periodicity_score, reverse=True)

    return results


def print_results(
    results: list[tuple[ContainerTimeseries, OscillationResult]],
    config: OscillationConfig,
) -> None:
    """Print analysis results to console."""
    print("\n" + "=" * 80)
    print("CPU OSCILLATION DETECTION RESULTS")
    print("=" * 80)
    print("\nConfiguration:")
    print(f"  Window size:          {config.window_size} samples")
    print(f"  Min periodicity:      {config.min_periodicity_score}")
    print(f"  Min amplitude:        {config.min_amplitude}%")
    print(f"  Period range:         {config.min_period}-{config.max_period} seconds")

    # Separate detected vs not detected
    detected = [(ts, r) for ts, r in results if r.detected]
    not_detected = [(ts, r) for ts, r in results if not r.detected]

    if detected:
        print(f"\n{'OSCILLATION DETECTED':=^80}")
        print(f"\n{'Container':<14} {'Period':>8} {'Freq':>8} {'Score':>8} {'Amp':>8} {'StdDev':>8} {'QoS':<12}")
        print("-" * 80)
        for ts, result in detected:
            print(
                f"{ts.container_short:<14} "
                f"{result.period:>7.1f}s "
                f"{result.frequency:>7.3f}Hz "
                f"{result.periodicity_score:>7.2f} "
                f"{result.amplitude:>7.1f}% "
                f"{result.stddev:>7.2f} "
                f"{ts.qos_class or 'unknown':<12}"
            )
    else:
        print("\nNo oscillation patterns detected above threshold.")

    print(f"\n{'ALL CONTAINERS':=^80}")
    print(f"\n{'Container':<14} {'Samples':>8} {'Score':>8} {'Period':>8} {'Amp':>8} {'StdDev':>8} {'Detected':<8}")
    print("-" * 80)
    for ts, result in results[:20]:  # Top 20 by periodicity score
        samples = len(ts.cpu_percent)
        detected_str = "YES" if result.detected else "-"
        period_str = f"{result.period:.1f}s" if result.period > 0 else "-"
        print(
            f"{ts.container_short:<14} "
            f"{samples:>8} "
            f"{result.periodicity_score:>7.2f} "
            f"{period_str:>8} "
            f"{result.amplitude:>7.1f}% "
            f"{result.stddev:>7.2f} "
            f"{detected_str:<8}"
        )

    if len(results) > 20:
        print(f"\n... and {len(results) - 20} more containers")


def plot_autocorrelation(
    ts: ContainerTimeseries,
    config: OscillationConfig,
) -> go.Figure:
    """Create autocorrelation plot for a single container."""
    samples = np.array(ts.cpu_percent)

    if len(samples) < config.window_size:
        window = samples
    else:
        window = samples[-config.window_size :]

    mean = np.mean(window)
    variance = np.var(window)

    # Compute autocorrelation for all lags
    lags = list(range(1, min(len(window) // 2, config.max_period + 5)))
    correlations = [autocorrelation(window, mean, variance, lag) for lag in lags]

    fig = make_subplots(
        rows=2,
        cols=1,
        subplot_titles=(f"CPU Usage - {ts.container_short}", "Autocorrelation"),
        vertical_spacing=0.15,
    )

    # CPU timeseries
    fig.add_trace(
        go.Scatter(
            x=list(range(len(window))),
            y=window,
            mode="lines",
            name="CPU %",
            line={"color": "blue"},
        ),
        row=1,
        col=1,
    )

    # Autocorrelation
    colors = ["green" if config.min_period <= lag <= config.max_period else "gray" for lag in lags]
    fig.add_trace(
        go.Bar(
            x=lags,
            y=correlations,
            name="Autocorrelation",
            marker_color=colors,
        ),
        row=2,
        col=1,
    )

    # Add threshold line
    fig.add_hline(
        y=config.min_periodicity_score,
        line_dash="dash",
        line_color="red",
        annotation_text=f"Threshold ({config.min_periodicity_score})",
        row=2,
        col=1,
    )

    fig.update_xaxes(title_text="Sample Index", row=1, col=1)
    fig.update_xaxes(title_text="Lag (seconds)", row=2, col=1)
    fig.update_yaxes(title_text="CPU %", row=1, col=1)
    fig.update_yaxes(title_text="Correlation", row=2, col=1)

    fig.update_layout(
        title=f"Oscillation Analysis: {ts.container_short}",
        height=600,
        showlegend=False,
    )

    return fig


def plot_all_results(
    results: list[tuple[ContainerTimeseries, OscillationResult]],
    config: OscillationConfig,
    output_path: Path | None = None,
    top_n: int = 6,
) -> None:
    """Create a combined plot showing top containers by periodicity score."""
    # Take top N containers
    top_results = results[:top_n]

    fig = make_subplots(
        rows=top_n,
        cols=2,
        subplot_titles=[
            title
            for ts, r in top_results
            for title in (
                f"{ts.container_short} (score={r.periodicity_score:.2f})",
                f"Autocorrelation (period={r.period:.1f}s)",
            )
        ],
        vertical_spacing=0.08,
        horizontal_spacing=0.1,
    )

    for idx, (ts, result) in enumerate(top_results, 1):
        samples = np.array(ts.cpu_percent)

        if len(samples) < config.window_size:
            window = samples
        else:
            window = samples[-config.window_size :]

        mean = np.mean(window)
        variance = np.var(window)

        # CPU timeseries
        color = "red" if result.detected else "blue"
        fig.add_trace(
            go.Scatter(
                x=list(range(len(window))),
                y=window,
                mode="lines",
                name=ts.container_short,
                line={"color": color},
                showlegend=False,
            ),
            row=idx,
            col=1,
        )

        # Autocorrelation
        lags = list(range(1, min(len(window) // 2, config.max_period + 5)))
        correlations = [autocorrelation(window, mean, variance, lag) for lag in lags]

        bar_colors = ["green" if config.min_period <= lag <= config.max_period else "lightgray" for lag in lags]
        fig.add_trace(
            go.Bar(
                x=lags,
                y=correlations,
                marker_color=bar_colors,
                showlegend=False,
            ),
            row=idx,
            col=2,
        )

        # Threshold line
        fig.add_hline(
            y=config.min_periodicity_score,
            line_dash="dash",
            line_color="red",
            row=idx,
            col=2,
        )

    fig.update_layout(
        title="CPU Oscillation Detection - Top Containers by Periodicity Score",
        height=200 * top_n,
    )

    if output_path:
        if output_path.suffix == ".html":
            fig.write_html(output_path)
        else:
            fig.write_image(output_path)
        print(f"Saved plot to {output_path}")
    else:
        fig.show()


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Detect CPU oscillation patterns using autocorrelation",
        formatter_class=argparse.RawDescriptionHelpFormatter,
        epilog="""
Examples:
    # Basic analysis
    uv run scripts/oscillation_detector.py metrics.parquet

    # Lower thresholds for more sensitive detection
    uv run scripts/oscillation_detector.py metrics.parquet --threshold 0.4 --min-amplitude 5.0

    # Generate HTML report with plots
    uv run scripts/oscillation_detector.py metrics.parquet --plot --output report.html

    # Analyze specific container
    uv run scripts/oscillation_detector.py metrics.parquet --container abc123456789
        """,
    )
    parser.add_argument("input", type=Path, help="Input parquet file")
    parser.add_argument(
        "-t",
        "--threshold",
        type=float,
        default=0.6,
        help="Minimum periodicity score (autocorrelation threshold, default: 0.6)",
    )
    parser.add_argument(
        "-a",
        "--min-amplitude",
        type=float,
        default=10.0,
        help="Minimum amplitude (peak-to-trough %%, default: 10.0)",
    )
    parser.add_argument(
        "-w",
        "--window",
        type=int,
        default=60,
        help="Analysis window size in samples (default: 60)",
    )
    parser.add_argument(
        "--min-period",
        type=int,
        default=2,
        help="Minimum period to detect in seconds (default: 2)",
    )
    parser.add_argument(
        "--max-period",
        type=int,
        default=30,
        help="Maximum period to detect in seconds (default: 30)",
    )
    parser.add_argument(
        "-c",
        "--container",
        type=str,
        help="Analyze only this container (prefix match)",
    )
    parser.add_argument(
        "-p",
        "--plot",
        action="store_true",
        help="Generate visualization plots",
    )
    parser.add_argument(
        "-o",
        "--output",
        type=Path,
        help="Output file for plots (HTML or image)",
    )
    parser.add_argument(
        "-n",
        "--top",
        type=int,
        default=6,
        help="Number of containers to show in plot (default: 6)",
    )

    args = parser.parse_args()

    if not args.input.exists():
        print(f"Error: File not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    config = OscillationConfig(
        window_size=args.window,
        min_periodicity_score=args.threshold,
        min_amplitude=args.min_amplitude,
        min_period=args.min_period,
        max_period=args.max_period,
    )

    print(f"Loading {args.input}...")
    df = load_and_prepare_data(args.input)
    print(f"Loaded {len(df):,} rows")

    print("Computing CPU timeseries...")
    timeseries = compute_cpu_timeseries(df)
    print(f"Found {len(timeseries)} containers")

    if args.container:
        # Filter to specific container
        filtered = {
            k: v
            for k, v in timeseries.items()
            if k.startswith(args.container) or v.container_short.startswith(args.container)
        }
        if not filtered:
            print(f"Error: No container matching '{args.container}'", file=sys.stderr)
            sys.exit(1)
        timeseries = filtered
        print(f"Filtered to {len(timeseries)} container(s)")

    print("Analyzing for oscillation patterns...")
    results = analyze_containers(timeseries, config)

    print_results(results, config)

    if args.plot or args.output:
        plot_all_results(results, config, args.output, args.top)


if __name__ == "__main__":
    main()
