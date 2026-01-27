"""Unit tests for metrics module."""

import numpy as np

from ..lib.data_loader import EvalDataset, GroundTruthWindow
from ..lib.metrics import (
    apply_point_adjustment,
    calculate_adjusted_f1,
    calculate_auc_roc,
    calculate_metrics,
    calculate_ucr_score,
)
from ..lib.pot_thresholding import POTResult


def test_calculate_ucr_score_hit():
    """Test UCR score when max is in window."""
    scores = np.array([0.1, 0.2, 0.9, 0.3, 0.2])  # Max at index 2
    y_true = np.array([0, 0, 1, 1, 0])

    windows = [GroundTruthWindow(start_timestamp=1002, end_timestamp=1003, start_index=2, end_index=4)]

    ucr, max_idx, in_window = calculate_ucr_score(y_true, scores, windows)

    assert ucr == 1.0
    assert max_idx == 2
    assert in_window is True


def test_calculate_ucr_score_miss():
    """Test UCR score when max is outside window."""
    scores = np.array([0.1, 0.2, 0.3, 0.9, 0.2])  # Max at index 3
    y_true = np.array([0, 1, 1, 0, 0])

    windows = [GroundTruthWindow(start_timestamp=1001, end_timestamp=1002, start_index=1, end_index=3)]

    ucr, max_idx, in_window = calculate_ucr_score(y_true, scores, windows)

    assert ucr == 0.0
    assert max_idx == 3
    assert in_window is False


def test_calculate_ucr_score_no_windows():
    """Test UCR score with no ground truth."""
    scores = np.array([0.1, 0.2, 0.9, 0.3, 0.2])
    y_true = np.array([0, 0, 0, 0, 0])
    windows = []

    ucr, max_idx, in_window = calculate_ucr_score(y_true, scores, windows)

    assert ucr == 0.0
    assert in_window is False


def test_apply_point_adjustment():
    """Test point-adjustment algorithm."""
    y_true = np.array([0, 0, 1, 1, 1, 0, 0])
    y_pred = np.array([0, 0, 0, 1, 0, 0, 0])  # Detects only one point in window

    windows = [GroundTruthWindow(start_timestamp=1002, end_timestamp=1004, start_index=2, end_index=5)]

    y_pred_adjusted = apply_point_adjustment(y_true, y_pred, windows)

    # Entire window should be marked as detected
    assert y_pred_adjusted[0] == 0
    assert y_pred_adjusted[1] == 0
    assert y_pred_adjusted[2] == 1  # Adjusted
    assert y_pred_adjusted[3] == 1  # Already detected
    assert y_pred_adjusted[4] == 1  # Adjusted
    assert y_pred_adjusted[5] == 0
    assert y_pred_adjusted[6] == 0


def test_apply_point_adjustment_no_detection():
    """Test point-adjustment when window not detected."""
    y_true = np.array([0, 0, 1, 1, 1, 0, 0])
    y_pred = np.array([0, 0, 0, 0, 0, 0, 0])  # No detections

    windows = [GroundTruthWindow(start_timestamp=1002, end_timestamp=1004, start_index=2, end_index=5)]

    y_pred_adjusted = apply_point_adjustment(y_true, y_pred, windows)

    # Should remain all zeros
    assert np.all(y_pred_adjusted == 0)


def test_calculate_adjusted_f1_perfect():
    """Test F1 with perfect predictions."""
    y_true = np.array([0, 0, 1, 1, 1, 0, 0])
    y_pred = np.array([0, 0, 1, 1, 1, 0, 0])

    f1, precision, recall = calculate_adjusted_f1(y_true, y_pred)

    assert f1 == 1.0
    assert precision == 1.0
    assert recall == 1.0


def test_calculate_adjusted_f1_no_predictions():
    """Test F1 with no predictions."""
    y_true = np.array([0, 0, 1, 1, 1, 0, 0])
    y_pred = np.array([0, 0, 0, 0, 0, 0, 0])

    f1, precision, recall = calculate_adjusted_f1(y_true, y_pred)

    assert f1 == 0.0
    assert precision == 0.0
    assert recall == 0.0


def test_calculate_adjusted_f1_all_negative():
    """Test F1 with no ground truth."""
    y_true = np.array([0, 0, 0, 0, 0, 0, 0])
    y_pred = np.array([0, 1, 0, 1, 0, 0, 0])

    f1, precision, recall = calculate_adjusted_f1(y_true, y_pred)

    assert f1 == 0.0


def test_calculate_auc_roc():
    """Test AUC-ROC calculation."""
    y_true = np.array([0, 0, 0, 1, 1, 1, 0])
    scores = np.array([0.1, 0.2, 0.3, 0.7, 0.8, 0.9, 0.2])

    auc = calculate_auc_roc(y_true, scores)

    assert 0.0 <= auc <= 1.0
    assert auc > 0.8  # Should be high for this case


def test_calculate_auc_roc_single_class():
    """Test AUC-ROC with single class."""
    y_true = np.array([1, 1, 1, 1, 1])
    scores = np.array([0.5, 0.6, 0.7, 0.8, 0.9])

    auc = calculate_auc_roc(y_true, scores)

    assert np.isnan(auc)  # Should be NaN


def test_calculate_metrics_integration():
    """Test full metrics calculation."""
    # Create mock dataset
    dataset = EvalDataset(
        timestamps=np.array([1000, 1001, 1002, 1003, 1004]),
        values=np.array([100, 200, 300, 400, 500]),
        anomaly_scores=np.array([0.1, 0.2, 0.8, 0.9, 0.2]),
        y_true=np.array([0, 0, 1, 1, 0]),
        windows=[GroundTruthWindow(start_timestamp=1002, end_timestamp=1003, start_index=2, end_index=4)],
        metric_name='test_metric',
    )

    # Create mock POT result
    pot_result = POTResult(
        threshold=0.5,
        initial_threshold=0.5,
        q=1e-4,
        n_total=5,
        n_peaks=2,
        gpd_shape=0.1,
        gpd_scale=0.2,
        y_pred_raw=np.array([0, 0, 1, 1, 0]),
    )

    metrics = calculate_metrics(dataset, pot_result)

    # Verify metrics structure
    assert metrics.ucr_score == 1.0  # Max score (0.9) is in window
    assert 0.0 <= metrics.adjusted_f1 <= 1.0
    assert 0.0 <= metrics.auc_roc <= 1.0
    assert metrics.computed_threshold == 0.5
    assert metrics.total_anomalies_found >= 0
