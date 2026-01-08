// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
)

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
	mu               sync.RWMutex
	reports          []timestampedReport
	storage          *timeSeriesStorage
	correlationState observer.CorrelationState
	server           *http.Server
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

// Start starts the HTTP server on the given address.
func (r *HTMLReporter) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", r.handleDashboard)
	mux.HandleFunc("/api/reports", r.handleAPIReports)
	mux.HandleFunc("/api/series", r.handleAPISeries)
	mux.HandleFunc("/api/series/list", r.handleAPISeriesList)
	mux.HandleFunc("/api/correlations", r.handleAPICorrelations)

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
    <div class="header">
        <h1>Observer Demo Dashboard</h1>
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
        let anomalyRanges = {}; // chartKey -> [{start, end}, ...]
        let correlationData = []; // list of correlations for timeline

        // Build a map of chart key -> anomaly time ranges from correlations
        function buildAnomalyRanges(correlations) {
            const ranges = {};
            for (const c of correlations || []) {
                for (const a of c.anomalies || []) {
                    const key = a.source;
                    if (!ranges[key]) ranges[key] = [];
                    if (a.timeRange && a.timeRange.start && a.timeRange.end) {
                        ranges[key].push({
                            start: a.timeRange.start,
                            end: a.timeRange.end
                        });
                    }
                }
            }
            return ranges;
        }

        async function fetchCorrelations() {
            try {
                const resp = await fetch('/api/correlations');
                const correlations = await resp.json();
                anomalyRanges = buildAnomalyRanges(correlations);
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

        async function fetchSeriesList() {
            try {
                const resp = await fetch('/api/series/list?namespace=demo');
                return await resp.json();
            } catch (e) {
                console.error('Failed to fetch series list:', e);
                return [];
            }
        }

        async function fetchSeries(name, agg) {
            try {
                const resp = await fetch('/api/series?namespace=demo&name=' + encodeURIComponent(name) + '&agg=' + agg);
                if (!resp.ok) return null;
                return await resp.json();
            } catch (e) {
                console.error('Failed to fetch series:', e);
                return null;
            }
        }

        // Aggregations that are analyzed (must match analysisAggregations in observer.go)
        const analysisAggregations = ['avg', 'count'];

        // Build Chart.js annotations from anomaly ranges for a given chart key
        function buildAnnotations(timestamps, ranges) {
            if (!ranges || ranges.length === 0 || !timestamps || timestamps.length === 0) {
                return {};
            }

            const annotations = {};
            let idx = 0;

            for (const range of ranges) {
                let startIdx = -1;
                let endIdx = -1;

                for (let i = 0; i < timestamps.length; i++) {
                    const ts = timestamps[i];
                    if (ts >= range.start && startIdx === -1) {
                        startIdx = i;
                    }
                    if (ts <= range.end) {
                        endIdx = i;
                    }
                }

                if (startIdx !== -1 && endIdx !== -1 && startIdx <= endIdx) {
                    annotations['box' + idx] = {
                        type: 'box',
                        xMin: startIdx,
                        xMax: endIdx,
                        backgroundColor: 'rgba(239, 68, 68, 0.15)',
                        borderColor: 'transparent'
                    };
                    annotations['lineStart' + idx] = {
                        type: 'line',
                        xMin: startIdx,
                        xMax: startIdx,
                        borderColor: 'rgba(239, 68, 68, 0.6)',
                        borderWidth: 2,
                        borderDash: [4, 4]
                    };
                    annotations['lineEnd' + idx] = {
                        type: 'line',
                        xMin: endIdx,
                        xMax: endIdx,
                        borderColor: 'rgba(239, 68, 68, 0.6)',
                        borderWidth: 2,
                        borderDash: [4, 4]
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
            const seriesList = await fetchSeriesList();
            const anomalousSources = new Set(Object.keys(anomalyRanges));

            // If no anomalies, show placeholder in anomaly section
            if (anomalousSources.size === 0 && Object.keys(anomalyCharts).length === 0) {
                const container = document.getElementById('anomaly-charts');
                if (!container.querySelector('.no-anomalies')) {
                    container.innerHTML = '<div class="no-anomalies">No anomalies detected yet</div>';
                }
            }

            for (const item of seriesList) {
                for (const agg of analysisAggregations) {
                    const chartKey = item.name + ':' + agg;
                    const series = await fetchSeries(item.name, agg);

                    // Create/update small chart for ALL metrics
                    const smallChartId = 'all-chart-' + chartKey.replace(/[^a-z0-9]/gi, '-');
                    if (!allCharts[chartKey]) {
                        allCharts[chartKey] = createChart(chartKey, smallChartId, '#all-charts', true);
                    }
                    if (series && series.points) {
                        const chart = allCharts[chartKey];
                        const timestamps = series.points.map(p => p.timestamp);
                        chart.data.labels = series.points.map(p => {
                            const d = new Date(p.timestamp * 1000);
                            return d.toLocaleTimeString();
                        });
                        chart.data.datasets[0].data = series.points.map(p => p.value);
                        chart.timestamps = timestamps;

                        const ranges = anomalyRanges[chartKey] || [];
                        chart.options.plugins.annotation.annotations = buildAnnotations(timestamps, ranges);
                        chart.update('none');
                    }

                    // Create/update large chart only for ANOMALOUS metrics
                    if (anomalousSources.has(chartKey)) {
                        const largeChartId = 'anomaly-chart-' + chartKey.replace(/[^a-z0-9]/gi, '-');
                        if (!anomalyCharts[chartKey]) {
                            anomalyCharts[chartKey] = createChart(chartKey, largeChartId, '#anomaly-charts', false);
                        }
                        if (series && series.points) {
                            const chart = anomalyCharts[chartKey];
                            const timestamps = series.points.map(p => p.timestamp);
                            chart.data.labels = series.points.map(p => {
                                const d = new Date(p.timestamp * 1000);
                                return d.toLocaleTimeString();
                            });
                            chart.data.datasets[0].data = series.points.map(p => p.value);
                            chart.timestamps = timestamps;

                            const ranges = anomalyRanges[chartKey] || [];
                            chart.options.plugins.annotation.annotations = buildAnnotations(timestamps, ranges);
                            chart.update('none');
                        }
                    }
                }
            }

            document.getElementById('status').textContent = 'Last update: ' + new Date().toLocaleTimeString();
        }

        function escapeHtml(text) {
            const div = document.createElement('div');
            div.textContent = text;
            return div.innerHTML;
        }

        // Initial load and periodic refresh
        fetchCorrelations();
        updateCharts();
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
func (r *HTMLReporter) handleAPISeries(w http.ResponseWriter, req *http.Request) {
	name := req.URL.Query().Get("name")
	namespace := req.URL.Query().Get("namespace")
	agg := parseAggregate(req.URL.Query().Get("agg"))

	if name == "" || namespace == "" {
		http.Error(w, "missing required parameters: name and namespace", http.StatusBadRequest)
		return
	}

	r.mu.RLock()
	storage := r.storage
	r.mu.RUnlock()

	if storage == nil {
		http.Error(w, "storage not configured", http.StatusServiceUnavailable)
		return
	}

	series := storage.GetSeries(namespace, name, nil, agg)
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
			Value:     p.Value,
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	if err := json.NewEncoder(w).Encode(resp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

// correlationOutput is the JSON structure for correlation responses.
type correlationOutput struct {
	Pattern     string          `json:"pattern"`
	Title       string          `json:"title"`
	Signals     []string        `json:"signals"`
	Anomalies   []anomalyOutput `json:"anomalies"`
	FirstSeen   int64           `json:"firstSeen"`   // unix seconds (from data)
	LastUpdated int64           `json:"lastUpdated"` // unix seconds (from data)
}

// timeRangeOutput is a JSON-serializable time range.
type timeRangeOutput struct {
	Start int64 `json:"start"`
	End   int64 `json:"end"`
}

// anomalyOutput is a JSON-serializable anomaly.
type anomalyOutput struct {
	Source      string          `json:"source"`
	Title       string          `json:"title"`
	Description string          `json:"description"`
	Tags        []string        `json:"tags"`
	TimeRange   timeRangeOutput `json:"timeRange"`
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
					Source:      a.Source,
					Title:       a.Title,
					Description: a.Description,
					Tags:        a.Tags,
					TimeRange: timeRangeOutput{
						Start: a.TimeRange.Start,
						End:   a.TimeRange.End,
					},
				}
			}
			correlations[i] = correlationOutput{
				Pattern:     ac.Pattern,
				Title:       ac.Title,
				Signals:     ac.Signals,
				Anomalies:   anomalies,
				FirstSeen:   ac.FirstSeen,
				LastUpdated: ac.LastUpdated,
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(correlations); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}
