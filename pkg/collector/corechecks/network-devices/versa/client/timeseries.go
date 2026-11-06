// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"fmt"
	"sort"
	"strings"
)

// timeseriesRow is one entity in one time bucket, laid out exactly like an aaData row so
// that the positional parser for the endpoint can consume it unchanged.
type timeseriesRow struct {
	timestamp int64
	row       []interface{}
}

// groupByFieldCount returns the number of group by fields in an Analytics query
// expression such as "slam(localsite,remotesite,fc)". A timeseries response does not
// return those fields as separate columns the way tableData does, only joined together
// into the series name, so the count is how we know where to split the name back apart.
func groupByFieldCount(query string) (int, error) {
	open := strings.Index(query, "(")
	closeParen := strings.LastIndex(query, ")")
	if open < 0 || closeParen < open {
		return 0, fmt.Errorf("could not parse group by fields from query %q", query)
	}
	args := strings.TrimSpace(query[open+1 : closeParen])
	if args == "" {
		return 0, nil
	}
	return strings.Count(args, ",") + 1, nil
}

// flattenTimeseries converts the per metric series of a timeseries response into
// tableData shaped rows, one per entity per time bucket: the drill key, then the group by
// fields recovered from it, then the metric values in the order they were requested.
//
// A bucket is only emitted once every requested metric has reported a value for it, since
// the parsers index columns positionally and a missing value would shift everything after
// it. The count of buckets dropped for that reason is returned so the caller can log it.
func flattenTimeseries(series []AnalyticsTimeseries, metrics []string, groupBy int) ([]timeseriesRow, int) {
	metricIndex := make(map[string]int, len(metrics))
	for i, metric := range metrics {
		metricIndex[metric] = i
	}

	// bucket collects the values of every requested metric for one entity at one time.
	type bucket struct {
		values []interface{}
		filled int
	}
	buckets := make(map[string]map[int64]*bucket)

	for _, s := range series {
		index, ok := metricIndex[s.Metric]
		if !ok {
			// A metric we did not ask for. Ignoring it is safer than appending it, which
			// would widen every row past what the parser expects.
			continue
		}
		byTime, ok := buckets[s.Name]
		if !ok {
			byTime = make(map[int64]*bucket)
			buckets[s.Name] = byTime
		}
		for _, point := range s.Data {
			if len(point) != 2 {
				continue
			}
			millis, ok := point[0].(float64)
			if !ok {
				continue
			}
			value, ok := point[1].(float64)
			if !ok {
				// Versa sends null for a bucket the entity did not report in.
				continue
			}
			timestamp := int64(millis)
			b, ok := byTime[timestamp]
			if !ok {
				b = &bucket{values: make([]interface{}, len(metrics))}
				byTime[timestamp] = b
			}
			if b.values[index] == nil {
				b.filled++
			}
			b.values[index] = value
		}
	}

	names := make([]string, 0, len(buckets))
	for name := range buckets {
		names = append(names, name)
	}
	sort.Strings(names)

	var rows []timeseriesRow
	var dropped int
	for _, name := range names {
		byTime := buckets[name]
		fields := strings.Split(name, ",")
		if len(fields) != groupBy {
			// The drill key does not split into the fields the query asked for, so we
			// cannot tell which value belongs to which column.
			dropped += len(byTime)
			continue
		}

		timestamps := make([]int64, 0, len(byTime))
		for timestamp := range byTime {
			timestamps = append(timestamps, timestamp)
		}
		sort.Slice(timestamps, func(i, j int) bool { return timestamps[i] < timestamps[j] })

		for _, timestamp := range timestamps {
			b := byTime[timestamp]
			if b.filled != len(metrics) {
				dropped++
				continue
			}
			row := make([]interface{}, 0, 1+groupBy+len(metrics))
			row = append(row, name)
			for _, field := range fields {
				row = append(row, field)
			}
			row = append(row, b.values...)
			rows = append(rows, timeseriesRow{timestamp: timestamp, row: row})
		}
	}

	return rows, dropped
}
