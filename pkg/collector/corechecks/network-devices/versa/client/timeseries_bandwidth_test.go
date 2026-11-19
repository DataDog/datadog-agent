// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client/fixtures"
	pkglog "github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Synthetic responses for the bandwidth endpoints, shaped after the query's group by
// fields. Validated with synthetic responses only; customer validation pending.

var bandwidthBuckets = []int64{1704067200000, 1704068100000, 1704069000000}

// timeseriesBody builds a timeseries response with one series per entity and metric,
// each metric value being base+bucket index.
func timeseriesBody(t *testing.T, names []string, metrics []string, base float64) string {
	var data []map[string]any
	for _, name := range names {
		for m, metric := range metrics {
			var points [][]float64
			for i, ts := range bandwidthBuckets {
				points = append(points, []float64{float64(ts), base + float64(m*10+i)})
			}
			data = append(data, map[string]any{"name": name, "type": "data", "metric": metric, "metricName": metric, "data": points})
		}
	}
	body, err := json.Marshal(map[string]any{"qTime": 1, "data": data})
	require.NoError(t, err)
	return string(body)
}

func tableBody(t *testing.T, rows [][]any) string {
	body, err := json.Marshal(map[string]any{"qTime": 1, "sEcho": 0, "iTotalDisplayRecords": len(rows), "iTotalRecords": len(rows), "aaData": rows})
	require.NoError(t, err)
	return string(body)
}

// requestCounts counts requests by query type. Handlers run in the server's goroutines,
// so every access is guarded.
type requestCounts struct {
	mu     sync.Mutex
	counts map[string]int
}

func newRequestCounts() *requestCounts {
	return &requestCounts{counts: map[string]int{}}
}

func (c *requestCounts) inc(queryType string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counts[queryType]++
}

func (c *requestCounts) snapshot() map[string]int {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make(map[string]int, len(c.counts))
	for k, v := range c.counts {
		out[k] = v
	}
	return out
}

// bandwidthServer serves tableBody for qt=tableData and timeseries for qt=timeseries for
// queries starting with prefix, and counts the requests of each kind.
func bandwidthServer(t *testing.T, prefix, table, timeseries string, counts *requestCounts) *Client {
	mock := SetupMockAPIServer()
	t.Cleanup(mock.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("q"), prefix) {
			qt := r.URL.Query().Get("qt")
			counts.inc(qt)
			if qt == QueryTypeTimeseries {
				w.Write([]byte(timeseries))
			} else {
				w.Write([]byte(table))
			}
			return
		}
		mock.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := testClient(server)
	require.NoError(t, err)
	client.directorEndpoint = server.URL
	WithAnalyticsQueryType(QueryTypeTimeseries)(client)
	return client
}

var interfaceMetricNames = []string{"rxUtil", "txUtil", "volume-rx", "volume-tx", "volume", "bw-rx", "bw-tx", "bandwidth"}
var bandwidthMetricNames = []string{"volume-tx", "volume-rx", "bw-tx", "bw-rx"}

func TestGetAnalyticsInterfacesTimeseries(t *testing.T) {
	counts := newRequestCounts()
	client := bandwidthServer(t, "intfUtil(", "",
		timeseriesBody(t, []string{"site-001,circuit-a,eth-1", "site-002,circuit-b,eth-2"}, interfaceMetricNames, 1), counts)

	metrics, err := client.GetAnalyticsInterfaces("datadog")
	require.NoError(t, err)
	// Two entities, three buckets each, of which the newest is held back.
	require.Len(t, metrics, 4)
	assert.Equal(t, AnalyticsInterfaceMetrics{
		DrillKey: "site-001,circuit-a,eth-1", Site: "site-001", AccessCkt: "circuit-a", Interface: "eth-1",
		RxUtil: 1, TxUtil: 11, VolumeRx: 21, VolumeTx: 31, Volume: 41, BandwidthRx: 51, BandwidthTx: 61, Bandwidth: 71,
		Timestamp: bandwidthBuckets[0],
	}, metrics[0])
	assert.Equal(t, bandwidthBuckets[1], metrics[1].Timestamp)
	assert.Equal(t, "site-002", metrics[2].Site)
	// The layout follows the group by fields, so no tableData call is needed.
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1}, counts.snapshot())
}

func linkUsageTableRow(key, site, circuit, ip string) []any {
	return []any{key, site, circuit, "100000000", "50000000", "broadband", "ethernet", ip, "isp-1", 9.0, 9.0, 9.0, 9.0}
}

// A response whose entity keys do not split into the layout's part count is not used:
// the call falls back to tableData, which carries no bucket timestamps.
func TestBandwidthTimeseriesKeyPartsMismatchFallsBackToTableData(t *testing.T) {
	counts := newRequestCounts()
	client := bandwidthServer(t, "linkusage(",
		tableBody(t, [][]any{linkUsageTableRow("site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1")}),
		timeseriesBody(t, []string{"site-001,circuit-a,10.0.0.1"}, bandwidthMetricNames, 100), counts)

	metrics, err := client.GetLinkUsageMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Zero(t, metrics[0].Timestamp)
	assert.Equal(t, 9.0, metrics[0].VolumeTx)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts.snapshot())
}

func TestInterfaceTimeseriesKeyPartsMismatchFallsBackToTableData(t *testing.T) {
	counts := newRequestCounts()
	client := bandwidthServer(t, "intfUtil(",
		tableBody(t, [][]any{{"site-001,circuit-a,eth-1", "site-001", "circuit-a", "eth-1", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0}}),
		timeseriesBody(t, []string{"site-001,circuit-a"}, interfaceMetricNames, 1), counts)

	metrics, err := client.GetAnalyticsInterfaces("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Zero(t, metrics[0].Timestamp)
	assert.Equal(t, "eth-1", metrics[0].Interface)
}

// One wrong part count among otherwise valid series is enough to use tableData for the
// whole call.
func TestBandwidthTimeseriesPartialKeyPartsMismatchFallsBackToTableData(t *testing.T) {
	counts := newRequestCounts()
	body := timeseriesBody(t, []string{"site-001,circuit-a,eth-1", "site-002,circuit-b,eth-2", "site-003,circuit-c"}, interfaceMetricNames, 1)
	client := bandwidthServer(t, "intfUtil(",
		tableBody(t, [][]any{{"site-001,circuit-a,eth-1", "site-001", "circuit-a", "eth-1", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0}}),
		body, counts)

	metrics, err := client.GetAnalyticsInterfaces("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Zero(t, metrics[0].Timestamp)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts.snapshot())
}

// The check builds a new client every run and passes the same WarnLimiter to each, so a
// warning that repeats on every run is logged once.
func TestWarnLimiterHoldsAcrossClients(t *testing.T) {
	var logs bytes.Buffer
	logger, err := pkglog.LoggerFromWriterWithMinLevelAndFullFormat(&logs, pkglog.DebugLvl)
	require.NoError(t, err)
	pkglog.SetupLogger(logger, pkglog.DebugStr)
	t.Cleanup(func() {
		pkglog.SetupLogger(pkglog.Default(), pkglog.DebugStr)
		logger.Close()
	})

	limiter := NewWarnLimiter()
	for run := 0; run < 2; run++ {
		counts := newRequestCounts()
		mock := SetupMockAPIServer()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Query().Get("q"), "intfUtil(") {
				counts.inc(r.URL.Query().Get("qt"))
				if r.URL.Query().Get("qt") == QueryTypeTimeseries {
					w.Write([]byte(timeseriesBody(t, []string{"site-001,circuit-a"}, interfaceMetricNames, 1)))
				} else {
					w.Write([]byte(tableBody(t, [][]any{{"site-001,circuit-a,eth-1", "site-001", "circuit-a", "eth-1", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0}})))
				}
				return
			}
			mock.Config.Handler.ServeHTTP(w, r)
		}))
		client, err := testClient(server, WithWarnLimiter(limiter))
		require.NoError(t, err)
		client.directorEndpoint = server.URL
		WithAnalyticsQueryType(QueryTypeTimeseries)(client)
		_, err = client.GetAnalyticsInterfaces("datadog")
		require.NoError(t, err)
		server.Close()
		mock.Close()
	}
	logger.Flush()
	var warnings, debugs int
	for _, line := range strings.Split(logs.String(), "\n") {
		if !strings.Contains(line, "an entity key has 2 parts where 3 were expected") {
			continue
		}
		if strings.Contains(line, "| WARN |") {
			warnings++
		} else if strings.Contains(line, "| DEBUG |") {
			debugs++
		}
	}
	assert.Equal(t, 1, warnings)
	assert.Equal(t, 1, debugs)
}

// linkstatus and the site query have no timeseries layout, so they request tableData
// only, even with analytics_query_type set to timeseries.
func TestLinkStatusAndSiteMetricsUseTableDataInTimeseriesMode(t *testing.T) {
	counts := newRequestCounts()
	client := bandwidthServer(t, "linkstatus(",
		tableBody(t, [][]any{{"site-001,circuit-a", "site-001", "circuit-a", 99.5}}),
		timeseriesBody(t, []string{"Up"}, []string{"availability"}, 97), counts)
	status, err := client.GetLinkStatusMetrics("datadog")
	require.NoError(t, err)
	assert.Equal(t, []LinkStatusMetrics{{DrillKey: "site-001,circuit-a", Site: "site-001", AccessCircuit: "circuit-a", Availability: 99.5}}, status)
	assert.Equal(t, map[string]int{QueryTypeTableData: 1}, counts.snapshot())

	counts = newRequestCounts()
	client = bandwidthServer(t, "linkusage(site,site.address",
		tableBody(t, [][]any{{"site-001", "1 Example Street", "40.0", "-70.0", "manual", 1.0, 2.0, 3.0, 4.0, 5.0}}),
		`{"code":-3,"error":"Joined queries are not allowed for this report"}`, counts)
	sites, err := client.GetSiteMetrics("datadog")
	require.NoError(t, err)
	assert.Equal(t, []SiteMetrics{{Site: "site-001", Address: "1 Example Street", Latitude: "40.0", Longitude: "-70.0", LocationSource: "manual",
		VolumeTx: 1, VolumeRx: 2, BandwidthTx: 3, BandwidthRx: 4, Availability: 5}}, sites)
	assert.Equal(t, map[string]int{QueryTypeTableData: 1}, counts.snapshot())
}

const versaReportError = `{"code":-3,"error":"Joined queries are not allowed for this report"}`

// errorServer answers timeseries requests for queries starting with prefix with HTTP 400
// and Versa's JSON error body, and tableData requests with table.
func errorServer(t *testing.T, prefix, table string, counts *requestCounts) *Client {
	mock := SetupMockAPIServer()
	t.Cleanup(mock.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("q"), prefix) {
			qt := r.URL.Query().Get("qt")
			counts.inc(qt)
			w.Header().Set("Content-Type", "application/json")
			if qt == QueryTypeTimeseries {
				w.WriteHeader(http.StatusBadRequest)
				w.Write([]byte(versaReportError))
				return
			}
			w.Write([]byte(table))
			return
		}
		mock.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := testClient(server, WithMaxAttempts(1))
	require.NoError(t, err)
	client.directorEndpoint = server.URL
	WithAnalyticsQueryType(QueryTypeTimeseries)(client)
	return client
}

func captureLogs(t *testing.T) func() string {
	var logs bytes.Buffer
	logger, err := pkglog.LoggerFromWriterWithMinLevelAndFullFormat(&logs, pkglog.TraceLvl)
	require.NoError(t, err)
	pkglog.SetupLogger(logger, pkglog.TraceStr)
	t.Cleanup(func() {
		pkglog.SetupLogger(pkglog.Default(), pkglog.DebugStr)
		logger.Close()
	})
	return func() string {
		logger.Flush()
		return logs.String()
	}
}

// A timeseries request Versa rejects is answered from tableData, for confirmed and
// pending layouts alike, and the fallback is recorded with its reason.
func TestTimeseriesHTTPErrorFallsBackToTableData(t *testing.T) {
	logs := captureLogs(t)

	counts := newRequestCounts()
	client := errorServer(t, "slam(", tableBody(t, [][]any{{"site-001,site-002,circuit-a,circuit-b,fc_ef", "site-001", "site-002", "circuit-a", "circuit-b", "fc_ef", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0}}), counts)
	sla, err := client.GetSLAMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, sla, 1)
	assert.Zero(t, sla[0].Timestamp)
	assert.Equal(t, 1.0, sla[0].Delay)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts.snapshot())
	assert.Equal(t, []Fallback{{Tenant: "datadog", Query: "slam", Reason: FallbackHTTPError}}, client.TakeFallbacks())
	assert.Empty(t, client.TakeFallbacks())

	for _, tt := range []struct {
		prefix string
		table  [][]any
		get    func(*Client) (int, error)
	}{
		{"linkusage(", [][]any{linkUsageTableRow("site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1")}, func(c *Client) (int, error) {
			m, err := c.GetLinkUsageMetrics("datadog")
			return len(m), err
		}},
		{"usage(", [][]any{{"site-001,circuit-a,10.0.0.1", "site-001", "circuit-a", "10.0.0.1", 1.0, 2.0, 3.0, 4.0}}, func(c *Client) (int, error) {
			m, err := c.GetDIAMetrics("datadog")
			return len(m), err
		}},
	} {
		client := errorServer(t, tt.prefix, tableBody(t, tt.table), newRequestCounts())
		n, err := tt.get(client)
		require.NoError(t, err, tt.prefix)
		assert.Equal(t, 1, n, tt.prefix)
		require.Len(t, client.TakeFallbacks(), 1, tt.prefix)
	}

	out := logs()
	assert.Contains(t, out, "Timeseries query slam: Versa rejected the timeseries request with HTTP 400 (Versa error code -3: joined queries are not allowed for this report), using tableData for it instead")
	assert.NotContains(t, out, "testpass")
}

// A fallback logs, at debug level, at most three timeseries series names and three
// tableData keys with their part counts, and every timeseries query logs how many series
// it received.
func TestFallbackLogsNameSamples(t *testing.T) {
	logs := captureLogs(t)
	var rows [][]any
	for i := 1; i <= 5; i++ {
		rows = append(rows, []any{fmt.Sprintf("site-00%d,circuit-a,eth-1", i), fmt.Sprintf("site-00%d", i), "circuit-a", "eth-1", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0})
	}
	client := bandwidthServer(t, "intfUtil(", tableBody(t, rows),
		timeseriesBody(t, []string{"site-001", "site-002", "site-003", "site-004", "site-005"}, interfaceMetricNames, 1), newRequestCounts())
	_, err := client.GetAnalyticsInterfaces("datadog")
	require.NoError(t, err)

	out := logs()
	assert.Contains(t, out, `Timeseries query "intfUtil(site,accCkt,intf)" returned 40 series for 5 entities`)
	var sample string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, "fell back to tableData; timeseries series names") {
			sample = line
		}
	}
	require.NotEmpty(t, sample)
	assert.Contains(t, sample, "| DEBUG |")
	assert.Equal(t, 3, strings.Count(sample, "(1 parts)"))
	assert.Equal(t, 3, strings.Count(sample, "(3 parts)"))
	assert.NotContains(t, sample, "site-004")
	assert.NotContains(t, out, "testpass")
}

// timeseriesHandlerServer serves the given handler for SLA timeseries requests and a
// one row SLA tableData response, so a test can shape the timeseries failure. The
// returned function reports how many requests a query type has had. It is guarded,
// because a handler that hijacks the connection can still be running in the server's
// goroutine when the test reads the count.
func timeseriesHandlerServer(t *testing.T, timeseries http.HandlerFunc, options ...ClientOptions) (*Client, func(string) int) {
	var mu sync.Mutex
	counts := map[string]int{}
	mock := SetupMockAPIServer()
	t.Cleanup(mock.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("q"), "slam(") {
			qt := r.URL.Query().Get("qt")
			mu.Lock()
			counts[qt]++
			mu.Unlock()
			if qt == QueryTypeTimeseries {
				timeseries(w, r)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(tableBody(t, [][]any{{"site-001,site-002,circuit-a,circuit-b,fc_ef", "site-001", "site-002", "circuit-a", "circuit-b", "fc_ef", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0}})))
			return
		}
		mock.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := testClient(server, append([]ClientOptions{WithMaxAttempts(1)}, options...)...)
	require.NoError(t, err)
	client.directorEndpoint = server.URL
	WithAnalyticsQueryType(QueryTypeTimeseries)(client)
	return client, func(qt string) int {
		mu.Lock()
		defer mu.Unlock()
		return counts[qt]
	}
}

// Authentication, transport, timeout and server errors are not answers to the timeseries
// request itself, so they surface as errors as before and are not counted as fallbacks.
func TestTimeseriesNonRejectionErrorsDoNotFallBack(t *testing.T) {
	// release unblocks the timeout handler independently of client cancellation. It is
	// closed by a cleanup registered after the server's, so it runs before server.Close
	// waits on the handler.
	var release chan struct{}
	jsonError := func(status int) http.HandlerFunc {
		return func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(status)
			w.Write([]byte(`{"code":-1,"error":"rejected"}`))
		}
	}
	for _, tt := range []struct {
		name    string
		handler http.HandlerFunc
		options []ClientOptions
	}{
		{"401", jsonError(http.StatusUnauthorized), nil},
		{"403", jsonError(http.StatusForbidden), nil},
		{"500", jsonError(http.StatusInternalServerError), nil},
		{"html login page", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>login</html>"))
		}, nil},
		{"transport error", func(w http.ResponseWriter, _ *http.Request) {
			conn, _, err := w.(http.Hijacker).Hijack()
			require.NoError(t, err)
			conn.Close()
		}, nil},
		{"timeout", func(_ http.ResponseWriter, r *http.Request) {
			// Block until the client gives up, or the test ends.
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}, []ClientOptions{WithTimeout(20 * time.Millisecond)}},
		// A 4xx without a Versa JSON error body is not a Versa answer either.
		{"400 without a Versa error body", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`{"message":"bad"}`))
		}, nil},
	} {
		t.Run(tt.name, func(t *testing.T) {
			release = make(chan struct{})
			client, counts := timeseriesHandlerServer(t, tt.handler, tt.options...)
			done := release
			t.Cleanup(func() { close(done) })
			metrics, err := client.GetSLAMetrics("datadog")
			require.Error(t, err)
			assert.Nil(t, metrics)
			assert.Empty(t, client.TakeFallbacks())
			assert.Zero(t, counts(QueryTypeTableData))
		})
	}
}

// A Versa rejection with a JSON error body, and a timeseries body that is not valid JSON,
// are answered from tableData.
func TestTimeseriesRejectionAndParseErrorFallBack(t *testing.T) {
	for _, tt := range []struct {
		name    string
		handler http.HandlerFunc
	}{
		{"400 with a Versa error body", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(versaReportError))
		}},
		{"unparsable body", func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"data": [`))
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client, counts := timeseriesHandlerServer(t, tt.handler)
			metrics, err := client.GetSLAMetrics("datadog")
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.Zero(t, metrics[0].Timestamp)
			assert.Equal(t, []Fallback{{Tenant: "datadog", Query: "slam", Reason: FallbackHTTPError}}, client.TakeFallbacks())
			assert.Equal(t, 1, counts(QueryTypeTableData))
		})
	}
}

// The pathcos and cos fallbacks report exactly what tableData mode reports for the same
// response, percent_drop included.
func TestQoSFallbackMatchesTableData(t *testing.T) {
	for _, tt := range []struct {
		prefix string
		table  string
		get    func(*Client) (any, error)
	}{
		{"pathcos(", fixtures.GetPathQoSMetrics, func(c *Client) (any, error) { return c.GetPathQoSMetrics("datadog") }},
		{"cos(", fixtures.GetCircuitQoSMetrics, func(c *Client) (any, error) { return c.GetCircuitQoSMetrics("datadog") }},
	} {
		t.Run(tt.prefix, func(t *testing.T) {
			fallbackClient := errorServer(t, tt.prefix, tt.table, newRequestCounts())
			fallback, err := tt.get(fallbackClient)
			require.NoError(t, err)
			require.Len(t, fallbackClient.TakeFallbacks(), 1)

			tableClient := errorServer(t, tt.prefix, tt.table, newRequestCounts())
			WithAnalyticsQueryType(QueryTypeTableData)(tableClient)
			table, err := tt.get(tableClient)
			require.NoError(t, err)

			assert.Equal(t, table, fallback)
		})
	}
}

// rejectionLines runs an SLA timeseries request that Versa rejects with body, and
// returns the WARN and DEBUG lines logged.
func rejectionLines(t *testing.T, body string) (warnings []string, debugs []string) {
	return responseLines(t, http.StatusBadRequest, body)
}

// responseLines runs an SLA timeseries request answered with status and body, and returns
// the WARN lines and the DEBUG and TRACE lines logged.
func responseLines(t *testing.T, status int, body string) (warnings []string, debugs []string) {
	logs := captureLogs(t)
	client, _ := timeseriesHandlerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		w.Write([]byte(body))
	})
	_, err := client.GetSLAMetrics("datadog")
	require.NoError(t, err)
	for _, line := range strings.Split(logs(), "\n") {
		if i := strings.Index(line, "| WARN | "); i >= 0 {
			msg := line[i+len("| WARN | "):]
			warnings = append(warnings, msg[strings.Index(msg, ") | ")+len(") | "):])
		} else if strings.Contains(line, "| DEBUG |") || strings.Contains(line, "| TRACE |") {
			debugs = append(debugs, line)
		}
	}
	return warnings, debugs
}

// The rejection warning is built only from the query name, HTTP status and Versa's numeric
// code; Versa's error text never reaches a WARN, DEBUG or TRACE line, whatever it contains.
func TestTimeseriesRejectionWarningOmitsVersaText(t *testing.T) {
	const prefix = "Timeseries query slam: Versa rejected the timeseries request with HTTP 400"
	const suffix = ", using tableData for it instead"
	secretTexts := []string{
		"password testpass for tenant datadog",
		"https://testuser:testpass@10.0.0.1:9182/versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN?q=slam",
		"dGVzdHBhc3M=", // base64 of the password
		"dGVzdHBhc3M",  // unpadded base64url of the password
		"test+pass",    // separators that a character filter would delete
		"test\npass",
		"test\u200bpass",
	}
	for _, tt := range []struct {
		name string
		code string
		want string
	}{
		{"known code -3", "-3", prefix + " (Versa error code -3: joined queries are not allowed for this report)" + suffix},
		{"known code -8", "-8", prefix + " (Versa error code -8: invalid metric)" + suffix},
		{"unknown code", "-42", prefix + " (Versa error code -42)" + suffix},
		{"string code", `"-3"`, prefix + suffix},
		{"null code", "null", prefix + suffix},
		{"fractional code", "-3.5", prefix + suffix},
		{"exponent code", "-3e0", prefix + suffix},
		{"overflowing code", "99999999999999999999", prefix + suffix},
		{"no code", "", prefix + suffix},
	} {
		for _, text := range secretTexts {
			t.Run(tt.name+"/"+text, func(t *testing.T) {
				fields := map[string]any{"error": text, "message": text}
				if tt.code != "" {
					fields["code"] = json.RawMessage(tt.code)
				}
				body, err := json.Marshal(fields)
				require.NoError(t, err)
				warnings, debugs := rejectionLines(t, string(body))
				assert.Equal(t, []string{tt.want}, warnings)
				for _, line := range debugs {
					assert.NotContains(t, line, "testpass")
					assert.NotContains(t, line, "dGVzdHBhc3M")
					assert.NotContains(t, line, "test+pass")
				}
			})
		}
	}

	// A body that cannot be parsed falls back without logging any of it, at any level.
	for _, text := range secretTexts {
		t.Run("unparsable body/"+text, func(t *testing.T) {
			warnings, debugs := responseLines(t, http.StatusOK, `{"data": "`+text)
			assert.Equal(t, []string{"Timeseries query slam: the timeseries response could not be parsed" + suffix}, warnings)
			for _, line := range debugs {
				assert.NotContains(t, line, "testpass")
				assert.NotContains(t, line, "dGVzdHBhc3M")
				assert.NotContains(t, line, "test+pass")
			}
		})
	}
}

// With zero volume, the pathcos and cos fallbacks keep Versa's percentdrop as tableData
// mode reports it, rather than marking it unset as a timeseries bucket would be.
func TestQoSFallbackMatchesTableDataWithZeroVolume(t *testing.T) {
	row := func(key, a, b string) []any {
		r := []any{key, a, b}
		for i := 0; i < 12; i++ {
			r = append(r, 1.0)
		}
		// volume-tx, totaldrop, percentdrop, bandwidth
		return append(r, 0.0, 0.0, 0.25, 0.0)
	}
	for _, tt := range []struct {
		prefix string
		table  string
		get    func(*Client) (any, error)
	}{
		{"pathcos(", tableBody(t, [][]any{row("site-001,site-002", "site-001", "site-002")}), func(c *Client) (any, error) { return c.GetPathQoSMetrics("datadog") }},
		{"cos(", tableBody(t, [][]any{row("site-001,circuit-a", "site-001", "circuit-a")}), func(c *Client) (any, error) { return c.GetCircuitQoSMetrics("datadog") }},
	} {
		t.Run(tt.prefix, func(t *testing.T) {
			fallbackClient := errorServer(t, tt.prefix, tt.table, newRequestCounts())
			fallback, err := tt.get(fallbackClient)
			require.NoError(t, err)
			require.Len(t, fallbackClient.TakeFallbacks(), 1)

			tableClient := errorServer(t, tt.prefix, tt.table, newRequestCounts())
			WithAnalyticsQueryType(QueryTypeTableData)(tableClient)
			table, err := tt.get(tableClient)
			require.NoError(t, err)

			assert.Equal(t, table, fallback)
			assert.Contains(t, fmt.Sprintf("%+v", fallback), "PercentDrop:0.25 Bandwidth:0 PercentDropUnset:false")
		})
	}
}

// Every analytics getter takes its layout from timeseriesLayouts under its own endpoint:
// in timeseries mode it requests qt=timeseries when only its own endpoint has an entry,
// and tableData when every endpoint but its own has one.
func TestGettersFollowTimeseriesLayouts(t *testing.T) {
	var requestedMu sync.Mutex
	var requested []string
	takeRequested := func() []string {
		requestedMu.Lock()
		defer requestedMu.Unlock()
		out := requested
		requested = nil
		return out
	}
	mock := SetupMockAPIServer()
	defer mock.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/features/") {
			qt := r.URL.Query().Get("qt")
			requestedMu.Lock()
			requested = append(requested, qt)
			requestedMu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			if qt == QueryTypeTimeseries {
				w.Write([]byte(`{"qTime":1,"data":[]}`))
			} else {
				w.Write([]byte(`{"qTime":1,"aaData":[]}`))
			}
			return
		}
		mock.Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	client, err := testClient(server)
	require.NoError(t, err)
	client.directorEndpoint = server.URL
	WithAnalyticsQueryType(QueryTypeTimeseries)(client)

	getters := map[Endpoint]func() error{
		EndpointSLAMetrics:                     func() error { _, err := client.GetSLAMetrics("datadog"); return err },
		EndpointQoSMetrics:                     func() error { _, err := client.GetPathQoSMetrics("datadog"); return err },
		EndpointCircuitQoSMetrics:              func() error { _, err := client.GetCircuitQoSMetrics("datadog"); return err },
		EndpointLinkStatusMetrics:              func() error { _, err := client.GetLinkStatusMetrics("datadog"); return err },
		EndpointLinkUsageMetrics:               func() error { _, err := client.GetLinkUsageMetrics("datadog"); return err },
		EndpointSiteMetrics:                    func() error { _, err := client.GetSiteMetrics("datadog"); return err },
		EndpointApplicationsByApplianceMetrics: func() error { _, err := client.GetApplicationsByAppliance("datadog"); return err },
		EndpointTopUserMetrics:                 func() error { _, err := client.GetTopUsers("datadog"); return err },
		EndpointTunnelMetrics:                  func() error { _, err := client.GetTunnelMetrics("datadog"); return err },
		EndpointDIAMetrics:                     func() error { _, err := client.GetDIAMetrics("datadog"); return err },
		EndpointInterfaceMetrics:               func() error { _, err := client.GetAnalyticsInterfaces("datadog"); return err },
	}
	require.Len(t, getters, 11)
	for endpoint := range timeseriesLayouts {
		require.Contains(t, getters, endpoint, "layout for an endpoint without a getter")
	}

	original := timeseriesLayouts
	t.Cleanup(func() { timeseriesLayouts = original })
	// layoutFor reuses an endpoint's real layout, or any valid one when it has none; the
	// stub's timeseries responses are empty, so the layout's shape does not matter.
	layoutFor := func(endpoint Endpoint) *timeseriesLayout {
		if layout := original[endpoint]; layout != nil {
			return layout
		}
		return &timeseriesLayout{drillKeyFields: 2}
	}

	for endpoint, get := range getters {
		t.Run(string(endpoint)+"/own entry only", func(t *testing.T) {
			timeseriesLayouts = map[Endpoint]*timeseriesLayout{endpoint: layoutFor(endpoint)}
			t.Cleanup(func() { timeseriesLayouts = original })
			takeRequested()
			require.NoError(t, get())
			assert.Equal(t, []string{QueryTypeTimeseries}, takeRequested())
		})
		t.Run(string(endpoint)+"/every other entry", func(t *testing.T) {
			timeseriesLayouts = map[Endpoint]*timeseriesLayout{}
			for other := range getters {
				if other != endpoint {
					timeseriesLayouts[other] = layoutFor(other)
				}
			}
			t.Cleanup(func() { timeseriesLayouts = original })
			takeRequested()
			require.NoError(t, get())
			assert.Equal(t, []string{QueryTypeTableData}, takeRequested())
		})
	}

	// With the real map, each getter requests what HasTimeseriesLayout reports.
	for endpoint, get := range getters {
		t.Run(string(endpoint)+"/real map", func(t *testing.T) {
			takeRequested()
			require.NoError(t, get())
			want := QueryTypeTableData
			if HasTimeseriesLayout(endpoint) {
				want = QueryTypeTimeseries
			}
			assert.Equal(t, []string{want}, takeRequested())
		})
	}
}
