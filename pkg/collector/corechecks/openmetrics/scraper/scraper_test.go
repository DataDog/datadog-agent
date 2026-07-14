// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package scraper

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics/servicecheck"
)

const testPrometheusPayload = `# TYPE go_goroutines gauge
go_goroutines 42
# TYPE http_requests_total counter
http_requests_total{method="GET"} 100
`

func newTestServer(payload string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(payload))
	}))
}

func newTestConfig(endpoint string) *Config {
	cfg := &Config{
		OpenMetricsEndpoint: endpoint,
		Namespace:           "test",
		Metrics:             []interface{}{".*"},
	}
	cfg.Resolve()
	return cfg
}

func TestScrapeBasicMetrics(t *testing.T) {
	srv := newTestServer(testPrometheusPayload)
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	scraper, err := NewScraper(cfg)
	require.NoError(t, err)

	sndr := &recordingSender{}
	err = scraper.Scrape(sndr)
	require.NoError(t, err)

	// Gauge: go_goroutines should be submitted as test.go_goroutines
	gauges := findCalls(sndr.calls, "Gauge", "test.go_goroutines")
	assert.Len(t, gauges, 1, "expected one gauge submission for go_goroutines")
	assert.Equal(t, 42.0, gauges[0].value)

	// Counter: http_requests_total should be submitted as test.http_requests_total.count
	counters := findCalls(sndr.calls, "MonotonicCount", "test.http_requests_total.count")
	assert.Len(t, counters, 1, "expected one monotonic count submission for http_requests_total")
	assert.Equal(t, 100.0, counters[0].value)
	assert.Contains(t, counters[0].tags, "method:GET")
}

func TestScrapeHealthServiceCheckOK(t *testing.T) {
	srv := newTestServer(testPrometheusPayload)
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	scraper, err := NewScraper(cfg)
	require.NoError(t, err)

	sndr := &recordingSender{}
	err = scraper.Scrape(sndr)
	require.NoError(t, err)

	require.Len(t, sndr.serviceChecks, 1)
	assert.Equal(t, serviceCheckHealth, sndr.serviceChecks[0].name)
	assert.Equal(t, servicecheck.ServiceCheckOK, sndr.serviceChecks[0].status)
	assert.Empty(t, sndr.serviceChecks[0].message)
}

func TestScrapeHealthServiceCheckCriticalOnFailure(t *testing.T) {
	// Point to a server that is not running.
	cfg := newTestConfig("http://127.0.0.1:1/metrics")
	scraper, err := NewScraper(cfg)
	require.NoError(t, err)

	sndr := &recordingSender{}
	err = scraper.Scrape(sndr)
	assert.Error(t, err, "scrape should return an error on connection failure")

	require.Len(t, sndr.serviceChecks, 1)
	assert.Equal(t, serviceCheckHealth, sndr.serviceChecks[0].name)
	assert.Equal(t, servicecheck.ServiceCheckCritical, sndr.serviceChecks[0].status)
	assert.NotEmpty(t, sndr.serviceChecks[0].message)
}

func TestScrapeExcludeMetrics(t *testing.T) {
	srv := newTestServer(testPrometheusPayload)
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	cfg.ExcludeMetrics = []string{"go_goroutines"}
	// Re-resolve after modifying config to apply changes.
	cfg.Resolve()

	scraper, err := NewScraper(cfg)
	require.NoError(t, err)

	sndr := &recordingSender{}
	err = scraper.Scrape(sndr)
	require.NoError(t, err)

	// go_goroutines should be excluded.
	gauges := findCalls(sndr.calls, "Gauge", "test.go_goroutines")
	assert.Len(t, gauges, 0, "go_goroutines should be excluded")

	// http_requests_total should still be present.
	counters := findCalls(sndr.calls, "MonotonicCount", "test.http_requests_total.count")
	assert.Len(t, counters, 1, "http_requests_total should not be excluded")
}

func TestScrapeNamespacePrefixing(t *testing.T) {
	payload := `# TYPE up gauge
up 1
`
	srv := newTestServer(payload)
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	cfg.Namespace = "myapp"
	cfg.Resolve()

	scraper, err := NewScraper(cfg)
	require.NoError(t, err)

	sndr := &recordingSender{}
	err = scraper.Scrape(sndr)
	require.NoError(t, err)

	gauges := findCalls(sndr.calls, "Gauge", "myapp.up")
	assert.Len(t, gauges, 1, "metric should be prefixed with the namespace")
	assert.Equal(t, 1.0, gauges[0].value)

	// Ensure no metric without namespace exists.
	unprefixed := findCalls(sndr.calls, "Gauge", "up")
	assert.Len(t, unprefixed, 0, "metric should not appear without namespace prefix")
}

func TestScrapeEmptyNamespace(t *testing.T) {
	payload := `# TYPE temperature gauge
temperature 23.5
`
	srv := newTestServer(payload)
	defer srv.Close()

	cfg := newTestConfig(srv.URL)
	cfg.Namespace = ""
	cfg.Resolve()

	scraper, err := NewScraper(cfg)
	require.NoError(t, err)

	sndr := &recordingSender{}
	err = scraper.Scrape(sndr)
	require.NoError(t, err)

	gauges := findCalls(sndr.calls, "Gauge", "temperature")
	assert.Len(t, gauges, 1, "with empty namespace, metric name should not be prefixed")
	assert.Equal(t, 23.5, gauges[0].value)
}
