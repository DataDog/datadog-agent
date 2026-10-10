// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package server

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/test/localdog/store"
)

// POST /api/ui/query/scalar: formulas & functions scalar queries (query value, toplist, table widgets).

type scalarQuery struct {
	DataSource string `json:"data_source"`
	Name       string `json:"name"`
	Query      string `json:"query"`
	Aggregator string `json:"aggregator"`
}

type scalarFormula struct {
	Formula string `json:"formula"`
	Alias   string `json:"alias"`
	Limit   *struct {
		Count int    `json:"count"`
		Order string `json:"order"`
	} `json:"limit"`
}

type scalarRequest struct {
	Attributes struct {
		From     int64           `json:"from"`
		To       int64           `json:"to"`
		Formulas []scalarFormula `json:"formulas"`
		Queries  []scalarQuery   `json:"queries"`
	} `json:"attributes"`
}

func (s *Server) handleScalar(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []string{err.Error()}})
		return
	}
	var reqs []scalarRequest
	if err := json.Unmarshal(body.Data, &reqs); err != nil {
		var single scalarRequest
		if err := json.Unmarshal(body.Data, &single); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"errors": []string{err.Error()}})
			return
		}
		reqs = []scalarRequest{single}
	}
	data := make([]any, 0, len(reqs))
	for _, req := range reqs {
		data = append(data, s.evalScalarRequest(req))
	}
	writeJSON(w, http.StatusOK, map[string]any{"data": data})
}

// reduceSeries collapses a timeseries into a single value.
func reduceSeries(vals []*float64, aggregator string) *float64 {
	var nums []float64
	for _, v := range vals {
		if v != nil {
			nums = append(nums, *v)
		}
	}
	if len(nums) == 0 {
		return nil
	}
	var out float64
	switch aggregator {
	case "last":
		out = nums[len(nums)-1]
	case "sum":
		out = store.Aggregate("sum", nums)
	case "min":
		out = store.Aggregate("min", nums)
	case "max":
		out = store.Aggregate("max", nums)
	case "percentile":
		sort.Float64s(nums)
		out = nums[len(nums)*95/100]
	default:
		out = store.Aggregate("avg", nums)
	}
	return &out
}

func (s *Server) evalScalarRequest(req scalarRequest) any {
	a := req.Attributes
	if a.To == 0 {
		a.To = time.Now().UnixMilli()
	}
	if a.From == 0 {
		a.From = a.To - 3600_000
	}
	results := map[string]*store.MetricQueryResult{}
	var groupKeys []string
	var errs []string
	for _, q := range a.Queries {
		mq, err := store.ParseMetricQuery(q.Query)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		if groupKeys == nil {
			groupKeys = mq.GroupBy
		}
		ts := s.store.QueryMetrics(mq, a.From, a.To, 0)
		resolveUnits(mq.Metric, ts)
		reduced := &store.MetricQueryResult{Times: []int64{a.From}, Interval: a.To - a.From}
		for _, sr := range ts.Series {
			reduced.Series = append(reduced.Series, &store.MetricResult{
				GroupTags: sr.GroupTags,
				Unit:      sr.Unit,
				Values:    []*float64{reduceSeries(sr.Values, q.Aggregator)},
			})
		}
		results[q.Name] = reduced
	}
	formulas := a.Formulas
	if len(formulas) == 0 {
		for _, q := range a.Queries {
			formulas = append(formulas, scalarFormula{Formula: q.Name})
		}
	}

	type row struct {
		tags   []string
		values []*float64
		unit   string
	}
	rows := map[string]*row{}
	var order []string
	for fi, f := range formulas {
		out, err := evalFormula(f.Formula, results, 1)
		if err != nil {
			errs = append(errs, err.Error())
			continue
		}
		for _, sr := range out {
			key := strings.Join(sr.GroupTags, ",")
			rw, ok := rows[key]
			if !ok {
				rw = &row{tags: sr.GroupTags, values: make([]*float64, len(formulas)), unit: sr.Unit}
				rows[key] = rw
				order = append(order, key)
			}
			rw.values[fi] = sr.Values[0]
		}
	}
	// sort by the first formula, descending unless asked otherwise, and apply its limit
	desc := true
	limit := 0
	if len(formulas) > 0 && formulas[0].Limit != nil {
		desc = !strings.EqualFold(formulas[0].Limit.Order, "asc")
		limit = formulas[0].Limit.Count
	}
	val := func(k string) float64 {
		if v := rows[k].values[0]; v != nil {
			return *v
		}
		if desc {
			return -1e308
		}
		return 1e308
	}
	sort.SliceStable(order, func(i, j int) bool {
		if desc {
			return val(order[i]) > val(order[j])
		}
		return val(order[i]) < val(order[j])
	})
	if limit > 0 && len(order) > limit {
		order = order[:limit]
	}

	columns := []any{}
	for gi, g := range groupKeys {
		values := make([][]string, 0, len(order))
		for _, k := range order {
			v := "N/A"
			if gi < len(rows[k].tags) {
				_, v, _ = strings.Cut(rows[k].tags[gi], ":")
			}
			values = append(values, []string{v})
		}
		columns = append(columns, map[string]any{"type": "group", "name": g, "values": values})
	}
	for fi, f := range formulas {
		name := f.Formula
		if f.Alias != "" {
			name = f.Alias
		}
		values := make([]*float64, 0, len(order))
		unit := ""
		for _, k := range order {
			values = append(values, rows[k].values[fi])
			unit = rows[k].unit
		}
		unitMeta := unitPair(unit)
		columns = append(columns, map[string]any{"type": "number", "name": name, "values": values, "meta": map[string]any{"unit": unitMeta}})
	}
	attrs := map[string]any{"columns": columns}
	if len(errs) > 0 {
		attrs["errors"] = strings.Join(errs, "; ")
	}
	return map[string]any{"type": "scalar_response", "attributes": attrs}
}
