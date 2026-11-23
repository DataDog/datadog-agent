// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package client

import (
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
// rest use tableData, deliberately.
//
// A timeseries series is named by the query's group by fields joined with commas. The
// tableData drill key is not always that name: for linkusage and usage it is only the
// leading site and circuit fields, while the attribute fields (accckt.ip and so on) follow
// it as columns. drillKeyParts rebuilds the tableData drill key from the name for those.
type timeseriesLayout struct {
	// drillKeyFields is the number of comma separated parts of a series name, which is
	// also the number of columns the parser expects between the drill key and the first
	// metric value.
	drillKeyFields int
	// drillKeyParts, when non zero, is how many leading name parts make the tableData
	// drill key. Zero means the whole name is the drill key.
	drillKeyParts int
	// strictKeyParts checks every series name splits into drillKeyFields parts before
	// using the timeseries response, and falls back to tableData for the call when one
	// does not.
	strictKeyParts bool
}

// Layouts for the endpoints whose timeseries responses have been checked against real
// Versa responses: SLA, circuit QoS, interface, link usage and DIA against our lab and
// a customer environment, and path QoS against a customer environment.
//
// linkstatus(site,accckt), the site query and tunnelstats have no timeseries layout and
// use tableData. A linkstatus timeseries response holds one series per link status value
// (Up, Down, Degraded, Indeterminable) for the tenant, with range points, not one per
// link, so it cannot give per link availability. The site query, with jq=siteStatus and
// the availability metric as the check runs it, is rejected as timeseries (code -3, joined
// queries are not allowed for this report); without jq it is rejected for availability
// (code -8, invalid metric). Only the variants without availability are accepted, so the
// check's site query stays on tableData, whose addresses can contain commas. In the lab,
// the check's tunnelstats query returned no rows in either mode, even with tunnels passing
// traffic, so its series names are not yet known.
var (
	// slam(localsite,remotesite,localaccckt,remoteaccckt,fc)
	slaTimeseriesLayout = &timeseriesLayout{drillKeyFields: 5, strictKeyParts: true}
	// pathcos(localsitename,remotesitename)
	pathQoSTimeseriesLayout = &timeseriesLayout{drillKeyFields: 2, strictKeyParts: true}
	// cos(site,accckt)
	circuitQoSTimeseriesLayout = &timeseriesLayout{drillKeyFields: 2, strictKeyParts: true}
	// intfUtil(site,accCkt,intf): the name is the tableData drill key.
	interfaceTimeseriesLayout = &timeseriesLayout{drillKeyFields: 3, strictKeyParts: true}
	// linkusage(site,accckt,accckt.uplinkBW,accckt.downlinkBW,accckt.type,accckt.media,
	// accckt.ip,accckt.isp): the name holds all eight fields, the drill key the first two.
	linkUsageTimeseriesLayout = &timeseriesLayout{drillKeyFields: 8, drillKeyParts: 2, strictKeyParts: true}
	// usage(site,accckt,accckt.ip): the name holds all three fields, the drill key the
	// first two.
	diaTimeseriesLayout = &timeseriesLayout{drillKeyFields: 3, drillKeyParts: 2, strictKeyParts: true}
)

// timeseriesLayouts maps each endpoint collected per bucket in timeseries mode to its
// layout. The getters read their layout from here, so it is the one place that decides
// which endpoints support timeseries.
var timeseriesLayouts = map[Endpoint]*timeseriesLayout{
	EndpointSLAMetrics:        slaTimeseriesLayout,
	EndpointQoSMetrics:        pathQoSTimeseriesLayout,
	EndpointCircuitQoSMetrics: circuitQoSTimeseriesLayout,
	EndpointInterfaceMetrics:  interfaceTimeseriesLayout,
	EndpointLinkUsageMetrics:  linkUsageTimeseriesLayout,
	EndpointDIAMetrics:        diaTimeseriesLayout,
}

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

// rebuildDrillKeys sets each row's drill key to its first drillKeyParts name parts joined
// with commas, as tableData reports it, for a layout whose names carry more fields.
func rebuildDrillKeys(rows []timeseriesRow, layout *timeseriesLayout) {
	if layout.drillKeyParts == 0 {
		return
	}
	for _, r := range rows {
		parts := r.row[1 : 1+layout.drillKeyParts]
		key := make([]string, len(parts))
		for i, p := range parts {
			key[i], _ = p.(string)
		}
		r.row[0] = strings.Join(key, ",")
	}
}

// flattenTimeseries converts the per metric series of a timeseries response into
// tableData shaped rows, one per entity per time bucket: the drill key, then the group by
// fields recovered from it, then the metric values in the order they were requested.
//
// A bucket is only emitted once every requested metric has reported a value for it, since
// the parsers index columns positionally and a missing value would shift everything after
// it. The newest bucket in the response is also held back, because Versa buckets are wall
// clock aligned and the current one is still filling: publishing it would pin a partial
// value at that timestamp, and the dedupe would then refuse the complete value. It is
// published once a newer bucket appears in a later response, which is normally the next
// collection. The holdback is by bucket time rather than per entity, so an
// entity that stops reporting still has its final, complete bucket published instead of
// having it held until it leaves the lookback window and is lost. The count of buckets
// dropped for either reason is returned so the caller can log it.
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
	// newest is the bucket Versa is still filling. Every entity reports into the same
	// wall clock aligned buckets, so any older bucket is closed, whichever entity it
	// belongs to.
	var newest int64
	for name, byTime := range buckets {
		names = append(names, name)
		for timestamp := range byTime {
			if timestamp > newest {
				newest = timestamp
			}
		}
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
			if timestamp == newest {
				// The newest bucket is still accumulating; it is published once a newer
				// bucket appears in a later response.
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
