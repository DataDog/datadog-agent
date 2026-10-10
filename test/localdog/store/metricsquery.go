// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package store

import (
	"fmt"
	"math"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// MetricQuery is a parsed metric query such as `avg:system.cpu.user{env:dev,!host:a} by {host}.rollup(max, 60)`.
type MetricQuery struct {
	SpaceAgg       string
	Metric         string
	Filters        []string
	GroupBy        []string
	RollupFn       string
	RollupInterval int64 // seconds, 0 = automatic
	AsCount        bool
	AsRate         bool
}

var metricQueryRe = regexp.MustCompile(`^\s*(?:(avg|sum|min|max|count):)?\s*([A-Za-z0-9_.\-]+)\s*(?:\{([^}]*)\})?\s*(?:by\s*\{([^}]*)\})?\s*(.*)$`)
var rollupRe = regexp.MustCompile(`\.rollup\(\s*([a-z]+)?\s*(?:,\s*(\d+))?\s*\)`)

// ParseMetricQuery parses a Datadog metric query.
func ParseMetricQuery(q string) (*MetricQuery, error) {
	m := metricQueryRe.FindStringSubmatch(q)
	if m == nil {
		return nil, fmt.Errorf("invalid metric query %q", q)
	}
	mq := &MetricQuery{SpaceAgg: m[1], Metric: m[2]}
	if mq.SpaceAgg == "" {
		mq.SpaceAgg = "avg"
	}
	for _, f := range strings.Split(m[3], ",") {
		f = strings.TrimSpace(f)
		if f != "" && f != "*" {
			mq.Filters = append(mq.Filters, f)
		}
	}
	for _, g := range strings.Split(m[4], ",") {
		g = strings.TrimSpace(g)
		if g != "" {
			mq.GroupBy = append(mq.GroupBy, g)
		}
	}
	rest := m[5]
	if r := rollupRe.FindStringSubmatch(rest); r != nil {
		mq.RollupFn = r[1]
		if r[2] != "" {
			mq.RollupInterval, _ = strconv.ParseInt(r[2], 10, 64)
		}
	}
	mq.AsCount = strings.Contains(rest, ".as_count()")
	mq.AsRate = strings.Contains(rest, ".as_rate()")
	return mq, nil
}

// seriesTagValues returns the values of a tag key on a series; host is a pseudo tag.
func seriesTagValues(ser *Series, key string) []string {
	if key == "host" && ser.Host != "" {
		return append([]string{ser.Host}, tagValues(ser.Tags, key)...)
	}
	return tagValues(ser.Tags, key)
}

func matchFilter(ser *Series, filter string) bool {
	negate := strings.HasPrefix(filter, "!") || strings.HasPrefix(filter, "-")
	if negate {
		filter = filter[1:]
	}
	key, value, hasValue := strings.Cut(filter, ":")
	var ok bool
	if !hasValue {
		// bare tag, e.g. `{production}`
		for _, t := range ser.Tags {
			if t == key {
				ok = true
			}
		}
	} else {
		for _, v := range seriesTagValues(ser, key) {
			if m, _ := path.Match(value, v); m || v == value {
				ok = true
				break
			}
		}
	}
	return ok != negate
}

// MetricResult is one output series of a metric query.
type MetricResult struct {
	GroupTags []string
	Unit      string
	Values    []*float64 // aligned with the query Times
}

// MetricQueryResult holds the output of a query on a fixed time grid.
type MetricQueryResult struct {
	Times    []int64 // unix milliseconds, bucket starts
	Interval int64   // milliseconds
	Series   []*MetricResult
}

// QueryMetrics evaluates a metric query over [fromMs, toMs] with buckets of intervalMs.
func (s *Store) QueryMetrics(mq *MetricQuery, fromMs, toMs, intervalMs int64) *MetricQueryResult {
	if mq.RollupInterval > 0 {
		intervalMs = mq.RollupInterval * 1000
	}
	if intervalMs <= 0 {
		intervalMs = defaultInterval(toMs - fromMs)
	}
	// Agent checks run every 15s and dogstatsd flushes every 10s: smaller buckets would be
	// mostly empty, which graphs draw as drops to zero.
	if intervalMs < minIntervalMs {
		intervalMs = minIntervalMs
	}
	start := (fromMs / intervalMs) * intervalMs
	nBuckets := int((toMs-start)/intervalMs) + 1
	if nBuckets > 5000 {
		nBuckets = 5000
	}
	res := &MetricQueryResult{Interval: intervalMs}
	for i := 0; i < nBuckets; i++ {
		res.Times = append(res.Times, start+int64(i)*intervalMs)
	}

	all := s.SeriesFor(mq.Metric, start/1000, toMs/1000)
	type group struct {
		tags   []string
		unit   string
		perSer [][]*float64
	}
	groups := map[string]*group{}
	var order []string
	for _, ser := range all {
		matched := true
		for _, f := range mq.Filters {
			if !matchFilter(ser, f) {
				matched = false
				break
			}
		}
		if !matched || len(ser.Points) == 0 {
			continue
		}
		var gtags []string
		for _, g := range mq.GroupBy {
			vals := seriesTagValues(ser, g)
			v := "N/A"
			if len(vals) > 0 {
				v = vals[0]
			}
			gtags = append(gtags, g+":"+v)
		}
		key := strings.Join(gtags, ",")
		gr, ok := groups[key]
		if !ok {
			gr = &group{tags: gtags, unit: ser.Unit}
			groups[key] = gr
			order = append(order, key)
		}
		gr.perSer = append(gr.perSer, rollup(ser, mq, start, intervalMs, nBuckets))
	}
	sort.Strings(order)
	for _, key := range order {
		gr := groups[key]
		out := &MetricResult{GroupTags: gr.tags, Unit: gr.unit, Values: make([]*float64, nBuckets)}
		for b := 0; b < nBuckets; b++ {
			var vals []float64
			for _, sv := range gr.perSer {
				if sv[b] != nil {
					vals = append(vals, *sv[b])
				}
			}
			if len(vals) == 0 {
				continue
			}
			v := Aggregate(mq.SpaceAgg, vals)
			out.Values[b] = &v
		}
		if out.GroupTags == nil {
			out.GroupTags = []string{}
		}
		res.Series = append(res.Series, out)
	}
	return res
}

// minIntervalMs is the smallest bucket size served for metric queries.
const minIntervalMs = 20_000

func defaultInterval(spanMs int64) int64 {
	// aim for ~150 points, with the agent's 10s flush as the floor
	steps := []int64{20, 30, 60, 120, 300, 600, 1800, 3600, 7200, 14400, 86400}
	for _, s := range steps {
		if spanMs/(s*1000) <= 200 {
			return s * 1000
		}
	}
	return 86400 * 1000
}

func rollup(ser *Series, mq *MetricQuery, start, intervalMs int64, n int) []*float64 {
	fn := mq.RollupFn
	// Rates (dogstatsd counters) hold per-second values over the flush interval; `.as_count()`
	// turns them back into totals. Counts hold totals; `.as_rate()` turns them into per-second.
	scale := 1.0
	summable := ser.Type == "count"
	if mq.AsCount && ser.Type == "rate" {
		scale = float64(max(ser.Interval, 1))
		summable = true
	}
	if fn == "" {
		fn = "avg"
		if summable {
			fn = "sum"
		}
	}
	buckets := make([][]float64, n)
	for _, p := range ser.Points {
		b := int((p.Timestamp*1000 - start) / intervalMs)
		if b < 0 || b >= n {
			continue
		}
		buckets[b] = append(buckets[b], p.Value*scale)
	}
	out := make([]*float64, n)
	for i, vals := range buckets {
		if len(vals) == 0 {
			continue
		}
		v := Aggregate(fn, vals)
		if ser.Type == "count" && mq.AsRate {
			v /= float64(intervalMs) / 1000
		}
		out[i] = &v
	}
	return out
}

// Aggregate reduces values with avg, sum, min, max or count.
func Aggregate(fn string, vals []float64) float64 {
	switch fn {
	case "sum":
		var t float64
		for _, v := range vals {
			t += v
		}
		return t
	case "min":
		m := math.Inf(1)
		for _, v := range vals {
			m = math.Min(m, v)
		}
		return m
	case "max":
		m := math.Inf(-1)
		for _, v := range vals {
			m = math.Max(m, v)
		}
		return m
	case "count":
		return float64(len(vals))
	}
	var t float64
	for _, v := range vals {
		t += v
	}
	return t / float64(len(vals))
}

// MetricTags returns all tags (including host:) seen on a metric.
func (s *Store) MetricTags(name string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	seen := map[string]bool{}
	for _, ser := range s.seriesNames[name] {
		if ser.Host != "" {
			seen["host:"+ser.Host] = true
		}
		for _, t := range ser.Tags {
			seen[t] = true
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

// MetricInfo describes a metric name for the metric picker.
type MetricInfo struct {
	Name      string  `json:"name"`
	Type      string  `json:"type"`
	Unit      string  `json:"unit,omitempty"`
	Series    int     `json:"series"`
	LastPoint int64   `json:"last_point"` // unix seconds
	LastValue float64 `json:"last_value"`
}

// Metrics lists metric names with summary info.
func (s *Store) Metrics() []MetricInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]MetricInfo, 0, len(s.seriesNames))
	for name, list := range s.seriesNames {
		mi := MetricInfo{Name: name, Series: len(list)}
		for _, ser := range list {
			mi.Type = ser.Type
			if ser.Unit != "" {
				mi.Unit = ser.Unit
			}
			if n := len(ser.Points); n > 0 && ser.Points[n-1].Timestamp >= mi.LastPoint {
				mi.LastPoint = ser.Points[n-1].Timestamp
				mi.LastValue = ser.Points[n-1].Value
			}
		}
		out = append(out, mi)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}
