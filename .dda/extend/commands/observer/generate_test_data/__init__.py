# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
from __future__ import annotations

import sys
from pathlib import Path
from typing import TYPE_CHECKING

import click
from dda.cli.base import dynamic_command, pass_app

if TYPE_CHECKING:
    from dda.cli.application import Application


@dynamic_command(
    short_help="Generate synthetic test data for evaluation framework",
    dependencies=[
        "pandas>=2.0.0",
        "numpy>=1.24.0",
        "pyarrow>=14.0.0",
    ],
)
@click.option("--output-dir", type=click.Path(), help="Output directory for test fixtures")
@pass_app
def cmd(app: Application, output_dir: str | None) -> None:
    """
    Generate synthetic test data for evaluation framework.

    Creates five test scenarios:
    - simple_incident: Single incident with clear signal (good detector)
    - multi_metric: Multiple correlated metrics
    - no_incident: Baseline without anomalies
    - multi_incident: Two separate incidents
    - bad_detector: Inverted scores (poor detector - high scores when normal, low during incident)

    Example:

        dda observer generate-test-data
        dda observer generate-test-data --output-dir /tmp/test_data
    """
    # Import here so dependencies are only needed when command runs
    sys.path.insert(0, str(Path(__file__).parent.parent / "eval" / "tests"))

    from generate_synthetic import (
        generate_bad_detector,
        generate_multi_incident,
        generate_multi_metric,
        generate_no_incident,
        generate_simple_incident,
    )

    # Set default output directory
    if output_dir is None:
        output_dir_path = Path(__file__).parent.parent / "eval" / "tests" / "fixtures"
    else:
        output_dir_path = Path(output_dir)

    # Create output directory
    output_dir_path.mkdir(parents=True, exist_ok=True)
    app.display_info(f"Output directory: {output_dir_path}\n")

    # Generate all scenarios
    generate_simple_incident(output_dir_path)
    app.display()
    generate_multi_metric(output_dir_path)
    app.display()
    generate_no_incident(output_dir_path)
    app.display()
    generate_multi_incident(output_dir_path)
    app.display()
    generate_bad_detector(output_dir_path)
    app.display()

    app.display_success("All synthetic test data generated successfully!")
    app.display_info(f"\nTest files created in: {output_dir_path}")
    app.display_info("\nRun evaluation with:")
    app.display("  dda observer eval \\")
    app.display(f"    --raw-metrics {output_dir_path}/simple_incident_metrics.parquet \\")
    app.display(f"    --findings {output_dir_path}/simple_incident_findings.parquet \\")
    app.display("    --verbose")
