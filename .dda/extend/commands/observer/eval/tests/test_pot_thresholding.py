"""Unit tests for POT thresholding module."""

import numpy as np
import pytest

from ..lib.pot_thresholding import (
    apply_threshold,
    compute_initial_threshold,
    compute_pot_threshold,
    fit_gpd,
)


def test_compute_initial_threshold():
    """Test percentile threshold computation."""
    scores = np.array([0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9, 1.0])

    # 90th percentile should be 0.9
    threshold = compute_initial_threshold(scores, 90.0)
    assert abs(threshold - 0.9) < 0.01

    # 50th percentile should be 0.5
    threshold = compute_initial_threshold(scores, 50.0)
    assert abs(threshold - 0.5) < 0.1


def test_compute_initial_threshold_invalid():
    """Test invalid percentile values."""
    scores = np.array([0.1, 0.2, 0.3])

    with pytest.raises(ValueError, match="Percentile must be in"):
        compute_initial_threshold(scores, -1)

    with pytest.raises(ValueError, match="Percentile must be in"):
        compute_initial_threshold(scores, 101)


def test_fit_gpd():
    """Test GPD fitting."""
    # Generate some random peaks
    np.random.seed(42)
    peaks = np.random.exponential(scale=2.0, size=100)

    shape, scale = fit_gpd(peaks)

    # Check that parameters are reasonable
    assert isinstance(shape, float)
    assert isinstance(scale, float)
    assert scale > 0  # Scale must be positive


def test_fit_gpd_empty():
    """Test GPD fitting with no peaks."""
    peaks = np.array([])

    with pytest.raises(ValueError, match="No peaks provided"):
        fit_gpd(peaks)


def test_compute_pot_threshold_simple():
    """Test POT threshold computation."""
    # Create scores with clear separation
    np.random.seed(42)
    normal_scores = np.random.uniform(0.0, 0.3, 90)
    anomaly_scores = np.random.uniform(0.7, 1.0, 10)
    scores = np.concatenate([normal_scores, anomaly_scores])

    result = compute_pot_threshold(scores, q=1e-4, initial_percentile=90.0)

    # Check result structure
    assert result.threshold > 0
    assert result.initial_threshold > 0
    assert result.n_total == 100
    assert result.n_peaks > 0
    assert len(result.y_pred_raw) == 100


def test_compute_pot_threshold_no_peaks():
    """Test POT when no peaks above threshold."""
    # All scores below threshold
    scores = np.random.uniform(0.0, 0.5, 100)

    result = compute_pot_threshold(scores, q=1e-4, initial_percentile=99.0)

    # Should fallback to percentile threshold
    assert result.n_peaks == 0
    assert result.threshold == result.initial_threshold
    assert result.gpd_shape == 0.0
    assert result.gpd_scale == 0.0


def test_compute_pot_threshold_invalid_q():
    """Test invalid q parameter."""
    scores = np.array([0.1, 0.2, 0.3])

    with pytest.raises(ValueError, match="q must be in"):
        compute_pot_threshold(scores, q=0.0)

    with pytest.raises(ValueError, match="q must be in"):
        compute_pot_threshold(scores, q=1.0)


def test_apply_threshold():
    """Test applying threshold to scores."""
    scores = np.array([0.1, 0.3, 0.5, 0.7, 0.9])
    threshold = 0.5

    y_pred = apply_threshold(scores, threshold)

    assert len(y_pred) == 5
    assert y_pred[0] == 0
    assert y_pred[1] == 0
    assert y_pred[2] == 0  # Equal to threshold
    assert y_pred[3] == 1
    assert y_pred[4] == 1


def test_compute_pot_threshold_exponential_case():
    """Test POT with exponential distribution (xi ≈ 0)."""
    # Generate exponential-like peaks
    np.random.seed(42)
    scores = np.random.exponential(scale=0.3, size=100)

    result = compute_pot_threshold(scores, q=1e-3, initial_percentile=80.0)

    # Should complete without errors
    assert result.threshold > 0
    assert result.n_peaks > 0
