// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
	"reflect"
	"sort"
	"strings"
)

// timeseriesRow is one entity in one time bucket, laid out exactly like an aaData row so
// that the positional parser for the endpoint can consume it unchanged.
type timeseriesRow struct {
	timestamp int64
	row       []interface{}
}

// timeseriesLayout describes how to rebuild tableData shaped rows from a timeseries
// response for one endpoint. Only endpoints that have a layout support timeseries; the
// rest fall back to tableData, deliberately.
//
// Two things make this per endpoint rather than derivable from the query. The drill key is
// not always the group by arguments joined together: linkusage and tunnelstats take
// attribute arguments such as accckt.ip that key on a subset, so counting the arguments
// over-counts the parts. And not every parser expects a leading drill key column at all:
// parseSiteMetrics reads the site name from column zero.
type timeseriesLayout struct {
	// drillKeyFields is the number of comma separated parts the drill key splits into,
	// which is also the number of columns the parser expects between the drill key and
	// the first metric value.
	drillKeyFields int
	// attributeColumns, when non zero, is the number of string columns the parser expects
	// between the drill key and the first metric value that a timeseries response cannot
	// supply. Attribute arguments such as accckt.ip key on a subset of the query, so they
	// are not in the series name; these columns are copied from the tableData row with the
	// same drill key instead, and a bucket whose entity has no such row is skipped.
	attributeColumns int
	// strictKeyParts checks every series name splits into drillKeyFields parts before
	// using the timeseries response, and falls back to tableData for the call when one
	// does not. It is set for layouts not yet checked against a real timeseries response.
	strictKeyParts bool
}

// Layouts for the endpoints verified against real timeseries responses. Adding one for a
// further endpoint means checking its drill key against its parser's column count first,
// ideally against a captured response.
var (
	slaTimeseriesLayout        = &timeseriesLayout{drillKeyFields: 5}
	pathQoSTimeseriesLayout    = &timeseriesLayout{drillKeyFields: 2}
	circuitQoSTimeseriesLayout = &timeseriesLayout{drillKeyFields: 2}
)

// Layouts for the bandwidth endpoints. Each follows the query's group by fields and its
// tableData drill key; they are validated with synthetic responses, and confirmation
// against customer provided Versa responses is pending. Until then each checks its key
// part count and falls back to tableData when a response does not match the expected
// layout, and the two with attribute arguments take those columns from tableData.
var (
	// intfUtil(site,accCkt,intf): plain arguments, so the series name holds every column.
	interfaceTimeseriesLayout = &timeseriesLayout{drillKeyFields: 3, strictKeyParts: true}
	// linkstatus(site,accckt): plain arguments, so the series name holds every column.
	linkStatusTimeseriesLayout = &timeseriesLayout{drillKeyFields: 2, strictKeyParts: true}
	// linkusage(site,accckt,accckt.uplinkBW,...): keyed on site,accckt; the parser also
	// needs site, accckt, uplink and downlink BW, type, media, ip and isp.
	linkUsageTimeseriesLayout = &timeseriesLayout{drillKeyFields: 2, attributeColumns: 8, strictKeyParts: true}
	// usage(site,accckt,accckt.ip): keyed on site,accckt,ip in tableData; site, accckt and
	// ip are still taken from tableData so a differently ordered key cannot mislabel them.
	diaTimeseriesLayout = &timeseriesLayout{drillKeyFields: 3, attributeColumns: 3, strictKeyParts: true}
	// tunnelstats(appliance,ipsecLocalIp,ipsecPeerIp,ipsecVpnProfName): keyed on
	// appliance,ipsecLocalIp in tableData, so the peer IP and VPN profile come from
	// tableData. Tunnels sharing an appliance and local IP but with different peers have
	// conflicting attributes and are skipped.
	tunnelTimeseriesLayout = &timeseriesLayout{drillKeyFields: 2, attributeColumns: 4, strictKeyParts: true}
)

// keyPartsMismatch returns the first series name that does not split into the layout's
// drill key part count, if any.
func keyPartsMismatch(series []AnalyticsTimeseries, layout *timeseriesLayout) (string, bool) {
	for _, s := range series {
		if len(strings.Split(s.Name, ",")) != layout.drillKeyFields {
			return s.Name, true
		}
	}
	return "", false
}

// joinAttributes replaces the drill key fields of each flattened row with the leading
// columns of the tableData row for the same entity: the drill key and attributeColumns
// attribute values. Attribute tags therefore always come from Versa's own response. An
// entity is skipped when it has no tableData row, or when its tableData rows disagree on
// those columns, since either choice would make the tags depend on row order; identical
// duplicate rows are fine. The counts of buckets skipped for each reason are returned.
func joinAttributes(rows []timeseriesRow, tableRows [][]interface{}, layout *timeseriesLayout) (joined []timeseriesRow, missing int, conflicting int) {
	prefix := 1 + layout.attributeColumns
	byKey := make(map[string][]interface{}, len(tableRows))
	conflicts := make(map[string]bool)
	for _, row := range tableRows {
		if len(row) < prefix {
			continue
		}
		key, ok := row[0].(string)
		if !ok {
			continue
		}
		if existing, seen := byKey[key]; seen && !reflect.DeepEqual(existing, row[:prefix]) {
			conflicts[key] = true
			continue
		}
		byKey[key] = row[:prefix]
	}
	joined = rows[:0]
	for _, r := range rows {
		key, _ := r.row[0].(string)
		if conflicts[key] {
			conflicting++
			continue
		}
		attrs, ok := byKey[key]
		if !ok {
			missing++
			continue
		}
		values := r.row[1+layout.drillKeyFields:]
		row := make([]interface{}, 0, prefix+len(values))
		row = append(row, attrs...)
		row = append(row, values...)
		joined = append(joined, timeseriesRow{timestamp: r.timestamp, row: row})
	}
	return joined, missing, conflicting
}

// flattenTimeseries converts the per metric series of a timeseries response into
// tableData shaped rows, one per entity per time bucket: the drill key, then the group by
// fields recovered from it, then the metric values in the order they were requested.
//
// A bucket is only emitted once every requested metric has reported a value for it, since
// the parsers index columns positionally and a missing value would shift everything after
// it. The newest bucket of each entity is also held back, because Versa buckets are wall
// clock aligned and the current one is still filling: publishing it would pin a partial
// value at that timestamp, and the dedupe would then refuse the complete value that
// arrives on the next run. The count of buckets dropped for either reason is returned so
// the caller can log it.
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

		for i, timestamp := range timestamps {
			if i == len(timestamps)-1 {
				// The newest bucket is still accumulating; it lands on the next run.
				dropped++
				continue
			}
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
