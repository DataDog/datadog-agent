// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	joined, missing, conflicting := joinAttributes(rows, [][]interface{}{
		{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 9.0},
		// An identical duplicate row is not a conflict.
		{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 8.0},
	}, layout)
	assert.Equal(t, 1, missing)
	assert.Zero(t, conflicting)
	require.Len(t, joined, 1)
	assert.Equal(t, []interface{}{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 1.0}, joined[0].row)
}

func TestGetLinkStatusMetricsTimeseries(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "linkstatus(", "",
		timeseriesBody(t, []string{"site-001,circuit-a"}, []string{"availability"}, 97), counts)

	metrics, err := client.GetLinkStatusMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	assert.Equal(t, LinkStatusMetrics{
		DrillKey: "site-001,circuit-a", Site: "site-001", AccessCircuit: "circuit-a",
		Availability: 97, Timestamp: bandwidthBuckets[0],
	}, metrics[0])
	assert.Equal(t, 98.0, metrics[1].Availability)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1}, counts)
}

func TestLinkStatusTimeseriesKeyPartsMismatchFallsBackToTableData(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "linkstatus(",
		tableBody(t, [][]any{{"site-001,circuit-a", "site-001", "circuit-a", 99.5}}),
		timeseriesBody(t, []string{"site-001"}, []string{"availability"}, 97), counts)

	metrics, err := client.GetLinkStatusMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Zero(t, metrics[0].Timestamp)
	assert.Equal(t, 99.5, metrics[0].Availability)
}

// Two tableData rows for one entity that disagree on an attribute make the entity
// unusable: its buckets are skipped rather than tagged from whichever row came last.
func TestJoinAttributesSkipsConflictingEntities(t *testing.T) {
	rows := []timeseriesRow{
		{timestamp: 1, row: []interface{}{"site-001,circuit-a", "site-001", "circuit-a", 1.0}},
		{timestamp: 2, row: []interface{}{"site-001,circuit-a", "site-001", "circuit-a", 2.0}},
		{timestamp: 1, row: []interface{}{"site-002,circuit-b", "site-002", "circuit-b", 3.0}},
	}
	layout := &timeseriesLayout{drillKeyFields: 2, attributeColumns: 3}
	joined, missing, conflicting := joinAttributes(rows, [][]interface{}{
		{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 9.0},
		{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.2", 9.0},
		{"site-002,circuit-b", "site-002", "circuit-b", "10.0.0.3", 9.0},
	}, layout)
	assert.Zero(t, missing)
	assert.Equal(t, 2, conflicting)
	require.Len(t, joined, 1)
	assert.Equal(t, "10.0.0.3", joined[0].row[3])
}

func TestGetLinkUsageMetricsTimeseriesSkipsConflictingAttributes(t *testing.T) {
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
	require.Len(t, metrics, 2)
	for _, m := range metrics {
		assert.Equal(t, "site-002,circuit-b", m.DrillKey)
		assert.Equal(t, "10.0.0.3", m.IP)
	}
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
		if strings.HasPrefix(q.Get("q"), "linkstatus(") && q.Get("qt") == QueryTypeTimeseries {
			w.Write([]byte(timeseriesBody(t, []string{"site-001,circuit-a"}, []string{"availability"}, 97)))
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

			status, err := client.GetLinkStatusMetrics("datadog")
			require.NoError(t, err)
			require.Len(t, status, 2)
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
			if strings.HasPrefix(r.URL.Query().Get("q"), "linkstatus(") {
				counts[r.URL.Query().Get("qt")]++
				if r.URL.Query().Get("qt") == QueryTypeTimeseries {
					w.Write([]byte(timeseriesBody(t, []string{"site-001"}, []string{"availability"}, 97)))
				} else {
					w.Write([]byte(tableBody(t, [][]any{{"site-001,circuit-a", "site-001", "circuit-a", 99.5}})))
				}
				return
			}
			mock.Config.Handler.ServeHTTP(w, r)
		}))
		client, err := testClient(server, WithWarnLimiter(limiter))
		require.NoError(t, err)
		client.directorEndpoint = server.URL
		WithAnalyticsQueryType(QueryTypeTimeseries)(client)
		_, err = client.GetLinkStatusMetrics("datadog")
		require.NoError(t, err)
		server.Close()
		mock.Close()
	}
	logger.Flush()
	var warnings, debugs int
	for _, line := range strings.Split(logs.String(), "\n") {
		if !strings.Contains(line, "returned an entity key with 1 parts") {
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
			// Two tunnels sharing an appliance and local IP: their attributes conflict.
			tunnelTableRow("appliance-002,10.0.0.2", "appliance-002", "10.0.0.2", "10.1.0.2", "profile-a"),
			tunnelTableRow("appliance-002,10.0.0.2", "appliance-002", "10.0.0.2", "10.1.0.3", "profile-b"),
		}),
		timeseriesBody(t, []string{"appliance-001,10.0.0.1", "appliance-002,10.0.0.2"}, tunnelMetricNames, 100), counts)

	metrics, err := client.GetTunnelMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	// Values keep the requested metric order, so they land in the same fields as for
	// tableData.
	assert.Equal(t, TunnelMetrics{
		DrillKey: "appliance-001,10.0.0.1", Appliance: "appliance-001", LocalIP: "10.0.0.1",
		RemoteIP: "10.1.0.1", VpnProfName: "profile-a",
		VolumeRx: 100, VolumeTx: 110, Timestamp: bandwidthBuckets[0],
	}, metrics[0])
	assert.Equal(t, bandwidthBuckets[1], metrics[1].Timestamp)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts)
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

var siteMetricNames = []string{"volume-tx", "volume-rx", "bw-tx", "bw-rx", "availability"}

func siteTableRow(site, address string) []any {
	return []any{site, address, "40.0", "-70.0", "manual", 9.0, 9.0, 9.0, 9.0, 9.0}
}

// Site metrics have no drill key column: the joined row starts with the site and its
// address, coordinates and location source from tableData.
func TestGetSiteMetricsTimeseriesJoinsAttributes(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "linkusage(site,site.address",
		tableBody(t, [][]any{
			siteTableRow("site-001", "1 Example Street"),
			siteTableRow("site-002", "2 Example Street"),
			siteTableRow("site-002", "3 Example Street"),
		}),
		// site-002 conflicts and site-003 has no tableData row; both are skipped.
		timeseriesBody(t, []string{"site-001", "site-002", "site-003"}, siteMetricNames, 10), counts)

	metrics, err := client.GetSiteMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 2)
	assert.Equal(t, SiteMetrics{
		Site: "site-001", Address: "1 Example Street", Latitude: "40.0", Longitude: "-70.0", LocationSource: "manual",
		VolumeTx: 10, VolumeRx: 20, BandwidthTx: 30, BandwidthRx: 40, Availability: 50,
		Timestamp: bandwidthBuckets[0],
	}, metrics[0])
	assert.Equal(t, bandwidthBuckets[1], metrics[1].Timestamp)
	assert.Equal(t, map[string]int{QueryTypeTimeseries: 1, QueryTypeTableData: 1}, counts)
}

func TestSiteTimeseriesKeyPartsMismatchFallsBackToTableData(t *testing.T) {
	counts := map[string]int{}
	client := bandwidthServer(t, "linkusage(site,site.address",
		tableBody(t, [][]any{siteTableRow("site-001", "1 Example Street")}),
		timeseriesBody(t, []string{"site-001,1 Example Street"}, siteMetricNames, 10), counts)

	metrics, err := client.GetSiteMetrics("datadog")
	require.NoError(t, err)
	require.Len(t, metrics, 1)
	assert.Zero(t, metrics[0].Timestamp)
	assert.Equal(t, "1 Example Street", metrics[0].Address)
}
