// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/localdog/store"
)

//go:embed units.json
var unitsJSON []byte

var canonicalUnits = func() map[string]map[string]any {
	m := map[string]map[string]any{}
	_ = json.Unmarshal(unitsJSON, &m)
	return m
}()

func unitObject(name string) any {
	if u, ok := canonicalUnits[name]; ok {
		return u
	}
	return nil
}

// apiSpan is the span shape served by GET /api/ui/trace/<id> (times are float seconds).
type apiSpan struct {
	TraceID      string             `json:"trace_id"`
	SpanID       string             `json:"span_id"`
	ParentID     string             `json:"parent_id"`
	ChildrenIDs  []string           `json:"children_ids"`
	Service      string             `json:"service"`
	Name         string             `json:"name"`
	Resource     string             `json:"resource"`
	ResourceHash string             `json:"resource_hash"`
	Type         string             `json:"type"`
	Start        float64            `json:"start"`
	End          float64            `json:"end"`
	Duration     float64            `json:"duration"`
	Error        int32              `json:"error"`
	Status       string             `json:"status"`
	Env          string             `json:"env"`
	Hostname     string             `json:"hostname"`
	OrgID        int                `json:"org_id"`
	Meta         map[string]string  `json:"meta"`
	Metrics      map[string]float64 `json:"metrics"`
	SpanLinks    []any              `json:"span_links"`
	SpanEvents   []any              `json:"span_events"`
}

type apiSpanMap struct {
	RootID string              `json:"root_id"`
	Spans  map[string]*apiSpan `json:"spans"`
}

func resourceHash(s string) string {
	// FNV-1a, enough to give each resource a stable id
	var h uint64 = 14695981039346656037
	for i := 0; i < len(s); i++ {
		h ^= uint64(s[i])
		h *= 1099511628211
	}
	return strconv.FormatUint(h, 16)
}

func toAPISpan(sp *store.Span) *apiSpan {
	meta := map[string]string{}
	for k, v := range sp.Meta {
		meta[k] = v
	}
	if sp.Version != "" {
		meta["version"] = sp.Version
	}
	if sp.Env != "" {
		meta["env"] = sp.Env
	}
	metrics := map[string]float64{}
	for k, v := range sp.Metrics {
		metrics[k] = v
	}
	if sp.IsTopLevel {
		metrics["_top_level"] = 1
	}
	status := "ok"
	if sp.Error != 0 {
		status = "error"
	}
	start := float64(sp.Start) / 1e9
	dur := float64(sp.Duration) / 1e9
	return &apiSpan{
		TraceID:      sp.TraceID,
		SpanID:       sp.SpanID,
		ParentID:     sp.ParentID,
		ChildrenIDs:  []string{},
		Service:      sp.Service,
		Name:         sp.Name,
		Resource:     sp.Resource,
		ResourceHash: resourceHash(sp.Service + sp.Resource),
		Type:         sp.Type,
		Start:        start,
		End:          start + dur,
		Duration:     dur,
		Error:        sp.Error,
		Status:       status,
		Env:          sp.Env,
		Hostname:     sp.Host,
		OrgID:        1,
		Meta:         meta,
		Metrics:      metrics,
		SpanLinks:    []any{},
		SpanEvents:   []any{},
	}
}

// buildTraceResponse groups spans into the main tree and orphaned trees.
func buildTraceResponse(spans []*store.Span) map[string]any {
	byID := map[string]*apiSpan{}
	for _, sp := range spans {
		byID[sp.SpanID] = toAPISpan(sp)
	}
	var roots []*apiSpan
	for _, sp := range spans {
		as := byID[sp.SpanID]
		if parent, ok := byID[as.ParentID]; ok && as.ParentID != as.SpanID {
			parent.ChildrenIDs = append(parent.ChildrenIDs, as.SpanID)
		} else {
			roots = append(roots, as)
		}
	}
	// the main root is the real root span if present, otherwise the earliest parentless span
	sort.SliceStable(roots, func(i, j int) bool {
		ri, rj := roots[i].ParentID == "0", roots[j].ParentID == "0"
		if ri != rj {
			return ri
		}
		return roots[i].Start < roots[j].Start
	})
	treeOf := func(root *apiSpan) *apiSpanMap {
		m := &apiSpanMap{RootID: root.SpanID, Spans: map[string]*apiSpan{}}
		stack := []*apiSpan{root}
		for len(stack) > 0 {
			cur := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			m.Spans[cur.SpanID] = cur
			for _, c := range cur.ChildrenIDs {
				stack = append(stack, byID[c])
			}
		}
		return m
	}
	resp := map[string]any{
		"is_truncated":         false,
		"entities":             nil,
		"span_id_to_entity_id": nil,
		"orphaned":             []*apiSpanMap{},
	}
	if len(roots) == 0 {
		resp["trace"] = &apiSpanMap{Spans: map[string]*apiSpan{}}
		return resp
	}
	resp["trace"] = treeOf(roots[0])
	orphaned := []*apiSpanMap{}
	for _, r := range roots[1:] {
		orphaned = append(orphaned, treeOf(r))
	}
	resp["orphaned"] = orphaned
	return resp
}

func (s *Server) handleTrace(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimPrefix(r.URL.Path, "/api/ui/trace/")
	id = strings.Trim(id, "/")
	id = s.resolveTraceID(id)
	spans := s.store.Trace(id)
	if len(spans) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"errors": []string{"trace not found"}})
		return
	}
	writeJSON(w, http.StatusOK, buildTraceResponse(spans))
}

// resolveTraceID accepts decimal 64-bit IDs as well as 128-bit hex IDs (as logged by OTel-style tracers).
func (s *Server) resolveTraceID(id string) string {
	if _, err := strconv.ParseUint(id, 10, 64); err == nil {
		return id
	}
	if len(id) == 32 {
		if low, err := strconv.ParseUint(id[16:], 16, 64); err == nil {
			return strconv.FormatUint(low, 10)
		}
	}
	if v, err := strconv.ParseUint(id, 16, 64); err == nil {
		return strconv.FormatUint(v, 10)
	}
	return id
}

// --- metrics: POST /api/ui/query/timeseries -------------------------------------------------

type tsQuery struct {
	DataSource string `json:"data_source"`
	Name       string `json:"name"`
	Query      string `json:"query"`
}

type tsFormula struct {
	Formula string `json:"formula"`
	Alias   string `json:"alias"`
}

type tsRequest struct {
	Type       string `json:"type"`
	Attributes struct {
		From     int64       `json:"from"`
		To       int64       `json:"to"`
		Interval int64       `json:"interval"`
		Formulas []tsFormula `json:"formulas"`
		Queries  []tsQuery   `json:"queries"`
	} `json:"attributes"`
}

func (s *Server) handleTimeseries(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []string{err.Error()}})
		return
	}
	// `data` is either a single request object or an array of them
	var reqs []tsRequest
	if err := json.Unmarshal(body.Data, &reqs); err != nil {
		var single tsRequest
		if err := json.Unmarshal(body.Data, &single); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []string{err.Error()}})
			return
		}
		reqs = []tsRequest{single}
	}
	var data []any
	var metas []any
	for _, req := range reqs {
		d, m := s.evalTimeseriesRequest(req)
		data = append(data, d)
		metas = append(metas, m)
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data, "meta": map[string]any{"responses": metas}})
}

func (s *Server) evalTimeseriesRequest(req tsRequest) (any, any) {
	a := req.Attributes
	if a.To == 0 {
		a.To = time.Now().UnixMilli()
	}
	if a.From == 0 {
		a.From = a.To - 3600_000
	}
	results := map[string]*store.MetricQueryResult{}
	var grid *store.MetricQueryResult
	var errs []string
	for _, q := range a.Queries {
		if q.DataSource != "" && q.DataSource != "metrics" {
			errs = append(errs, fmt.Sprintf("data source %q is not supported by localdog", q.DataSource))
			continue
		}
		mq, err := store.ParseMetricQuery(q.Query)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		res := s.store.QueryMetrics(mq, a.From, a.To, a.Interval)
		resolveUnits(mq.Metric, res)
		results[q.Name] = res
		if grid == nil {
			grid = res
		}
	}
	formulas := a.Formulas
	if len(formulas) == 0 {
		for _, q := range a.Queries {
			formulas = append(formulas, tsFormula{Formula: q.Name})
		}
	}
	series := []any{}
	values := [][]*float64{}
	times := []int64{}
	interval := a.Interval
	if grid != nil {
		times = grid.Times
		interval = grid.Interval
	}
	for fi, f := range formulas {
		out, err := evalFormula(f.Formula, results, len(times))
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		for _, sr := range out {
			meta := map[string]any{
				"query_index": fi,
				"group_tags":  sr.GroupTags,
				"unit":        unitPair(sr.Unit),
			}
			if f.Alias != "" {
				meta["alias"] = f.Alias
			}
			series = append(series, meta)
			values = append(values, sr.Values)
		}
	}
	attrs := map[string]any{"series": series, "times": times, "values": values}
	if len(errs) > 0 {
		attrs["errors"] = strings.Join(errs, "; ")
	}
	return map[string]any{"type": "timeseries_response", "attributes": attrs},
		map[string]any{"from_date": a.From, "to_date": a.To, "interval": interval}
}

// --- localdog native endpoints --------------------------------------------------------------

func (s *Server) handleMetricsList(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(r.URL.Query().Get("q"))
	all := s.store.Metrics()
	out := make([]store.MetricInfo, 0, len(all))
	for _, m := range all {
		if q == "" || strings.Contains(strings.ToLower(m.Name), q) {
			out = append(out, m)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"metrics": out})
}

func (s *Server) handleMetricTags(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("metric")
	writeJSON(w, http.StatusOK, map[string]any{"metric": name, "tags": s.store.MetricTags(name)})
}

func (s *Server) handleStats(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Stats())
}

func (s *Server) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodDelete {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	s.store.Reset()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
