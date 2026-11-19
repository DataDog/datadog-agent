// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package report

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client"
)

// The testdata/lab files are anonymized tableData and timeseries responses captured from a
// Versa lab with traffic, for the same window. Site, circuit, interface, IP and ISP names
// were replaced consistently across files.

type labEndpoint struct {
	name   string // testdata file prefix
	prefix string // analytics query prefix
	// metrics are the requested metric ids, in request order, and gauges the metric
	// each is submitted as, without the versa. prefix.
	metrics, gauges []string
	// keyTags are the tags whose values, joined with commas, make the drill key, and
	// keyParts how many leading series name parts make it.
	keyTags  []string
	keyParts int
	// collect gets the endpoint's rows and sends them, returning the number of rows.
	collect func(c *client.Client, s *Sender) ([]any, error)
}

var labEndpoints = []labEndpoint{
	{"sla", "slam(",
		[]string{"delay", "fwdDelayVar", "revDelayVar", "fwdLossRatio", "revLossRatio", "pduLossRatio"},
		[]string{"sla.delay", "sla.fwd_delay_var", "sla.rev_delay_var", "sla.fwd_loss_ratio", "sla.rev_loss_ratio", "sla.pdu_loss_ratio"},
		[]string{"local_site", "remote_site", "local_access_circuit", "remote_access_circuit", "forwarding_class"}, 5, func(c *client.Client, s *Sender) ([]any, error) {
			m, err := c.GetSLAMetrics("datadog")
			s.SendSLAMetrics("datadog", m, nil)
			return toAny(m), err
		}},
	{"cos", "cos(",
		[]string{"betx", "betxdrop", "eftx", "eftxdrop", "aftx", "aftxdrop", "nctx", "nctxdrop", "bebandwidth", "efbandwidth", "afbandwidth", "ncbandwidth", "volume-tx", "totaldrop", "percentdrop", "bandwidth"},
		[]string{"circuit_qos.best_effort_tx", "circuit_qos.best_effort_tx_drop", "circuit_qos.expedited_forward_tx", "circuit_qos.expedited_forward_drop", "circuit_qos.assured_forward_tx", "circuit_qos.assured_forward_drop", "circuit_qos.network_control_tx", "circuit_qos.network_control_drop", "circuit_qos.best_effort_bandwidth", "circuit_qos.expedited_forward_bw", "circuit_qos.assured_forward_bw", "circuit_qos.network_control_bw", "circuit_qos.volume_tx", "circuit_qos.total_drop", "circuit_qos.percent_drop", "circuit_qos.bandwidth"},
		[]string{"site", "access_circuit"}, 2, func(c *client.Client, s *Sender) ([]any, error) {
			m, err := c.GetCircuitQoSMetrics("datadog")
			s.SendCircuitQoSMetrics("datadog", m, nil)
			return toAny(m), err
		}},
	{"intfutil", "intfUtil(",
		[]string{"rxUtil", "txUtil", "volume-rx", "volume-tx", "volume", "bw-rx", "bw-tx", "bandwidth"},
		[]string{"interface.rx_util", "interface.tx_util", "interface.volume_rx", "interface.volume_tx", "interface.volume", "interface.bandwidth_rx", "interface.bandwidth_tx", "interface.bandwidth"},
		[]string{"site", "access_circuit", "interface"}, 3, func(c *client.Client, s *Sender) ([]any, error) {
			m, err := c.GetAnalyticsInterfaces("datadog")
			s.SendAnalyticsInterfaceMetrics("datadog", m, nil)
			return toAny(m), err
		}},
	{"linkusage", "linkusage(site,accckt",
		[]string{"volume-tx", "volume-rx", "bw-tx", "bw-rx"},
		[]string{"link.volume_tx", "link.volume_rx", "link.bandwidth_tx", "link.bandwidth_rx"},
		[]string{"site", "access_circuit"}, 2, func(c *client.Client, s *Sender) ([]any, error) {
			m, err := c.GetLinkUsageMetrics("datadog")
			s.SendLinkUsageMetrics("datadog", m, nil)
			return toAny(m), err
		}},
	{"dia", "usage(",
		[]string{"volume-tx", "volume-rx", "bw-tx", "bw-rx"},
		[]string{"dia.volume_tx", "dia.volume_rx", "dia.bandwidth_tx", "dia.bandwidth_rx"},
		[]string{"site", "access_circuit"}, 2, func(c *client.Client, s *Sender) ([]any, error) {
			m, err := c.GetDIAMetrics("datadog")
			s.SendDIAMetrics("datadog", m, nil)
			return toAny(m), err
		}},
}

func readLab(t *testing.T, name string) []byte {
	b, err := os.ReadFile(filepath.Join("testdata", "lab", name+".json"))
	require.NoError(t, err)
	return b
}

// labClient serves the lab responses for one endpoint and fails any other qt.
// The returned function snapshots the request counts by query type; handlers run in the
// server's goroutines, so the counts are guarded.
func labClient(t *testing.T, ep labEndpoint, queryType string) (*client.Client, func() map[string]int) {
	var mu sync.Mutex
	counts := map[string]int{}
	mockAPI := client.SetupMockAPIServer()
	t.Cleanup(mockAPI.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("q"), ep.prefix) {
			qt := r.URL.Query().Get("qt")
			mu.Lock()
			counts[qt]++
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.Write(readLab(t, ep.name+"_"+qt))
			return
		}
		mockAPI.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	c, err := client.NewTestClient(server, client.WithAnalyticsQueryType(queryType))
	require.NoError(t, err)
	return c, func() map[string]int {
		mu.Lock()
		defer mu.Unlock()
		out := make(map[string]int, len(counts))
		for k, v := range counts {
			out[k] = v
		}
		return out
	}
}

// expectedBuckets returns, per series name, the bucket timestamps (ms) that should be
// published: those where every series of the entity has a value, minus the newest.
func expectedBuckets(t *testing.T, raw []byte) (map[string][]int64, int) {
	var resp struct {
		Data []struct {
			Name   string          `json:"name"`
			Metric string          `json:"metric"`
			Data   [][]interface{} `json:"data"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(raw, &resp))
	metricsPerEntity := map[string]map[string]bool{}
	filled := map[string]map[int64]int{}
	for _, s := range resp.Data {
		if metricsPerEntity[s.Name] == nil {
			metricsPerEntity[s.Name] = map[string]bool{}
			filled[s.Name] = map[int64]int{}
		}
		metricsPerEntity[s.Name][s.Metric] = true
		for _, p := range s.Data {
			if p[1] != nil {
				filled[s.Name][int64(p[0].(float64))]++
			}
		}
	}
	out := map[string][]int64{}
	total := 0
	for name, byTime := range filled {
		var all, complete []int64
		for ts := range byTime {
			all = append(all, ts)
		}
		sort.Slice(all, func(i, j int) bool { return all[i] < all[j] })
		for _, ts := range all[:len(all)-1] {
			if byTime[ts] == len(metricsPerEntity[name]) {
				complete = append(complete, ts)
			}
		}
		out[name] = complete
		total += len(complete)
	}
	return out, total
}

type submission struct {
	tags  string
	ts    float64
	value float64
	// key is the drill key rebuilt from the tags.
	key string
}

// valueKey identifies one submitted value: metric, entity and bucket time in seconds.
type valueKey struct {
	metric string
	key    string
	ts     float64
}

// submissions runs the endpoint in a query type and returns, per metric name, the sorted
// tag sets and timestamps submitted.
func submissions(t *testing.T, ep labEndpoint, queryType string) (map[string][]submission, []any, map[string]int) {
	TimeNow = mockTimeNow
	c, counts := labClient(t, ep, queryType)
	ms := mocksender.NewMockSender(t, "testID")
	ms.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	ms.On("GaugeWithTimestamp", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	rows, err := ep.collect(c, NewSender(ms, "default"))
	require.NoError(t, err)
	require.Empty(t, c.TakeFallbacks())
	out := map[string][]submission{}
	for _, call := range ms.Calls {
		var tags []string
		var ts float64
		switch call.Method {
		case "Gauge":
			tags = call.Arguments.Get(3).([]string)
		case "GaugeWithTimestamp":
			tags = call.Arguments.Get(3).([]string)
			ts = call.Arguments.Get(4).(float64)
		default:
			continue
		}
		sorted := append([]string(nil), tags...)
		sort.Strings(sorted)
		out[call.Arguments.String(0)] = append(out[call.Arguments.String(0)], submission{
			tags:  strings.Join(sorted, ","),
			ts:    ts,
			value: call.Arguments.Get(1).(float64),
			key:   keyFromTags(ep, tags),
		})
	}
	return out, rows, counts()
}

func toAny[T any](rows []T) []any {
	out := make([]any, len(rows))
	for i := range rows {
		out[i] = rows[i]
	}
	return out
}

// identity returns a row's string fields, the drill key and the tag sources, keyed by
// field name; numeric fields and the bucket timestamp are left out.
func identity(row any) map[string]string {
	out := map[string]string{}
	v := reflect.ValueOf(row)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).Kind() == reflect.String {
			out[v.Type().Field(i).Name] = v.Field(i).String()
		}
	}
	return out
}

// TestLabResponsesReplay replays the lab responses through client, parser and sender in
// both modes. Timeseries publishes every complete bucket but the newest, at its bucket
// time, under exactly the tags tableData mode submits for the same entity.
func TestLabResponsesReplay(t *testing.T) {
	for _, ep := range labEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			table, tableRows, tableCounts := submissions(t, ep, client.QueryTypeTableData)
			assert.Equal(t, map[string]int{client.QueryTypeTableData: 1}, tableCounts)

			series, seriesRows, seriesCounts := submissions(t, ep, client.QueryTypeTimeseries)
			assert.Equal(t, map[string]int{client.QueryTypeTimeseries: 1}, seriesCounts)

			buckets, total := expectedBuckets(t, readLab(t, ep.name+"_timeseries"))
			require.Positive(t, total)
			assert.Equal(t, total, len(seriesRows), "rows after hold back and all or nothing")
			assert.Len(t, buckets, len(tableRows), "one tableData row per timeseries entity")

			// Every timeseries row has the string fields of a tableData row.
			tableIDs := map[string]bool{}
			for _, row := range tableRows {
				b, _ := json.Marshal(identity(row))
				tableIDs[string(b)] = true
			}
			seriesIDs := map[string]bool{}
			for _, row := range seriesRows {
				b, _ := json.Marshal(identity(row))
				seriesIDs[string(b)] = true
			}
			assert.Equal(t, tableIDs, seriesIDs, "parser string fields")

			wantTimes := map[float64]int{}
			for _, list := range buckets {
				for _, ms := range list {
					wantTimes[float64(ms)/1000]++
				}
			}
			require.Equal(t, sortedKeys(table), sortedKeys(series), "metric names")
			for metric, subs := range series {
				gotTimes := map[float64]int{}
				tagSet := map[string]bool{}
				for _, s := range subs {
					gotTimes[s.ts]++
					tagSet[s.tags] = true
				}
				assert.Equal(t, wantTimes, gotTimes, metric)
				tableTags := map[string]bool{}
				for _, s := range table[metric] {
					assert.Zero(t, s.ts)
					tableTags[s.tags] = true
				}
				assert.Equal(t, tableTags, tagSet, metric)
			}
		})
	}
}

func sortedKeys(m map[string][]submission) []string {
	var keys []string
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// With one bucket per entity, as a window covering a single 15 minute bucket returns,
// every bucket is the newest and is held back, so circuit QoS publishes nothing. With the
// lab's two buckets per entity it publishes one each (TestLabResponsesReplay).
func TestLabCircuitQoSSingleBucketIsHeldBack(t *testing.T) {
	var resp map[string]any
	require.NoError(t, json.Unmarshal(readLab(t, "cos_timeseries"), &resp))
	for _, s := range resp["data"].([]any) {
		series := s.(map[string]any)
		series["data"] = series["data"].([]any)[:1]
	}
	single, err := json.Marshal(resp)
	require.NoError(t, err)

	mockAPI := client.SetupMockAPIServer()
	defer mockAPI.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("q"), "cos(") {
			w.Header().Set("Content-Type", "application/json")
			w.Write(single)
			return
		}
		mockAPI.Config.Handler.ServeHTTP(w, r)
	}))
	defer server.Close()
	c, err := client.NewTestClient(server, client.WithAnalyticsQueryType(client.QueryTypeTimeseries))
	require.NoError(t, err)
	rows, err := c.GetCircuitQoSMetrics("datadog")
	require.NoError(t, err)
	assert.Empty(t, rows)
}

func keyFromTags(ep labEndpoint, tags []string) string {
	values := map[string]string{}
	for _, tag := range tags {
		if name, value, ok := strings.Cut(tag, ":"); ok {
			if _, seen := values[name]; !seen {
				values[name] = value
			}
		}
	}
	parts := make([]string, len(ep.keyTags))
	for i, name := range ep.keyTags {
		parts[i] = values[name]
	}
	return strings.Join(parts, ",")
}

func submittedValues(subs map[string][]submission) map[valueKey]float64 {
	out := map[valueKey]float64{}
	for metric, list := range subs {
		for _, s := range list {
			out[valueKey{metric, s.key, s.ts}] = s.value
		}
	}
	return out
}

// expectedTableValues reads each tableData cell for the gauge it is submitted as.
func expectedTableValues(t *testing.T, ep labEndpoint) map[valueKey]float64 {
	var resp client.AnalyticsMetricsResponse
	require.NoError(t, json.Unmarshal(readLab(t, ep.name+"_tableData"), &resp))
	out := map[valueKey]float64{}
	for _, row := range resp.AaData {
		first := len(row) - len(ep.metrics)
		for i, gauge := range ep.gauges {
			out[valueKey{versaMetricPrefix + gauge, row[0].(string), 0}] = row[first+i].(float64)
		}
	}
	return out
}

// expectedSeriesValues reads, for every published bucket, each series value for the
// gauge it is submitted as. Circuit QoS percent_drop is the bucket's totaldrop over
// volume-tx, and is not sent for a bucket with no volume.
func expectedSeriesValues(t *testing.T, ep labEndpoint) map[valueKey]float64 {
	raw := readLab(t, ep.name+"_timeseries")
	buckets, _ := expectedBuckets(t, raw)
	var resp client.AnalyticsTimeseriesResponse
	require.NoError(t, json.Unmarshal(raw, &resp))
	values := map[string]map[string]map[int64]float64{} // name, metric, ms
	for _, s := range resp.Data {
		if values[s.Name] == nil {
			values[s.Name] = map[string]map[int64]float64{}
		}
		values[s.Name][s.Metric] = map[int64]float64{}
		for _, p := range s.Data {
			if v, ok := p[1].(float64); ok {
				values[s.Name][s.Metric][int64(p[0].(float64))] = v
			}
		}
	}
	out := map[valueKey]float64{}
	for name, published := range buckets {
		key := strings.Join(strings.Split(name, ",")[:ep.keyParts], ",")
		for _, ms := range published {
			for i, metric := range ep.metrics {
				v := values[name][metric][ms]
				if metric == "percentdrop" && ep.name == "cos" {
					volume := values[name]["volume-tx"][ms]
					if volume == 0 {
						continue
					}
					v = values[name]["totaldrop"][ms] / volume
				}
				out[valueKey{versaMetricPrefix + ep.gauges[i], key, float64(ms) / 1000}] = v
			}
		}
	}
	return out
}

// TestLabResponsesReplayValues checks every submitted value against the lab response it
// came from: each tableData gauge equals its tableData cell, and each timeseries gauge
// equals its series value at that bucket, for the same entity.
func TestLabResponsesReplayValues(t *testing.T) {
	for _, ep := range labEndpoints {
		t.Run(ep.name, func(t *testing.T) {
			table, _, _ := submissions(t, ep, client.QueryTypeTableData)
			assert.Equal(t, expectedTableValues(t, ep), submittedValues(table), "tableData values")

			series, _, _ := submissions(t, ep, client.QueryTypeTimeseries)
			want := expectedSeriesValues(t, ep)
			require.NotEmpty(t, want)
			assert.Equal(t, want, submittedValues(series), "timeseries values")
		})
	}
}

// The lab circuit QoS capture has buckets with nonzero drops, where Versa's own
// timeseries percentdrop is 0. Timeseries submits totaldrop/volume-tx for those buckets,
// tableData keeps Versa's value, and both are the same fraction over the whole window.
func TestLabCircuitQoSPercentDrop(t *testing.T) {
	ep := labEndpoints[1]
	require.Equal(t, "cos", ep.name)

	var series client.AnalyticsTimeseriesResponse
	require.NoError(t, json.Unmarshal(readLab(t, "cos_timeseries"), &series))
	values := map[string]map[string]map[float64]float64{} // entity, metric, seconds
	for _, s := range series.Data {
		if values[s.Name] == nil {
			values[s.Name] = map[string]map[float64]float64{}
		}
		values[s.Name][s.Metric] = map[float64]float64{}
		for _, p := range s.Data {
			values[s.Name][s.Metric][p[0].(float64)/1000] = p[1].(float64)
		}
	}

	submitted, _, _ := submissions(t, ep, client.QueryTypeTimeseries)
	var withDrops int
	for _, s := range submitted[versaMetricPrefix+"circuit_qos.percent_drop"] {
		drop := values[s.key]["totaldrop"][s.ts]
		volume := values[s.key]["volume-tx"][s.ts]
		assert.Equal(t, drop/volume, s.value, "%s at %v", s.key, s.ts)
		if drop > 0 {
			withDrops++
			assert.Zero(t, values[s.key]["percentdrop"][s.ts], "Versa's timeseries percentdrop")
			assert.Positive(t, s.value)
		}
	}
	assert.Positive(t, withDrops, "published buckets with drops")

	var table client.AnalyticsMetricsResponse
	require.NoError(t, json.Unmarshal(readLab(t, "cos_tableData"), &table))
	tableSubmitted, _, _ := submissions(t, ep, client.QueryTypeTableData)
	tableDrop := map[string]float64{}
	for _, s := range tableSubmitted[versaMetricPrefix+"circuit_qos.percent_drop"] {
		tableDrop[s.key] = s.value
	}
	for _, row := range table.AaData {
		key := row[0].(string)
		// Columns 15 to 17 are volume-tx, totaldrop and percentdrop. Versa's tableData
		// percentdrop is passed through unchanged.
		assert.Equal(t, row[17].(float64), tableDrop[key], key)

		// Over the whole window, the timeseries buckets add up to the tableData row, so
		// their drop over volume is the tableData fraction. The comparison uses every
		// bucket, including the newest one the check holds back.
		var drop, volume float64
		for ts, v := range values[key]["totaldrop"] {
			drop += v
			volume += values[key]["volume-tx"][ts]
		}
		assert.Equal(t, row[16].(float64), drop, key)
		assert.Equal(t, row[15].(float64), volume, key)
		if volume > 0 {
			assert.InDelta(t, row[17].(float64), drop/volume, 1e-12, key)
		}
	}
}

// Site metrics use tableData. Their tags come straight from the row, including an
// address that contains commas.
func TestLabSiteTableData(t *testing.T) {
	var table client.AnalyticsMetricsResponse
	require.NoError(t, json.Unmarshal(readLab(t, "site_tableData"), &table))
	ep := labEndpoint{name: "site", prefix: "linkusage(site,site.address"}
	c, counts := labClient(t, ep, client.QueryTypeTimeseries)
	metrics, err := c.GetSiteMetrics("datadog")
	require.NoError(t, err)
	assert.Equal(t, map[string]int{client.QueryTypeTableData: 1}, counts())

	ms := mocksender.NewMockSender(t, "testID")
	ms.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	NewSender(ms, "default").SendSiteMetrics("datadog", metrics, nil)

	var want, got []string
	var commaAddress bool
	for _, row := range table.AaData {
		address := row[1].(string)
		commaAddress = commaAddress || strings.Contains(address, ",")
		tags := []string{"site:" + row[0].(string), "address:" + address, "latitude:" + row[2].(string), "longitude:" + row[3].(string), "location_source:" + row[4].(string)}
		for range 5 {
			want = append(want, strings.Join(tags, "|"))
		}
	}
	for _, call := range ms.Calls {
		if call.Method == "Gauge" {
			got = append(got, strings.Join(call.Arguments.Get(3).([]string), "|"))
		}
	}
	require.True(t, commaAddress, "fixture has an address with commas")
	sort.Strings(want)
	sort.Strings(got)
	assert.Equal(t, want, got)
}
