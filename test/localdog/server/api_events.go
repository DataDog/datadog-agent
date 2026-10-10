// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"encoding/json"
	"math"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/localdog/store"
)

// This file implements the event-platform analytics API (`/api/v1/logs-analytics/*?type=<track>`)
// used by the Datadog UI list streams, facet lists and timelines, for the `logs` and `trace` tracks.

// item is a searchable event (log or span) normalised for analytics.
type item struct {
	ev   store.Event
	ts   int64 // unix ms
	id   string
	wrap func() map[string]any
	num  func(path string) (float64, bool)
}

type timeRange struct {
	From any `json:"from"`
	To   any `json:"to"`
}

var relTimeRe = regexp.MustCompile(`^now(?:-(\d+)([smhdw]))?$`)

func parseTimeValue(v any, now time.Time) int64 {
	switch t := v.(type) {
	case float64:
		return int64(t)
	case string:
		if n, err := strconv.ParseInt(t, 10, 64); err == nil {
			return n
		}
		if m := relTimeRe.FindStringSubmatch(t); m != nil {
			if m[1] == "" {
				return now.UnixMilli()
			}
			n, _ := strconv.Atoi(m[1])
			unit := map[string]time.Duration{"s": time.Second, "m": time.Minute, "h": time.Hour, "d": 24 * time.Hour, "w": 7 * 24 * time.Hour}[m[2]]
			return now.Add(-time.Duration(n) * unit).UnixMilli()
		}
		if ts, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return ts.UnixMilli()
		}
	}
	return 0
}

func (tr *timeRange) bounds() (int64, int64) {
	now := time.Now()
	from, to := int64(0), now.UnixMilli()
	if tr != nil {
		if f := parseTimeValue(tr.From, now); f != 0 {
			from = f
		}
		if t := parseTimeValue(tr.To, now); t != 0 {
			to = t
		}
	}
	if from == 0 {
		from = to - 15*60*1000
	}
	return from, to
}

func isoMs(ms int64) string {
	return time.UnixMilli(ms).UTC().Format("2006-01-02T15:04:05.000Z")
}

// nest turns flat dotted keys ("http.status_code") into nested objects, as the UI expects in `custom`.
func nest(dst map[string]any, key string, value any) {
	parts := strings.Split(key, ".")
	cur := dst
	for i, p := range parts {
		if i == len(parts)-1 {
			if existing, ok := cur[p].(map[string]any); ok && value != nil {
				// keep both the object and the scalar reachable
				existing["_value"] = value
				return
			}
			cur[p] = value
			return
		}
		next, ok := cur[p].(map[string]any)
		if !ok {
			if cur[p] != nil {
				// a scalar already lives here: fall back to a flat key
				cur[strings.Join(parts[i:], ".")] = value
				return
			}
			next = map[string]any{}
			cur[p] = next
		}
		cur = next
	}
}

func logItem(l *store.Log) item {
	return item{
		ev: l,
		ts: l.Timestamp,
		id: l.ID,
		wrap: func() map[string]any {
			ev := map[string]any{
				"timestamp": isoMs(l.Timestamp),
				"status":    l.Status,
				"service":   l.Service,
				"host":      l.Host,
				"source":    l.Source,
				"message":   l.Message,
				"tags":      nonNil(l.Tags),
				"custom":    l.Attributes,
			}
			if l.TraceID != "" {
				ev["trace_id"] = l.TraceID
			}
			if l.SpanID != "" {
				ev["span_id"] = l.SpanID
			}
			return ev
		},
		num: func(path string) (float64, bool) { return numField(l, path) },
	}
}

func spanTags(sp *store.Span) []string {
	tags := []string{}
	if sp.Env != "" {
		tags = append(tags, "env:"+sp.Env)
	}
	if sp.Version != "" {
		tags = append(tags, "version:"+sp.Version)
	}
	if sp.Host != "" {
		tags = append(tags, "host:"+sp.Host)
	}
	tags = append(tags, "service:"+sp.Service)
	return tags
}

func spanItem(sp *store.Span) item {
	ms := sp.Start / 1e6
	return item{
		ev: sp,
		ts: ms,
		id: sp.TraceID + "-" + sp.SpanID,
		wrap: func() map[string]any {
			status := "ok"
			if sp.Error != 0 {
				status = "error"
			}
			custom := map[string]any{}
			for k, v := range sp.Meta {
				nest(custom, k, v)
			}
			for k, v := range sp.Metrics {
				if !strings.HasPrefix(k, "_") {
					nest(custom, k, v)
				}
			}
			custom["duration"] = sp.Duration
			custom["env"] = sp.Env
			custom["service"] = sp.Service
			custom["resource_name"] = sp.Resource
			custom["operation_name"] = sp.Name
			custom["trace_id"] = sp.TraceID
			custom["span_id"] = sp.SpanID
			custom["parent_id"] = sp.ParentID
			custom["type"] = sp.Type
			custom["start"] = sp.Start
			if sp.Version != "" {
				custom["version"] = sp.Version
			}
			return map[string]any{
				"timestamp":             isoMs(ms),
				"status":                status,
				"service":               sp.Service,
				"host":                  sp.Host,
				"env":                   sp.Env,
				"resource_name":         sp.Resource,
				"resource_hash":         resourceHash(sp.Service + sp.Resource),
				"operation_name":        sp.Name,
				"trace_id":              sp.TraceID,
				"span_id":               sp.SpanID,
				"parent_id":             sp.ParentID,
				"type":                  sp.Type,
				"duration":              sp.Duration,
				"start_timestamp":       ms,
				"trace_expiration_date": ms + 15*60*1000,
				"tags":                  spanTags(sp),
				"attributes":            map[string]any{"duration": sp.Duration},
				"custom":                custom,
				"is_root":               sp.IsRoot,
				"is_top_level":          sp.IsTopLevel,
			}
		},
		num: func(path string) (float64, bool) { return numField(sp, path) },
	}
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}

func numField(ev store.Event, path string) (float64, bool) {
	vals := ev.Field(path)
	if len(vals) == 0 && !strings.HasPrefix(path, "@") {
		vals = ev.Field("@" + path)
	}
	for _, v := range vals {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f, true
		}
	}
	return 0, false
}

// collect returns the events of a track matching the search query in [from, to], newest first.
func (s *Server) collect(track, query string, from, to int64, limit int) []item {
	q := store.ParseQuery(query)
	var out []item
	switch track {
	case "trace", "spans", "apm":
		// The trace explorer's default view is service-entry spans; `@_top_level:1` etc. still work.
		s.store.Spans(func(sp *store.Span) bool {
			ms := sp.Start / 1e6
			if ms < from || ms > to {
				return true
			}
			if q.Match(sp) {
				out = append(out, spanItem(sp))
			}
			return limit <= 0 || len(out) < limit
		})
	default:
		s.store.Logs(func(l *store.Log) bool {
			if l.Timestamp < from || l.Timestamp > to {
				return true
			}
			if q.Match(l) {
				out = append(out, logItem(l))
			}
			return limit <= 0 || len(out) < limit
		})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].ts > out[j].ts })
	return out
}

func trackOf(r *http.Request) string {
	t := r.URL.Query().Get("type")
	if t == "" {
		t = "logs"
	}
	return t
}

func doneResponse(result any, hitCount int) map[string]any {
	return map[string]any{
		"status":    "done",
		"elapsed":   1,
		"hitCount":  hitCount,
		"type":      "status",
		"requestId": "localdog-" + strconv.FormatInt(time.Now().UnixNano(), 36),
		"result":    result,
	}
}

type searchBody struct {
	Query string `json:"query"`
}

type sortSpec struct {
	Time *struct {
		Order string `json:"order"`
	} `json:"time"`
	Field *struct {
		Path  string `json:"path"`
		Order string `json:"order"`
	} `json:"field"`
}

type listBody struct {
	List struct {
		Limit  int         `json:"limit"`
		Time   *timeRange  `json:"time"`
		Search *searchBody `json:"search"`
		Sorts  []sortSpec  `json:"sorts"`
		Paging *struct {
			After string `json:"after"`
		} `json:"paging"`
		ComputeCount bool `json:"computeCount"`
	} `json:"list"`
}

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet || r.Method == http.MethodDelete {
		// polling / cancellation of a request id: we always answer synchronously
		writeJSON(w, http.StatusOK, doneResponse(map[string]any{"events": []any{}, "count": 0}, 0))
		return
	}
	var body listBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []string{err.Error()}})
		return
	}
	lb := body.List
	from, to := lb.Time.bounds()
	query := ""
	if lb.Search != nil {
		query = lb.Search.Query
	}
	limit := lb.Limit
	if limit <= 0 || limit > 5000 {
		limit = 100
	}
	items := s.collect(trackOf(r), query, from, to, 0)
	ascending := false
	for _, so := range lb.Sorts {
		if so.Time != nil && so.Time.Order == "asc" {
			ascending = true
		}
		if so.Field != nil {
			path, desc := so.Field.Path, so.Field.Order != "asc"
			sort.SliceStable(items, func(i, j int) bool {
				a, okA := items[i].num(path)
				b, okB := items[j].num(path)
				if okA && okB {
					if desc {
						return a > b
					}
					return a < b
				}
				return okA && !okB
			})
		}
	}
	if ascending {
		for i, j := 0, len(items)-1; i < j; i, j = i+1, j-1 {
			items[i], items[j] = items[j], items[i]
		}
	}
	total := len(items)
	start := 0
	if lb.Paging != nil && lb.Paging.After != "" {
		if n, err := strconv.Atoi(lb.Paging.After); err == nil && n > 0 && n <= len(items) {
			start = n
		}
	}
	end := min(start+limit, len(items))
	events := make([]map[string]any, 0, end-start)
	for _, it := range items[start:end] {
		events = append(events, map[string]any{
			"id":            it.id,
			"event_id":      it.id,
			"datadog.index": "main",
			"columns":       []any{},
			"event":         it.wrap(),
		})
	}
	result := map[string]any{"events": events, "count": total}
	if end < len(items) {
		result["paging"] = map[string]any{"after": strconv.Itoa(end)}
	}
	writeJSON(w, http.StatusOK, doneResponse(result, total))
}

type computeSpec struct {
	Metric      string `json:"metric"`
	Aggregation string `json:"aggregation"`
	Output      string `json:"output"`
	Interval    any    `json:"interval"`
}

type aggregateBody struct {
	Aggregate struct {
		Compute []struct {
			Timeseries *computeSpec `json:"timeseries"`
			Total      *computeSpec `json:"total"`
			Any        *struct {
				Columns []string `json:"columns"`
				Output  string   `json:"output"`
			} `json:"any"`
		} `json:"compute"`
		GroupBy []struct {
			Field *struct {
				ID      string `json:"id"`
				Output  string `json:"output"`
				Limit   int    `json:"limit"`
				Missing any    `json:"missing"`
				Sort    *struct {
					Metric *struct {
						ID    string `json:"id"`
						Order string `json:"order"`
					} `json:"metric"`
				} `json:"sort"`
			} `json:"field"`
			Time *struct {
				Interval any    `json:"interval"`
				Output   string `json:"output"`
			} `json:"time"`
		} `json:"groupBy"`
		Time   *timeRange  `json:"time"`
		Search *searchBody `json:"search"`
	} `json:"aggregate"`
}

func intervalMs(v any, span int64) int64 {
	switch t := v.(type) {
	case float64:
		if t > 0 {
			return int64(t)
		}
	case string:
		if d, err := time.ParseDuration(t); err == nil && d > 0 {
			return d.Milliseconds()
		}
		if n, err := strconv.ParseInt(t, 10, 64); err == nil && n > 0 {
			return n
		}
	}
	// ~100 buckets by default
	iv := span / 100
	for _, step := range []int64{1000, 5000, 10000, 30000, 60000, 300000, 600000, 1800000, 3600000, 14400000, 86400000} {
		if iv <= step {
			return step
		}
	}
	return 86400000
}

func computeAgg(agg string, metric string, items []item) float64 {
	if agg == "count" || metric == "count" || metric == "" {
		if agg == "cardinality" {
			seen := map[string]bool{}
			for _, it := range items {
				for _, v := range it.ev.Field(metric) {
					seen[v] = true
				}
			}
			return float64(len(seen))
		}
		return float64(len(items))
	}
	if agg == "cardinality" {
		seen := map[string]bool{}
		for _, it := range items {
			for _, v := range it.ev.Field(metric) {
				seen[v] = true
			}
		}
		return float64(len(seen))
	}
	var vals []float64
	for _, it := range items {
		if v, ok := it.num(metric); ok {
			vals = append(vals, v)
		}
	}
	if len(vals) == 0 {
		return 0
	}
	switch agg {
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
	case "avg":
		var t float64
		for _, v := range vals {
			t += v
		}
		return t / float64(len(vals))
	}
	if strings.HasPrefix(agg, "pc") || agg == "median" {
		p := 50.0
		if agg != "median" {
			p, _ = strconv.ParseFloat(agg[2:], 64)
		}
		sort.Float64s(vals)
		idx := int(math.Ceil(p/100*float64(len(vals)))) - 1
		idx = max(0, min(idx, len(vals)-1))
		return vals[idx]
	}
	return float64(len(vals))
}

func groupValue(it item, path string, missing any) (string, bool) {
	vals := it.ev.Field(path)
	if len(vals) == 0 || vals[0] == "" {
		if m, ok := missing.(string); ok && m != "" {
			return m, true
		}
		return "", false
	}
	return vals[0], true
}

func (s *Server) handleAggregate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusOK, doneResponse(map[string]any{"values": []any{}}, 0))
		return
	}
	var body aggregateBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []string{err.Error()}})
		return
	}
	ab := body.Aggregate
	from, to := ab.Time.bounds()
	query := ""
	if ab.Search != nil {
		query = ab.Search.Query
	}
	items := s.collect(trackOf(r), query, from, to, 0)

	// group items by the field group-bys (time group-bys are folded into timeseries computes)
	type group struct {
		by    map[string]any
		items []item
	}
	groups := []*group{{by: map[string]any{}, items: items}}
	var timeGroupInterval any
	for _, gb := range ab.GroupBy {
		if gb.Time != nil {
			timeGroupInterval = gb.Time.Interval
			continue
		}
		if gb.Field == nil {
			continue
		}
		f := gb.Field
		out := f.Output
		if out == "" {
			out = f.ID
		}
		var next []*group
		for _, g := range groups {
			sub := map[string]*group{}
			var order []string
			for _, it := range g.items {
				v, ok := groupValue(it, f.ID, f.Missing)
				if !ok {
					continue
				}
				ng, exists := sub[v]
				if !exists {
					by := map[string]any{}
					for k, val := range g.by {
						by[k] = val
					}
					by[out] = v
					ng = &group{by: by}
					sub[v] = ng
					order = append(order, v)
				}
				ng.items = append(ng.items, it)
			}
			// sort groups by the requested metric (default: count desc) and apply the limit
			sortAgg, sortMetric, desc := "count", "count", true
			if f.Sort != nil && f.Sort.Metric != nil {
				parts := strings.Split(f.Sort.Metric.ID, ":")
				if len(parts) == 2 {
					sortMetric, sortAgg = parts[0], parts[1]
				}
				desc = f.Sort.Metric.Order != "asc"
			}
			sort.SliceStable(order, func(i, j int) bool {
				a := computeAgg(sortAgg, sortMetric, sub[order[i]].items)
				b := computeAgg(sortAgg, sortMetric, sub[order[j]].items)
				if desc {
					return a > b
				}
				return a < b
			})
			limit := f.Limit
			if limit <= 0 {
				limit = 10
			}
			if len(order) > limit {
				order = order[:limit]
			}
			for _, k := range order {
				next = append(next, sub[k])
			}
		}
		groups = next
	}

	values := []any{}
	for _, g := range groups {
		metrics := map[string]any{}
		for _, c := range ab.Compute {
			switch {
			case c.Timeseries != nil:
				cs := c.Timeseries
				iv := intervalMs(firstNonNil(cs.Interval, timeGroupInterval), to-from)
				start := (from / iv) * iv
				n := int((to-start)/iv) + 1
				buckets := make([][]item, n)
				for _, it := range g.items {
					b := int((it.ts - start) / iv)
					if b >= 0 && b < n {
						buckets[b] = append(buckets[b], it)
					}
				}
				points := make([]map[string]any, 0, n)
				for i, b := range buckets {
					v := 0.0
					if len(b) > 0 {
						v = computeAgg(cs.Aggregation, cs.Metric, b)
					}
					points = append(points, map[string]any{"time": isoMs(start + int64(i)*iv), "value": v})
				}
				metrics[outputName(cs)] = points
			case c.Total != nil:
				metrics[outputName(c.Total)] = computeAgg(c.Total.Aggregation, c.Total.Metric, g.items)
			case c.Any != nil:
				row := map[string]any{}
				if len(g.items) > 0 {
					for _, col := range c.Any.Columns {
						if v := g.items[0].ev.Field(col); len(v) > 0 {
							row[col] = v[0]
						}
					}
				}
				metrics[c.Any.Output] = []any{row}
			}
		}
		values = append(values, map[string]any{"by": g.by, "metrics": metrics})
	}
	writeJSON(w, http.StatusOK, doneResponse(map[string]any{"values": values}, len(items)))
}

func firstNonNil(vals ...any) any {
	for _, v := range vals {
		if v != nil {
			return v
		}
	}
	return nil
}

func outputName(c *computeSpec) string {
	if c.Output != "" {
		return c.Output
	}
	return c.Metric + ":" + c.Aggregation
}

type facetInfoBody struct {
	FacetInfo *struct {
		Path       string      `json:"path"`
		Limit      int         `json:"limit"`
		Time       *timeRange  `json:"time"`
		Search     *searchBody `json:"search"`
		TermSearch *searchBody `json:"termSearch"`
	} `json:"facet_info"`
	FacetRangeInfo *struct {
		Path   string      `json:"path"`
		Time   *timeRange  `json:"time"`
		Search *searchBody `json:"search"`
	} `json:"facet_range_info"`
}

func (s *Server) handleFacetInfo(w http.ResponseWriter, r *http.Request) {
	var body facetInfoBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []string{err.Error()}})
		return
	}
	track := trackOf(r)
	if fr := body.FacetRangeInfo; fr != nil {
		from, to := fr.Time.bounds()
		query := ""
		if fr.Search != nil {
			query = fr.Search.Query
		}
		items := s.collect(track, query, from, to, 0)
		lo, hi := math.Inf(1), math.Inf(-1)
		for _, it := range items {
			if v, ok := it.num(fr.Path); ok {
				lo, hi = math.Min(lo, v), math.Max(hi, v)
			}
		}
		if math.IsInf(lo, 1) {
			lo, hi = 0, 0
		}
		writeJSON(w, http.StatusOK, doneResponse(map[string]any{"min": lo, "max": hi}, len(items)))
		return
	}
	fi := body.FacetInfo
	if fi == nil {
		writeJSON(w, http.StatusOK, doneResponse(map[string]any{"fields": []any{}}, 0))
		return
	}
	from, to := fi.Time.bounds()
	query := ""
	if fi.Search != nil {
		query = fi.Search.Query
	}
	term := ""
	if fi.TermSearch != nil {
		term = strings.ToLower(fi.TermSearch.Query)
	}
	items := s.collect(track, query, from, to, 0)
	counts := map[string]int{}
	for _, it := range items {
		for _, v := range it.ev.Field(fi.Path) {
			if v == "" || (term != "" && !strings.Contains(strings.ToLower(v), term)) {
				continue
			}
			counts[v]++
		}
	}
	fields := make([]map[string]any, 0, len(counts))
	for k, v := range counts {
		fields = append(fields, map[string]any{"field": k, "value": v})
	}
	sort.Slice(fields, func(i, j int) bool {
		a, b := fields[i]["value"].(int), fields[j]["value"].(int)
		if a != b {
			return a > b
		}
		return fields[i]["field"].(string) < fields[j]["field"].(string)
	})
	limit := fi.Limit
	if limit <= 0 {
		limit = 10
	}
	if len(fields) > limit {
		fields = fields[:limit]
	}
	writeJSON(w, http.StatusOK, doneResponse(map[string]any{"fields": fields}, len(items)))
}

func (s *Server) handleFetchOne(w http.ResponseWriter, r *http.Request) {
	var body struct {
		FetchOne struct {
			ID string `json:"id"`
		} `json:"fetch_one"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []string{err.Error()}})
		return
	}
	id := body.FetchOne.ID
	var found map[string]any
	switch trackOf(r) {
	case "trace", "spans", "apm":
		traceID, spanID, _ := strings.Cut(id, "-")
		for _, sp := range s.store.Trace(traceID) {
			if sp.SpanID == spanID {
				found = spanItem(sp).wrap()
			}
		}
	default:
		s.store.Logs(func(l *store.Log) bool {
			if l.ID == id {
				found = logItem(l).wrap()
				return false
			}
			return true
		})
	}
	if found == nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"status": "error", "errors": []string{"event not found"}})
		return
	}
	writeJSON(w, http.StatusOK, doneResponse(found, 1))
}

// --- facet list -----------------------------------------------------------------------------

func coreFacet(id, name, path, typ string, values []map[string]string, defaults []string) map[string]any {
	if values == nil {
		values = []map[string]string{}
	}
	if defaults == nil {
		defaults = []string{}
	}
	facetType := "list"
	if typ != "string" {
		facetType = "range"
	}
	return map[string]any{
		"id": id, "name": name, "path": path, "source": "core", "type": typ, "facetType": facetType,
		"groups": []string{"Core"}, "values": values, "defaultValues": defaults, "editable": false,
		"bundled": true, "bundledAndUsed": true, "bounded": len(values) > 0, "description": name,
	}
}

func attrFacet(path, typ, group string) map[string]any {
	facetType := "list"
	if typ != "string" {
		facetType = "range"
	}
	name := path
	if i := strings.LastIndex(path, "."); i >= 0 {
		name = path[i+1:]
	}
	return map[string]any{
		"id": "log_" + path, "name": name, "path": path, "source": "log", "type": typ, "facetType": facetType,
		"groups": []string{group}, "values": []any{}, "defaultValues": []any{}, "editable": false,
		"bundled": false, "bundledAndUsed": false, "bounded": false, "description": "",
	}
}

var logStatusValues = []map[string]string{
	{"color": "#64040A", "name": "Emergency", "id": "emergency"},
	{"color": "#950814", "name": "Alert", "id": "alert"},
	{"color": "#CA0812", "name": "Critical", "id": "critical"},
	{"color": "#E3554C", "name": "Error", "id": "error"},
	{"color": "#EDB359", "name": "Warn", "id": "warn"},
	{"color": "#9AAABA", "name": "Notice", "id": "notice"},
	{"color": "#9BCCE4", "name": "Info", "id": "info"},
	{"color": "#C4C4C4", "name": "Debug", "id": "debug"},
	{"color": "#99cce5", "name": "Ok", "id": "ok"},
}

// collectAttributePaths walks attribute objects and records leaf paths and whether they are numeric.
func collectAttributePaths(prefix string, v any, out map[string]string, depth int) {
	if depth > 4 {
		return
	}
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			p := k
			if prefix != "" {
				p = prefix + "." + k
			}
			collectAttributePaths(p, child, out, depth+1)
		}
	case float64, int64, int:
		if _, ok := out[prefix]; !ok {
			out[prefix] = "double"
		}
	case string, bool:
		out[prefix] = "string"
	}
}

func (s *Server) handleFacetList(w http.ResponseWriter, r *http.Request) {
	// /api/ui/event-platform/<track>/facets
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	track := "logs"
	if len(parts) >= 4 {
		track = parts[3]
	}
	var facets []map[string]any
	paths := map[string]string{}
	switch track {
	case "trace", "spans", "apm":
		facets = append(facets,
			coreFacet("core_service", "Service", "service", "string", nil, nil),
			coreFacet("core_env", "Env", "env", "string", nil, nil),
			coreFacet("core_resource_name", "Resource", "resource_name", "string", nil, nil),
			coreFacet("core_operation_name", "Operation Name", "operation_name", "string", nil, nil),
			coreFacet("core_status", "Status", "status", "string", []map[string]string{
				{"color": "#E3554C", "name": "Error", "id": "error"}, {"color": "#99cce5", "name": "Ok", "id": "ok"},
			}, nil),
			coreFacet("core_host", "Host", "host", "string", nil, nil),
			coreFacet("core_type", "Span Type", "type", "string", nil, nil),
			attrFacet("duration", "integer", "Measures"),
		)
		n := 0
		s.store.Spans(func(sp *store.Span) bool {
			for k, v := range sp.Meta {
				if !strings.HasPrefix(k, "_") {
					collectAttributePaths(k, v, paths, 0)
				}
			}
			for k := range sp.Metrics {
				if !strings.HasPrefix(k, "_") {
					paths[k] = "double"
				}
			}
			n++
			return n < 2000
		})
	default:
		facets = append(facets,
			coreFacet("core_service", "Service", "service", "string", nil, nil),
			coreFacet("core_status", "Status", "status", "string", logStatusValues, []string{"error", "warn", "info"}),
			coreFacet("core_host", "Host", "host", "string", nil, nil),
			coreFacet("core_source", "Source", "source", "string", nil, nil),
		)
		n := 0
		s.store.Logs(func(l *store.Log) bool {
			collectAttributePaths("", l.Attributes, paths, 0)
			n++
			return n < 2000
		})
	}
	keys := make([]string, 0, len(paths))
	for k := range paths {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > 150 {
		keys = keys[:150]
	}
	for _, k := range keys {
		group := "Attributes"
		if i := strings.Index(k, "."); i > 0 {
			group = k[:i]
		}
		facets = append(facets, attrFacet(k, paths[k], group))
	}
	writeJSON(w, http.StatusOK, map[string]any{"facets": map[string]any{track: facets}})
}
