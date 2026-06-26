# SPDX-FileCopyrightText: 2025-present Datadog, Inc. <dev@datadoghq.com>
#
# SPDX-License-Identifier: MIT
"""
Fakeintake client management and query utilities.

Handles automatic client building and metric querying.
"""

from __future__ import annotations

import json
import os
import subprocess
from pathlib import Path
from typing import TYPE_CHECKING

if TYPE_CHECKING:
    from dda.cli.application import Application

# Configuration constants
DEFAULT_DISPLAY_LIMIT = 5


def _get_project_root() -> Path:
    """Get the datadog-agent project root directory."""
    # This file is at .dda/extend/pythonpath/lab/fakeintake_client.py
    # Project root is 4 levels up
    return Path(__file__).parents[4]


def _get_client_path() -> Path:
    """Get the fakeintakectl client path."""
    # Allow override via environment variable
    env_path = os.getenv("FAKEINTAKE_CLIENT_PATH")
    if env_path:
        return Path(env_path)
    return _get_project_root() / "test/fakeintake/build/fakeintakectl"


FAKEINTAKE_CLIENT_PATH = _get_client_path()


def ensure_client_built(app: Application) -> Path:
    """
    Ensure fakeintake client is built and return its path.

    Returns:
        Path to the fakeintakectl binary

    Raises:
        RuntimeError: If build fails
    """
    if not FAKEINTAKE_CLIENT_PATH.exists():
        app.display_info("Building fakeintake client CLI...")
        try:
            app.subprocess.run(["dda", "inv", "fakeintake.build"], check=True)
            app.display_success("Fakeintake client built")
        except subprocess.CalledProcessError as e:
            raise RuntimeError(f"Failed to build fakeintake client: {e}") from e

    return FAKEINTAKE_CLIENT_PATH


def query_metrics(
    fakeintake_url: str,
    metric_name: str | None = None,
    list_metrics: bool = False,
    output_format: str = "pretty",
    display_limit: int = DEFAULT_DISPLAY_LIMIT,
) -> str:
    """
    Query fakeintake for metrics using the client.

    Args:
        fakeintake_url: Fakeintake URL (e.g., http://localhost:8080)
        metric_name: Specific metric to query
        list_metrics: If True, list all available metrics
        output_format: Output format ("pretty", "json")
        display_limit: Maximum number of entries to display in pretty format

    Returns:
        Formatted output

    Raises:
        RuntimeError: If client is not built
        ValueError: If neither metric_name nor list_metrics is specified
    """
    client_path = FAKEINTAKE_CLIENT_PATH
    if not client_path.exists():
        raise RuntimeError(
            f"Fakeintake client not found at {client_path}. " "Run 'dda inv fakeintake.build' to build it."
        )

    if list_metrics:
        # List all metric names
        result = subprocess.run(
            [str(client_path), "--url", fakeintake_url, "get", "metric", "names"],
            capture_output=True,
            text=True,
            check=True,
        )
        return result.stdout

    if metric_name:
        # Query specific metric
        result = subprocess.run(
            [str(client_path), "--url", fakeintake_url, "filter", "metrics", "--name", metric_name],
            capture_output=True,
            text=True,
            check=True,
        )

        if output_format == "json":
            return result.stdout

        # Pretty format
        try:
            data = json.loads(result.stdout)
            if not isinstance(data, list):
                return f"Unexpected response format: {type(data).__name__}"

            if not data:
                return f"No data found for metric: {metric_name}"

            output = [f"\n📊 Metric: {metric_name}"]
            output.append("=" * 60)

            for entry in data[:display_limit]:
                resources = entry.get("resources", [])
                points = entry.get("points", [])

                if resources:
                    host = resources[0].get("name", "unknown")
                    output.append(f"\n🖥️  Host: {host}")

                if points:
                    point = points[0]
                    value = point.get("value", "N/A")
                    timestamp = point.get("timestamp", "N/A")
                    output.append(f"   Value: {value}")
                    output.append(f"   Timestamp: {timestamp}")

            if len(data) > display_limit:
                output.append(f"\n... and {len(data) - display_limit} more entries")

            return "\n".join(output)

        except json.JSONDecodeError as e:
            return f"Failed to parse fakeintake response: {e}\n\nRaw output:\n{result.stdout}"

    raise ValueError("Must specify either metric_name or list_metrics=True")
