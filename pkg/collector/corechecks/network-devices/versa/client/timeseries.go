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
	// noDrillKeyColumn marks a parser that reads its first attribute from column zero
	// instead of a drill key column. Its joined rows start with the attribute columns,
	// whose first value is the entity key.
	noDrillKeyColumn bool
	// valueOrder, when set, lists the requested metrics in the order the parser reads
	// their values, for a parser whose positions differ from the request order.
	valueOrder []string
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
	// tableData. When tunnels share an appliance and local IP but differ in peer or
	// profile, the key is ambiguous and the call uses tableData.
	//
	// parseTunnelMetrics reads VolumeRx before VolumeTx while the query requests volume-tx
	// first. The tableData column order is to be confirmed against a customer response, so
	// tableData is left as is; timeseries series are named, so valueOrder places each one
	// in the field of the same name.
	tunnelTimeseriesLayout = &timeseriesLayout{drillKeyFields: 2, attributeColumns: 4, strictKeyParts: true, valueOrder: []string{"volume-rx", "volume-tx"}}
	// linkusage(site,site.address,site.latitude,site.longitude,site.locationSource) with
	// jq siteStatus: keyed on site, and parseSiteMetrics has no drill key column, so the
	// joined row is the site, address, latitude, longitude and location source from
	// tableData followed by the metric values.
	siteTimeseriesLayout = &timeseriesLayout{drillKeyFields: 1, attributeColumns: 5, strictKeyParts: true, noDrillKeyColumn: true}
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
// attribute values, so attribute tags always come from Versa's own response. Buckets of
// an entity with no tableData row are dropped and counted in missing. A key whose
// tableData rows disagree on those columns is ambiguous, since the timeseries response
// cannot tell its entities apart; the number of such keys is returned and the caller
// uses tableData instead. Identical duplicate rows are not ambiguous.
func joinAttributes(rows []timeseriesRow, tableRows [][]interface{}, layout *timeseriesLayout) (joined []timeseriesRow, missing int, ambiguousKeys int) {
	prefix := 1 + layout.attributeColumns
	if layout.noDrillKeyColumn {
		prefix = layout.attributeColumns
	}
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
	if len(conflicts) > 0 {
		return nil, 0, len(conflicts)
	}
	joined = rows[:0]
	for _, r := range rows {
		key, _ := r.row[0].(string)
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
	return joined, missing, 0
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
