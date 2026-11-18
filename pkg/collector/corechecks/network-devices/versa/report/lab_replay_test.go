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
	// collect gets the endpoint's rows and sends them, returning the number of rows.
	collect func(c *client.Client, s *Sender) ([]any, error)
}

var labEndpoints = []labEndpoint{
	{"sla", "slam(", func(c *client.Client, s *Sender) ([]any, error) {
		m, err := c.GetSLAMetrics("datadog")
		s.SendSLAMetrics("datadog", m, nil)
		return toAny(m), err
	}},
	{"cos", "cos(", func(c *client.Client, s *Sender) ([]any, error) {
		m, err := c.GetCircuitQoSMetrics("datadog")
		s.SendCircuitQoSMetrics("datadog", m, nil)
		return toAny(m), err
	}},
	{"intfutil", "intfUtil(", func(c *client.Client, s *Sender) ([]any, error) {
		m, err := c.GetAnalyticsInterfaces("datadog")
		s.SendAnalyticsInterfaceMetrics("datadog", m, nil)
		return toAny(m), err
	}},
	{"linkusage", "linkusage(site,accckt", func(c *client.Client, s *Sender) ([]any, error) {
		m, err := c.GetLinkUsageMetrics("datadog")
		s.SendLinkUsageMetrics("datadog", m, nil)
		return toAny(m), err
	}},
	{"dia", "usage(", func(c *client.Client, s *Sender) ([]any, error) {
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
func labClient(t *testing.T, ep labEndpoint, queryType string) (*client.Client, map[string]int) {
	counts := map[string]int{}
	mockAPI := client.SetupMockAPIServer()
	t.Cleanup(mockAPI.Close)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Query().Get("q"), ep.prefix) {
			qt := r.URL.Query().Get("qt")
			counts[qt]++
			w.Header().Set("Content-Type", "application/json")
			w.Write(readLab(t, ep.name+"_"+qt))
			return
		}
		mockAPI.Config.Handler.ServeHTTP(w, r)
	}))
	t.Cleanup(server.Close)
	c, err := client.NewTestClient(server, client.WithAnalyticsQueryType(queryType))
	require.NoError(t, err)
	return c, counts
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
	tags string
	ts   float64
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
		out[call.Arguments.String(0)] = append(out[call.Arguments.String(0)], submission{strings.Join(sorted, ","), ts})
	}
	return out, rows, counts
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
