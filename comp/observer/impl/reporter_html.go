// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

// parseInt64 parses a string to int64.
func parseInt64(s string) (int64, error) {
	return strconv.ParseInt(s, 10, 64)
}

// sanitizeFloat replaces Inf, NaN, and extremely large values with 0 for JSON compatibility.
// Extremely large values (> 1e15) can cause Chart.js to crash.
func sanitizeFloat(v float64) float64 {
	if math.IsInf(v, 0) || math.IsNaN(v) {
		return 0
	}
	// Cap extremely large values to prevent Chart.js crashes
	if v > 1e15 || v < -1e15 {
		return 0
	}
	return v
}

const maxReportBuffer = 100

// timestampedReport wraps a ReportOutput with a timestamp.
type timestampedReport struct {
	Title     string            `json:"title"`
	Body      string            `json:"body"`
	Timestamp time.Time         `json:"timestamp"`
	Metadata  map[string]string `json:"metadata"`
}

// HTMLReporter is an HTTP server that displays reports and metrics on a local webpage.
type HTMLReporter struct {
	mu                    sync.RWMutex
	reports               []timestampedReport
	storage               *timeSeriesStorage
	correlationState      observer.CorrelationState
	rawAnomalyState       observer.RawAnomalyState
	timeClusterCorrelator *TimeClusterCorrelator
	graphSketchCorrelator *GraphSketchCorrelator
	server                *http.Server
}

// NewHTMLReporter creates a new HTMLReporter.
func NewHTMLReporter() *HTMLReporter {
	return &HTMLReporter{
		reports: make([]timestampedReport, 0, maxReportBuffer),
	}
}

// Name returns the reporter name.
func (r *HTMLReporter) Name() string {
	return "html_reporter"
}

// Report adds a report to the buffer.
func (r *HTMLReporter) Report(report observer.ReportOutput) {
	r.mu.Lock()
	defer r.mu.Unlock()

	tr := timestampedReport{
		Title:     report.Title,
		Body:      report.Body,
		Timestamp: time.Now(),
		Metadata:  report.Metadata,
	}

	// Prepend to keep most recent first
	r.reports = append([]timestampedReport{tr}, r.reports...)

	// Cap at maxReportBuffer (evict oldest)
	if len(r.reports) > maxReportBuffer {
		r.reports = r.reports[:maxReportBuffer]
	}
}

// SetStorage sets the metric storage for querying series data.
func (r *HTMLReporter) SetStorage(storage *timeSeriesStorage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storage = storage
}

// SetCorrelationState sets the correlation state source for querying active correlations.
func (r *HTMLReporter) SetCorrelationState(state observer.CorrelationState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.correlationState = state
}

// SetRawAnomalyState sets the raw anomaly state source for querying individual anomalies.
func (r *HTMLReporter) SetRawAnomalyState(state observer.RawAnomalyState) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.rawAnomalyState = state
}

// SetTimeClusterCorrelator sets the time cluster correlator for visualization.
func (r *HTMLReporter) SetTimeClusterCorrelator(tc *TimeClusterCorrelator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.timeClusterCorrelator = tc
}

// SetGraphSketchCorrelator sets the GraphSketch correlator for visualization.
func (r *HTMLReporter) SetGraphSketchCorrelator(gsc *GraphSketchCorrelator) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.graphSketchCorrelator = gsc
}

// Start starts the HTTP server on the given address.
func (r *HTMLReporter) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", r.handleDashboard)
	mux.HandleFunc("/graphsketch-correlation", r.handleGraphPage)
	mux.HandleFunc("/timecluster", r.handleTimeClusterPage)
	mux.HandleFunc("/api/reports", r.handleAPIReports)
	mux.HandleFunc("/api/series", r.handleAPISeries)
	mux.HandleFunc("/api/series/list", r.handleAPISeriesList)
	mux.HandleFunc("/api/series/batch", r.handleAPISeriesBatch)
	mux.HandleFunc("/api/correlations", r.handleAPICorrelations)
	mux.HandleFunc("/api/raw-anomalies", r.handleAPIRawAnomalies)
	mux.HandleFunc("/api/graphsketch/edges", r.handleAPIGraphSketchEdges)
	mux.HandleFunc("/api/graphsketch/stats", r.handleAPIGraphSketchStats)
	mux.HandleFunc("/api/timecluster/clusters", r.handleAPITimeClusterClusters)
	mux.HandleFunc("/api/timecluster/stats", r.handleAPITimeClusterStats)

	r.server = &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := r.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			// Log error but don't crash - server might be stopped intentionally
		}
	}()

	return nil
}

// Stop stops the HTTP server.
func (r *HTMLReporter) Stop() error {
	if r.server == nil {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	return r.server.Shutdown(ctx)
}

// handleDashboard serves the HTML dashboard.
func (r *HTMLReporter) handleDashboard(w http.ResponseWriter, req *http.Request) {
	if req.URL.Path != "/" {
		http.NotFound(w, req)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>Observer Demo Dashboard</title>
    <script src="https://cdn.jsdelivr.net/npm/chart.js"></script>
    <script src="https://cdn.jsdelivr.net/npm/chartjs-plugin-annotation"></script>
    <style>
        * { box-sizing: border-box; }
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            margin: 0;
            padding: 0;
            background-color: #1a1a2e;
            color: #eee;
            height: 100vh;
            overflow: hidden;
        }
        .header {
            padding: 15px 20px;
            border-bottom: 2px solid #632ca6;
            background: #1a1a2e;
        }
        .header h1 {
            margin: 0;
            font-size: 1.4em;
            color: #fff;
        }
        .main-layout {
            display: grid;
            grid-template-columns: 40% 60%;
            height: calc(100vh - 60px - 40px);
        }
        .timeline-panel {
            background: #16162a;
            border-right: 1px solid #3a3a5e;
            overflow-y: auto;
            padding: 15px;
        }
        .timeline-panel h2 {
            color: #aaa;
            font-size: 0.9em;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin: 0 0 15px 0;
        }
        .charts-panel {
            overflow-y: auto;
            padding: 15px;
        }
        .charts-section {
            margin-bottom: 25px;
        }
        .charts-section h2 {
            color: #aaa;
            font-size: 0.9em;
            text-transform: uppercase;
            letter-spacing: 0.5px;
            margin: 0 0 15px 0;
        }
        .charts-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(400px, 1fr));
            gap: 15px;
        }
        .charts-grid-small {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 10px;
        }
        .chart-container {
            background: #2a2a4e;
            border-radius: 8px;
            padding: 15px;
            height: 220px;
        }
        .chart-container-small {
            background: #2a2a4e;
            border-radius: 6px;
            padding: 10px;
            height: 120px;
        }
        .no-anomalies {
            background: #2a2a4e;
            border-radius: 8px;
            padding: 30px;
            color: #666;
            text-align: center;
            font-size: 0.9em;
        }
        .timeline-item {
            position: relative;
            padding: 12px 15px 12px 25px;
            margin-bottom: 10px;
            background: #2a2a4e;
            border-radius: 6px;
            border-left: 3px solid #632ca6;
        }
        .timeline-item::before {
            content: '';
            position: absolute;
            left: -8px;
            top: 50%;
            transform: translateY(-50%);
            width: 10px;
            height: 10px;
            background: #632ca6;
            border-radius: 50%;
            border: 2px solid #16162a;
        }
        .timeline-item.correlated {
            border-left-color: #ef4444;
        }
        .timeline-item.correlated::before {
            background: #ef4444;
        }
        .timeline-time {
            font-size: 0.75em;
            color: #888;
            margin-bottom: 4px;
        }
        .timeline-source {
            font-size: 0.85em;
            color: #8b5cf6;
            font-family: monospace;
            margin-bottom: 4px;
        }
        .timeline-title {
            font-size: 0.9em;
            color: #fff;
            font-weight: 500;
        }
        .timeline-desc {
            font-size: 0.8em;
            color: #aaa;
            margin-top: 4px;
        }
        .timeline-signals {
            margin-top: 10px;
        }
        .timeline-signal {
            background: rgba(255,255,255,0.05);
            border-radius: 4px;
            padding: 8px 10px;
            margin-top: 6px;
        }
        .signal-source {
            display: block;
            font-size: 0.8em;
            color: #8b5cf6;
            font-family: monospace;
            margin-bottom: 3px;
        }
        .signal-desc {
            display: block;
            font-size: 0.8em;
            color: #ccc;
        }
        .correlation-badge {
            display: inline-block;
            background: linear-gradient(135deg, #632ca6 0%, #8b5cf6 100%);
            color: #fff;
            font-size: 0.7em;
            padding: 2px 8px;
            border-radius: 10px;
            margin-top: 6px;
        }
        .status-bar {
            position: fixed;
            bottom: 0;
            left: 0;
            right: 0;
            background: #2a2a4e;
            padding: 8px 20px;
            font-size: 0.85em;
            color: #888;
            border-top: 1px solid #3a3a5e;
            height: 40px;
        }
        .status-dot {
            display: inline-block;
            width: 8px;
            height: 8px;
            border-radius: 50%;
            background: #4ade80;
            margin-right: 8px;
            animation: pulse 2s infinite;
        }
        @keyframes pulse {
            0%, 100% { opacity: 1; }
            50% { opacity: 0.5; }
        }
    </style>
</head>
<body>
    <div class="header" style="display:flex;justify-content:space-between;align-items:center;">
        <h1>Observer Demo Dashboard</h1>
        <nav style="display:flex;gap:15px;">
            <a href="/graphsketch-correlation" style="color:#888;text-decoration:none;font-size:0.9em;">GraphSketch Correlator</a>
            <a href="/timecluster" style="color:#888;text-decoration:none;font-size:0.9em;">TimeCluster Correlator</a>
        </nav>
    </div>

    <div class="main-layout">
        <div class="timeline-panel">
            <h2>Anomaly Timeline</h2>
            <div id="timeline">
                <div class="no-anomalies">Waiting for anomalies...</div>
            </div>
        </div>

        <div class="charts-panel">
            <div class="charts-section">
                <h2>Anomalous Metrics</h2>
                <div class="charts-grid" id="anomaly-charts">
                    <div class="no-anomalies">No anomalies detected yet</div>
                </div>
            </div>
            <div class="charts-section">
                <h2>All Metrics</h2>
                <div class="charts-grid-small" id="all-charts"></div>
            </div>
        </div>
    </div>

    <div class="status-bar">
        <span class="status-dot"></span>
        <span id="status">Connecting...</span>
    </div>

    <script>
        const anomalyCharts = {};  // charts for anomalous metrics
        const allCharts = {};      // charts for all metrics
        const chartColors = ['#8b5cf6', '#06b6d4', '#f59e0b', '#ef4444', '#22c55e'];
        let anomalyRanges = {}; // chartKey -> [{start, end, analyzerName}, ...]
        let correlationData = []; // list of correlations for timeline
        let rawAnomalyData = []; // raw anomalies from all analyzers

        // Track last timestamp per chart for incremental fetching
        const lastTimestamps = {};  // chartKey -> last timestamp received
        // Store accumulated data per chart (timestamps and values)
        const chartData = {};  // chartKey -> { timestamps: [], values: [], labels: [] }

        // Analyzer colors for distinguishing different detection algorithms
        const analyzerColors = {
            'cusum_detector': { bg: 'rgba(239, 68, 68, 0.15)', border: '#ef4444' },  // red
            'arima_detector': { bg: 'rgba(59, 130, 246, 0.15)', border: '#3b82f6' }, // blue
            'mad_detector':   { bg: 'rgba(34, 197, 94, 0.15)', border: '#22c55e' },  // green
            'default':        { bg: 'rgba(168, 85, 247, 0.15)', border: '#a855f7' }  // purple
        };

        function getAnalyzerColor(analyzerName) {
            return analyzerColors[analyzerName] || analyzerColors['default'];
        }

        // Build a map of chart key -> anomaly timestamps from raw anomalies
        function buildAnomalyRanges(rawAnomalies) {
            const ranges = {};
            for (const a of rawAnomalies || []) {
                const key = a.source;
                if (!ranges[key]) ranges[key] = [];
                if (a.timestamp) {
                    ranges[key].push({
                        timestamp: a.timestamp,
                        analyzerName: a.analyzerName || 'unknown'
                    });
                }
            }
            return ranges;
        }

        async function fetchRawAnomalies() {
            try {
                const resp = await fetch('/api/raw-anomalies');
                rawAnomalyData = await resp.json() || [];
                anomalyRanges = buildAnomalyRanges(rawAnomalyData);
                renderRawAnomalySummary(rawAnomalyData);
            } catch (e) {
                console.error('Failed to fetch raw anomalies:', e);
            }
        }

        async function fetchCorrelations() {
            try {
                const resp = await fetch('/api/correlations');
                const correlations = await resp.json();
                correlationData = correlations || [];
                renderTimeline(correlationData);
            } catch (e) {
                console.error('Failed to fetch correlations:', e);
            }
        }

        function formatTime(unixSeconds) {
            if (!unixSeconds) return '';
            const d = new Date(unixSeconds * 1000);
            return d.toLocaleTimeString();
        }

        function renderTimeline(correlations) {
            const container = document.getElementById('timeline');
            if (!correlations || correlations.length === 0) {
                container.innerHTML = '<div class="no-anomalies">No correlated anomalies detected yet</div>';
                return;
            }

            // Sort by lastUpdated (most recent first)
            const sorted = [...correlations].sort((a, b) => (b.lastUpdated || 0) - (a.lastUpdated || 0));

            let html = '';
            for (const c of sorted) {
                const timeStr = formatTime(c.firstSeen) + ' - ' + formatTime(c.lastUpdated);
                html += '<div class="timeline-item correlated">';
                html += '<div class="timeline-time">' + escapeHtml(timeStr) + '</div>';
                html += '<div class="timeline-title">' + escapeHtml(c.title) + '</div>';
                html += '<div class="timeline-signals">';
                for (const a of c.anomalies || []) {
                    html += '<div class="timeline-signal">';
                    html += '<span class="signal-source">' + escapeHtml(a.source) + '</span>';
                    html += '<span class="signal-desc">' + escapeHtml(a.description) + '</span>';
                    html += '</div>';
                }
                html += '</div>';
                html += '</div>';
            }
            container.innerHTML = html;
        }

        function renderRawAnomalySummary(rawAnomalies) {
            // Group anomalies by analyzer
            const byAnalyzer = {};
            for (const a of rawAnomalies || []) {
                const name = a.analyzerName || 'unknown';
                if (!byAnalyzer[name]) byAnalyzer[name] = [];
                byAnalyzer[name].push(a);
            }

            // Find or create summary container
            let summaryDiv = document.getElementById('raw-anomaly-summary');
            if (!summaryDiv) {
                const timelinePanel = document.querySelector('.timeline-panel');
                summaryDiv = document.createElement('div');
                summaryDiv.id = 'raw-anomaly-summary';
                summaryDiv.innerHTML = '<h3 style="margin: 16px 0 8px 0; color: #a1a1aa;">Raw Anomalies by Analyzer</h3>';
                timelinePanel.insertBefore(summaryDiv, timelinePanel.querySelector('#timeline'));
            }

            if (Object.keys(byAnalyzer).length === 0) {
                summaryDiv.innerHTML = '<h3 style="margin: 16px 0 8px 0; color: #a1a1aa;">Raw Anomalies by Analyzer</h3><div class="no-anomalies">No anomalies detected yet</div>';
                return;
            }

            let html = '<h3 style="margin: 16px 0 8px 0; color: #a1a1aa;">Raw Anomalies by Analyzer</h3>';
            html += '<div style="display: flex; flex-wrap: wrap; gap: 8px; margin-bottom: 16px;">';
            for (const [analyzer, anomalies] of Object.entries(byAnalyzer)) {
                const color = getAnalyzerColor(analyzer);
                const sources = [...new Set(anomalies.map(a => a.source))];
                html += '<div style="background: ' + color.bg + '; border-left: 3px solid ' + color.border + '; padding: 8px 12px; border-radius: 4px; min-width: 140px;">';
                html += '<div style="font-weight: 500; color: ' + color.border + ';">' + escapeHtml(analyzer) + '</div>';
                html += '<div style="color: #d4d4d8; font-size: 0.875rem;">' + anomalies.length + ' anomalies</div>';
                html += '<div style="color: #71717a; font-size: 0.75rem;">' + sources.length + ' metrics</div>';
                html += '</div>';
            }
            html += '</div>';
            summaryDiv.innerHTML = html;
        }

        async function fetchSeriesList() {
            try {
                const resp = await fetch('/api/series/list?namespace=demo');
                return await resp.json();
            } catch (e) {
                console.error('Failed to fetch series list:', e);
                return [];
            }
        }

        async function fetchSeries(name, agg, tags, since) {
            try {
                let url = '/api/series?namespace=demo&name=' + encodeURIComponent(name) + '&agg=' + agg;
                if (tags && tags.length > 0) {
                    url += '&tags=' + encodeURIComponent(tags.join(','));
                }
                // Add since parameter for incremental fetching
                if (since && since > 0) {
                    url += '&since=' + since;
                }
                const resp = await fetch(url);
                if (!resp.ok) return null;
                return await resp.json();
            } catch (e) {
                console.error('Failed to fetch series:', e);
                return null;
            }
        }

        // Aggregations that are analyzed (must match analysisAggregations in observer.go)
        const analysisAggregations = ['avg', 'count'];

        // Build Chart.js annotations from anomaly timestamps for a given chart key
        // Uses analyzer-specific colors when analyzerName is present
        function buildAnnotations(timestamps, anomalies) {
            if (!anomalies || anomalies.length === 0 || !timestamps || timestamps.length === 0) {
                return {};
            }

            const annotations = {};
            let idx = 0;

            for (const anomaly of anomalies) {
                // Find the index of the closest timestamp
                let pointIdx = -1;
                for (let i = 0; i < timestamps.length; i++) {
                    if (timestamps[i] >= anomaly.timestamp) {
                        pointIdx = i;
                        break;
                    }
                }
                // If no exact match, use the last point if timestamp is beyond the data
                if (pointIdx === -1 && anomaly.timestamp > timestamps[timestamps.length - 1]) {
                    pointIdx = timestamps.length - 1;
                }

                if (pointIdx !== -1) {
                    const color = getAnalyzerColor(anomaly.analyzerName);
                    // Draw a vertical line at the anomaly detection point
                    annotations['line' + idx] = {
                        type: 'line',
                        xMin: pointIdx,
                        xMax: pointIdx,
                        borderColor: color.border,
                        borderWidth: 2,
                        label: {
                            display: false
                        }
                    };
                    idx++;
                }
            }

            return annotations;
        }

        function createChart(name, containerId, containerSelector, isSmall) {
            const container = document.querySelector(containerSelector);

            // Remove "no anomalies" placeholder if present
            const placeholder = container.querySelector('.no-anomalies');
            if (placeholder) placeholder.remove();

            const div = document.createElement('div');
            div.className = isSmall ? 'chart-container-small' : 'chart-container';
            div.id = containerId;
            div.innerHTML = '<canvas></canvas>';
            container.appendChild(div);

            const ctx = div.querySelector('canvas').getContext('2d');
            const colorIdx = (Object.keys(allCharts).length + Object.keys(anomalyCharts).length) % chartColors.length;

            const chart = new Chart(ctx, {
                type: 'line',
                data: {
                    labels: [],
                    datasets: [{
                        label: name,
                        data: [],
                        borderColor: chartColors[colorIdx],
                        backgroundColor: chartColors[colorIdx] + '33',
                        tension: 0.3,
                        fill: true,
                        pointRadius: 0
                    }]
                },
                options: {
                    responsive: true,
                    maintainAspectRatio: false,
                    animation: false,
                    plugins: {
                        legend: {
                            display: !isSmall,
                            labels: { color: '#aaa' }
                        },
                        title: {
                            display: isSmall,
                            text: name,
                            color: '#aaa',
                            font: { size: 10 }
                        },
                        annotation: {
                            annotations: {}
                        }
                    },
                    scales: {
                        x: {
                            display: !isSmall,
                            ticks: { color: '#666', maxTicksLimit: 6 },
                            grid: { color: '#3a3a5e' }
                        },
                        y: {
                            ticks: { color: '#666', maxTicksLimit: isSmall ? 3 : 6 },
                            grid: { color: '#3a3a5e' }
                        }
                    }
                }
            });

            chart.timestamps = [];
            return chart;
        }

        async function updateCharts() {
            const anomalousSources = new Set(Object.keys(anomalyRanges));

            // If no anomalies, show placeholder in anomaly section
            if (anomalousSources.size === 0 && Object.keys(anomalyCharts).length === 0) {
                const container = document.getElementById('anomaly-charts');
                if (!container.querySelector('.no-anomalies')) {
                    container.innerHTML = '<div class="no-anomalies">No anomalies detected yet</div>';
                }
            }

            // Fetch all series data in a single batch request
            let batchData;
            try {
                const resp = await fetch('/api/series/batch', {
                    method: 'POST',
                    headers: { 'Content-Type': 'application/json' },
                    body: JSON.stringify({
                        namespace: 'demo',
                        since: lastTimestamps
                    })
                });
                batchData = await resp.json();
            } catch (e) {
                console.error('Failed to fetch batch series:', e);
                return;
            }

            let totalNewPoints = 0;
            const updatedKeys = new Set();

            // Process all series from batch response
            for (const [chartKey, series] of Object.entries(batchData.series || {})) {
                // Initialize accumulated data if needed
                if (!chartData[chartKey]) {
                    chartData[chartKey] = { timestamps: [], values: [], labels: [] };
                }

                // Append new points to accumulated data
                if (series.points && series.points.length > 0) {
                    updatedKeys.add(chartKey);
                    totalNewPoints += series.points.length;

                    for (const p of series.points) {
                        chartData[chartKey].timestamps.push(p.timestamp);
                        chartData[chartKey].values.push(p.value);
                        chartData[chartKey].labels.push(new Date(p.timestamp * 1000).toLocaleTimeString());
                    }

                    // Update last timestamp for next incremental fetch
                    const lastPoint = series.points[series.points.length - 1];
                    lastTimestamps[chartKey] = lastPoint.timestamp;
                }
            }

            // Update charts for all known series
            for (const chartKey of Object.keys(chartData)) {
                const data = chartData[chartKey];
                const hasNewPoints = updatedKeys.has(chartKey);

                // Create/update small chart for ALL metrics
                const smallChartId = 'all-chart-' + chartKey.replace(/[^a-z0-9]/gi, '-');
                if (!allCharts[chartKey]) {
                    allCharts[chartKey] = createChart(chartKey, smallChartId, '#all-charts', true);
                }

                // Only update chart if we have data and either new points or it's the first render
                if (data.timestamps.length > 0 && (hasNewPoints || allCharts[chartKey].data.datasets[0].data.length === 0)) {
                    const chart = allCharts[chartKey];
                    chart.data.labels = data.labels;
                    chart.data.datasets[0].data = data.values;
                    chart.timestamps = data.timestamps;

                    const ranges = anomalyRanges[chartKey] || [];
                    chart.options.plugins.annotation.annotations = buildAnnotations(data.timestamps, ranges);
                    chart.update('none');
                }

                // Create/update large chart only for ANOMALOUS metrics
                if (anomalousSources.has(chartKey)) {
                    const largeChartId = 'anomaly-chart-' + chartKey.replace(/[^a-z0-9]/gi, '-');
                    if (!anomalyCharts[chartKey]) {
                        anomalyCharts[chartKey] = createChart(chartKey, largeChartId, '#anomaly-charts', false);
                    }

                    if (data.timestamps.length > 0 && (hasNewPoints || anomalyCharts[chartKey].data.datasets[0].data.length === 0)) {
                        const chart = anomalyCharts[chartKey];
                        chart.data.labels = data.labels;
                        chart.data.datasets[0].data = data.values;
                        chart.timestamps = data.timestamps;

                        const ranges = anomalyRanges[chartKey] || [];
                        chart.options.plugins.annotation.annotations = buildAnnotations(data.timestamps, ranges);
                        chart.update('none');
                    }
                }
            }

            document.getElementById('status').textContent = 'Last update: ' + new Date().toLocaleTimeString() + ' (' + totalNewPoints + ' new points)';
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Initial load and periodic refresh
        fetchRawAnomalies();
        fetchCorrelations();
        updateCharts();
        setInterval(fetchRawAnomalies, 1000);
        setInterval(fetchCorrelations, 1000);
        setInterval(updateCharts, 1000);
    </script>
</body>
</html>
`

	_, _ = w.Write([]byte(html))
}

// handleAPIReports returns JSON array of recent reports.
func (r *HTMLReporter) handleAPIReports(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	reports := make([]timestampedReport, len(r.reports))
	copy(reports, r.reports)
	r.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(reports); err != nil {
		log.Printf("[500] /api/reports: failed to encode: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// seriesResponse is the JSON response for the /api/series endpoint.
type seriesResponse struct {
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	Tags      []string      `json:"tags"`
	Points    []pointOutput `json:"points"`
}

// pointOutput is a JSON-serializable point.
type pointOutput struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// parseAggregate parses an aggregation string to Aggregate type.
func parseAggregate(s string) Aggregate {
	switch s {
	case "sum":
		return AggregateSum
	case "count":
		return AggregateCount
	case "min":
		return AggregateMin
	case "max":
		return AggregateMax
	default:
		return AggregateAverage
	}
}

// handleAPISeries returns JSON series data.
// Supports incremental fetching via ?since=<timestamp> parameter.
// When since is provided, only returns points with timestamp > since.
func (r *HTMLReporter) handleAPISeries(w http.ResponseWriter, req *http.Request) {
	name := req.URL.Query().Get("name")
	namespace := req.URL.Query().Get("namespace")
	agg := parseAggregate(req.URL.Query().Get("agg"))

	if name == "" || namespace == "" {
		http.Error(w, "missing required parameters: name and namespace", http.StatusBadRequest)
		return
	}

	// Parse 'since' parameter for delta updates (unix timestamp in seconds)
	var since int64
	if sinceParam := req.URL.Query().Get("since"); sinceParam != "" {
		var err error
		since, err = parseInt64(sinceParam)
		if err != nil {
			http.Error(w, "invalid since parameter: must be unix timestamp", http.StatusBadRequest)
			return
		}
	}

	// Parse tags from query string
	var tags []string
	if tagsParam := req.URL.Query().Get("tags"); tagsParam != "" {
		tags = strings.Split(tagsParam, ",")
	}

	r.mu.RLock()
	storage := r.storage
	r.mu.RUnlock()

	if storage == nil {
		http.Error(w, "storage not configured", http.StatusServiceUnavailable)
		return
	}

	series := storage.GetSeriesSince(namespace, name, tags, agg, since)
	if series == nil {
		http.Error(w, "series not found", http.StatusNotFound)
		return
	}

	resp := seriesResponse{
		Namespace: series.Namespace,
		Name:      series.Name,
		Tags:      series.Tags,
		Points:    make([]pointOutput, len(series.Points)),
	}
	for i, p := range series.Points {
		resp.Points[i] = pointOutput{
			Timestamp: p.Timestamp,
			Value:     sanitizeFloat(p.Value),
		}
	}

	// Encode to buffer first so we can return proper error if encoding fails
	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[500] /api/series: failed to marshal: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// escapeHTML escapes special HTML characters.
func escapeHTML(s string) string {
	var result []byte
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&':
			result = append(result, []byte("&amp;")...)
		case '<':
			result = append(result, []byte("&lt;")...)
		case '>':
			result = append(result, []byte("&gt;")...)
		case '"':
			result = append(result, []byte("&quot;")...)
		case '\'':
			result = append(result, []byte("&#39;")...)
		default:
			result = append(result, s[i])
		}
	}
	return string(result)
}

// seriesListItem is metadata about an available series.
type seriesListItem struct {
	Namespace string   `json:"namespace"`
	Name      string   `json:"name"`
	Tags      []string `json:"tags"`
}

// handleAPISeriesList returns a list of all available series.
func (r *HTMLReporter) handleAPISeriesList(w http.ResponseWriter, req *http.Request) {
	namespace := req.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "demo" // default namespace
	}
	agg := parseAggregate(req.URL.Query().Get("agg"))

	r.mu.RLock()
	storage := r.storage
	r.mu.RUnlock()

	if storage == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("[]"))
		return
	}

	allSeries := storage.AllSeries(namespace, agg)
	items := make([]seriesListItem, len(allSeries))
	for i, s := range allSeries {
		items[i] = seriesListItem{
			Namespace: s.Namespace,
			Name:      s.Name,
			Tags:      s.Tags,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(items); err != nil {
		log.Printf("[500] /api/series/list: failed to encode: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// batchSeriesRequest is the request body for batch series fetching.
type batchSeriesRequest struct {
	Namespace string           `json:"namespace"`
	Since     map[string]int64 `json:"since"` // chartKey -> last timestamp
}

// batchSeriesResponse contains all series data in one response.
type batchSeriesResponse struct {
	Series map[string]seriesResponse `json:"series"` // chartKey -> series data
}

// handleAPISeriesBatch returns all series data in a single request.
// Accepts POST with JSON body containing namespace and since timestamps per series.
// Returns only new points for each series (points with timestamp > since[key]).
func (r *HTMLReporter) handleAPISeriesBatch(w http.ResponseWriter, req *http.Request) {
	// Parse request - support both GET (simple) and POST (with since data)
	namespace := "demo"
	since := make(map[string]int64)

	if req.Method == "POST" {
		var body batchSeriesRequest
		if err := json.NewDecoder(req.Body).Decode(&body); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		if body.Namespace != "" {
			namespace = body.Namespace
		}
		since = body.Since
	} else {
		// GET request - just use query param for namespace
		if ns := req.URL.Query().Get("namespace"); ns != "" {
			namespace = ns
		}
	}

	r.mu.RLock()
	storage := r.storage
	r.mu.RUnlock()

	if storage == nil {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"series":{}}`))
		return
	}

	// Get all series for both aggregations
	aggregations := []Aggregate{AggregateAverage, AggregateCount}
	aggNames := []string{"avg", "count"}

	resp := batchSeriesResponse{
		Series: make(map[string]seriesResponse),
	}

	for aggIdx, agg := range aggregations {
		allSeries := storage.AllSeries(namespace, agg)
		aggName := aggNames[aggIdx]

		for _, s := range allSeries {
			chartKey := s.Name + ":" + aggName
			sinceTs := since[chartKey] // 0 if not present = get all

			// Get points since last timestamp
			series := storage.GetSeriesSince(namespace, s.Name, s.Tags, agg, sinceTs)
			if series == nil || len(series.Points) == 0 {
				continue
			}

			points := make([]pointOutput, len(series.Points))
			for i, p := range series.Points {
				points[i] = pointOutput{
					Timestamp: p.Timestamp,
					Value:     sanitizeFloat(p.Value),
				}
			}

			resp.Series[chartKey] = seriesResponse{
				Namespace: series.Namespace,
				Name:      series.Name,
				Tags:      series.Tags,
				Points:    points,
			}
		}
	}

	data, err := json.Marshal(resp)
	if err != nil {
		log.Printf("[500] /api/series/batch: failed to marshal: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// correlationOutput is the JSON structure for correlation responses.
type correlationOutput struct {
	Pattern     string          `json:"pattern"`
	Title       string          `json:"title"`
	Sources     []string        `json:"sources"`
	Anomalies   []anomalyOutput `json:"anomalies"`
	FirstSeen   int64           `json:"firstSeen"`   // unix seconds (from data)
	LastUpdated int64           `json:"lastUpdated"` // unix seconds (from data)
}

// anomalyOutput is a JSON-serializable anomaly.
type anomalyOutput struct {
	Source      string   `json:"source"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Tags        []string `json:"tags"`
	Timestamp   int64    `json:"timestamp"`
	Score       *float64 `json:"score,omitempty"`
}

// rawAnomalyOutput is the JSON structure for raw anomaly API responses.
type rawAnomalyOutput struct {
	Source       string   `json:"source"`
	AnalyzerName string   `json:"analyzerName"`
	Title        string   `json:"title"`
	Description  string   `json:"description"`
	Tags         []string `json:"tags"`
	Timestamp    int64    `json:"timestamp"`
	Score        *float64 `json:"score,omitempty"`
}

// handleAPICorrelations returns currently active correlations.
func (r *HTMLReporter) handleAPICorrelations(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	correlationState := r.correlationState
	r.mu.RUnlock()

	var correlations []correlationOutput
	if correlationState != nil {
		active := correlationState.ActiveCorrelations()
		correlations = make([]correlationOutput, len(active))
		for i, ac := range active {
			anomalies := make([]anomalyOutput, len(ac.Anomalies))
			for j, a := range ac.Anomalies {
				anomalies[j] = anomalyOutput{
					Source:      string(a.Source),
					Title:       a.Title,
					Description: a.Description,
					Tags:        a.Tags,
					Timestamp:   a.Timestamp,
					Score:       a.Score,
				}
			}
			correlations[i] = correlationOutput{
				Pattern:     ac.Pattern,
				Title:       ac.Title,
				Sources:     seriesIDsToStringSlice(ac.MemberSeriesIDs),
				Anomalies:   anomalies,
				FirstSeen:   ac.FirstSeen,
				LastUpdated: ac.LastUpdated,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(correlations); err != nil {
		log.Printf("[500] /api/correlations: failed to encode: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func seriesIDsToStringSlice(ids []observer.SeriesID) []string {
	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = string(id)
	}
	return out
}

// handleAPIRawAnomalies returns all raw anomalies from TimeSeriesAnalysis implementations.
func (r *HTMLReporter) handleAPIRawAnomalies(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	rawState := r.rawAnomalyState
	r.mu.RUnlock()

	var anomalies []rawAnomalyOutput
	if rawState != nil {
		raw := rawState.RawAnomalies()
		anomalies = make([]rawAnomalyOutput, len(raw))
		for i, a := range raw {
			anomalies[i] = rawAnomalyOutput{
				Source:       string(a.Source),
				AnalyzerName: a.AnalyzerName,
				Title:        a.Title,
				Description:  a.Description,
				Tags:         a.Tags,
				Timestamp:    a.Timestamp,
				Score:        a.Score,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(anomalies); err != nil {
		log.Printf("[500] /api/raw-anomalies: failed to encode: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleGraphPage serves the dedicated GraphSketch visualization page.
func (r *HTMLReporter) handleGraphPage(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(graphPageHTML))
}

// handleAPIGraphSketchEdges returns learned edges from GraphSketchCorrelator.
func (r *HTMLReporter) handleAPIGraphSketchEdges(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	correlationState := r.correlationState
	r.mu.RUnlock()

	var edges []map[string]interface{}

	// Type-assert to GraphSketchCorrelator to access edges
	if gsCorrelator, ok := correlationState.(*GraphSketchCorrelator); ok {
		learnedEdges := gsCorrelator.GetLearnedEdges()
		edges = make([]map[string]interface{}, 0, len(learnedEdges))
		for _, e := range learnedEdges {
			edges = append(edges, map[string]interface{}{
				"source1":      e.Source1,
				"source2":      e.Source2,
				"observations": e.Observations,
				"frequency":    e.Frequency,
			})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(edges); err != nil {
		log.Printf("[500] /api/graphsketch/edges: failed to encode: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// handleAPIGraphSketchStats returns statistics from GraphSketchCorrelator.
func (r *HTMLReporter) handleAPIGraphSketchStats(w http.ResponseWriter, req *http.Request) {
	r.mu.RLock()
	correlationState := r.correlationState
	r.mu.RUnlock()

	stats := map[string]interface{}{
		"correlator_type": "unknown",
		"available":       false,
	}

	// Type-assert to GraphSketchCorrelator to access stats
	if gsCorrelator, ok := correlationState.(*GraphSketchCorrelator); ok {
		stats = gsCorrelator.GetStats()
		stats["correlator_type"] = "graphsketch"
		stats["available"] = true
	} else if correlationState != nil {
		stats["correlator_type"] = "other"
		stats["available"] = false
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(stats); err != nil {
		log.Printf("[500] /api/graphsketch/stats: failed to encode: %v", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// graphPageHTML is the dedicated graph visualization page.
const graphPageHTML = `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <title>GraphSketch Correlation Graph</title>
    <style>
        * { box-sizing: border-box; margin: 0; padding: 0; }
        body { 
            font-family: 'SF Mono', 'Monaco', 'Inconsolata', monospace;
            background: #0f172a; 
            color: #e2e8f0; 
            min-height: 100vh;
            padding: 20px;
        }
        a { color: #8b5cf6; text-decoration: none; font-size: 0.85em; }
        a:hover { text-decoration: underline; }
        h1 { color: #c4b5fd; font-size: 1.4em; margin: 8px 0 4px 0; }
        #stats { color: #6b7280; font-size: 0.85em; margin-bottom: 15px; }
        #graph-svg { 
            width: 100%; 
            height: 600px; 
            background: #1e293b; 
            border-radius: 8px;
        }
        .legend {
            display: flex;
            align-items: center;
            gap: 15px;
            padding: 10px 0;
            font-size: 0.8em;
            color: #9ca3af;
        }
        .legend-item { display: flex; align-items: center; gap: 5px; }
        .legend-line { width: 30px; height: 3px; }
        h2 { color: #f87171; font-size: 1.1em; margin: 20px 0 10px 0; }
        .edge-grid {
            display: grid;
            grid-template-columns: repeat(auto-fill, minmax(320px, 1fr));
            gap: 2px 15px;
            font-size: 0.8em;
        }
        .edge-row {
            display: flex;
            align-items: center;
            gap: 4px;
            padding: 2px 0;
            white-space: nowrap;
        }
        .edge-src { color: #9ca3af; max-width: 100px; overflow: hidden; text-overflow: ellipsis; }
        .edge-arrow { color: #6b7280; flex-shrink: 0; }
        .edge-freq { font-weight: bold; flex-shrink: 0; }
        .edge-raw { color: #4b5563; font-size: 0.9em; flex-shrink: 0; }
    </style>
</head>
<body>
    <a href="/">← Back to Dashboard</a>
    <h1>GraphSketch Correlation Graph</h1>
    <div id="stats">Loading...</div>
    <svg id="graph-svg"></svg>
    <div class="legend">
        <div class="legend-item"><div class="legend-line" style="background: #fbbf24;"></div> Weak</div>
        <div class="legend-item"><div class="legend-line" style="background: #f97316;"></div> Medium</div>
        <div class="legend-item"><div class="legend-line" style="background: #dc2626;"></div> Strong</div>
    </div>
    <h2>All Edges (sorted by strength)</h2>
    <div id="edge-grid" class="edge-grid"></div>

    <script>
        let edges = [];
        let stats = {};

        async function fetchData() {
            try {
                const [edgesResp, statsResp] = await Promise.all([
                    fetch('/api/graphsketch/edges'),
                    fetch('/api/graphsketch/stats')
                ]);
                edges = await edgesResp.json() || [];
                stats = await statsResp.json() || {};
                render();
            } catch (e) {
                console.error('Failed to fetch:', e);
            }
        }

        function render() {
            if (!stats.available) {
                document.getElementById('stats').textContent = 'GraphSketch Correlator not active. Use -graphsketch-correlator flag.';
                return;
            }
            
            document.getElementById('stats').textContent = 
                edges.length + ' edges, ' + (stats.unique_sources || 0) + ' sources, ' +
                (stats.total_observations || 0) + ' observations';
            
            renderGraph();
            renderEdgeGrid();
        }

        function renderGraph() {
            const svg = document.getElementById('graph-svg');
            if (!edges || edges.length === 0) {
                svg.innerHTML = '<text x="50%" y="50%" text-anchor="middle" fill="#6b7280">Waiting for co-occurring anomalies...</text>';
                return;
            }

            const rect = svg.getBoundingClientRect();
            const width = rect.width || 800;
            const height = 600;

            const nodeSet = new Set();
            edges.forEach(e => { nodeSet.add(e.source1); nodeSet.add(e.source2); });
            const nodes = Array.from(nodeSet);

            const centerX = width / 2;
            const centerY = height / 2;
            const radius = Math.min(width, height) * 0.38;
            const positions = {};
            nodes.forEach((node, i) => {
                const angle = (2 * Math.PI * i) / nodes.length - Math.PI / 2;
                positions[node] = {
                    x: centerX + radius * Math.cos(angle),
                    y: centerY + radius * Math.sin(angle),
                    label: node.split(':')[0]
                };
            });

            let maxFreq = 1;
            edges.forEach(e => { if (e.frequency > maxFreq) maxFreq = e.frequency; });

            const sortedEdges = [...edges].sort((a, b) => a.frequency - b.frequency);

            let svgContent = '';

            sortedEdges.forEach(edge => {
                const p1 = positions[edge.source1];
                const p2 = positions[edge.source2];
                if (!p1 || !p2) return;

                const ratio = edge.frequency / maxFreq;
                const thickness = 1 + ratio * ratio * 10;
                const opacity = 0.3 + ratio * ratio * 0.7;

                let r, g, b;
                if (ratio < 0.5) {
                    r = 251; g = Math.round(191 - ratio * 2 * 80); b = Math.round(36 - ratio * 2 * 19);
                } else {
                    const t = (ratio - 0.5) * 2;
                    r = Math.round(251 - t * 31); g = Math.round(111 - t * 73); b = Math.round(17 + t * 21);
                }

                svgContent += '<line x1="' + p1.x + '" y1="' + p1.y + '" x2="' + p2.x + '" y2="' + p2.y + '" ' +
                    'stroke="rgb(' + r + ',' + g + ',' + b + ')" stroke-width="' + thickness + '" stroke-opacity="' + opacity + '"/>';
            });

            nodes.forEach((node, i) => {
                const pos = positions[node];
                const angle = (2 * Math.PI * i) / nodes.length - Math.PI / 2;
                svgContent += '<circle cx="' + pos.x + '" cy="' + pos.y + '" r="12" fill="#8b5cf6" stroke="#fff" stroke-width="2"/>';
                
                const lx = centerX + (radius + 22) * Math.cos(angle);
                const ly = centerY + (radius + 22) * Math.sin(angle);
                const anchor = Math.cos(angle) > 0.3 ? 'start' : (Math.cos(angle) < -0.3 ? 'end' : 'middle');
                svgContent += '<text x="' + lx + '" y="' + ly + '" fill="#9ca3af" font-size="10" text-anchor="' + anchor + '" dominant-baseline="middle">' + pos.label + '</text>';
            });

            svg.innerHTML = svgContent;
        }

        function renderEdgeGrid() {
            const grid = document.getElementById('edge-grid');
            if (!edges || edges.length === 0) {
                grid.innerHTML = '';
                return;
            }

            const sortedEdges = [...edges].sort((a, b) => b.frequency - a.frequency);

            let maxFreq = 1;
            sortedEdges.forEach(e => { if (e.frequency > maxFreq) maxFreq = e.frequency; });

            // Shorten source names - get last meaningful part
            function shorten(s) {
                const parts = s.split(':');
                const name = parts[0];
                // Remove common prefixes
                return name.replace(/^(cgroup\.v2\.|smaps_rollup\.|smaps\.)/, '');
            }

            let html = '';
            sortedEdges.forEach(edge => {
                const pct = Math.min(100, (edge.frequency / maxFreq) * 100);
                const color = pct < 50 ? '#fbbf24' : (pct < 75 ? '#f97316' : '#dc2626');
                const s1 = shorten(edge.source1);
                const s2 = shorten(edge.source2);
                // Format frequency with appropriate precision
                const freqStr = edge.frequency >= 1000 ? edge.frequency.toFixed(1) : 
                               edge.frequency >= 100 ? edge.frequency.toFixed(2) : 
                               edge.frequency.toFixed(3);
                html += '<div class="edge-row">' +
                    '<span class="edge-src" title="' + edge.source1 + '">' + s1 + '</span>' +
                    '<span class="edge-arrow">—</span>' +
                    '<span class="edge-src" title="' + edge.source2 + '">' + s2 + '</span>' +
                    '<span class="edge-freq" style="color:' + color + '">' + freqStr + '</span>' +
                    '<span class="edge-raw">(' + edge.observations + 'x)</span>' +
                    '</div>';
            });
            grid.innerHTML = html;
        }

        fetchData();
        setInterval(fetchData, 3000);
    </script>
</body>
</html>
`

// handleTimeClusterPage serves the TimeCluster visualization page.
func (r *HTMLReporter) handleTimeClusterPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(timeClusterPageHTML))
}

// handleAPITimeClusterClusters returns all time clusters.
func (r *HTMLReporter) handleAPITimeClusterClusters(w http.ResponseWriter, _ *http.Request) {
	r.mu.RLock()
	tc := r.timeClusterCorrelator
	r.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if tc == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"clusters": []interface{}{},
			"error":    "TimeClusterCorrelator not enabled",
		})
		return
	}

	clusters := tc.GetClusters()
	json.NewEncoder(w).Encode(map[string]interface{}{
		"clusters": clusters,
	})
}

// handleAPITimeClusterStats returns TimeCluster statistics.
func (r *HTMLReporter) handleAPITimeClusterStats(w http.ResponseWriter, _ *http.Request) {
	r.mu.RLock()
	tc := r.timeClusterCorrelator
	r.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")

	if tc == nil {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"enabled": false,
			"error":   "TimeClusterCorrelator not enabled",
		})
		return
	}

	stats := tc.GetStats()
	stats["enabled"] = true
	json.NewEncoder(w).Encode(stats)
}

const timeClusterPageHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Time Cluster Correlator</title>
    <style>
        * { margin: 0; padding: 0; box-sizing: border-box; }
        body { 
            font-family: 'JetBrains Mono', 'Fira Code', monospace;
            background: #0f0f0f; 
            color: #e0e0e0; 
            min-height: 100vh;
            padding: 20px;
        }
        .header { 
            display: flex; 
            justify-content: space-between; 
            align-items: center;
            margin-bottom: 20px;
            padding-bottom: 15px;
            border-bottom: 1px solid #333;
        }
        h1 { 
            color: #22d3ee;
            font-size: 1.5em;
            font-weight: 600;
        }
        .nav { display: flex; gap: 15px; }
        .nav a { 
            color: #888; 
            text-decoration: none;
            font-size: 0.85em;
            transition: color 0.2s;
        }
        .nav a:hover { color: #22d3ee; }
        .stats-bar {
            display: flex;
            gap: 30px;
            margin-bottom: 25px;
            padding: 15px;
            background: #1a1a1a;
            border-radius: 8px;
        }
        .stat { display: flex; flex-direction: column; }
        .stat-label { color: #666; font-size: 0.75em; margin-bottom: 3px; }
        .stat-value { color: #22d3ee; font-size: 1.4em; font-weight: 600; }
        .container { display: flex; gap: 25px; flex-wrap: wrap; }
        .panel {
            flex: 1;
            min-width: 400px;
            background: #1a1a1a;
            border-radius: 8px;
            padding: 20px;
            max-height: 80vh;
            overflow-y: auto;
        }
        .panel-title {
            color: #888;
            font-size: 0.85em;
            margin-bottom: 15px;
            text-transform: uppercase;
            letter-spacing: 1px;
        }
        .cluster {
            background: #222;
            border-radius: 6px;
            padding: 15px;
            margin-bottom: 15px;
            border-left: 3px solid #22d3ee;
        }
        .cluster-header {
            display: flex;
            justify-content: space-between;
            align-items: center;
            margin-bottom: 10px;
        }
        .cluster-id {
            color: #22d3ee;
            font-weight: 600;
            font-size: 0.9em;
        }
        .cluster-count {
            background: #22d3ee;
            color: #0f0f0f;
            padding: 2px 8px;
            border-radius: 10px;
            font-size: 0.75em;
            font-weight: 600;
        }
        .cluster-time {
            color: #666;
            font-size: 0.75em;
            margin-bottom: 10px;
        }
        .cluster-sources {
            display: flex;
            flex-wrap: wrap;
            gap: 5px;
        }
        .source-tag {
            background: #333;
            color: #9ca3af;
            padding: 3px 8px;
            border-radius: 4px;
            font-size: 0.7em;
        }
        .timeline {
            background: #222;
            border-radius: 6px;
            padding: 20px;
            margin-bottom: 20px;
        }
        .timeline-bar {
            height: 120px;
            background: #1a1a1a;
            border-radius: 4px;
            position: relative;
            margin-top: 10px;
        }
        .timeline-cluster {
            position: absolute;
            height: 100%;
            background: rgba(34, 211, 238, 0.3);
            border: 1px solid #22d3ee;
            border-radius: 3px;
            display: flex;
            align-items: center;
            justify-content: center;
            font-size: 0.7em;
            color: #22d3ee;
            overflow: hidden;
            cursor: pointer;
        }
        .timeline-cluster:hover {
            background: rgba(34, 211, 238, 0.5);
        }
        .empty-state {
            color: #555;
            text-align: center;
            padding: 50px 20px;
            font-style: italic;
        }
    </style>
</head>
<body>
    <div class="header">
        <h1>Time Cluster Correlator</h1>
        <nav class="nav">
            <a href="/">Dashboard</a>
            <a href="/graphsketch-correlation">GraphSketch</a>
            <a href="/timecluster">TimeCluster</a>
        </nav>
    </div>

    <div class="stats-bar" id="stats">
        <div class="stat">
            <span class="stat-label">Total Clusters</span>
            <span class="stat-value" id="total-clusters">—</span>
        </div>
        <div class="stat">
            <span class="stat-label">Total Anomalies</span>
            <span class="stat-value" id="total-anomalies">—</span>
        </div>
        <div class="stat">
            <span class="stat-label">Slack (seconds)</span>
            <span class="stat-value" id="slack-seconds">—</span>
        </div>
        <div class="stat">
            <span class="stat-label">Window (seconds)</span>
            <span class="stat-value" id="window-seconds">—</span>
        </div>
        <div class="stat">
            <span class="stat-label">Largest Cluster</span>
            <span class="stat-value" id="largest-cluster">—</span>
        </div>
    </div>

    <div class="timeline" id="timeline-container">
        <div class="panel-title">Timeline</div>
        <div class="timeline-bar" id="timeline"></div>
    </div>

    <div class="container">
        <div class="panel">
            <div class="panel-title">Clusters (by size)</div>
            <div id="cluster-list"></div>
        </div>
    </div>

    <script>
        let clusters = [];
        let stats = {};

        async function fetchData() {
            try {
                const [statsRes, clustersRes] = await Promise.all([
                    fetch('/api/timecluster/stats'),
                    fetch('/api/timecluster/clusters')
                ]);
                stats = await statsRes.json();
                const clusterData = await clustersRes.json();

                if (stats.enabled === false) {
                    document.getElementById('stats').innerHTML = '<div class="empty-state">TimeClusterCorrelator not enabled. Run with -time-cluster flag.</div>';
                    return;
                }

                document.getElementById('total-clusters').textContent = stats.total_clusters || 0;
                document.getElementById('total-anomalies').textContent = stats.total_anomalies || 0;
                document.getElementById('slack-seconds').textContent = stats.slack_seconds || 0;
                document.getElementById('window-seconds').textContent = stats.window_seconds || 0;
                document.getElementById('largest-cluster').textContent = stats.largest_cluster_size || 0;

                clusters = clusterData.clusters || [];
                renderTimeline();
                renderClusterList();
            } catch (e) {
                console.error('Failed to fetch TimeCluster data:', e);
            }
        }

        function renderTimeline() {
            const timeline = document.getElementById('timeline');
            
            if (!clusters || clusters.length === 0) {
                timeline.innerHTML = '<div style="color:#555;text-align:center;padding:20px;">No clusters yet (min 2 anomalies)</div>';
                return;
            }

            // Find time range and max anomaly count
            let minTime = Infinity, maxTime = -Infinity, maxCount = 1;
            clusters.forEach(c => {
                if (c.start_time < minTime) minTime = c.start_time;
                if (c.end_time > maxTime) maxTime = c.end_time;
                if (c.anomaly_count > maxCount) maxCount = c.anomaly_count;
            });

            const range = maxTime - minTime || 1;

            let html = '';
            clusters.forEach((c, i) => {
                const left = ((c.start_time - minTime) / range) * 100;
                // Width: based on time span, minimum 3%
                const w = Math.max(3, ((c.end_time - c.start_time) / range) * 100);
                // Height: proportional to anomaly count (30% to 100%)
                const sizeFactor = c.anomaly_count / maxCount;
                const heightPct = 30 + sizeFactor * 70;
                // Color: intensity based on size (more anomalies = more vivid cyan)
                const saturation = 50 + sizeFactor * 40;
                const lightness = 55 - sizeFactor * 15;
                html += '<div class="timeline-cluster" style="left:' + left + '%;width:' + w + '%;height:' + heightPct + '%;top:' + (100 - heightPct) / 2 + '%;background:hsla(190,' + saturation + '%,' + lightness + '%,0.8);border-color:hsl(190,' + saturation + '%,' + (lightness - 10) + '%);font-size:' + (10 + sizeFactor * 6) + 'px;font-weight:' + (sizeFactor > 0.5 ? 'bold' : 'normal') + '" title="Cluster ' + c.id + ': ' + c.anomaly_count + ' anomalies (' + (c.end_time - c.start_time) + 's)">' + c.anomaly_count + '</div>';
            });
            timeline.innerHTML = html;
        }

        function renderClusterList() {
            const list = document.getElementById('cluster-list');
            
            if (!clusters || clusters.length === 0) {
                list.innerHTML = '<div class="empty-state">No clusters detected yet</div>';
                return;
            }

            function formatTime(unix) {
                const d = new Date(unix * 1000);
                return d.toLocaleTimeString();
            }

            function shorten(s) {
                const parts = s.split(':');
                const name = parts[0];
                return name.replace(/^(cgroup\.v2\.|smaps_rollup\.|smaps\.)/, '');
            }

            let html = '';
            clusters.forEach(c => {
                html += '<div class="cluster">';
                html += '<div class="cluster-header">';
                html += '<span class="cluster-id">Cluster #' + c.id + '</span>';
                html += '<span class="cluster-count">' + c.anomaly_count + ' anomalies</span>';
                html += '</div>';
                html += '<div class="cluster-time">' + formatTime(c.start_time) + ' → ' + formatTime(c.end_time) + ' (' + (c.end_time - c.start_time) + 's)</div>';
                html += '<div class="cluster-sources">';
                c.sources.slice(0, 20).forEach(s => {
                    html += '<span class="source-tag" title="' + s + '">' + shorten(s) + '</span>';
                });
                if (c.sources.length > 20) {
                    html += '<span class="source-tag">+' + (c.sources.length - 20) + ' more</span>';
                }
                html += '</div>';
                html += '</div>';
            });
            list.innerHTML = html;
        }

        fetchData();
        setInterval(fetchData, 2000);
    </script>
</body>
</html>
`
