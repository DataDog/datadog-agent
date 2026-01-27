"""Data loading and preparation for evaluation framework."""

import logging
from dataclasses import dataclass
from pathlib import Path

import numpy as np
import pandas as pd


@dataclass
class GroundTruthWindow:
    """Represents a ground truth incident window."""

    start_timestamp: int
    end_timestamp: int
    start_index: int = -1
    end_index: int = -1


@dataclass
class EvalDataset:
    """Complete evaluation dataset with aligned data and ground truth."""

    timestamps: np.ndarray  # aligned timestamps
    values: np.ndarray  # raw metric values
    anomaly_scores: np.ndarray  # from findings
    y_true: np.ndarray  # binary ground truth (0/1)
    windows: list[GroundTruthWindow]
    metric_name: str  # primary metric being evaluated


def load_parquet_file(path: str) -> pd.DataFrame:
    """
    Load a parquet file and validate it exists.

    Args:
        path: Path to parquet file

    Returns:
        DataFrame with parquet contents

    Raises:
        FileNotFoundError: If file doesn't exist
        ValueError: If file is empty or invalid
    """
    file_path = Path(path)
    if not file_path.exists():
        raise FileNotFoundError(f"Parquet file not found: {path}")

    df = pd.read_parquet(file_path)

    if len(df) == 0:
        raise ValueError(f"Parquet file is empty: {path}")

    return df


def extract_incident_windows(df_metrics: pd.DataFrame, verbose: bool = False) -> list[GroundTruthWindow]:
    """
    Extract incident windows from observer.incident metric.

    The observer.incident metric has:
    - value=1.0 at incident start timestamp
    - value=0.0 at incident end timestamp

    Args:
        df_metrics: DataFrame with columns [timestamp, metric_name, value, tags]
        verbose: Whether to print debug information

    Returns:
        List of GroundTruthWindow objects
    """
    # Filter to observer.incident metric only
    incident_df = df_metrics[df_metrics['metric_name'] == 'observer.incident'].copy()

    if len(incident_df) == 0:
        if verbose:
            logging.info("No observer.incident metric found - no ground truth")
        return []  # No ground truth

    incident_df = incident_df.sort_values('timestamp')

    windows = []
    start_ts = None

    for _, row in incident_df.iterrows():
        if row['value'] == 1.0:  # Incident start
            if start_ts is not None:
                # Warning: nested incident start
                logging.warning("Nested incident start at %s", row['timestamp'])
            start_ts = row['timestamp']

        elif row['value'] == 0.0:  # Incident end
            if start_ts is None:
                # Error: end without start
                logging.error("Incident end without start at %s", row['timestamp'])
                continue

            windows.append(
                GroundTruthWindow(
                    start_timestamp=start_ts,
                    end_timestamp=row['timestamp'],
                )
            )
            start_ts = None

    # Handle open incident at end
    if start_ts is not None:
        windows.append(
            GroundTruthWindow(
                start_timestamp=start_ts,
                end_timestamp=int(incident_df['timestamp'].max()),
            )
        )
        logging.warning("Open incident at end of data")

    return windows


def align_with_findings(
    df_metrics: pd.DataFrame, df_findings: pd.DataFrame, metric_name: str, verbose: bool = False
) -> pd.DataFrame:
    """
    Align metric values with findings (anomaly scores).

    1. Filter metrics to the specified metric_name
    2. Inner join on timestamp with findings
    3. Sort by timestamp

    Args:
        df_metrics: Raw metrics DataFrame
        df_findings: Findings DataFrame with anomaly_score
        metric_name: Name of metric to evaluate
        verbose: Whether to print debug information

    Returns:
        Aligned DataFrame with columns [timestamp, value, anomaly_score]
    """
    # Validate columns
    required_metric_cols = {'timestamp', 'metric_name', 'value'}
    if not required_metric_cols.issubset(df_metrics.columns):
        missing = required_metric_cols - set(df_metrics.columns)
        raise ValueError(f"Missing columns in raw_metrics: {missing}")

    required_finding_cols = {'timestamp', 'anomaly_score'}
    if not required_finding_cols.issubset(df_findings.columns):
        missing = required_finding_cols - set(df_findings.columns)
        raise ValueError(f"Missing columns in findings: {missing}")

    # Filter to the metric being evaluated (not observer.incident)
    df_metric = df_metrics[df_metrics['metric_name'] == metric_name].copy()

    if len(df_metric) == 0:
        raise ValueError(f"No data found for metric '{metric_name}'")

    initial_metric_count = len(df_metric)
    initial_findings_count = len(df_findings)

    # Inner join on timestamp
    df_aligned = df_metric.merge(
        df_findings, on='timestamp', how='inner', suffixes=('_metric', '_finding')
    ).sort_values('timestamp')

    if len(df_aligned) == 0:
        raise ValueError("No overlapping timestamps between metrics and findings")

    if verbose:
        dropped_metric = initial_metric_count - len(df_aligned)
        dropped_findings = initial_findings_count - len(df_aligned)
        if dropped_metric > 0:
            logging.info("Dropped %d metric rows without matching findings", dropped_metric)
        if dropped_findings > 0:
            logging.info("Dropped %d findings without matching metrics", dropped_findings)

    return df_aligned[['timestamp', 'value', 'anomaly_score']].reset_index(drop=True)


def build_ground_truth(df_aligned: pd.DataFrame, windows: list[GroundTruthWindow], verbose: bool = False) -> np.ndarray:
    """
    Build binary ground truth vector from incident windows.

    Sets y_true[i] = 1 if timestamp[i] is within any incident window.

    Args:
        df_aligned: Aligned DataFrame with timestamp column
        windows: List of ground truth windows
        verbose: Whether to print debug information

    Returns:
        Binary numpy array (0/1) of same length as df_aligned
    """
    timestamps = df_aligned['timestamp'].values
    y_true = np.zeros(len(timestamps), dtype=int)

    # Update window indices based on aligned timestamps
    for window in windows:
        # Find indices where timestamp is in [start, end]
        mask = (timestamps >= window.start_timestamp) & (timestamps <= window.end_timestamp)
        indices = np.where(mask)[0]

        if len(indices) == 0:
            logging.warning(
                "Window [%d, %d] has no corresponding timestamps in aligned data",
                window.start_timestamp,
                window.end_timestamp,
            )
            continue

        # Update window indices
        window.start_index = int(indices[0])
        window.end_index = int(indices[-1] + 1)

        # Mark as ground truth
        y_true[indices] = 1

        if verbose:
            logging.info(
                "Ground truth window: [%d, %d] -> indices [%d, %d), %d points",
                window.start_timestamp,
                window.end_timestamp,
                window.start_index,
                window.end_index,
                len(indices),
            )

    return y_true


def load_and_prepare_data(
    metrics_path: str, findings_path: str, metric_name: str, verbose: bool = False
) -> EvalDataset:
    """
    Load and prepare complete evaluation dataset.

    Args:
        metrics_path: Path to raw_metrics.parquet
        findings_path: Path to findings.parquet
        metric_name: Name of metric to evaluate
        verbose: Whether to print debug information

    Returns:
        EvalDataset with all components ready for evaluation

    Raises:
        FileNotFoundError: If files don't exist
        ValueError: If data is invalid or misaligned
    """
    # Load parquet files
    df_metrics = load_parquet_file(metrics_path)
    df_findings = load_parquet_file(findings_path)

    # Extract ground truth windows
    windows = extract_incident_windows(df_metrics, verbose=verbose)

    # Align metrics with findings
    df_aligned = align_with_findings(df_metrics, df_findings, metric_name, verbose=verbose)

    # Build binary ground truth
    y_true = build_ground_truth(df_aligned, windows, verbose=verbose)

    # Extract arrays
    timestamps = df_aligned['timestamp'].values
    values = df_aligned['value'].values
    anomaly_scores = df_aligned['anomaly_score'].values

    # Validate no NaN in critical columns
    if np.any(np.isnan(anomaly_scores)):
        raise ValueError("anomaly_score contains NaN values")

    return EvalDataset(
        timestamps=timestamps,
        values=values,
        anomaly_scores=anomaly_scores,
        y_true=y_true,
        windows=windows,
        metric_name=metric_name,
    )
