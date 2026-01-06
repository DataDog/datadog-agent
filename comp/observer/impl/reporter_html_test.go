// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observerimpl

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	observer "github.com/DataDog/datadog-agent/comp/observer/def"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHTMLReporter_Name(t *testing.T) {
	r := NewHTMLReporter()
	assert.Equal(t, "html_reporter", r.Name())
}

func TestHTMLReporter_Report_AddsToBuffer(t *testing.T) {
	r := NewHTMLReporter()

	r.Report(observer.ReportOutput{
		Title: "Test Report",
		Body:  "Test Body",
		Metadata: map[string]string{
			"key": "value",
		},
	})

	r.mu.RLock()
	defer r.mu.RUnlock()

	require.Len(t, r.reports, 1)
	assert.Equal(t, "Test Report", r.reports[0].Title)
	assert.Equal(t, "Test Body", r.reports[0].Body)
	assert.Equal(t, "value", r.reports[0].Metadata["key"])
	assert.False(t, r.reports[0].Timestamp.IsZero())
}

func TestHTMLReporter_Report_MostRecentFirst(t *testing.T) {
	r := NewHTMLReporter()

	r.Report(observer.ReportOutput{Title: "First"})
	r.Report(observer.ReportOutput{Title: "Second"})
	r.Report(observer.ReportOutput{Title: "Third"})

	r.mu.RLock()
	defer r.mu.RUnlock()

	require.Len(t, r.reports, 3)
	assert.Equal(t, "Third", r.reports[0].Title)
	assert.Equal(t, "Second", r.reports[1].Title)
	assert.Equal(t, "First", r.reports[2].Title)
}

func TestHTMLReporter_Report_BufferLimitedTo100(t *testing.T) {
	r := NewHTMLReporter()

	// Add 105 reports
	for i := 0; i < 105; i++ {
		r.Report(observer.ReportOutput{
			Title: "Report",
		})
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	assert.Len(t, r.reports, 100)
}

func TestHTMLReporter_Report_OldestEvicted(t *testing.T) {
	r := NewHTMLReporter()

	// Add 100 reports
	for i := 0; i < 100; i++ {
		r.Report(observer.ReportOutput{
			Title: "Old",
		})
	}

	// Add one more
	r.Report(observer.ReportOutput{
		Title: "New",
	})

	r.mu.RLock()
	defer r.mu.RUnlock()

	// Should have 100 reports with "New" at the front
	require.Len(t, r.reports, 100)
	assert.Equal(t, "New", r.reports[0].Title)
	// Last one should still be "Old" (oldest kept)
	assert.Equal(t, "Old", r.reports[99].Title)
}

func TestHTMLReporter_Dashboard_ReturnsHTML(t *testing.T) {
	r := NewHTMLReporter()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	r.handleDashboard(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Header().Get("Content-Type"), "text/html")
	assert.Contains(t, rec.Body.String(), "Observer Demo Dashboard")
	assert.Contains(t, rec.Body.String(), `<meta http-equiv="refresh" content="2">`)
}

func TestHTMLReporter_Dashboard_ShowsReports(t *testing.T) {
	r := NewHTMLReporter()

	r.Report(observer.ReportOutput{
		Title: "Test Title",
		Body:  "Test Body",
		Metadata: map[string]string{
			"env": "prod",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	r.handleDashboard(rec, req)

	body := rec.Body.String()
	assert.Contains(t, body, "Test Title")
	assert.Contains(t, body, "Test Body")
	assert.Contains(t, body, "env=prod")
}

func TestHTMLReporter_Dashboard_EscapesHTML(t *testing.T) {
	r := NewHTMLReporter()

	r.Report(observer.ReportOutput{
		Title: "<script>alert('xss')</script>",
		Body:  "Body with <b>html</b>",
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	r.handleDashboard(rec, req)

	body := rec.Body.String()
	assert.NotContains(t, body, "<script>")
	assert.Contains(t, body, "&lt;script&gt;")
}

func TestHTMLReporter_Dashboard_NotFound(t *testing.T) {
	r := NewHTMLReporter()

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	rec := httptest.NewRecorder()

	r.handleDashboard(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTMLReporter_APIReports_ReturnsJSON(t *testing.T) {
	r := NewHTMLReporter()

	r.Report(observer.ReportOutput{
		Title: "Test Report",
		Body:  "Test Body",
		Metadata: map[string]string{
			"key": "value",
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	rec := httptest.NewRecorder()

	r.handleAPIReports(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var reports []timestampedReport
	err := json.Unmarshal(rec.Body.Bytes(), &reports)
	require.NoError(t, err)

	require.Len(t, reports, 1)
	assert.Equal(t, "Test Report", reports[0].Title)
	assert.Equal(t, "Test Body", reports[0].Body)
	assert.Equal(t, "value", reports[0].Metadata["key"])
}

func TestHTMLReporter_APIReports_EmptyArray(t *testing.T) {
	r := NewHTMLReporter()

	req := httptest.NewRequest(http.MethodGet, "/api/reports", nil)
	rec := httptest.NewRecorder()

	r.handleAPIReports(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)

	var reports []timestampedReport
	err := json.Unmarshal(rec.Body.Bytes(), &reports)
	require.NoError(t, err)
	assert.Len(t, reports, 0)
}

// mockStorageReader implements MetricStorageReader for testing.
type mockStorageReader struct {
	series map[string]*observer.Series
}

func (m *mockStorageReader) GetSeries(namespace, name string, tags []string) *observer.Series {
	key := namespace + "|" + name
	return m.series[key]
}

func (m *mockStorageReader) AllSeries(namespace string) []observer.Series {
	var result []observer.Series
	for _, s := range m.series {
		if s.Namespace == namespace {
			result = append(result, *s)
		}
	}
	return result
}

func TestHTMLReporter_APISeries_ReturnsJSON(t *testing.T) {
	r := NewHTMLReporter()

	storage := &mockStorageReader{
		series: map[string]*observer.Series{
			"test|my.metric": {
				Namespace: "test",
				Name:      "my.metric",
				Tags:      []string{"env:prod"},
				Points: []observer.Point{
					{Timestamp: 1000, Value: 10.5},
					{Timestamp: 1001, Value: 20.5},
				},
			},
		},
	}
	r.SetStorage(storage)

	req := httptest.NewRequest(http.MethodGet, "/api/series?namespace=test&name=my.metric", nil)
	rec := httptest.NewRecorder()

	r.handleAPISeries(rec, req)

	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))

	var resp seriesResponse
	err := json.Unmarshal(rec.Body.Bytes(), &resp)
	require.NoError(t, err)

	assert.Equal(t, "test", resp.Namespace)
	assert.Equal(t, "my.metric", resp.Name)
	assert.Equal(t, []string{"env:prod"}, resp.Tags)
	require.Len(t, resp.Points, 2)
	assert.Equal(t, int64(1000), resp.Points[0].Timestamp)
	assert.Equal(t, 10.5, resp.Points[0].Value)
}

func TestHTMLReporter_APISeries_MissingParams(t *testing.T) {
	r := NewHTMLReporter()

	tests := []struct {
		name string
		url  string
	}{
		{"missing both", "/api/series"},
		{"missing name", "/api/series?namespace=test"},
		{"missing namespace", "/api/series?name=metric"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.url, nil)
			rec := httptest.NewRecorder()

			r.handleAPISeries(rec, req)

			assert.Equal(t, http.StatusBadRequest, rec.Code)
		})
	}
}

func TestHTMLReporter_APISeries_NoStorage(t *testing.T) {
	r := NewHTMLReporter()

	req := httptest.NewRequest(http.MethodGet, "/api/series?namespace=test&name=metric", nil)
	rec := httptest.NewRecorder()

	r.handleAPISeries(rec, req)

	assert.Equal(t, http.StatusServiceUnavailable, rec.Code)
}

func TestHTMLReporter_APISeries_NotFound(t *testing.T) {
	r := NewHTMLReporter()

	storage := &mockStorageReader{
		series: map[string]*observer.Series{},
	}
	r.SetStorage(storage)

	req := httptest.NewRequest(http.MethodGet, "/api/series?namespace=test&name=nonexistent", nil)
	rec := httptest.NewRecorder()

	r.handleAPISeries(rec, req)

	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestHTMLReporter_StartStop(t *testing.T) {
	r := NewHTMLReporter()

	// Start on a random available port
	err := r.Start("127.0.0.1:0")
	require.NoError(t, err)

	// Give server time to start
	// We can't easily make requests because we don't know the port
	// but we can verify Stop works without error
	err = r.Stop()
	assert.NoError(t, err)
}

func TestHTMLReporter_Stop_NoServer(t *testing.T) {
	r := NewHTMLReporter()

	// Stop without Start should not error
	err := r.Stop()
	assert.NoError(t, err)
}

func TestHTMLReporter_IntegrationWithHTTPServer(t *testing.T) {
	r := NewHTMLReporter()

	// Add test data
	r.Report(observer.ReportOutput{
		Title:    "Integration Test",
		Body:     "Testing full HTTP stack",
		Metadata: map[string]string{"test": "true"},
	})

	storage := &mockStorageReader{
		series: map[string]*observer.Series{
			"demo|cpu.usage": {
				Namespace: "demo",
				Name:      "cpu.usage",
				Points:    []observer.Point{{Timestamp: 1000, Value: 50.0}},
			},
		},
	}
	r.SetStorage(storage)

	// Create test server using the handler
	mux := http.NewServeMux()
	mux.HandleFunc("/", r.handleDashboard)
	mux.HandleFunc("/api/reports", r.handleAPIReports)
	mux.HandleFunc("/api/series", r.handleAPISeries)
	server := httptest.NewServer(mux)
	defer server.Close()

	// Test dashboard
	resp, err := http.Get(server.URL + "/")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.True(t, strings.HasPrefix(resp.Header.Get("Content-Type"), "text/html"))

	// Test API reports
	resp, err = http.Get(server.URL + "/api/reports")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))

	// Test API series
	resp, err = http.Get(server.URL + "/api/series?namespace=demo&name=cpu.usage")
	require.NoError(t, err)
	defer resp.Body.Close()
	assert.Equal(t, http.StatusOK, resp.StatusCode)
	assert.Equal(t, "application/json", resp.Header.Get("Content-Type"))
}

func TestEscapeHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"hello", "hello"},
		{"<script>", "&lt;script&gt;"},
		{"a & b", "a &amp; b"},
		{`"quoted"`, "&quot;quoted&quot;"},
		{"it's", "it&#39;s"},
		{"<b>bold</b>", "&lt;b&gt;bold&lt;/b&gt;"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, escapeHTML(tt.input))
		})
	}
}
