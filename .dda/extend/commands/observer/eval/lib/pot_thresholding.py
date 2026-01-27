"""Peaks-Over-Threshold (POT) thresholding using Extreme Value Theory."""

import logging
from dataclasses import dataclass

import numpy as np
from scipy import stats


@dataclass
class POTResult:
    """Result of POT threshold computation."""

    threshold: float  # final POT threshold
    initial_threshold: float  # percentile threshold
    q: float  # probability parameter
    n_total: int  # total data points
    n_peaks: int  # peaks used for fitting
    gpd_shape: float  # xi parameter (GPD shape)
    gpd_scale: float  # sigma parameter (GPD scale)
    y_pred_raw: np.ndarray  # binary predictions (0/1)


def compute_initial_threshold(scores: np.ndarray, percentile: float) -> float:
    """
    Compute initial threshold from percentile.

    Args:
        scores: Anomaly scores
        percentile: Percentile value (0-100)

    Returns:
        Threshold value
    """
    if percentile < 0 or percentile > 100:
        raise ValueError(f"Percentile must be in [0, 100], got {percentile}")

    return float(np.percentile(scores, percentile))


def fit_gpd(peaks: np.ndarray) -> tuple[float, float]:
    """
    Fit Generalized Pareto Distribution to peaks.

    Args:
        peaks: Exceedances over initial threshold (peaks - u)

    Returns:
        (shape, scale) parameters (xi, sigma)

    Raises:
        ValueError: If fitting fails
    """
    if len(peaks) == 0:
        raise ValueError("No peaks provided for GPD fitting")

    try:
        # Fit GPD using scipy
        # genpareto parameters: (c, loc, scale) where c is shape (xi)
        shape, loc, scale = stats.genpareto.fit(peaks, floc=0)
        return float(shape), float(scale)

    except Exception as e:
        raise ValueError(f"GPD fitting failed: {e}") from e


def compute_pot_threshold(
    scores: np.ndarray, q: float = 1e-4, initial_percentile: float = 98.0, verbose: bool = False
) -> POTResult:
    """
    Compute POT threshold using Extreme Value Theory.

    Implements Equation 2 from the plan:
        th = u + (sigma/xi) * ((n*q/n_u)^(-xi) - 1)

    Args:
        scores: Anomaly scores
        q: Probability parameter (default: 1e-4)
        initial_percentile: Initial threshold percentile (default: 98.0)
        verbose: Whether to print debug information

    Returns:
        POTResult with threshold and predictions

    Raises:
        ValueError: If parameters are invalid
    """
    if q <= 0 or q >= 1:
        raise ValueError(f"q must be in (0, 1), got {q}")

    n_total = len(scores)
    if n_total == 0:
        raise ValueError("Empty scores array")

    # Step 1: Compute initial threshold
    initial_threshold = compute_initial_threshold(scores, initial_percentile)

    if verbose:
        logging.info("Initial threshold (%sth percentile): %.6f", initial_percentile, initial_threshold)

    # Step 2: Extract peaks (exceedances over threshold)
    peaks_mask = scores > initial_threshold
    n_peaks = np.sum(peaks_mask)

    if n_peaks == 0:
        logging.warning("No peaks above initial threshold - using percentile threshold")
        y_pred = (scores > initial_threshold).astype(int)
        return POTResult(
            threshold=initial_threshold,
            initial_threshold=initial_threshold,
            q=q,
            n_total=n_total,
            n_peaks=0,
            gpd_shape=0.0,
            gpd_scale=0.0,
            y_pred_raw=y_pred,
        )

    # Extract exceedances (peaks - u)
    exceedances = scores[peaks_mask] - initial_threshold

    if verbose:
        logging.info("Peaks above threshold: %d/%d (%.2f%%)", n_peaks, n_total, 100 * n_peaks / n_total)

    # Step 3: Fit GPD to exceedances
    try:
        shape, scale = fit_gpd(exceedances)

        if verbose:
            logging.info("GPD parameters: shape (xi) = %.6f, scale (sigma) = %.6f", shape, scale)

    except ValueError as e:
        logging.warning("GPD fitting failed: %s - using percentile threshold", e)
        y_pred = (scores > initial_threshold).astype(int)
        return POTResult(
            threshold=initial_threshold,
            initial_threshold=initial_threshold,
            q=q,
            n_total=n_total,
            n_peaks=n_peaks,
            gpd_shape=0.0,
            gpd_scale=0.0,
            y_pred_raw=y_pred,
        )

    # Step 4: Compute POT threshold using Equation 2
    # th = u + (sigma/xi) * ((n*q/n_u)^(-xi) - 1)

    if abs(shape) < 1e-6:  # exponential case (xi ≈ 0)
        # Use exponential distribution limit
        threshold = initial_threshold - scale * np.log(n_total * q / n_peaks)
        if verbose:
            logging.info("Using exponential distribution (xi ≈ 0)")

    else:
        # General GPD case
        inner_term = (n_total * q / n_peaks) ** (-shape)
        threshold = initial_threshold + (scale / shape) * (inner_term - 1)

    # Ensure threshold is at least the initial threshold
    threshold = max(threshold, initial_threshold)

    if verbose:
        logging.info("POT threshold: %.6f", threshold)

    # Step 5: Apply threshold to get predictions
    y_pred = (scores > threshold).astype(int)

    return POTResult(
        threshold=float(threshold),
        initial_threshold=float(initial_threshold),
        q=q,
        n_total=n_total,
        n_peaks=n_peaks,
        gpd_shape=shape,
        gpd_scale=scale,
        y_pred_raw=y_pred,
    )


def apply_threshold(scores: np.ndarray, threshold: float) -> np.ndarray:
    """
    Apply threshold to scores to get binary predictions.

    Args:
        scores: Anomaly scores
        threshold: Threshold value

    Returns:
        Binary predictions (0/1)
    """
    return (scores > threshold).astype(int)
