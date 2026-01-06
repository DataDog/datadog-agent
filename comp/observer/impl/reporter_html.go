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
	mu      sync.RWMutex
	reports []timestampedReport
	storage observer.MetricStorageReader
	server  *http.Server
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

// SetStorage sets the metric storage reader for querying series data.
func (r *HTMLReporter) SetStorage(storage observer.MetricStorageReader) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.storage = storage
}

// Start starts the HTTP server on the given address.
func (r *HTMLReporter) Start(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", r.handleDashboard)
	mux.HandleFunc("/api/reports", r.handleAPIReports)
	mux.HandleFunc("/api/series", r.handleAPISeries)

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

	r.mu.RLock()
	reports := make([]timestampedReport, len(r.reports))
	copy(reports, r.reports)
	r.mu.RUnlock()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)

	html := `<!DOCTYPE html>
<html>
<head>
    <meta charset="utf-8">
    <meta http-equiv="refresh" content="2">
    <title>Observer Demo Dashboard</title>
    <style>
        body {
            font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, sans-serif;
            margin: 0;
            padding: 20px;
            background-color: #f5f5f5;
        }
        h1 {
            color: #333;
            border-bottom: 2px solid #632ca6;
            padding-bottom: 10px;
        }
        .report-list {
            list-style: none;
            padding: 0;
        }
        .report-item {
            background: white;
            border: 1px solid #ddd;
            border-radius: 4px;
            padding: 15px;
            margin-bottom: 10px;
            box-shadow: 0 1px 3px rgba(0,0,0,0.1);
        }
        .report-title {
            font-weight: bold;
            color: #632ca6;
            margin-bottom: 5px;
        }
        .report-body {
            color: #555;
            margin-bottom: 10px;
        }
        .report-meta {
            font-size: 0.85em;
            color: #888;
        }
        .report-timestamp {
            font-size: 0.85em;
            color: #888;
            float: right;
        }
        .metadata-item {
            display: inline-block;
            background: #eee;
            padding: 2px 6px;
            border-radius: 3px;
            margin-right: 5px;
        }
        .no-reports {
            color: #888;
            font-style: italic;
        }
    </style>
</head>
<body>
    <h1>Observer Demo Dashboard</h1>
`

	if len(reports) == 0 {
		html += `    <p class="no-reports">No reports yet.</p>
`
	} else {
		html += `    <ul class="report-list">
`
		for _, report := range reports {
			html += `        <li class="report-item">
            <span class="report-timestamp">` + report.Timestamp.Format(time.RFC3339) + `</span>
            <div class="report-title">` + escapeHTML(report.Title) + `</div>
            <div class="report-body">` + escapeHTML(report.Body) + `</div>
`
			if len(report.Metadata) > 0 {
				html += `            <div class="report-meta">`
				for k, v := range report.Metadata {
					html += `<span class="metadata-item">` + escapeHTML(k) + `=` + escapeHTML(v) + `</span>`
				}
				html += `</div>
`
			}
			html += `        </li>
`
		}
		html += `    </ul>
`
	}

	html += `</body>
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

// handleAPISeries returns JSON series data.
func (r *HTMLReporter) handleAPISeries(w http.ResponseWriter, req *http.Request) {
	name := req.URL.Query().Get("name")
	namespace := req.URL.Query().Get("namespace")

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

	series := storage.GetSeries(namespace, name, nil)
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
