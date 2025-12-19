#!/usr/bin/env -S uv run
# /// script
# requires-python = ">=3.10"
# dependencies = [
#     "pyarrow>=15.0.0",
#     "pandas>=2.0.0",
#     "dash>=2.14.0",
#     "plotly>=5.18.0",
# ]
# ///
"""
Interactive metrics viewer for fine-grained-monitor Parquet data.

REQ-FM-005: Visualize Metrics Interactively

Provides a web-based interactive timeseries viewer with:
- Container filtering via dropdown
- High-resolution CPU usage display at full sampling rate
- Pan/zoom interactions for exploring time ranges
- Multi-container overlay for pattern comparison

Usage:
    uv run scripts/metrics_viewer.py metrics.parquet
    uv run scripts/metrics_viewer.py metrics.parquet --port 8051
"""

from __future__ import annotations

import argparse
import sys
import webbrowser
from pathlib import Path
from threading import Timer

import pandas as pd
import plotly.graph_objects as go
import pyarrow.parquet as pq
from dash import Dash, Input, Output, callback, dcc, html


def extract_label(labels: list[tuple], key: str) -> str | None:
    """Extract a label value from the labels list."""
    if labels is None:
        return None
    for k, v in labels:
        if k == key:
            return v
    return None


def load_cpu_data(filepath: Path) -> pd.DataFrame:
    """Load Parquet file and compute CPU usage percentages."""
    print(f"Loading {filepath}...")

    # Read only the columns we need
    table = pq.read_table(
        filepath,
        columns=["metric_name", "time", "value_int", "value_float", "labels"],
    )
    df = table.to_pandas()
    print(f"Loaded {len(df):,} total rows")

    # Filter to CPU usage metric only
    cpu_metric = "cgroup.v2.cpu.stat.usage_usec"
    df = df[df["metric_name"] == cpu_metric].copy()
    print(f"Filtered to {len(df):,} CPU metric rows")

    if df.empty:
        print("Error: No CPU metrics found in data")
        sys.exit(1)

    # Extract labels
    df["container_id"] = df["labels"].apply(lambda x: extract_label(x, "container_id"))
    df["qos_class"] = df["labels"].apply(lambda x: extract_label(x, "qos_class"))

    # Drop rows without container_id
    df = df.dropna(subset=["container_id"])

    # Create short container ID for display
    df["container_short"] = df["container_id"].apply(lambda x: x[:12] if x else "unknown")

    # Combine value columns
    df["value"] = df["value_float"].combine_first(df["value_int"])

    # Sort by container and time for delta computation
    df = df.sort_values(["container_id", "time"])

    # Compute deltas
    df["value_delta"] = df.groupby("container_id")["value"].diff()
    df["time_delta_s"] = df.groupby("container_id")["time"].diff().dt.total_seconds()

    # Filter out invalid deltas
    df = df.dropna(subset=["value_delta", "time_delta_s"])
    df = df[(df["value_delta"] >= 0) & (df["time_delta_s"] > 0)]

    # Compute CPU percentage (usec/sec / 10000 = % of one core)
    df["cpu_percent"] = (df["value_delta"] / df["time_delta_s"]) / 10000

    print(f"Computed {len(df):,} CPU percentage samples")

    return df[["time", "container_id", "container_short", "qos_class", "cpu_percent"]]


def get_top_containers(df: pd.DataFrame, n: int = 20) -> list[str]:
    """Get top N containers by average CPU usage."""
    avg_cpu = df.groupby("container_short")["cpu_percent"].mean()
    return avg_cpu.nlargest(n).index.tolist()


def create_app(df: pd.DataFrame) -> Dash:
    """Create the Dash application."""
    app = Dash(__name__)

    # Get all unique containers and top containers
    all_containers = sorted(df["container_short"].unique())
    top_containers = get_top_containers(df, 10)

    # Pre-compute time range
    time_min = df["time"].min()
    time_max = df["time"].max()

    app.layout = html.Div(
        [
            html.H1(
                "Container CPU Usage Viewer",
                style={"textAlign": "center", "marginBottom": "20px"},
            ),
            html.Div(
                [
                    html.Div(
                        [
                            html.Label("Select Containers:", style={"fontWeight": "bold"}),
                            dcc.Dropdown(
                                id="container-dropdown",
                                options=[{"label": c, "value": c} for c in all_containers],
                                value=top_containers[:5],  # Start with top 5
                                multi=True,
                                placeholder="Select containers to display...",
                                style={"width": "100%"},
                            ),
                        ],
                        style={"width": "60%", "display": "inline-block", "paddingRight": "20px"},
                    ),
                    html.Div(
                        [
                            html.Label("Quick Select:", style={"fontWeight": "bold"}),
                            html.Button("Top 5", id="btn-top5", n_clicks=0, style={"marginRight": "10px"}),
                            html.Button("Top 10", id="btn-top10", n_clicks=0, style={"marginRight": "10px"}),
                            html.Button("Clear", id="btn-clear", n_clicks=0),
                        ],
                        style={"width": "35%", "display": "inline-block", "verticalAlign": "top"},
                    ),
                ],
                style={"marginBottom": "20px", "padding": "10px"},
            ),
            html.Div(
                [
                    html.Span(f"Time range: {time_min} to {time_max}", style={"marginRight": "20px"}),
                    html.Span(f"Containers: {len(all_containers)}", style={"marginRight": "20px"}),
                    html.Span(f"Samples: {len(df):,}"),
                ],
                style={"marginBottom": "10px", "color": "#666"},
            ),
            dcc.Graph(
                id="cpu-graph",
                style={"height": "70vh"},
                config={
                    "scrollZoom": True,
                    "displayModeBar": True,
                    "modeBarButtonsToAdd": ["drawline", "eraseshape"],
                },
            ),
            # Store the data for callbacks
            dcc.Store(id="data-store", data={"top5": top_containers[:5], "top10": top_containers[:10]}),
        ],
        style={"padding": "20px", "fontFamily": "sans-serif"},
    )

    @callback(
        Output("container-dropdown", "value"),
        Input("btn-top5", "n_clicks"),
        Input("btn-top10", "n_clicks"),
        Input("btn-clear", "n_clicks"),
        Input("data-store", "data"),
        prevent_initial_call=True,
    )
    def update_selection(n_top5: int, n_top10: int, n_clear: int, data: dict) -> list[str]:
        """Handle quick select buttons."""
        from dash import ctx

        if ctx.triggered_id == "btn-top5":
            return data["top5"]
        elif ctx.triggered_id == "btn-top10":
            return data["top10"]
        elif ctx.triggered_id == "btn-clear":
            return []
        return data["top5"]

    @callback(Output("cpu-graph", "figure"), Input("container-dropdown", "value"))
    def update_graph(selected_containers: list[str]) -> go.Figure:
        """Update the graph based on selected containers."""
        fig = go.Figure()

        if not selected_containers:
            fig.add_annotation(
                text="Select containers from the dropdown above",
                xref="paper",
                yref="paper",
                x=0.5,
                y=0.5,
                showarrow=False,
                font={"size": 20},
            )
            fig.update_layout(
                title="CPU Usage Over Time",
                xaxis_title="Time",
                yaxis_title="CPU Usage (%)",
            )
            return fig

        # Filter data for selected containers
        mask = df["container_short"].isin(selected_containers)
        plot_df = df[mask]

        # Add a trace for each container using scattergl for performance
        for container in selected_containers:
            container_data = plot_df[plot_df["container_short"] == container]
            if container_data.empty:
                continue

            qos = container_data["qos_class"].iloc[0] if not container_data["qos_class"].isna().all() else "unknown"

            fig.add_trace(
                go.Scattergl(
                    x=container_data["time"],
                    y=container_data["cpu_percent"],
                    mode="lines",
                    name=f"{container} ({qos})",
                    line={"width": 1},
                    hovertemplate="%{y:.2f}%<br>%{x}<extra>%{fullData.name}</extra>",
                )
            )

        fig.update_layout(
            title="CPU Usage Over Time (Pan/Zoom to explore, Double-click to reset)",
            xaxis_title="Time",
            yaxis_title="CPU Usage (%)",
            hovermode="x unified",
            legend={"orientation": "h", "yanchor": "bottom", "y": 1.02, "xanchor": "right", "x": 1},
            margin={"l": 60, "r": 20, "t": 80, "b": 60},
        )

        # Enable range slider for easy navigation
        fig.update_xaxes(rangeslider_visible=True, rangeslider_thickness=0.05)

        return fig

    return app


def open_browser(port: int) -> None:
    """Open the browser after a short delay."""
    webbrowser.open_new(f"http://127.0.0.1:{port}/")


def main() -> None:
    parser = argparse.ArgumentParser(description="Interactive CPU metrics viewer for fine-grained-monitor data")
    parser.add_argument("input", type=Path, help="Input Parquet file")
    parser.add_argument(
        "-p",
        "--port",
        type=int,
        default=8050,
        help="Port for the web server (default: 8050)",
    )
    parser.add_argument(
        "--no-browser",
        action="store_true",
        help="Don't automatically open the browser",
    )
    parser.add_argument(
        "--debug",
        action="store_true",
        help="Run in debug mode with auto-reload",
    )

    args = parser.parse_args()

    if not args.input.exists():
        print(f"Error: File not found: {args.input}", file=sys.stderr)
        sys.exit(1)

    # Load and prepare data
    df = load_cpu_data(args.input)

    # Create app
    app = create_app(df)

    # Open browser after server starts
    if not args.no_browser:
        Timer(1.5, open_browser, args=[args.port]).start()

    print(f"\nStarting server at http://127.0.0.1:{args.port}/")
    print("Press Ctrl+C to stop\n")

    app.run(debug=args.debug, port=args.port)


if __name__ == "__main__":
    main()
