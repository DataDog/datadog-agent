"""Unit tests for data_loader module."""

import pandas as pd
import pytest

from ..lib.data_loader import (
    GroundTruthWindow,
    align_with_findings,
    build_ground_truth,
    extract_incident_windows,
)


def test_extract_incident_windows_simple():
    """Test extraction of a simple incident window."""
    df = pd.DataFrame(
        [
            {'timestamp': 1000, 'metric_name': 'heap.used_mb', 'value': 512.0},
            {'timestamp': 1040, 'metric_name': 'observer.incident', 'value': 1.0},
            {'timestamp': 1060, 'metric_name': 'observer.incident', 'value': 0.0},
            {'timestamp': 1100, 'metric_name': 'heap.used_mb', 'value': 512.0},
        ]
    )

    windows = extract_incident_windows(df)

    assert len(windows) == 1
    assert windows[0].start_timestamp == 1040
    assert windows[0].end_timestamp == 1060


def test_extract_incident_windows_multiple():
    """Test extraction of multiple incident windows."""
    df = pd.DataFrame(
        [
            {'timestamp': 1040, 'metric_name': 'observer.incident', 'value': 1.0},
            {'timestamp': 1060, 'metric_name': 'observer.incident', 'value': 0.0},
            {'timestamp': 1130, 'metric_name': 'observer.incident', 'value': 1.0},
            {'timestamp': 1160, 'metric_name': 'observer.incident', 'value': 0.0},
        ]
    )

    windows = extract_incident_windows(df)

    assert len(windows) == 2
    assert windows[0].start_timestamp == 1040
    assert windows[0].end_timestamp == 1060
    assert windows[1].start_timestamp == 1130
    assert windows[1].end_timestamp == 1160


def test_extract_incident_windows_none():
    """Test with no incident markers."""
    df = pd.DataFrame(
        [
            {'timestamp': 1000, 'metric_name': 'heap.used_mb', 'value': 512.0},
            {'timestamp': 1100, 'metric_name': 'heap.used_mb', 'value': 512.0},
        ]
    )

    windows = extract_incident_windows(df)

    assert len(windows) == 0


def test_extract_incident_windows_open_incident():
    """Test incident that doesn't close."""
    df = pd.DataFrame(
        [
            {'timestamp': 1040, 'metric_name': 'observer.incident', 'value': 1.0},
            {'timestamp': 1100, 'metric_name': 'heap.used_mb', 'value': 512.0},
        ]
    )

    windows = extract_incident_windows(df)

    assert len(windows) == 1
    assert windows[0].start_timestamp == 1040
    assert windows[0].end_timestamp == 1040  # Max timestamp


def test_align_with_findings():
    """Test alignment of metrics with findings."""
    df_metrics = pd.DataFrame(
        [
            {'timestamp': 1000, 'metric_name': 'heap.used_mb', 'value': 512.0, 'tags': '[]'},
            {'timestamp': 1001, 'metric_name': 'heap.used_mb', 'value': 520.0, 'tags': '[]'},
            {'timestamp': 1002, 'metric_name': 'heap.used_mb', 'value': 530.0, 'tags': '[]'},
        ]
    )

    df_findings = pd.DataFrame(
        [
            {'timestamp': 1000, 'anomaly_score': 0.1},
            {'timestamp': 1001, 'anomaly_score': 0.2},
            {'timestamp': 1002, 'anomaly_score': 0.8},
        ]
    )

    df_aligned = align_with_findings(df_metrics, df_findings, 'heap.used_mb')

    assert len(df_aligned) == 3
    assert list(df_aligned.columns) == ['timestamp', 'value', 'anomaly_score']
    assert df_aligned['anomaly_score'].iloc[2] == 0.8


def test_align_with_findings_missing_metric():
    """Test error when metric not found."""
    df_metrics = pd.DataFrame(
        [
            {'timestamp': 1000, 'metric_name': 'heap.used_mb', 'value': 512.0, 'tags': '[]'},
        ]
    )

    df_findings = pd.DataFrame(
        [
            {'timestamp': 1000, 'anomaly_score': 0.1},
        ]
    )

    with pytest.raises(ValueError, match="No data found for metric"):
        align_with_findings(df_metrics, df_findings, 'nonexistent_metric')


def test_build_ground_truth():
    """Test building binary ground truth from windows."""
    df_aligned = pd.DataFrame(
        {
            'timestamp': [1000, 1001, 1002, 1003, 1004],
            'value': [1, 2, 3, 4, 5],
            'anomaly_score': [0.1, 0.2, 0.8, 0.9, 0.1],
        }
    )

    windows = [GroundTruthWindow(start_timestamp=1002, end_timestamp=1003)]

    y_true = build_ground_truth(df_aligned, windows)

    assert len(y_true) == 5
    assert y_true[0] == 0
    assert y_true[1] == 0
    assert y_true[2] == 1  # In window
    assert y_true[3] == 1  # In window
    assert y_true[4] == 0

    # Check that window indices were updated
    assert windows[0].start_index == 2
    assert windows[0].end_index == 4


def test_build_ground_truth_multiple_windows():
    """Test with multiple ground truth windows."""
    df_aligned = pd.DataFrame(
        {'timestamp': list(range(1000, 1010)), 'value': list(range(10)), 'anomaly_score': [0.1] * 10}
    )

    windows = [
        GroundTruthWindow(start_timestamp=1002, end_timestamp=1003),
        GroundTruthWindow(start_timestamp=1007, end_timestamp=1008),
    ]

    y_true = build_ground_truth(df_aligned, windows)

    assert y_true[2] == 1
    assert y_true[3] == 1
    assert y_true[4] == 0
    assert y_true[5] == 0
    assert y_true[6] == 0
    assert y_true[7] == 1
    assert y_true[8] == 1
