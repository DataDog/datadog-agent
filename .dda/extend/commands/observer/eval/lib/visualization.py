"""Visualization and output formatting."""

import matplotlib

matplotlib.use('Agg')  # Non-interactive backend
import matplotlib.pyplot as plt
import numpy as np
from data_loader import EvalDataset
from metrics import MetricsResult
from pot_thresholding import POTResult


def generate_summary_json(metrics: MetricsResult) -> dict:
    """
    Generate JSON summary of metrics.

    Args:
        metrics: MetricsResult object

    Returns:
        Dictionary suitable for JSON serialization
    """
    return {
        "UCR_Score": metrics.ucr_score,
        "Adjusted_F1": round(metrics.adjusted_f1, 4),
        "AUC_ROC": round(metrics.auc_roc, 4) if not np.isnan(metrics.auc_roc) else None,
        "Computed_Threshold": round(metrics.computed_threshold, 6),
        "Total_Anomalies_Found": metrics.total_anomalies_found,
        "Precision": round(metrics.precision, 4),
        "Recall": round(metrics.recall, 4),
        "True_Positives": metrics.true_positives,
        "False_Positives": metrics.false_positives,
        "False_Negatives": metrics.false_negatives,
        "Max_Score_Timestamp": metrics.max_score_timestamp,
        "Max_Score_In_Window": metrics.max_score_in_window,
    }


def print_summary(metrics: MetricsResult, verbose: bool = False):
    """
    Print human-readable summary to console.

    Args:
        metrics: MetricsResult object
        verbose: Whether to print detailed information
    """
    print("\nEvaluation Results:")
    print(f"  UCR Score:        {metrics.ucr_score}")
    print(f"  Adjusted F1:      {metrics.adjusted_f1:.4f}")
    if not np.isnan(metrics.auc_roc):
        print(f"  AUC-ROC:          {metrics.auc_roc:.4f}")
    else:
        print("  AUC-ROC:          N/A")
    print(f"  Threshold:        {metrics.computed_threshold:.6f}")
    print(f"  Anomalies Found:  {metrics.total_anomalies_found}")

    if verbose:
        print("\nDetailed Metrics:")
        print(f"  Precision:        {metrics.precision:.4f}")
        print(f"  Recall:           {metrics.recall:.4f}")
        print(f"  True Positives:   {metrics.true_positives}")
        print(f"  False Positives:  {metrics.false_positives}")
        print(f"  False Negatives:  {metrics.false_negatives}")
        print(f"  Max Score At:     t={metrics.max_score_timestamp}")
        print(f"  Max In Window:    {metrics.max_score_in_window}")


def generate_plot(dataset: EvalDataset, pot_result: POTResult, metrics: MetricsResult, output_path: str):
    """
    Generate 3-panel visualization plot.

    Panel 1: Raw metric values with ground truth windows
    Panel 2: Anomaly scores with threshold and predictions
    Panel 3: Binary comparison (y_true vs y_pred_adjusted)

    Args:
        dataset: Evaluation dataset
        pot_result: POT result
        metrics: Metrics result
        output_path: Path to save plot
    """
    # Extract data
    timestamps = dataset.timestamps
    values = dataset.values
    scores = dataset.anomaly_scores
    y_true = dataset.y_true
    y_pred_raw = pot_result.y_pred_raw
    windows = dataset.windows

    # Apply point adjustment for visualization
    from metrics import apply_point_adjustment

    y_pred_adjusted = apply_point_adjustment(y_true, y_pred_raw, windows)

    # Create figure with 3 subplots
    fig, axes = plt.subplots(3, 1, figsize=(14, 10), sharex=True)

    # Panel 1: Raw Metric Values
    ax1 = axes[0]
    ax1.plot(timestamps, values, 'b-', linewidth=0.8, label=dataset.metric_name)

    # Shade ground truth windows
    for window in windows:
        if window.start_index >= 0 and window.end_index > window.start_index:
            ax1.axvspan(
                timestamps[window.start_index],
                timestamps[window.end_index - 1],
                alpha=0.2,
                color='green',
                label='Ground Truth' if window == windows[0] else "",
            )

    ax1.set_ylabel('Metric Value')
    ax1.set_title(f'Raw Metric: {dataset.metric_name}')
    ax1.legend(loc='upper right')
    ax1.grid(True, alpha=0.3)

    # Panel 2: Anomaly Scores
    ax2 = axes[1]
    ax2.plot(timestamps, scores, 'b-', linewidth=0.8, label='Anomaly Score', alpha=0.7)

    # Draw threshold line
    ax2.axhline(
        pot_result.threshold,
        color='red',
        linestyle='--',
        linewidth=1.5,
        label=f'Threshold ({pot_result.threshold:.4f})',
    )

    # Shade ground truth windows
    for window in windows:
        if window.start_index >= 0 and window.end_index > window.start_index:
            ax2.axvspan(timestamps[window.start_index], timestamps[window.end_index - 1], alpha=0.2, color='green')

    # Mark predictions (raw, before adjustment)
    pred_indices = np.where(y_pred_raw == 1)[0]
    if len(pred_indices) > 0:
        ax2.scatter(
            timestamps[pred_indices], scores[pred_indices], color='red', s=20, alpha=0.6, label='Predictions', zorder=5
        )

    # Mark max score
    max_idx = np.argmax(scores)
    ax2.scatter(
        timestamps[max_idx],
        scores[max_idx],
        color='orange',
        s=100,
        marker='*',
        label='Max Score',
        zorder=10,
        edgecolors='black',
        linewidths=0.5,
    )

    ax2.set_ylabel('Anomaly Score')
    ax2.set_title('Anomaly Scores with POT Threshold')
    ax2.legend(loc='upper right')
    ax2.grid(True, alpha=0.3)

    # Panel 3: Binary Comparison (Heatmap)
    ax3 = axes[2]

    # Create 2D array for heatmap: [y_true row, y_pred_adjusted row]
    comparison = np.vstack([y_true, y_pred_adjusted])

    # Plot heatmap
    im = ax3.imshow(
        comparison,
        aspect='auto',
        cmap='RdYlGn',
        interpolation='nearest',
        extent=[timestamps[0], timestamps[-1], -0.5, 1.5],
    )

    ax3.set_yticks([0, 1])
    ax3.set_yticklabels(['Ground Truth', 'Predictions'])
    ax3.set_xlabel('Timestamp')
    ax3.set_title('Binary Comparison (after point-adjustment)')

    # Add colorbar
    cbar = plt.colorbar(im, ax=ax3, orientation='horizontal', pad=0.1)
    cbar.set_label('Anomaly (1 = Yes, 0 = No)')

    # Add metrics text
    auc_text = f"{metrics.auc_roc:.3f}" if not np.isnan(metrics.auc_roc) else "N/A"

    metrics_text = (
        f"UCR: {metrics.ucr_score:.2f} | "
        f"F1: {metrics.adjusted_f1:.3f} | "
        f"AUC: {auc_text} | "
        f"P: {metrics.precision:.3f} | "
        f"R: {metrics.recall:.3f}"
    )
    fig.text(
        0.5,
        0.02,
        metrics_text,
        ha='center',
        fontsize=10,
        bbox={'boxstyle': 'round', 'facecolor': 'wheat', 'alpha': 0.5},
    )

    plt.tight_layout(rect=[0, 0.03, 1, 1])
    plt.savefig(output_path, dpi=150, bbox_inches='tight')
    plt.close(fig)
