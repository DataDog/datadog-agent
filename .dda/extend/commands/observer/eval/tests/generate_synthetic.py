#!/usr/bin/env python3
"""Generate synthetic parquet test data for observer evaluation framework."""

import argparse
from pathlib import Path

import numpy as np
import pandas as pd


def generate_simple_incident(output_dir: Path) -> tuple[Path, Path]:
    """
    Generate simple incident scenario.

    Scenario:
    - 100 timestamps (t=1000 to t=1099)
    - One metric: heap.used_mb
    - Incident: t=1040 to t=1060 (20 seconds)
    - Clear anomaly signal during incident
    """
    print("Generating simple_incident scenario...")

    timestamps = np.arange(1000, 1100, 1)  # 100 seconds

    # Generate heap metric
    heap_values = np.ones(100) * 512  # Baseline 512 MB
    heap_values[40:60] = 900  # Incident: high heap usage (900 MB)
    heap_values += np.random.normal(0, 10, 100)  # Add noise

    # Generate anomaly scores (simulated detector output)
    scores = np.zeros(100)
    scores[40:60] = np.random.uniform(0.7, 0.9, 20)  # High scores during incident
    scores[:40] = np.random.uniform(0.0, 0.2, 40)  # Low baseline scores
    scores[60:] = np.random.uniform(0.0, 0.2, 40)

    # Build raw_metrics dataframe
    rows = []
    for i, ts in enumerate(timestamps):
        rows.append(
            {
                'timestamp': int(ts),
                'metric_name': 'heap.used_mb',
                'value': float(heap_values[i]),
                'tags': '["host:demo"]',
            }
        )

    # Add incident markers
    rows.append({'timestamp': 1040, 'metric_name': 'observer.incident', 'value': 1.0, 'tags': '[]'})
    rows.append({'timestamp': 1060, 'metric_name': 'observer.incident', 'value': 0.0, 'tags': '[]'})

    df_metrics = pd.DataFrame(rows).sort_values('timestamp')

    # Build findings dataframe
    df_findings = pd.DataFrame({'timestamp': timestamps, 'anomaly_score': scores})

    # Write parquet files
    metrics_path = output_dir / "simple_incident_metrics.parquet"
    findings_path = output_dir / "simple_incident_findings.parquet"

    df_metrics.to_parquet(metrics_path, index=False)
    df_findings.to_parquet(findings_path, index=False)

    print(f"  Created: {metrics_path}")
    print(f"  Created: {findings_path}")

    return metrics_path, findings_path


def generate_multi_metric(output_dir: Path) -> tuple[Path, Path]:
    """
    Generate multiple metrics scenario.

    Scenario:
    - 150 timestamps
    - Three metrics: heap.used_mb, gc.pause_ms, latency_p99
    - Incident: t=1050 to t=1100
    - Correlated anomalies across metrics
    """
    print("Generating multi_metric scenario...")

    timestamps = np.arange(1000, 1150, 1)  # 150 seconds

    # Generate three correlated metrics
    heap_values = np.ones(150) * 512
    heap_values[50:100] = 900  # Incident
    heap_values += np.random.normal(0, 10, 150)

    gc_values = np.ones(150) * 15  # Baseline 15ms
    gc_values[50:100] = 50  # Incident: long GC pauses
    gc_values += np.random.normal(0, 2, 150)

    latency_values = np.ones(150) * 100  # Baseline 100ms
    latency_values[50:100] = 500  # Incident: high latency
    latency_values += np.random.normal(0, 10, 150)

    # Generate anomaly scores
    scores = np.zeros(150)
    scores[50:100] = np.random.uniform(0.6, 0.95, 50)  # High during incident
    scores[:50] = np.random.uniform(0.0, 0.25, 50)
    scores[100:] = np.random.uniform(0.0, 0.25, 50)

    # Build raw_metrics dataframe
    rows = []
    for i, ts in enumerate(timestamps):
        rows.append(
            {
                'timestamp': int(ts),
                'metric_name': 'heap.used_mb',
                'value': float(heap_values[i]),
                'tags': '["host:demo"]',
            }
        )
        rows.append(
            {'timestamp': int(ts), 'metric_name': 'gc.pause_ms', 'value': float(gc_values[i]), 'tags': '["host:demo"]'}
        )
        rows.append(
            {
                'timestamp': int(ts),
                'metric_name': 'latency_p99',
                'value': float(latency_values[i]),
                'tags': '["host:demo"]',
            }
        )

    # Add incident markers
    rows.append({'timestamp': 1050, 'metric_name': 'observer.incident', 'value': 1.0, 'tags': '[]'})
    rows.append({'timestamp': 1100, 'metric_name': 'observer.incident', 'value': 0.0, 'tags': '[]'})

    df_metrics = pd.DataFrame(rows).sort_values('timestamp')

    # Build findings dataframe
    df_findings = pd.DataFrame({'timestamp': timestamps, 'anomaly_score': scores})

    # Write parquet files
    metrics_path = output_dir / "multi_metric_metrics.parquet"
    findings_path = output_dir / "multi_metric_findings.parquet"

    df_metrics.to_parquet(metrics_path, index=False)
    df_findings.to_parquet(findings_path, index=False)

    print(f"  Created: {metrics_path}")
    print(f"  Created: {findings_path}")

    return metrics_path, findings_path


def generate_no_incident(output_dir: Path) -> tuple[Path, Path]:
    """
    Generate no incident scenario (baseline).

    Scenario:
    - 100 timestamps
    - Normal data only, no anomalies
    - No observer.incident metric
    """
    print("Generating no_incident scenario...")

    timestamps = np.arange(1000, 1100, 1)

    # Generate normal heap metric
    heap_values = np.ones(100) * 512
    heap_values += np.random.normal(0, 10, 100)

    # Generate normal anomaly scores (low)
    scores = np.random.uniform(0.0, 0.3, 100)

    # Build raw_metrics dataframe (no incident markers)
    rows = []
    for i, ts in enumerate(timestamps):
        rows.append(
            {
                'timestamp': int(ts),
                'metric_name': 'heap.used_mb',
                'value': float(heap_values[i]),
                'tags': '["host:demo"]',
            }
        )

    df_metrics = pd.DataFrame(rows)

    # Build findings dataframe
    df_findings = pd.DataFrame({'timestamp': timestamps, 'anomaly_score': scores})

    # Write parquet files
    metrics_path = output_dir / "no_incident_metrics.parquet"
    findings_path = output_dir / "no_incident_findings.parquet"

    df_metrics.to_parquet(metrics_path, index=False)
    df_findings.to_parquet(findings_path, index=False)

    print(f"  Created: {metrics_path}")
    print(f"  Created: {findings_path}")

    return metrics_path, findings_path


def generate_multi_incident(output_dir: Path) -> tuple[Path, Path]:
    """
    Generate multiple incidents scenario.

    Scenario:
    - 200 timestamps
    - Two separate incidents with recovery period between
    - Incident 1: t=1050 to t=1080
    - Incident 2: t=1130 to t=1160
    """
    print("Generating multi_incident scenario...")

    timestamps = np.arange(1000, 1200, 1)  # 200 seconds

    # Generate heap metric with two incidents
    heap_values = np.ones(200) * 512
    heap_values[50:80] = 850  # First incident
    heap_values[130:160] = 900  # Second incident
    heap_values += np.random.normal(0, 10, 200)

    # Generate anomaly scores
    scores = np.zeros(200)
    scores[50:80] = np.random.uniform(0.65, 0.85, 30)  # First incident
    scores[130:160] = np.random.uniform(0.70, 0.90, 30)  # Second incident
    scores[:50] = np.random.uniform(0.0, 0.2, 50)
    scores[80:130] = np.random.uniform(0.0, 0.2, 50)
    scores[160:] = np.random.uniform(0.0, 0.2, 40)

    # Build raw_metrics dataframe
    rows = []
    for i, ts in enumerate(timestamps):
        rows.append(
            {
                'timestamp': int(ts),
                'metric_name': 'heap.used_mb',
                'value': float(heap_values[i]),
                'tags': '["host:demo"]',
            }
        )

    # Add incident markers for both incidents
    rows.append({'timestamp': 1050, 'metric_name': 'observer.incident', 'value': 1.0, 'tags': '[]'})
    rows.append({'timestamp': 1080, 'metric_name': 'observer.incident', 'value': 0.0, 'tags': '[]'})
    rows.append({'timestamp': 1130, 'metric_name': 'observer.incident', 'value': 1.0, 'tags': '[]'})
    rows.append({'timestamp': 1160, 'metric_name': 'observer.incident', 'value': 0.0, 'tags': '[]'})

    df_metrics = pd.DataFrame(rows).sort_values('timestamp')

    # Build findings dataframe
    df_findings = pd.DataFrame({'timestamp': timestamps, 'anomaly_score': scores})

    # Write parquet files
    metrics_path = output_dir / "multi_incident_metrics.parquet"
    findings_path = output_dir / "multi_incident_findings.parquet"

    df_metrics.to_parquet(metrics_path, index=False)
    df_findings.to_parquet(findings_path, index=False)

    print(f"  Created: {metrics_path}")
    print(f"  Created: {findings_path}")

    return metrics_path, findings_path


def generate_bad_detector(output_dir: Path) -> tuple[Path, Path]:
    """
    Generate bad detector scenario (inverted scores).

    Scenario:
    - 100 timestamps (t=1000 to t=1099)
    - One metric: heap.used_mb
    - Incident: t=1040 to t=1060 (20 seconds)
    - BAD DETECTOR: High scores when NORMAL, low scores during INCIDENT
    - Expected results: UCR=0, Low F1, Poor AUC
    """
    print("Generating bad_detector scenario...")

    timestamps = np.arange(1000, 1100, 1)  # 100 seconds

    # Generate heap metric (same as simple_incident)
    heap_values = np.ones(100) * 512  # Baseline 512 MB
    heap_values[40:60] = 900  # Incident: high heap usage (900 MB)
    heap_values += np.random.normal(0, 10, 100)  # Add noise

    # Generate INVERTED anomaly scores (detector is broken/bad)
    scores = np.zeros(100)
    # HIGH scores during NORMAL periods (false alarms)
    scores[:40] = np.random.uniform(0.6, 0.9, 40)  # High scores before incident
    scores[60:] = np.random.uniform(0.6, 0.9, 40)  # High scores after incident
    # LOW scores during INCIDENT (missed detection)
    scores[40:60] = np.random.uniform(0.0, 0.3, 20)  # Low scores during incident

    # Build raw_metrics dataframe
    rows = []
    for i, ts in enumerate(timestamps):
        rows.append(
            {
                'timestamp': int(ts),
                'metric_name': 'heap.used_mb',
                'value': float(heap_values[i]),
                'tags': '["host:demo"]',
            }
        )

    # Add incident markers
    rows.append({'timestamp': 1040, 'metric_name': 'observer.incident', 'value': 1.0, 'tags': '[]'})
    rows.append({'timestamp': 1060, 'metric_name': 'observer.incident', 'value': 0.0, 'tags': '[]'})

    df_metrics = pd.DataFrame(rows).sort_values('timestamp')

    # Build findings dataframe
    df_findings = pd.DataFrame({'timestamp': timestamps, 'anomaly_score': scores})

    # Write parquet files
    metrics_path = output_dir / "bad_detector_metrics.parquet"
    findings_path = output_dir / "bad_detector_findings.parquet"

    df_metrics.to_parquet(metrics_path, index=False)
    df_findings.to_parquet(findings_path, index=False)

    print(f"  Created: {metrics_path}")
    print(f"  Created: {findings_path}")
    print("  Note: This detector has INVERTED scores (high when normal, low during incident)")

    return metrics_path, findings_path


def main():
    """Generate all synthetic test scenarios."""
    parser = argparse.ArgumentParser(description="Generate synthetic parquet test data for observer evaluation")
    parser.add_argument(
        "--output-dir",
        type=Path,
        default=Path(__file__).parent / "fixtures",
        help="Output directory for parquet files (default: ./fixtures)",
    )

    args = parser.parse_args()

    # Create output directory
    args.output_dir.mkdir(parents=True, exist_ok=True)
    print(f"Output directory: {args.output_dir}\n")

    # Generate all scenarios
    generate_simple_incident(args.output_dir)
    print()
    generate_multi_metric(args.output_dir)
    print()
    generate_no_incident(args.output_dir)
    print()
    generate_multi_incident(args.output_dir)
    print()
    generate_bad_detector(args.output_dir)
    print()

    print("All synthetic test data generated successfully!")


if __name__ == "__main__":
    main()
