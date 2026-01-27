# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

import json
import sys
from pathlib import Path
from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Evaluate observer anomaly detection performance",
    dependencies=[
        "pandas>=2.0.0",
        "numpy>=1.24.0",
        "scipy>=1.10.0",
        "scikit-learn>=1.3.0",
        "pyarrow>=14.0.0",
        "matplotlib>=3.7.0",
    ],
)
@click.option("--raw-metrics", required=True, type=click.Path(exists=True), help="Path to raw_metrics.parquet file")
@click.option("--findings", required=True, type=click.Path(exists=True), help="Path to findings.parquet file")
@click.option("--metric-name", default="heap.used_mb", help="Metric name to evaluate (default: heap.used_mb)")
@click.option("--output", type=click.Path(), help="Output JSON file path (default: stdout)")
@click.option("--plot", type=click.Path(), help="Save visualization plot to this path")
@click.option("--q", default=1e-4, type=float, help="POT probability parameter (default: 1e-4)")
@click.option("--initial-percentile", default=98.0, type=float, help="Initial threshold percentile (default: 98.0)")
@click.option("--verbose", is_flag=True, help="Print debug information")
@pass_app
def cmd(
    app: Application,
    raw_metrics: str,
    findings: str,
    metric_name: str,
    output: str | None,
    plot: str | None,
    q: float,
    initial_percentile: float,
    verbose: bool,
) -> None:
    """
    Evaluate observer anomaly detection performance.

    Loads parquet data, applies POT thresholding, and calculates metrics.

    Example:

        dda observer eval --raw-metrics data/metrics.parquet --findings data/findings.parquet --plot results.png
    """
    # Import here so dependencies are only needed when command runs
    # Add lib directory to path for imports
    lib_path = Path(__file__).parent / "lib"
    if str(lib_path) not in sys.path:
        sys.path.insert(0, str(lib_path))

    from data_loader import load_and_prepare_data
    from metrics import calculate_metrics
    from pot_thresholding import compute_pot_threshold
    from visualization import generate_plot, generate_summary_json, print_summary

    try:
        # Phase 1: Load and prepare data
        if verbose:
            app.display_info("Loading data from:")
            app.display_info(f"  Raw metrics: {raw_metrics}")
            app.display_info(f"  Findings: {findings}")
            app.display_info(f"  Evaluating metric: {metric_name}")

        dataset = load_and_prepare_data(raw_metrics, findings, metric_name, verbose=verbose)

        if verbose:
            app.display_info("\nDataset loaded:")
            app.display_info(f"  Total timestamps: {len(dataset.timestamps)}")
            app.display_info(f"  Ground truth windows: {len(dataset.windows)}")
            app.display_info(f"  Anomalous points: {dataset.y_true.sum()}")

        # Phase 2: Apply POT thresholding
        if verbose:
            app.display_info(f"\nApplying POT thresholding (q={q}, percentile={initial_percentile})")

        pot_result = compute_pot_threshold(
            dataset.anomaly_scores, q=q, initial_percentile=initial_percentile, verbose=verbose
        )

        if verbose:
            app.display_info(f"  Computed threshold: {pot_result.threshold:.6f}")
            app.display_info(f"  Total predictions: {pot_result.y_pred_raw.sum()}")

        # Phase 3: Calculate metrics
        if verbose:
            app.display_info("\nCalculating metrics...")

        metrics = calculate_metrics(dataset, pot_result)

        # Phase 4: Output results
        summary = generate_summary_json(metrics)

        if output:
            output_path = Path(output)
            output_path.write_text(json.dumps(summary, indent=2))
            if verbose:
                app.display_success(f"\nResults written to: {output}")
        else:
            app.display(json.dumps(summary, indent=2))

        if plot:
            if verbose:
                app.display_info(f"\nGenerating plot: {plot}")
            generate_plot(dataset, pot_result, metrics, plot)
            if verbose:
                app.display_success(f"Plot saved to: {plot}")

        if verbose:
            app.display("\n" + "=" * 60)
            print_summary(metrics, verbose=True)
            app.display("=" * 60)

    except Exception as e:
        app.display_error(f"Error: {e}")
        if verbose:
            import traceback

            traceback.print_exc()
        sys.exit(1)
