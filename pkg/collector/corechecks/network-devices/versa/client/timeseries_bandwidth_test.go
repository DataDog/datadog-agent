// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
	joined, missing := joinAttributes(rows, [][]interface{}{
		{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 9.0},
	}, layout)
	assert.Equal(t, 1, missing)
	require.Len(t, joined, 1)
	assert.Equal(t, []interface{}{"site-001,circuit-a", "site-001", "circuit-a", "10.0.0.1", 1.0}, joined[0].row)
}
