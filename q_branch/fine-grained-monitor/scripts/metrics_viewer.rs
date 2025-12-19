#!/usr/bin/env -S cargo +nightly -Zscript

---
[dependencies]
parquet = { version = "54", features = ["arrow"] }
arrow = "54"
axum = "0.7"
tokio = { version = "1", features = ["full"] }
serde = { version = "1", features = ["derive"] }
serde_json = "1"
anyhow = "1"
clap = { version = "4", features = ["derive"] }
tower-http = { version = "0.5", features = ["cors"] }

[profile.dev]
opt-level = 3
---

//! Rust-native metrics viewer with web frontend.
//!
//! Fast Parquet loading with axum HTTP server serving Plotly.js frontend.
//!
//! Usage:
//!     ./metrics_viewer.rs metrics.parquet
//!     ./metrics_viewer.rs metrics.parquet --port 8080

use anyhow::{Context, Result};
use arrow::array::{Array, Float64Array, MapArray, StringArray, StructArray, UInt64Array};
use arrow::datatypes::DataType;
use axum::{
    extract::{Query, State},
    response::{Html, IntoResponse, Json},
    routing::get,
    Router,
};
use clap::Parser;
use parquet::arrow::arrow_reader::ParquetRecordBatchReaderBuilder;
use serde::{Deserialize, Serialize};
use std::collections::HashMap;
use std::fs::File;
use std::net::SocketAddr;
use std::path::PathBuf;
use std::sync::Arc;
use tower_http::cors::CorsLayer;

#[derive(Parser, Debug)]
#[command(name = "metrics_viewer")]
#[command(about = "Interactive metrics viewer with Rust backend")]
struct Args {
    /// Input parquet file
    input: PathBuf,

    /// Port for web server
    #[arg(short, long, default_value = "8050")]
    port: u16,

    /// Don't open browser automatically
    #[arg(long)]
    no_browser: bool,
}

#[derive(Debug, Clone, Serialize)]
struct ContainerInfo {
    id: String,
    short_id: String,
    qos_class: Option<String>,
    sample_count: usize,
    avg_cpu: f64,
    max_cpu: f64,
}

#[derive(Debug, Clone, Serialize)]
struct TimeseriesPoint {
    time_ms: i64,
    cpu_percent: f64,
}

#[derive(Debug, Clone)]
struct ContainerData {
    info: ContainerInfo,
    timeseries: Vec<TimeseriesPoint>,
}

struct AppState {
    containers: HashMap<String, ContainerData>,
    container_list: Vec<ContainerInfo>,
}

fn extract_label(labels: &[(String, String)], key: &str) -> Option<String> {
    labels
        .iter()
        .find(|(k, _)| k == key)
        .map(|(_, v)| v.clone())
}

fn extract_labels_from_column(col: &dyn Array, row: usize) -> Result<Vec<(String, String)>> {
    let map_array = col
        .as_any()
        .downcast_ref::<MapArray>()
        .context("Labels column is not a MapArray")?;

    if map_array.is_null(row) {
        return Ok(vec![]);
    }

    let start = map_array.value_offsets()[row] as usize;
    let end = map_array.value_offsets()[row + 1] as usize;

    let entries = map_array.entries();
    let struct_array = entries
        .as_any()
        .downcast_ref::<StructArray>()
        .context("Map entries is not a StructArray")?;

    let keys = struct_array
        .column(0)
        .as_any()
        .downcast_ref::<StringArray>()
        .context("Missing key column in labels")?;

    let vals = struct_array
        .column(1)
        .as_any()
        .downcast_ref::<StringArray>()
        .context("Missing value column in labels")?;

    let mut result = Vec::with_capacity(end - start);
    for i in start..end {
        if !keys.is_null(i) && !vals.is_null(i) {
            result.push((keys.value(i).to_string(), vals.value(i).to_string()));
        }
    }

    Ok(result)
}

fn load_data(path: &PathBuf) -> Result<AppState> {
    eprintln!("Loading {:?}...", path);

    let file = File::open(path).context("Failed to open file")?;
    let builder = ParquetRecordBatchReaderBuilder::try_new(file)?;

    let schema = builder.schema();
    let parquet_schema = builder.parquet_schema();
    let projection: Vec<usize> = ["metric_name", "time", "value_int", "value_float", "labels"]
        .iter()
        .filter_map(|name| schema.index_of(name).ok())
        .collect();

    let projection_mask =
        parquet::arrow::ProjectionMask::roots(&parquet_schema, projection);

    let reader = builder
        .with_projection(projection_mask)
        .with_batch_size(65536)
        .build()?;

    // Collect raw data: container_id -> (timestamps, values, qos_class, last_value, last_time)
    let mut raw_data: HashMap<String, (Vec<i64>, Vec<f64>, Option<String>, f64, i64)> =
        HashMap::new();

    let mut total_rows = 0u64;
    let cpu_metric = "cgroup.v2.cpu.stat.usage_usec";

    for batch_result in reader {
        let batch = batch_result?;
        total_rows += batch.num_rows() as u64;

        let metric_names = batch
            .column_by_name("metric_name")
            .and_then(|c| c.as_any().downcast_ref::<StringArray>())
            .context("Missing metric_name column")?;

        let times = batch
            .column_by_name("time")
            .context("Missing time column")?;

        let values_int = batch
            .column_by_name("value_int")
            .and_then(|c| c.as_any().downcast_ref::<UInt64Array>());

        let values_float = batch
            .column_by_name("value_float")
            .and_then(|c| c.as_any().downcast_ref::<Float64Array>());

        let labels_col = batch
            .column_by_name("labels")
            .context("Missing labels column")?;

        let time_values: Vec<i64> = match times.data_type() {
            DataType::Timestamp(_, _) => {
                let ts_array = times
                    .as_any()
                    .downcast_ref::<arrow::array::TimestampMillisecondArray>()
                    .context("Failed to cast timestamp")?;
                (0..ts_array.len()).map(|i| ts_array.value(i)).collect()
            }
            _ => anyhow::bail!("Unexpected time column type"),
        };

        for row in 0..batch.num_rows() {
            let metric = metric_names.value(row);
            if metric != cpu_metric {
                continue;
            }

            let value = if let Some(arr) = values_float {
                if !arr.is_null(row) {
                    arr.value(row)
                } else if let Some(int_arr) = values_int {
                    if !int_arr.is_null(row) {
                        int_arr.value(row) as f64
                    } else {
                        continue;
                    }
                } else {
                    continue;
                }
            } else if let Some(int_arr) = values_int {
                if !int_arr.is_null(row) {
                    int_arr.value(row) as f64
                } else {
                    continue;
                }
            } else {
                continue;
            };

            let time = time_values[row];

            let labels = extract_labels_from_column(labels_col, row)?;
            let container_id = match extract_label(&labels, "container_id") {
                Some(id) => id,
                None => continue,
            };
            let qos_class = extract_label(&labels, "qos_class");

            let entry = raw_data
                .entry(container_id.clone())
                .or_insert_with(|| (Vec::new(), Vec::new(), qos_class.clone(), -1.0, 0));

            // Compute delta if we have a previous value
            if entry.3 >= 0.0 && time > entry.4 {
                let value_delta = value - entry.3;
                let time_delta_ms = time - entry.4;

                if value_delta >= 0.0 && time_delta_ms > 0 {
                    let cpu_percent = value_delta / (time_delta_ms as f64) / 10.0;
                    entry.0.push(time);
                    entry.1.push(cpu_percent);
                }
            }

            entry.3 = value;
            entry.4 = time;
            if entry.2.is_none() {
                entry.2 = qos_class;
            }
        }
    }

    eprintln!("Loaded {} rows, found {} containers", total_rows, raw_data.len());

    // Build container data
    let mut containers: HashMap<String, ContainerData> = HashMap::new();

    for (id, (timestamps, cpu_values, qos_class, _, _)) in raw_data {
        if cpu_values.is_empty() {
            continue;
        }

        let short_id = if id.len() > 12 {
            id[..12].to_string()
        } else {
            id.clone()
        };

        let avg_cpu = cpu_values.iter().sum::<f64>() / cpu_values.len() as f64;
        let max_cpu = cpu_values.iter().cloned().fold(f64::NEG_INFINITY, f64::max);

        let timeseries: Vec<TimeseriesPoint> = timestamps
            .into_iter()
            .zip(cpu_values.iter())
            .map(|(t, &cpu)| TimeseriesPoint {
                time_ms: t,
                cpu_percent: cpu,
            })
            .collect();

        let info = ContainerInfo {
            id: id.clone(),
            short_id: short_id.clone(),
            qos_class,
            sample_count: timeseries.len(),
            avg_cpu,
            max_cpu,
        };

        containers.insert(short_id, ContainerData { info, timeseries });
    }

    // Build sorted container list (by avg CPU descending)
    let mut container_list: Vec<ContainerInfo> = containers.values().map(|c| c.info.clone()).collect();
    container_list.sort_by(|a, b| b.avg_cpu.partial_cmp(&a.avg_cpu).unwrap_or(std::cmp::Ordering::Equal));

    eprintln!("Processed {} containers with data", containers.len());

    Ok(AppState {
        containers,
        container_list,
    })
}

// API handlers

async fn index_handler() -> Html<&'static str> {
    Html(INDEX_HTML)
}

async fn containers_handler(State(state): State<Arc<AppState>>) -> Json<Vec<ContainerInfo>> {
    Json(state.container_list.clone())
}

#[derive(Deserialize)]
struct TimeseriesQuery {
    containers: String,
}

#[derive(Serialize)]
struct TimeseriesResponse {
    container: String,
    data: Vec<TimeseriesPoint>,
}

async fn timeseries_handler(
    State(state): State<Arc<AppState>>,
    Query(query): Query<TimeseriesQuery>,
) -> impl IntoResponse {
    let container_ids: Vec<&str> = query.containers.split(',').collect();

    let mut result: Vec<TimeseriesResponse> = Vec::new();

    for id in container_ids {
        if let Some(container) = state.containers.get(id) {
            result.push(TimeseriesResponse {
                container: id.to_string(),
                data: container.timeseries.clone(),
            });
        }
    }

    Json(result)
}

#[tokio::main]
async fn main() -> Result<()> {
    let args = Args::parse();

    if !args.input.exists() {
        anyhow::bail!("File not found: {:?}", args.input);
    }

    let state = Arc::new(load_data(&args.input)?);

    let app = Router::new()
        .route("/", get(index_handler))
        .route("/api/containers", get(containers_handler))
        .route("/api/timeseries", get(timeseries_handler))
        .layer(CorsLayer::permissive())
        .with_state(state);

    let addr = SocketAddr::from(([127, 0, 0, 1], args.port));

    if !args.no_browser {
        let url = format!("http://{}", addr);
        eprintln!("\nOpening browser at {}", url);
        // Try to open browser (best effort)
        #[cfg(target_os = "macos")]
        let _ = std::process::Command::new("open").arg(&url).spawn();
        #[cfg(target_os = "linux")]
        let _ = std::process::Command::new("xdg-open").arg(&url).spawn();
    }

    eprintln!("Server running at http://{}", addr);
    eprintln!("Press Ctrl+C to stop\n");

    let listener = tokio::net::TcpListener::bind(addr).await?;
    axum::serve(listener, app).await?;

    Ok(())
}

const INDEX_HTML: &str = r##"<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Container CPU Metrics Viewer</title>
    <script src="https://cdn.plot.ly/plotly-2.27.0.min.js"></script>
    <style>
        * { box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;
            margin: 0;
            padding: 20px;
            background: #f5f5f5;
        }
        h1 {
            text-align: center;
            color: #333;
            margin-bottom: 20px;
        }
        .controls {
            background: white;
            padding: 15px;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            margin-bottom: 20px;
            display: flex;
            gap: 20px;
            align-items: flex-start;
            flex-wrap: wrap;
        }
        .control-group {
            flex: 1;
            min-width: 300px;
        }
        label {
            font-weight: 600;
            display: block;
            margin-bottom: 5px;
            color: #555;
        }
        select {
            width: 100%;
            padding: 8px;
            border: 1px solid #ddd;
            border-radius: 4px;
            font-size: 14px;
            min-height: 150px;
        }
        .buttons {
            display: flex;
            gap: 10px;
            flex-wrap: wrap;
        }
        button {
            padding: 8px 16px;
            border: none;
            border-radius: 4px;
            cursor: pointer;
            font-size: 14px;
            transition: background 0.2s;
        }
        .btn-primary { background: #007bff; color: white; }
        .btn-primary:hover { background: #0056b3; }
        .btn-success { background: #28a745; color: white; }
        .btn-success:hover { background: #1e7e34; }
        .btn-secondary { background: #6c757d; color: white; }
        .btn-secondary:hover { background: #545b62; }
        .btn-warning { background: #ffc107; color: #333; }
        .btn-warning:hover { background: #d39e00; }
        #chart {
            background: white;
            border-radius: 8px;
            box-shadow: 0 2px 4px rgba(0,0,0,0.1);
            padding: 10px;
        }
        .status {
            color: #666;
            font-size: 14px;
            margin-top: 10px;
        }
        .loading {
            text-align: center;
            padding: 40px;
            color: #666;
        }
    </style>
</head>
<body>
    <h1>Container CPU Metrics Viewer</h1>

    <div class="controls">
        <div class="control-group">
            <label>Select Containers (Ctrl/Cmd+click for multiple):</label>
            <select id="containerSelect" multiple></select>
        </div>
        <div class="control-group">
            <label>Quick Actions:</label>
            <div class="buttons">
                <button class="btn-primary" onclick="selectTop(5)">Top 5</button>
                <button class="btn-primary" onclick="selectTop(10)">Top 10</button>
                <button class="btn-secondary" onclick="clearSelection()">Clear</button>
                <button class="btn-success" onclick="rescaleY()">Rescale Y-Axis</button>
                <button class="btn-warning" onclick="resetZoom()">Reset Zoom</button>
            </div>
            <div class="status" id="status">Loading containers...</div>
        </div>
    </div>

    <div id="chart">
        <div class="loading">Loading data...</div>
    </div>

    <script>
        let containers = [];
        let currentData = {};
        let currentXRange = null;

        // Fetch container list on load
        async function loadContainers() {
            try {
                const response = await fetch('/api/containers');
                containers = await response.json();

                const select = document.getElementById('containerSelect');
                select.innerHTML = containers.map((c, i) =>
                    `<option value="${c.short_id}" ${i < 5 ? 'selected' : ''}>` +
                    `${c.short_id} (${c.qos_class || 'unknown'}) - avg: ${c.avg_cpu.toFixed(1)}%</option>`
                ).join('');

                document.getElementById('status').textContent =
                    `${containers.length} containers loaded`;

                // Load initial data
                await loadTimeseries();
            } catch (err) {
                document.getElementById('status').textContent = 'Error loading containers: ' + err;
            }
        }

        async function loadTimeseries() {
            const select = document.getElementById('containerSelect');
            const selected = Array.from(select.selectedOptions).map(o => o.value);

            if (selected.length === 0) {
                Plotly.newPlot('chart', [], {
                    title: 'Select containers to display',
                    xaxis: { title: 'Time' },
                    yaxis: { title: 'CPU Usage (%)' }
                });
                return;
            }

            document.getElementById('status').textContent = 'Loading timeseries...';

            try {
                const response = await fetch(`/api/timeseries?containers=${selected.join(',')}`);
                const data = await response.json();

                currentData = {};
                data.forEach(d => { currentData[d.container] = d.data; });

                plotData(selected);

                const totalPoints = data.reduce((sum, d) => sum + d.data.length, 0);
                document.getElementById('status').textContent =
                    `Showing ${selected.length} containers, ${totalPoints.toLocaleString()} points`;
            } catch (err) {
                document.getElementById('status').textContent = 'Error loading data: ' + err;
            }
        }

        function plotData(containerIds, yRange = null) {
            const traces = containerIds.map((id, i) => {
                const data = currentData[id] || [];
                const container = containers.find(c => c.short_id === id);
                const qos = container?.qos_class || 'unknown';

                return {
                    x: data.map(p => new Date(p.time_ms)),
                    y: data.map(p => p.cpu_percent),
                    type: 'scattergl',
                    mode: 'lines',
                    name: `${id} (${qos})`,
                    line: { width: 1 }
                };
            });

            const layout = {
                title: 'CPU Usage Over Time (Pan/Zoom to explore, Double-click to reset)',
                xaxis: {
                    title: 'Time',
                    rangeslider: { visible: true, thickness: 0.05 }
                },
                yaxis: {
                    title: 'CPU Usage (%)',
                    range: yRange
                },
                hovermode: 'x unified',
                legend: {
                    orientation: 'h',
                    yanchor: 'bottom',
                    y: 1.02,
                    xanchor: 'right',
                    x: 1
                },
                margin: { l: 60, r: 20, t: 80, b: 60 }
            };

            // Preserve x range if set
            if (currentXRange) {
                layout.xaxis.range = currentXRange;
            }

            Plotly.newPlot('chart', traces, layout, { scrollZoom: true });

            // Track x range changes
            document.getElementById('chart').on('plotly_relayout', function(eventData) {
                if (eventData['xaxis.range[0]'] !== undefined) {
                    currentXRange = [eventData['xaxis.range[0]'], eventData['xaxis.range[1]']];
                } else if (eventData['xaxis.range']) {
                    currentXRange = eventData['xaxis.range'];
                } else if (eventData['xaxis.autorange']) {
                    currentXRange = null;
                }
            });
        }

        function selectTop(n) {
            const select = document.getElementById('containerSelect');
            Array.from(select.options).forEach((opt, i) => {
                opt.selected = i < n;
            });
            loadTimeseries();
        }

        function clearSelection() {
            const select = document.getElementById('containerSelect');
            Array.from(select.options).forEach(opt => { opt.selected = false; });
            loadTimeseries();
        }

        function rescaleY() {
            if (!currentXRange) {
                document.getElementById('status').textContent = 'Zoom in first, then click Rescale Y-Axis';
                return;
            }

            const select = document.getElementById('containerSelect');
            const selected = Array.from(select.selectedOptions).map(o => o.value);

            // Find y range for visible x range
            let yMin = Infinity, yMax = -Infinity;
            const xMin = new Date(currentXRange[0]).getTime();
            const xMax = new Date(currentXRange[1]).getTime();

            selected.forEach(id => {
                const data = currentData[id] || [];
                data.forEach(p => {
                    if (p.time_ms >= xMin && p.time_ms <= xMax) {
                        yMin = Math.min(yMin, p.cpu_percent);
                        yMax = Math.max(yMax, p.cpu_percent);
                    }
                });
            });

            if (yMin !== Infinity) {
                const padding = (yMax - yMin) * 0.05 || 1;
                plotData(selected, [Math.max(0, yMin - padding), yMax + padding]);
                document.getElementById('status').textContent =
                    `Y-axis rescaled to ${yMin.toFixed(1)}% - ${yMax.toFixed(1)}%`;
            }
        }

        function resetZoom() {
            currentXRange = null;
            const select = document.getElementById('containerSelect');
            const selected = Array.from(select.selectedOptions).map(o => o.value);
            plotData(selected);
        }

        // Event listeners
        document.getElementById('containerSelect').addEventListener('change', loadTimeseries);

        // Initial load
        loadContainers();
    </script>
</body>
</html>
"##;
