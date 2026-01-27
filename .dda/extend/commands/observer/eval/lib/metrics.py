"""Metrics calculation for anomaly detection evaluation."""

import logging
from dataclasses import dataclass

import numpy as np
from data_loader import EvalDataset, GroundTruthWindow
from pot_thresholding import POTResult
from sklearn.metrics import precision_recall_fscore_support, roc_auc_score


@dataclass
class MetricsResult:
    """Evaluation metrics result."""

    ucr_score: float  # 0 or 1 (top-1 accuracy)
    adjusted_f1: float  # F1 with point-adjustment
    auc_roc: float  # AUC ROC
    computed_threshold: float  # from POT
    total_anomalies_found: int  # count of predictions

    # Diagnostics
    precision: float
    recall: float
    true_positives: int
    false_positives: int
    false_negatives: int
    max_score_timestamp: int
    max_score_in_window: bool


def calculate_ucr_score(
    y_true: np.ndarray, scores: np.ndarray, windows: list[GroundTruthWindow]
) -> tuple[float, int, bool]:
    """
    Calculate UCR score (top-1 accuracy).

    UCR score = 1 if the maximum anomaly score occurs within any ground truth window.

    Args:
        y_true: Binary ground truth (not used, but kept for consistency)
        scores: Anomaly scores
        windows: Ground truth windows

    Returns:
        (ucr_score, max_score_timestamp, max_in_window)
    """
    if len(windows) == 0:
        logging.warning("No ground truth windows - UCR score = 0")
        return 0.0, 0, False

    if len(scores) == 0:
        return 0.0, 0, False

    # Find index of maximum score
    max_idx = int(np.argmax(scores))

    # Check if max_idx is within any window
    max_in_window = False
    for window in windows:
        if window.start_index <= max_idx < window.end_index:
            max_in_window = True
            break

    ucr_score = 1.0 if max_in_window else 0.0

    return ucr_score, max_idx, max_in_window


def apply_point_adjustment(y_true: np.ndarray, y_pred: np.ndarray, windows: list[GroundTruthWindow]) -> np.ndarray:
    """
    Apply point-adjustment to predictions.

    If ANY point in a ground truth window is detected, mark the ENTIRE window as detected.

    Args:
        y_true: Binary ground truth
        y_pred: Binary predictions
        windows: Ground truth windows

    Returns:
        Adjusted binary predictions
    """
    y_pred_adjusted = y_pred.copy()

    for window in windows:
        # Check if any point in this window was detected
        if window.start_index >= 0 and window.end_index > window.start_index:
            window_predictions = y_pred[window.start_index : window.end_index]
            if np.sum(window_predictions) > 0:
                # Mark entire window as detected
                y_pred_adjusted[window.start_index : window.end_index] = 1

    return y_pred_adjusted


def calculate_adjusted_f1(y_true: np.ndarray, y_pred_adjusted: np.ndarray) -> tuple[float, float, float]:
    """
    Calculate F1 score with adjusted predictions.

    Args:
        y_true: Binary ground truth
        y_pred_adjusted: Adjusted binary predictions

    Returns:
        (f1_score, precision, recall)
    """
    # Handle edge case: no positive predictions
    if np.sum(y_pred_adjusted) == 0:
        if np.sum(y_true) == 0:
            # No ground truth and no predictions - perfect (but trivial)
            return 1.0, 1.0, 1.0
        else:
            # Ground truth exists but no predictions - zero recall
            return 0.0, 0.0, 0.0

    # Handle edge case: no positive ground truth
    if np.sum(y_true) == 0:
        # No ground truth but have predictions - all false positives
        return 0.0, 0.0, 0.0

    # Calculate precision, recall, F1
    precision, recall, f1, _ = precision_recall_fscore_support(
        y_true, y_pred_adjusted, average='binary', zero_division=0.0
    )

    return float(f1), float(precision), float(recall)


def calculate_auc_roc(y_true: np.ndarray, scores: np.ndarray) -> float:
    """
    Calculate AUC-ROC.

    Args:
        y_true: Binary ground truth
        scores: Anomaly scores (continuous)

    Returns:
        AUC-ROC score
    """
    # Handle edge cases
    if np.sum(y_true) == 0 or np.sum(y_true) == len(y_true):
        # All negative or all positive - AUC undefined
        logging.warning("AUC-ROC undefined: ground truth has only one class")
        return float('nan')

    try:
        auc_score = roc_auc_score(y_true, scores)
        return float(auc_score)
    except Exception as e:
        logging.error("AUC-ROC calculation failed: %s", e)
        return float('nan')


def calculate_metrics(dataset: EvalDataset, pot_result: POTResult) -> MetricsResult:
    """
    Calculate all evaluation metrics.

    Args:
        dataset: Evaluation dataset
        pot_result: POT thresholding result

    Returns:
        MetricsResult with all metrics
    """
    # Extract data
    y_true = dataset.y_true
    y_pred_raw = pot_result.y_pred_raw
    scores = dataset.anomaly_scores
    windows = dataset.windows
    timestamps = dataset.timestamps

    # Calculate UCR score
    ucr_score, max_idx, max_in_window = calculate_ucr_score(y_true, scores, windows)

    # Apply point-adjustment
    y_pred_adjusted = apply_point_adjustment(y_true, y_pred_raw, windows)

    # Calculate adjusted F1
    adjusted_f1, precision, recall = calculate_adjusted_f1(y_true, y_pred_adjusted)

    # Calculate AUC-ROC
    auc_roc = calculate_auc_roc(y_true, scores)

    # Calculate confusion matrix metrics
    true_positives = int(np.sum((y_pred_adjusted == 1) & (y_true == 1)))
    false_positives = int(np.sum((y_pred_adjusted == 1) & (y_true == 0)))
    false_negatives = int(np.sum((y_pred_adjusted == 0) & (y_true == 1)))

    # Total anomalies found (before adjustment)
    total_anomalies_found = int(np.sum(y_pred_raw))

    # Max score timestamp
    max_score_timestamp = int(timestamps[max_idx]) if len(timestamps) > 0 else 0

    return MetricsResult(
        ucr_score=ucr_score,
        adjusted_f1=adjusted_f1,
        auc_roc=auc_roc,
        computed_threshold=pot_result.threshold,
        total_anomalies_found=total_anomalies_found,
        precision=precision,
        recall=recall,
        true_positives=true_positives,
        false_positives=false_positives,
        false_negatives=false_negatives,
        max_score_timestamp=max_score_timestamp,
        max_score_in_window=max_in_window,
    )
