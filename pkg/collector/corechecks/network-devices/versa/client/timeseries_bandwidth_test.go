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

// bandwidthServer serves tableBody for qt=tableData and timeseries for qt=timeseries for
// queries starting with prefix, and counts the requests of each kind.
func bandwidthServer(t *testing.T, prefix, table, timeseries string, counts map[string]int) *Client {
	mock := SetupMockAPIServer()
	t.Cleanup(mock.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("q"), prefix) {
			qt := r.URL.Query().Get("qt")
			counts[qt]++
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
	counts := map[string]int{}
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
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1}, counts)
}

func linkUsageTableRow(key, site, circuit, ip string) []any {
	return []any{key, site, circuit, "100000000", "50000000", "broadband", "ethernet", ip, "isp-1", 9.0, 9.0, 9.0, 9.0}
}

func TestGetLinkUsageMetricsTimeseriesJoinsAttributes(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "linkusage(",
		tableBody(t, [][]any{linkUsageTableRow("site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1")}),
		// site-002,circuit-b has no tableData row, so its buckets are skipped.
		timeseriesBody(t, []string{"site-001,circuit-a", "site-002,circuit-b"}, bandwidthMetricNames, 100), counts)

	metrics, err := client.GetLinkUsageMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	assert.Equal(t, LinkUsageMetrics{
		DrillKey: "site-001,circuit-a", Site: "site-001", AccessCircuit: "circuit-a",
		UplinkBandwidth: "100000000", DownlinkBandwidth: "50000000", Type: "broadband", Media: "ethernet",
		IP: "10.0.0.1", ISP: "isp-1",
		VolumeTx: 100, VolumeRx: 110, BandwidthTx: 120, BandwidthRx: 130,
		Timestamp: bandwidthBuckets[0],
	}, metrics[0])
	assert.Equal(t, 101.0, metrics[1].VolumeTx)
	assert.Equal(t, bandwidthBuckets[1], metrics[1].Timestamp)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts)
}

func TestGetDIAMetricsTimeseriesJoinsAttributes(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "usage(",
		tableBody(t, [][]any{{"site-001,circuit-a,10.0.0.1", "site-001", "circuit-a", "10.0.0.1", 9.0, 9.0, 9.0, 9.0}}),
		timeseriesBody(t, []string{"site-001,circuit-a,10.0.0.1"}, bandwidthMetricNames, 5), counts)

	metrics, err := client.GetDIAMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	assert.Equal(t, DIAMetrics{
		DrillKey: "site-001,circuit-a,10.0.0.1", Site: "site-001", AccessCircuit: "circuit-a", IP: "10.0.0.1",
		VolumeTx: 5, VolumeRx: 15, BandwidthTx: 25, BandwidthRx: 35,
		Timestamp: bandwidthBuckets[0],
	}, metrics[0])
}

// A response whose entity keys do not split into the layout's part count is not used:
// the call falls back to tableData, which carries no bucket timestamps.
func TestBandwidthTimeseriesKeyPartsMismatchFallsBackToTableData(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "linkusage(",
		tableBody(t, [][]any{linkUsageTableRow("site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1")}),
		timeseriesBody(t, []string{"site-001,circuit-a,10.0.0.1"}, bandwidthMetricNames, 100), counts)

	metrics, err := client.GetLinkUsageMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Zero(t, metrics[0].Timestamp)
	assert.Equal(t, 9.0, metrics[0].VolumeTx)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts)
}

func TestInterfaceTimeseriesKeyPartsMismatchFallsBackToTableData(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "intfUtil(",
		tableBody(t, [][]any{{"site-001,circuit-a,eth-1", "site-001", "circuit-a", "eth-1", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0}}),
		timeseriesBody(t, []string{"site-001,circuit-a"}, interfaceMetricNames, 1), counts)

	metrics, err := client.GetAnalyticsInterfaces("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Zero(t, metrics[0].Timestamp)
	assert.Equal(t, "eth-1", metrics[0].Interface)
}

func TestJoinAttributes(t *testing.T) {
	rows := []timeseriesRow{
		{timestamp: 1, row: []interface{}{"site-001,circuit-a", "site-001", "circuit-a", 1.0}},
		{timestamp: 1, row: []interface{}{"site-002,circuit-b", "site-002", "circuit-b", 2.0}},
	}
	layout := &timeseriesLayout{drillKeyFields: 2, attributeColumns: 3}
	joined, missing, ambiguous := joinAttributes(rows, [][]interface{}{
		{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 9.0},
		// An identical duplicate row is not ambiguous.
		{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 8.0},
	}, layout)
	assert.Equal(t, 1, missing)
	assert.Zero(t, ambiguous)
	require.Len(t, joined, 1)
	assert.Equal(t, []interface{}{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 1.0}, joined[0].row)
}

// Two tableData rows for one key that disagree on an attribute make the key ambiguous:
// no rows are joined and the number of ambiguous keys is returned, so the caller can use
// tableData instead of tagging from whichever row came last.
func TestJoinAttributesReportsAmbiguousKeys(t *testing.T) {
	rows := []timeseriesRow{
		{timestamp: 1, row: []interface{}{"site-001,circuit-a", "site-001", "circuit-a", 1.0}},
		{timestamp: 1, row: []interface{}{"site-002,circuit-b", "site-002", "circuit-b", 3.0}},
	}
	layout := &timeseriesLayout{drillKeyFields: 2, attributeColumns: 3}
	joined, _, ambiguous := joinAttributes(rows, [][]interface{}{
		{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 9.0},
		{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.2", 9.0},
		{"site-002,circuit-b", "site-002", "circuit-b", "10.0.0.3", 9.0},
	}, layout)
	assert.Equal(t, 1, ambiguous)
	assert.Empty(t, joined)
}

// An ambiguous key makes the whole call use tableData, so every entity tableData reports
// is still reported, with its own tags.
func TestGetLinkUsageMetricsTimeseriesAmbiguousKeyFallsBackToTableData(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "linkusage(",
		tableBody(t, [][]any{
			linkUsageTableRow("site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1"),
			linkUsageTableRow("site-001,circuit-a", "site-001", "circuit-a", "10.0.0.2"),
			linkUsageTableRow("site-002,circuit-b", "site-002", "circuit-b", "10.0.0.3"),
		}),
		timeseriesBody(t, []string{"site-001,circuit-a", "site-002,circuit-b"}, bandwidthMetricNames, 100), counts)

	metrics, err := client.GetLinkUsageMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 3)
	var ips []string
	for _, m := range metrics {
		assert.Zero(t, m.Timestamp)
		ips = append(ips, m.IP)
	}
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}, ips)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts)
}

// One wrong part count among otherwise valid series is enough to use tableData for the
// whole call.
func TestBandwidthTimeseriesPartialKeyPartsMismatchFallsBackToTableData(t *testing.T) {
	counts := map[string]int{}
	body := timeseriesBody(t, []string{"site-001,circuit-a,eth-1", "site-002,circuit-b,eth-2", "site-003,circuit-c"}, interfaceMetricNames, 1)
	client := bandwidthServer(t, "intfUtil(",
		tableBody(t, [][]any{{"site-001,circuit-a,eth-1", "site-001", "circuit-a", "eth-1", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0, 7.0, 8.0}}),
		body, counts)

	metrics, err := client.GetAnalyticsInterfaces("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Zero(t, metrics[0].Timestamp)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts)
}

// failingJoinServer serves a valid link usage timeseries response, and a tableData
// response that fails from the given page on, with page size one. Link status requests
// are served normally so another query on the same client can be checked.
func failingJoinServer(t *testing.T, failFromPage int) *Client {
	mock := SetupMockAPIServer()
	t.Cleanup(mock.Close)
	timeseries := timeseriesBody(t, []string{"site-001,circuit-a", "site-002,circuit-b"}, bandwidthMetricNames, 100)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if strings.HasPrefix(q.Get("q"), "linkusage(") {
			if q.Get("qt") == QueryTypeTimeseries {
				w.Write([]byte(timeseries))
				return
			}
			page := 1
			if q.Get("from-count") != "0" {
				page = 2
			}
			if page >= failFromPage {
				http.Error(w, "unavailable", http.StatusInternalServerError)
				return
			}
			w.Write([]byte(tableBody(t, [][]any{linkUsageTableRow("site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1")})))
			return
		}
		if strings.HasPrefix(q.Get("q"), "intfUtil(") && q.Get("qt") == QueryTypeTimeseries {
			w.Write([]byte(timeseriesBody(t, []string{"site-001,circuit-a,eth-1"}, interfaceMetricNames, 1)))
			return
		}
		mock.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	client, err := testClient(server, WithMaxCount(1), WithMaxAttempts(1))
	require.NoError(t, err)
	client.directorEndpoint = server.URL
	WithAnalyticsQueryType(QueryTypeTimeseries)(client)
	return client
}

// A failed tableData join request, on its first or a later page, returns an error for
// the query instead of tagging buckets from a partial attribute set; other queries on the
// same client are unaffected.
func TestLinkUsageTimeseriesJoinRequestFailure(t *testing.T) {
	for _, tt := range []struct {
		name         string
		failFromPage int
	}{
		{"first page", 1},
		{"later page", 2},
	} {
		t.Run(tt.name, func(t *testing.T) {
			client := failingJoinServer(t, tt.failFromPage)

			metrics, err := client.GetLinkUsageMetrics("datadog")
			require.Error(t, err)
			assert.Nil(t, metrics)

			interfaces, err := client.GetAnalyticsInterfaces("datadog")
			require.NoError(t, err)
			require.Len(t, interfaces, 2)
		})
	}
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
		counts := map[string]int{}
		mock := SetupMockAPIServer()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Query().Get("q"), "intfUtil(") {
				counts[r.URL.Query().Get("qt")]++
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

var tunnelMetricNames = []string{"volume-tx", "volume-rx"}

func tunnelTableRow(key, appliance, local, peer, profile string) []any {
	return []any{key, appliance, local, peer, profile, 9.0, 9.0}
}

func TestGetTunnelMetricsTimeseriesJoinsAttributes(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "tunnelstats(",
		tableBody(t, [][]any{
			tunnelTableRow("appliance-001,10.0.0.1", "appliance-001", "10.0.0.1", "10.1.0.1", "profile-a"),
			tunnelTableRow("appliance-002,10.0.0.2", "appliance-002", "10.0.0.2", "10.1.0.2", "profile-a"),
		}),
		// volume-tx is 100 and volume-rx 110 in the first bucket.
		timeseriesBody(t, []string{"appliance-001,10.0.0.1", "appliance-002,10.0.0.2"}, tunnelMetricNames, 100), counts)

	metrics, err := client.GetTunnelMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 4)
	// Each named series lands in the field of the same name, whatever the parser's
	// column positions.
	assert.Equal(t, TunnelMetrics{
		DrillKey: "appliance-001,10.0.0.1", Appliance: "appliance-001", LocalIP: "10.0.0.1",
		RemoteIP: "10.1.0.1", VpnProfName: "profile-a",
		VolumeTx: 100, VolumeRx: 110, Timestamp: bandwidthBuckets[0],
	}, metrics[0])
	assert.Equal(t, bandwidthBuckets[1], metrics[1].Timestamp)
	assert.Equal(t, "10.1.0.2", metrics[2].RemoteIP)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts)
}

// Several peers on one appliance and local IP share a timeseries key, so the call uses
// tableData and every tunnel is reported with its own tags.
func TestTunnelTimeseriesSharedKeyFallsBackToTableData(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "tunnelstats(",
		tableBody(t, [][]any{
			tunnelTableRow("appliance-001,10.0.0.1", "appliance-001", "10.0.0.1", "10.1.0.1", "profile-a"),
			tunnelTableRow("appliance-001,10.0.0.1", "appliance-001", "10.0.0.1", "10.1.0.2", "profile-a"),
			tunnelTableRow("appliance-001,10.0.0.1", "appliance-001", "10.0.0.1", "10.1.0.3", "profile-b"),
		}),
		timeseriesBody(t, []string{"appliance-001,10.0.0.1"}, tunnelMetricNames, 100), counts)

	metrics, err := client.GetTunnelMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 3)
	var peers []string
	for _, m := range metrics {
		assert.Zero(t, m.Timestamp)
		assert.Equal(t, "appliance-001", m.Appliance)
		peers = append(peers, m.RemoteIP+"/"+m.VpnProfName)
	}
	assert.Equal(t, []string{"10.1.0.1/profile-a", "10.1.0.2/profile-a", "10.1.0.3/profile-b"}, peers)
}

func TestTunnelTimeseriesKeyPartsMismatchFallsBackToTableData(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "tunnelstats(",
		tableBody(t, [][]any{tunnelTableRow("appliance-001,10.0.0.1", "appliance-001", "10.0.0.1", "10.1.0.1", "profile-a")}),
		timeseriesBody(t, []string{"appliance-001,10.0.0.1,10.1.0.1,profile-a"}, tunnelMetricNames, 100), counts)

	metrics, err := client.GetTunnelMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Zero(t, metrics[0].Timestamp)
	assert.Equal(t, "10.1.0.1", metrics[0].RemoteIP)
}

// linkstatus and the site query have no timeseries layout, so they request tableData
// only, even with analytics_query_type set to timeseries.
func TestLinkStatusAndSiteMetricsUseTableDataInTimeseriesMode(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "linkstatus(",
		tableBody(t, [][]any{{"site-001,circuit-a", "site-001", "circuit-a", 99.5}}),
		timeseriesBody(t, []string{"Up"}, []string{"availability"}, 97), counts)
	status, err := client.GetLinkStatusMetrics("datadog")
	require.NoError(t, err)
	assert.Equal(t, []LinkStatusMetrics{{DrillKey: "site-001,circuit-a", Site: "site-001", AccessCircuit: "circuit-a", Availability: 99.5}}, status)
	assert.Equal(t, map[string]int{QueryTypeTableData: 1}, counts)

	counts = map[string]int{}
	client = bandwidthServer(t, "linkusage(site,site.address",
		tableBody(t, [][]any{{"site-001", "1 Example Street", "40.0", "-70.0", "manual", 1.0, 2.0, 3.0, 4.0, 5.0}}),
		`{"code":-3,"error":"Joined queries are not allowed for this report"}`, counts)
	sites, err := client.GetSiteMetrics("datadog")
	require.NoError(t, err)
	assert.Equal(t, []SiteMetrics{{Site: "site-001", Address: "1 Example Street", Latitude: "40.0", Longitude: "-70.0", LocationSource: "manual",
		VolumeTx: 1, VolumeRx: 2, BandwidthTx: 3, BandwidthRx: 4, Availability: 5}}, sites)
	assert.Equal(t, map[string]int{QueryTypeTableData: 1}, counts)
}

const versaReportError = `{"code":-3,"error":"Joined queries are not allowed for this report"}`

// errorServer answers timeseries requests for queries starting with prefix with HTTP 400
// and Versa's JSON error body, and tableData requests with table.
func errorServer(t *testing.T, prefix, table string, counts map[string]int) *Client {
	mock := SetupMockAPIServer()
	t.Cleanup(mock.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("q"), prefix) {
			qt := r.URL.Query().Get("qt")
			counts[qt]++
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
	logger, err := pkglog.LoggerFromWriterWithMinLevelAndFullFormat(&logs, pkglog.DebugLvl)
	require.NoError(t, err)
	pkglog.SetupLogger(logger, pkglog.DebugStr)
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

	counts := map[string]int{}
	client := errorServer(t, "slam(", tableBody(t, [][]any{{"site-001,site-002,circuit-a,circuit-b,fc_ef", "site-001", "site-002", "circuit-a", "circuit-b", "fc_ef", 1.0, 2.0, 3.0, 4.0, 5.0, 6.0}}), counts)
	sla, err := client.GetSLAMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, sla, 1)
	assert.Zero(t, sla[0].Timestamp)
	assert.Equal(t, 1.0, sla[0].Delay)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts)
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
		{"tunnelstats(", [][]any{tunnelTableRow("appliance-001,10.0.0.1", "appliance-001", "10.0.0.1", "10.1.0.1", "profile-a")}, func(c *Client) (int, error) {
			m, err := c.GetTunnelMetrics("datadog")
			return len(m), err
		}},
	} {
		client := errorServer(t, tt.prefix, tableBody(t, tt.table), map[string]int{})
		n, err := tt.get(client)
		require.NoError(t, err, tt.prefix)
		assert.Equal(t, 1, n, tt.prefix)
		require.Len(t, client.TakeFallbacks(), 1, tt.prefix)
	}

	out := logs()
	assert.Contains(t, out, "Timeseries query slam: Versa rejected the timeseries request with HTTP 400: Joined queries are not allowed for this report")
	assert.NotContains(t, out, "testpass")
}

// When most timeseries entities have no tableData row, the call is answered from tableData
// instead of reporting only the few that matched.
func TestJoinUnmatchedFallsBackToTableData(t *testing.T) {
	for _, tt := range []struct {
		name  string
		names []string
	}{
		{"no entity matches", []string{"site-009,circuit-x", "site-010,circuit-y"}},
		{"most entities do not match", []string{"site-001,circuit-a", "site-009,circuit-x", "site-010,circuit-y"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			counts := map[string]int{}
			client := bandwidthServer(t, "linkusage(",
				tableBody(t, [][]any{linkUsageTableRow("site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1")}),
				timeseriesBody(t, tt.names, bandwidthMetricNames, 100), counts)
			metrics, err := client.GetLinkUsageMetrics("datadog")
			require.NoError(t, err)
			require.Len(t, metrics, 1)
			assert.Zero(t, metrics[0].Timestamp)
			assert.Equal(t, 9.0, metrics[0].VolumeTx)
			assert.Equal(t, []Fallback{{Tenant: "datadog", Query: "linkusage", Reason: FallbackJoinUnmatched}}, client.TakeFallbacks())
		})
	}
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
		timeseriesBody(t, []string{"site-001", "site-002", "site-003", "site-004", "site-005"}, interfaceMetricNames, 1), map[string]int{})
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
// one row SLA tableData response, so a test can shape the timeseries failure.
func timeseriesHandlerServer(t *testing.T, timeseries http.HandlerFunc, options ...ClientOptions) (*Client, map[string]int) {
	counts := map[string]int{}
	mock := SetupMockAPIServer()
	t.Cleanup(mock.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("q"), "slam(") {
			qt := r.URL.Query().Get("qt")
			counts[qt]++
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
	return client, counts
}

// Authentication, transport, timeout and server errors are not answers to the timeseries
// request itself, so they surface as errors as before and are not counted as fallbacks.
func TestTimeseriesNonRejectionErrorsDoNotFallBack(t *testing.T) {
	testDone := make(chan struct{})
	defer close(testDone)
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
			case <-testDone:
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
			client, counts := timeseriesHandlerServer(t, tt.handler, tt.options...)
			metrics, err := client.GetSLAMetrics("datadog")
			require.Error(t, err)
			assert.Nil(t, metrics)
			assert.Empty(t, client.TakeFallbacks())
			assert.Zero(t, counts[QueryTypeTableData])
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
			assert.Equal(t, 1, counts[QueryTypeTableData])
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
			fallbackClient := errorServer(t, tt.prefix, tt.table, map[string]int{})
			fallback, err := tt.get(fallbackClient)
			require.NoError(t, err)
			require.Len(t, fallbackClient.TakeFallbacks(), 1)

			tableClient := errorServer(t, tt.prefix, tt.table, map[string]int{})
			WithAnalyticsQueryType(QueryTypeTableData)(tableClient)
			table, err := tt.get(tableClient)
			require.NoError(t, err)

			assert.Equal(t, table, fallback)
		})
	}
}

// The fallback warning carries the query name, HTTP status and a sanitized Versa message
// only, even when Versa echoes the request, the tenant and credentials back.
func TestTimeseriesRejectionWarningIsSanitized(t *testing.T) {
	logs := captureLogs(t)
	echo := `{"code":-3,"error":"Invalid request https://testuser:testpass@10.0.0.1:9182/versa/analytics/v1.0.0/data/provider/tenants/datadog/features/SDWAN?q=slam&count=2000 from director.example:9182 password testpass for tenant datadog"}`
	client, _ := timeseriesHandlerServer(t, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(echo))
	})
	_, err := client.GetSLAMetrics("datadog")
	require.NoError(t, err)

	var warning string
	for _, line := range strings.Split(logs(), "\n") {
		if strings.Contains(line, "| WARN |") && strings.Contains(line, "Timeseries query slam") {
			warning = line
		}
	}
	require.NotEmpty(t, warning)
	assert.Contains(t, warning, "Versa rejected the timeseries request with HTTP 400, using tableData for it instead")
	for _, leaked := range []string{"testpass", "testuser", "https://", "10.0.0.1", "9182", "/tenants/", "datadog", "count=2000", "director.example"} {
		assert.NotContains(t, warning, leaked)
	}
}

func TestSanitizeAPIMessage(t *testing.T) {
	// A message with nothing sensitive keeps its words, minus request like tokens.
	client := &Client{username: "operator", password: "s3cret-pass", sessionToken: "csrf-abc123"}
	message := client.sanitizeAPIMessage("Invalid metric: AVAILABILITY at /versa/x?y=1 host:8443\n\n  "+strings.Repeat("x", 200), "tenant-one")
	assert.True(t, strings.HasPrefix(message, "Invalid metric: AVAILABILITY at x"), message)
	assert.NotContains(t, message, "/versa")
	assert.NotContains(t, message, "8443")
	assert.LessOrEqual(t, len(message), maxLoggedMessageLength)

	filler := strings.Repeat("a", 195) + " "
	for _, tt := range []struct {
		name    string
		client  *Client
		tenant  string
		message string
	}{
		{"secret straddling the 200 character mark", &Client{password: "Pa55word!"}, "t1", filler + "Pa55word!"},
		{"one character password", &Client{password: "x"}, "tenant-one", "invalid value x"},
		{"two character password", &Client{password: "q7"}, "tenant-one", "rejected q7"},
		{"password containing the username", &Client{username: "admin", password: "admin2024"}, "tenant-one", "bad login admin2024"},
		{"username containing the password", &Client{username: "ops-admin", password: "admin"}, "tenant-one", "unknown user ops-admin"},
		{"secret in another letter case", &Client{password: "Secret-Value"}, "tenant-one", "rejected SECRET-VALUE"},
		{"secret repeated", &Client{password: "hunter22"}, "tenant-one", "hunter22 and hunter22 again"},
		{"url encoded secret", &Client{password: "p@ss&word"}, "tenant-one", "bad value p%40ss%26word"},
		{"json escaped secret", &Client{password: "a&b/c"}, "tenant-one", `bad value a\u0026b\/c`},
		{"session token", &Client{sessionToken: "csrf-abc123"}, "tenant-one", "token csrf-abc123 expired"},
		{"tenant", &Client{}, "Tenant-One", "no access to tenant-one"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Empty(t, tt.client.sanitizeAPIMessage(tt.message, tt.tenant))
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
			fallbackClient := errorServer(t, tt.prefix, tt.table, map[string]int{})
			fallback, err := tt.get(fallbackClient)
			require.NoError(t, err)
			require.Len(t, fallbackClient.TakeFallbacks(), 1)

			tableClient := errorServer(t, tt.prefix, tt.table, map[string]int{})
			WithAnalyticsQueryType(QueryTypeTableData)(tableClient)
			table, err := tt.get(tableClient)
			require.NoError(t, err)

			assert.Equal(t, table, fallback)
			assert.Contains(t, fmt.Sprintf("%+v", fallback), "PercentDrop:0.25 Bandwidth:0 PercentDropUnset:false")
		})
	}
}
