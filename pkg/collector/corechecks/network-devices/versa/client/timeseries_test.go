// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

package client

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/network-devices/versa/client/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestTimeseriesLayoutsMatchTableData pins the contract that makes the whole flatten
// approach work: for an endpoint we claim supports timeseries, the drill key must split
// into exactly drillKeyFields parts, and the parser's expected row width must be the drill
// key plus those parts plus one column per requested metric.
//
// This is a regression test. The drill key is not always the group by arguments joined:
// linkusage and tunnelstats take attribute arguments that key on a subset, and site
// metrics have no drill key column at all, which is why those endpoints have no layout and
// stay on tableData.
func TestTimeseriesLayoutsMatchTableData(t *testing.T) {
	tests := []struct {
		name        string
		fixture     string
		layout      *timeseriesLayout
		metricCount int
		parserWidth int
	}{
		{name: "sla", fixture: fixtures.GetSLAMetrics, layout: slaTimeseriesLayout, metricCount: 6, parserWidth: 12},
		{name: "path qos", fixture: fixtures.GetPathQoSMetrics, layout: pathQoSTimeseriesLayout, metricCount: 16, parserWidth: 19},
		{name: "circuit qos", fixture: fixtures.GetCircuitQoSMetrics, layout: circuitQoSTimeseriesLayout, metricCount: 16, parserWidth: 19},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var resp AnalyticsMetricsResponse
			require.NoError(t, json.Unmarshal([]byte(tt.fixture), &resp))
			require.NotEmpty(t, resp.AaData)

			drillKey, ok := resp.AaData[0][0].(string)
			require.True(t, ok, "column zero must be the drill key")
			assert.Len(t, strings.Split(drillKey, ","), tt.layout.drillKeyFields)

			assert.Equal(t, tt.parserWidth, len(resp.AaData[0]), "tableData row width")
			assert.Equal(t, tt.parserWidth, 1+tt.layout.drillKeyFields+tt.metricCount,
				"flattened row width must match what the parser expects")
		})
	}
}

func series(name, metric string, points ...[]interface{}) AnalyticsTimeseries {
	return AnalyticsTimeseries{Name: name, Metric: metric, Data: points}
}

func point(ts int64, value interface{}) []interface{} {
	return []interface{}{float64(ts), value}
}

func TestFlattenTimeseries(t *testing.T) {
	const name = "branch-1,hub-1,INET-1,INET-1,fc_nc"
	rows, dropped := flattenTimeseries([]AnalyticsTimeseries{
		// Deliberately newest first, to show the output does not depend on input order.
		series(name, "delay", point(1700001800000, 97.0), point(1700000900000, 101.0), point(1700000000000, 99.0)),
		series(name, "fwdLossRatio", point(1700000000000, 0.0), point(1700000900000, 0.5), point(1700001800000, 0.0)),
	}, []string{"delay", "fwdLossRatio"}, 5)

	// Three buckets in, two out: the newest is held back as still filling.
	require.Equal(t, 1, dropped)
	require.Len(t, rows, 2)

	assert.Equal(t, int64(1700000000000), rows[0].timestamp)
	assert.Equal(t, []interface{}{
		name, "branch-1", "hub-1", "INET-1", "INET-1", "fc_nc",
		float64(99), float64(0),
	}, rows[0].row)

	assert.Equal(t, int64(1700000900000), rows[1].timestamp)
	assert.Equal(t, []interface{}{
		name, "branch-1", "hub-1", "INET-1", "INET-1", "fc_nc",
		float64(101), float64(0.5),
	}, rows[1].row)
}

func TestFlattenTimeseriesHoldsBackNewestBucketPerEntity(t *testing.T) {
	rows, dropped := flattenTimeseries([]AnalyticsTimeseries{
		series("branch-1,hub-1", "delay", point(1700000000000, 99.0), point(1700000900000, 98.0)),
		series("branch-2,hub-1", "delay", point(1700000000000, 50.0), point(1700000900000, 51.0)),
	}, []string{"delay"}, 2)

	// One bucket held back for each entity, not one overall.
	require.Equal(t, 2, dropped)
	require.Len(t, rows, 2)
	assert.Equal(t, "branch-1,hub-1", rows[0].row[0])
	assert.Equal(t, int64(1700000000000), rows[0].timestamp)
	assert.Equal(t, "branch-2,hub-1", rows[1].row[0])
	assert.Equal(t, int64(1700000000000), rows[1].timestamp)
}

func TestFlattenTimeseriesDropsIncompleteBuckets(t *testing.T) {
	rows, dropped := flattenTimeseries([]AnalyticsTimeseries{
		series("branch-1,hub-1", "delay",
			point(1700000000000, 99.0), point(1700000900000, 101.0), point(1700001800000, 97.0)),
		series("branch-1,hub-1", "fwdLossRatio",
			// Versa reports null for a bucket the entity did not report in. Emitting a row
			// for it would shift the remaining columns, so the bucket is dropped.
			point(1700000000000, 0.0), point(1700000900000, nil), point(1700001800000, 0.0)),
	}, []string{"delay", "fwdLossRatio"}, 2)

	// The incomplete middle bucket plus the newest one held back.
	require.Equal(t, 2, dropped)
	require.Len(t, rows, 1)
	assert.Equal(t, int64(1700000000000), rows[0].timestamp)
}

func TestFlattenTimeseriesIgnoresUnrequestedMetrics(t *testing.T) {
	rows, dropped := flattenTimeseries([]AnalyticsTimeseries{
		series("branch-1,hub-1", "delay", point(1700000000000, 99.0), point(1700000900000, 98.0)),
		series("branch-1,hub-1", "somethingElse", point(1700000000000, 7.0), point(1700000900000, 7.0)),
	}, []string{"delay"}, 2)

	require.Equal(t, 1, dropped)
	require.Len(t, rows, 1)
	assert.Equal(t, []interface{}{"branch-1,hub-1", "branch-1", "hub-1", float64(99)}, rows[0].row)
}

func TestFlattenTimeseriesDropsUnsplittableNames(t *testing.T) {
	// The layout says three fields but the drill key only splits into two, so there is no
	// way to tell which value belongs in which column.
	rows, dropped := flattenTimeseries([]AnalyticsTimeseries{
		series("branch-1,hub-1", "delay", point(1700000000000, 99.0), point(1700000900000, 98.0)),
	}, []string{"delay"}, 3)

	assert.Empty(t, rows)
	assert.Equal(t, 2, dropped)
}

func TestFlattenTimeseriesRepeatedSeriesDoesNotDoubleCount(t *testing.T) {
	rows, dropped := flattenTimeseries([]AnalyticsTimeseries{
		series("branch-1,hub-1", "delay", point(1700000000000, 99.0), point(1700000900000, 98.0)),
		series("branch-1,hub-1", "delay", point(1700000000000, 42.0)),
	}, []string{"delay"}, 2)

	require.Equal(t, 1, dropped)
	require.Len(t, rows, 1)
	// The later series wins rather than widening the row or emitting it twice.
	assert.Equal(t, float64(42), rows[0].row[3])
}

// TestFlattenTimeseriesRealShape runs a response shaped like a real Director reply (many
// paths, sixteen buckets, all six SLA metrics, one null) through flatten and the SLA
// parser, pinning row count, row width and the per path holdback.
func TestFlattenTimeseriesRealShape(t *testing.T) {
	var resp AnalyticsTimeseriesResponse
	require.NoError(t, json.Unmarshal([]byte(fixtures.GetSLAMetricsTimeseriesRealShape), &resp))
	metrics := []string{"delay", "fwdDelayVar", "revDelayVar", "fwdLossRatio", "revLossRatio", "pduLossRatio"}
	require.Len(t, resp.Data, 6*len(metrics))

	rows, dropped := flattenTimeseries(resp.Data, metrics, slaTimeseriesLayout.drillKeyFields)

	// Six newest buckets held back (one per path) plus the bucket with the null.
	assert.Equal(t, 7, dropped)
	require.Len(t, rows, 6*16-7)

	const newest = int64(1793966400000 + 15*900000)
	perPath := map[string]int{}
	for _, r := range rows {
		assert.Len(t, r.row, 12, "row width must match parseSLAMetrics")
		assert.NotEqual(t, newest, r.timestamp, "the still filling bucket must be held back")
		perPath[r.row[0].(string)]++
	}
	require.Len(t, perPath, 6)
	for name, n := range perPath {
		if name == "site-002,hub-002,circuit-a,circuit-b,fc_be" {
			assert.Equal(t, 14, n, name)
		} else {
			assert.Equal(t, 15, n, name)
		}
	}

	data := make([][]interface{}, len(rows))
	for i, r := range rows {
		data[i] = r.row
	}
	parsed, err := parseSLAMetrics(data)
	require.NoError(t, err)
	require.Len(t, parsed, len(rows))
	assert.Equal(t, "site-001", parsed[0].LocalSite)
	assert.Equal(t, "fc_ef", parsed[0].ForwardingClass)
	assert.Equal(t, 20.0, parsed[0].Delay)
	assert.Equal(t, 22.5, parsed[0].PDULossRatio)
}
