// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2019 Datadog, Inc.

package stats

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/stats/quantile"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	"github.com/stretchr/testify/assert"
)

const defaultEnv = "default"

func testWeightedSpans() WeightedTrace {
	spans := []pb.Span{
		{Service: "A", Name: "A.foo", Resource: "α", Duration: 1},
		{Service: "A", Name: "A.foo", Resource: "β", Duration: 2, Error: 1},
		{Service: "B", Name: "B.foo", Resource: "γ", Duration: 3},
		{Service: "B", Name: "B.foo", Resource: "ε", Duration: 4, Error: 404},
		{Service: "B", Name: "B.foo", Resource: "ζ", Duration: 5, Meta: map[string]string{"version": "1.3"}},
		{Service: "B", Name: "sql.query", Resource: "ζ", Duration: 6, Meta: map[string]string{"version": "1.4"}},
		{Service: "C", Name: "sql.query", Resource: "δ", Duration: 7},
		{Service: "C", Name: "sql.query", Resource: "δ", Duration: 8},
	}
	tws := make(WeightedTrace, len(spans))
	for i := range spans {
		tws[i] = &WeightedSpan{
			Span:     &spans[i],
			Weight:   1,
			TopLevel: true,
		}
	}
	return tws
}

func testTrace() pb.Trace {
	// Data below represents a trace with some sublayers, so that we make sure,
	// those data are correctly calculated when aggregating in HandleSpan()
	// A |---------------------------------------------------------------| duration: 100
	// B   |----------------------|                                        duration: 20
	// C     |-----| |---|                                                 duration: 5+3
	trace := pb.Trace{
		&pb.Span{TraceID: 42, SpanID: 42, ParentID: 0, Service: "A",
			Name: "A.foo", Type: "web", Resource: "α", Start: 0, Duration: 100,
			Metrics: map[string]float64{sampler.KeySamplingRateGlobal: 0.5}},
		&pb.Span{TraceID: 42, SpanID: 100, ParentID: 42, Service: "B",
			Name: "B.bar", Type: "web", Resource: "α", Start: 1, Duration: 20},
		&pb.Span{TraceID: 42, SpanID: 2000, ParentID: 100, Service: "C",
			Name: "sql.query", Type: "sql", Resource: "SELECT value FROM table",
			Start: 2, Duration: 5},
		&pb.Span{TraceID: 42, SpanID: 3000, ParentID: 100, Service: "C",
			Name: "sql.query", Type: "sql", Resource: "SELECT ololololo... value FROM table",
			Start: 10, Duration: 3, Error: 1},
	}

	traceutil.ComputeTopLevel(trace)
	return trace
}

func testTraceTopLevel() pb.Trace {
	// Data below represents a trace with some sublayers, so that we make sure,
	// those data are correctly calculated when aggregating in HandleSpan()
	// In this case, the sublayers B and C have been merged into B,
	// showing what happens when some spans are not marked as top-level.
	// A |---------------------------------------------------------------| duration: 100
	// B   |----------------------|                                        duration: 20
	// B     |-----| |---|                                                 duration: 5+3
	trace := pb.Trace{
		&pb.Span{TraceID: 42, SpanID: 42, ParentID: 0, Service: "A",
			Name: "A.foo", Type: "web", Resource: "α", Start: 0, Duration: 100,
			Metrics: map[string]float64{sampler.KeySamplingRateGlobal: 1}},
		&pb.Span{TraceID: 42, SpanID: 100, ParentID: 42, Service: "B",
			Name: "B.bar", Type: "web", Resource: "α", Start: 1, Duration: 20},
		&pb.Span{TraceID: 42, SpanID: 2000, ParentID: 100, Service: "B",
			Name: "B.bar.1", Type: "web", Resource: "α",
			Start: 2, Duration: 5},
		&pb.Span{TraceID: 42, SpanID: 3000, ParentID: 100, Service: "B",
			Name: "B.bar.2", Type: "web", Resource: "α",
			Start: 10, Duration: 3, Error: 1},
	}

	traceutil.ComputeTopLevel(trace)
	return trace
}

func TestGrainKey(t *testing.T) {
	assert := assert.New(t)
	gk := GrainKey("serve", "duration", "service:webserver")
	assert.Equal("serve|duration|service:webserver", gk)
}

type expectedCount struct {
	value    float64
	topLevel float64
}

type expectedDistribution struct {
	entries  []quantile.Entry
	topLevel float64
}

func TestBucketDefault(t *testing.T) {
	assert := assert.New(t)

	srb := NewRawBucket(0, 1e9)

	// No custom aggregators only the defaults
	aggr := []string{}
	for _, s := range testWeightedSpans() {
		t.Logf("weight: %f, topLevel: %v", s.Weight, s.TopLevel)
		srb.HandleSpan(s, defaultEnv, aggr, nil)
	}
	sb := srb.Export()

	expectedCounts := map[string]expectedCount{
		"A.foo|duration|env:default,resource:α,service:A":     {value: 1, topLevel: 1},
		"A.foo|duration|env:default,resource:β,service:A":     {value: 2, topLevel: 1},
		"B.foo|duration|env:default,resource:γ,service:B":     {value: 3, topLevel: 1},
		"B.foo|duration|env:default,resource:ε,service:B":     {value: 4, topLevel: 1},
		"B.foo|duration|env:default,resource:ζ,service:B":     {value: 5, topLevel: 1},
		"sql.query|duration|env:default,resource:ζ,service:B": {value: 6, topLevel: 1},
		"sql.query|duration|env:default,resource:δ,service:C": {value: 15, topLevel: 2},
		"A.foo|errors|env:default,resource:α,service:A":       {value: 0, topLevel: 1},
		"A.foo|errors|env:default,resource:β,service:A":       {value: 1, topLevel: 1},
		"B.foo|errors|env:default,resource:γ,service:B":       {value: 0, topLevel: 1},
		"B.foo|errors|env:default,resource:ε,service:B":       {value: 1, topLevel: 1},
		"B.foo|errors|env:default,resource:ζ,service:B":       {value: 0, topLevel: 1},
		"sql.query|errors|env:default,resource:ζ,service:B":   {value: 0, topLevel: 1},
		"sql.query|errors|env:default,resource:δ,service:C":   {value: 0, topLevel: 2},
		"A.foo|hits|env:default,resource:α,service:A":         {value: 1, topLevel: 1},
		"A.foo|hits|env:default,resource:β,service:A":         {value: 1, topLevel: 1},
		"B.foo|hits|env:default,resource:γ,service:B":         {value: 1, topLevel: 1},
		"B.foo|hits|env:default,resource:ε,service:B":         {value: 1, topLevel: 1},
		"B.foo|hits|env:default,resource:ζ,service:B":         {value: 1, topLevel: 1},
		"sql.query|hits|env:default,resource:ζ,service:B":     {value: 1, topLevel: 1},
		"sql.query|hits|env:default,resource:δ,service:C":     {value: 2, topLevel: 2},
	}

	assert.Len(sb.Counts, len(expectedCounts), "Missing counts!")
	for ckey, c := range sb.Counts {
		val, ok := expectedCounts[ckey]
		if !ok {
			assert.Fail("Unexpected count %s", ckey)
		}
		assert.Equal(val.value, c.Value, "Count %s wrong value", ckey)
		assert.Equal(val.topLevel, c.TopLevel, "Count %s wrong topLevel", ckey)
	}

	expectedDistributions := map[string]expectedDistribution{
		"A.foo|duration|env:default,resource:α,service:A": {
			entries: []quantile.Entry{{V: 1, G: 1, Delta: 0}}, topLevel: 1},
		"A.foo|duration|env:default,resource:β,service:A": {
			entries: []quantile.Entry{{V: 2, G: 1, Delta: 0}}, topLevel: 1},
		"B.foo|duration|env:default,resource:γ,service:B": {
			entries: []quantile.Entry{{V: 3, G: 1, Delta: 0}}, topLevel: 1},
		"B.foo|duration|env:default,resource:ε,service:B": {
			entries: []quantile.Entry{{V: 4, G: 1, Delta: 0}}, topLevel: 1},
		"B.foo|duration|env:default,resource:ζ,service:B": {
			entries: []quantile.Entry{{V: 5, G: 1, Delta: 0}}, topLevel: 1},
		"sql.query|duration|env:default,resource:ζ,service:B": {
			entries: []quantile.Entry{{V: 6, G: 1, Delta: 0}}, topLevel: 1},
		"sql.query|duration|env:default,resource:δ,service:C": {
			entries: []quantile.Entry{{V: 7, G: 1, Delta: 0}, {V: 8, G: 1, Delta: 0}}, topLevel: 2},
	}

	for k, v := range sb.Distributions {
		t.Logf("%v: %v", k, v.Summary.Entries)
	}
	assert.Len(sb.Distributions, len(expectedDistributions), "Missing distributions!")
	for dkey, d := range sb.Distributions {
		val, ok := expectedDistributions[dkey]
		if !ok {
			assert.Fail("Unexpected distribution %s", dkey)
		}
		assert.Equal(val.entries, d.Summary.Entries, "Distribution %s wrong value", dkey)
		assert.Equal(val.topLevel, d.TopLevel, "Distribution %s wrong topLevel", dkey)
	}

	expectedErrDistributions := map[string]expectedDistribution{
		"A.foo|duration|env:default,resource:α,service:A": {
			entries: nil, topLevel: 1},
		"A.foo|duration|env:default,resource:β,service:A": {
			entries: []quantile.Entry{{V: 2, G: 1, Delta: 0}}, topLevel: 1},
		"B.foo|duration|env:default,resource:γ,service:B": {
			entries: nil, topLevel: 1},
		"B.foo|duration|env:default,resource:ε,service:B": {
			entries: []quantile.Entry{{V: 4, G: 1, Delta: 0}}, topLevel: 1},
		"B.foo|duration|env:default,resource:ζ,service:B": {
			entries: nil, topLevel: 1},
		"sql.query|duration|env:default,resource:ζ,service:B": {
			entries: nil, topLevel: 1},
		"sql.query|duration|env:default,resource:δ,service:C": {
			entries: nil, topLevel: 2},
	}

	for k, v := range sb.ErrDistributions {
		t.Logf("%v: %v", k, v.Summary.Entries)
	}
	assert.Len(sb.ErrDistributions, len(expectedErrDistributions), "Missing distributions!")
	for dkey, d := range sb.ErrDistributions {
		val, ok := expectedErrDistributions[dkey]
		if !ok {
			assert.Fail("Unexpected distribution %s", dkey)
		}
		assert.Equal(val.entries, d.Summary.Entries, "ErrDistribution %s wrong value", dkey)
		assert.Equal(val.topLevel, d.TopLevel, "ErrDistribution %s wrong topLevel", dkey)
	}
}

func TestBucketExtraAggregators(t *testing.T) {
	assert := assert.New(t)

	srb := NewRawBucket(0, 1e9)

	// one custom aggregator
	aggr := []string{"version"}
	for _, s := range testWeightedSpans() {
		srb.HandleSpan(s, defaultEnv, aggr, nil)
	}
	sb := srb.Export()

	expectedCounts := map[string]expectedCount{
		"A.foo|duration|env:default,resource:α,service:A":                 {value: 1, topLevel: 1},
		"A.foo|duration|env:default,resource:β,service:A":                 {value: 2, topLevel: 1},
		"B.foo|duration|env:default,resource:γ,service:B":                 {value: 3, topLevel: 1},
		"B.foo|duration|env:default,resource:ε,service:B":                 {value: 4, topLevel: 1},
		"sql.query|duration|env:default,resource:δ,service:C":             {value: 15, topLevel: 2},
		"A.foo|errors|env:default,resource:α,service:A":                   {value: 0, topLevel: 1},
		"A.foo|errors|env:default,resource:β,service:A":                   {value: 1, topLevel: 1},
		"B.foo|errors|env:default,resource:γ,service:B":                   {value: 0, topLevel: 1},
		"B.foo|errors|env:default,resource:ε,service:B":                   {value: 1, topLevel: 1},
		"sql.query|errors|env:default,resource:δ,service:C":               {value: 0, topLevel: 2},
		"A.foo|hits|env:default,resource:α,service:A":                     {value: 1, topLevel: 1},
		"A.foo|hits|env:default,resource:β,service:A":                     {value: 1, topLevel: 1},
		"B.foo|hits|env:default,resource:γ,service:B":                     {value: 1, topLevel: 1},
		"B.foo|hits|env:default,resource:ε,service:B":                     {value: 1, topLevel: 1},
		"sql.query|hits|env:default,resource:δ,service:C":                 {value: 2, topLevel: 2},
		"sql.query|errors|env:default,resource:ζ,service:B,version:1.4":   {value: 0, topLevel: 1},
		"sql.query|hits|env:default,resource:ζ,service:B,version:1.4":     {value: 1, topLevel: 1},
		"sql.query|duration|env:default,resource:ζ,service:B,version:1.4": {value: 6, topLevel: 1},
		"B.foo|errors|env:default,resource:ζ,service:B,version:1.3":       {value: 0, topLevel: 1},
		"B.foo|duration|env:default,resource:ζ,service:B,version:1.3":     {value: 5, topLevel: 1},
		"B.foo|hits|env:default,resource:ζ,service:B,version:1.3":         {value: 1, topLevel: 1},
	}

	assert.Len(sb.Counts, len(expectedCounts), "Missing counts!")
	for ckey, c := range sb.Counts {
		val, ok := expectedCounts[ckey]
		if !ok {
			assert.Fail("Unexpected count %s", ckey)
		}
		assert.Equal(val.value, c.Value, "Count %s wrong value", ckey)
		assert.Equal(val.topLevel, c.TopLevel, "Count %s wrong topLevel", ckey)
		keyFields := strings.Split(ckey, "|")
		tags := NewTagSetFromString(keyFields[2])
		assert.Equal(tags, c.TagSet, "bad tagset for count %s", ckey)
	}
}

func TestBucketMany(t *testing.T) {
	if testing.Short() {
		return
	}

	assert := assert.New(t)

	templateSpan := &WeightedSpan{
		Span:     &pb.Span{Service: "A", Name: "A.foo", Resource: "α", Duration: 7},
		Weight:   1,
		TopLevel: true,
	}
	const n = 100000

	srb := NewRawBucket(0, 1e9)

	// No custom aggregators only the defaults
	aggr := []string{}
	for i := 0; i < n; i++ {
		s := templateSpan
		s.Resource = "α" + strconv.Itoa(i)
		srbCopy := *srb
		srbCopy.HandleSpan(s, defaultEnv, aggr, nil)
	}
	sb := srb.Export()

	assert.Len(sb.Counts, 3*n, "Missing counts %d != %d", len(sb.Counts), 3*n)
	for ckey, c := range sb.Counts {
		if strings.Contains(ckey, "|duration|") {
			assert.Equal(7.0, c.Value, "duration %s wrong value", ckey)
		}
		if strings.Contains(ckey, "|errors|") {
			assert.Equal(0.0, c.Value, "errors %s wrong value", ckey)
		}
		if strings.Contains(ckey, "|hits|") {
			assert.Equal(1.0, c.Value, "hits %s wrong value", ckey)
		}
	}
}

func TestBucketSublayers(t *testing.T) {
	assert := assert.New(t)

	tr := testTrace()
	sublayers := ComputeSublayers(tr)
	root := traceutil.GetRoot(tr)
	SetSublayersOnSpan(root, sublayers)

	wt := NewWeightedTrace(tr, root)

	assert.NotNil(sublayers)

	srb := NewRawBucket(0, 1e9)

	// No custom aggregators only the defaults
	aggr := []string{}
	for _, s := range wt {
		srb.HandleSpan(s, defaultEnv, aggr, sublayers)
	}
	sb := srb.Export()

	expectedCounts := map[string]expectedCount{
		"A.foo|_sublayers.duration.by_service|env:default,resource:α,service:A,sublayer_service:A":                                        {value: 160, topLevel: 2},
		"A.foo|_sublayers.duration.by_service|env:default,resource:α,service:A,sublayer_service:B":                                        {value: 24, topLevel: 2},
		"A.foo|_sublayers.duration.by_service|env:default,resource:α,service:A,sublayer_service:C":                                        {value: 16, topLevel: 2},
		"A.foo|_sublayers.duration.by_type|env:default,resource:α,service:A,sublayer_type:sql":                                            {value: 16, topLevel: 2},
		"A.foo|_sublayers.duration.by_type|env:default,resource:α,service:A,sublayer_type:web":                                            {value: 184, topLevel: 2},
		"A.foo|_sublayers.span_count|env:default,resource:α,service:A,:":                                                                  {value: 8, topLevel: 2},
		"A.foo|duration|env:default,resource:α,service:A":                                                                                 {value: 200, topLevel: 2},
		"A.foo|errors|env:default,resource:α,service:A":                                                                                   {value: 0, topLevel: 2},
		"A.foo|hits|env:default,resource:α,service:A":                                                                                     {value: 2, topLevel: 2},
		"B.bar|_sublayers.duration.by_service|env:default,resource:α,service:B,sublayer_service:A":                                        {value: 160, topLevel: 2},
		"B.bar|_sublayers.duration.by_service|env:default,resource:α,service:B,sublayer_service:B":                                        {value: 24, topLevel: 2},
		"B.bar|_sublayers.duration.by_service|env:default,resource:α,service:B,sublayer_service:C":                                        {value: 16, topLevel: 2},
		"B.bar|_sublayers.duration.by_type|env:default,resource:α,service:B,sublayer_type:sql":                                            {value: 16, topLevel: 2},
		"B.bar|_sublayers.duration.by_type|env:default,resource:α,service:B,sublayer_type:web":                                            {value: 184, topLevel: 2},
		"B.bar|_sublayers.span_count|env:default,resource:α,service:B,:":                                                                  {value: 8, topLevel: 2},
		"B.bar|duration|env:default,resource:α,service:B":                                                                                 {value: 40, topLevel: 2},
		"B.bar|errors|env:default,resource:α,service:B":                                                                                   {value: 0, topLevel: 2},
		"B.bar|hits|env:default,resource:α,service:B":                                                                                     {value: 2, topLevel: 2},
		"sql.query|_sublayers.duration.by_service|env:default,resource:SELECT ololololo... value FROM table,service:C,sublayer_service:A": {value: 160, topLevel: 2},
		"sql.query|_sublayers.duration.by_service|env:default,resource:SELECT ololololo... value FROM table,service:C,sublayer_service:B": {value: 24, topLevel: 2},
		"sql.query|_sublayers.duration.by_service|env:default,resource:SELECT ololololo... value FROM table,service:C,sublayer_service:C": {value: 16, topLevel: 2},
		"sql.query|_sublayers.duration.by_service|env:default,resource:SELECT value FROM table,service:C,sublayer_service:A":              {value: 160, topLevel: 2},
		"sql.query|_sublayers.duration.by_service|env:default,resource:SELECT value FROM table,service:C,sublayer_service:B":              {value: 24, topLevel: 2},
		"sql.query|_sublayers.duration.by_service|env:default,resource:SELECT value FROM table,service:C,sublayer_service:C":              {value: 16, topLevel: 2},
		"sql.query|_sublayers.duration.by_type|env:default,resource:SELECT ololololo... value FROM table,service:C,sublayer_type:sql":     {value: 16, topLevel: 2},
		"sql.query|_sublayers.duration.by_type|env:default,resource:SELECT ololololo... value FROM table,service:C,sublayer_type:web":     {value: 184, topLevel: 2},
		"sql.query|_sublayers.duration.by_type|env:default,resource:SELECT value FROM table,service:C,sublayer_type:sql":                  {value: 16, topLevel: 2},
		"sql.query|_sublayers.duration.by_type|env:default,resource:SELECT value FROM table,service:C,sublayer_type:web":                  {value: 184, topLevel: 2},
		"sql.query|_sublayers.span_count|env:default,resource:SELECT ololololo... value FROM table,service:C,:":                           {value: 8, topLevel: 2},
		"sql.query|_sublayers.span_count|env:default,resource:SELECT value FROM table,service:C,:":                                        {value: 8, topLevel: 2},
		"sql.query|duration|env:default,resource:SELECT ololololo... value FROM table,service:C":                                          {value: 6, topLevel: 2},
		"sql.query|duration|env:default,resource:SELECT value FROM table,service:C":                                                       {value: 10, topLevel: 2},
		"sql.query|errors|env:default,resource:SELECT ololololo... value FROM table,service:C":                                            {value: 2, topLevel: 2},
		"sql.query|errors|env:default,resource:SELECT value FROM table,service:C":                                                         {value: 0, topLevel: 2},
		"sql.query|hits|env:default,resource:SELECT ololololo... value FROM table,service:C":                                              {value: 2, topLevel: 2},
		"sql.query|hits|env:default,resource:SELECT value FROM table,service:C":                                                           {value: 2, topLevel: 2},
	}

	assert.Len(sb.Counts, len(expectedCounts), "Missing counts!")
	for ckey, c := range sb.Counts {
		val, ok := expectedCounts[ckey]
		if !ok {
			assert.Fail("Unexpected count %s", ckey)
		}
		assert.Equal(val.value, c.Value, "Count %s wrong value", ckey)
		assert.Equal(val.topLevel, c.TopLevel, "Count %s wrong topLevel", ckey)
		keyFields := strings.Split(ckey, "|")
		tags := NewTagSetFromString(keyFields[2])
		assert.Equal(tags, c.TagSet, "bad tagset for count %s", ckey)
	}

	expectedDistributions := map[string]expectedDistribution{
		"A.foo|duration|env:default,resource:α,service:A": {
			entries: []quantile.Entry{{V: 100, G: 1, Delta: 0}}, topLevel: 2},
		"B.bar|duration|env:default,resource:α,service:B": {
			entries: []quantile.Entry{{V: 20, G: 1, Delta: 0}}, topLevel: 2},
		"sql.query|duration|env:default,resource:SELECT value FROM table,service:C": {
			entries: []quantile.Entry{{V: 5, G: 1, Delta: 0}}, topLevel: 2},
		"sql.query|duration|env:default,resource:SELECT ololololo... value FROM table,service:C": {
			entries: []quantile.Entry{{V: 3, G: 1, Delta: 0}}, topLevel: 2},
	}

	assert.Len(sb.Distributions, len(expectedDistributions), "Missing distributions!")
	for dkey, d := range sb.Distributions {
		val, ok := expectedDistributions[dkey]
		if !ok {
			assert.Fail("Unexpected distribution %s", dkey)
		}
		assert.Equal(val.entries, d.Summary.Entries, "Distribution %s wrong value", dkey)
		assert.Equal(val.topLevel, d.TopLevel, "Distribution %s wrong topLevel", dkey)
		keyFields := strings.Split(dkey, "|")
		tags := NewTagSetFromString(keyFields[2])
		assert.Equal(tags, d.TagSet, "bad tagset for distribution %s", dkey)
	}
}

func TestBucketSublayersTopLevel(t *testing.T) {
	assert := assert.New(t)

	tr := testTraceTopLevel()
	sublayers := ComputeSublayers(tr)
	root := traceutil.GetRoot(tr)
	SetSublayersOnSpan(root, sublayers)

	wt := NewWeightedTrace(tr, root)

	assert.NotNil(sublayers)

	srb := NewRawBucket(0, 1e9)

	// No custom aggregators only the defaults
	aggr := []string{}
	for _, s := range wt {
		srb.HandleSpan(s, defaultEnv, aggr, sublayers)
	}
	sb := srb.Export()

	expectedCounts := map[string]expectedCount{
		"A.foo|_sublayers.duration.by_service|env:default,resource:α,service:A,sublayer_service:A": {value: 80, topLevel: 1},
		"A.foo|_sublayers.duration.by_service|env:default,resource:α,service:A,sublayer_service:B": {value: 20, topLevel: 1},
		"A.foo|_sublayers.duration.by_type|env:default,resource:α,service:A,sublayer_type:web":     {value: 100, topLevel: 1},
		"A.foo|_sublayers.span_count|env:default,resource:α,service:A,:":                           {value: 4, topLevel: 1},
		"A.foo|hits|env:default,resource:α,service:A":                                              {value: 1, topLevel: 1},
		"A.foo|errors|env:default,resource:α,service:A":                                            {value: 0, topLevel: 1},
		"A.foo|duration|env:default,resource:α,service:A":                                          {value: 100, topLevel: 1},
		"B.bar|_sublayers.duration.by_service|env:default,resource:α,service:B,sublayer_service:A": {value: 80, topLevel: 1},
		"B.bar|_sublayers.duration.by_service|env:default,resource:α,service:B,sublayer_service:B": {value: 20, topLevel: 1},
		"B.bar|_sublayers.duration.by_type|env:default,resource:α,service:B,sublayer_type:web":     {value: 100, topLevel: 1},
		"B.bar|_sublayers.span_count|env:default,resource:α,service:B,:":                           {value: 4, topLevel: 1},
		"B.bar|hits|env:default,resource:α,service:B":                                              {value: 1, topLevel: 1},
		"B.bar|errors|env:default,resource:α,service:B":                                            {value: 0, topLevel: 1},
		"B.bar|duration|env:default,resource:α,service:B":                                          {value: 20, topLevel: 1},
		// [TODO] the ultimate target is to *NOT* compute & store the counts below, which have topLevel == 0
		"B.bar.1|_sublayers.duration.by_service|env:default,resource:α,service:B,sublayer_service:A": {value: 80, topLevel: 0},
		"B.bar.1|_sublayers.duration.by_service|env:default,resource:α,service:B,sublayer_service:B": {value: 20, topLevel: 0},
		"B.bar.1|_sublayers.duration.by_type|env:default,resource:α,service:B,sublayer_type:web":     {value: 100, topLevel: 0},
		"B.bar.1|_sublayers.span_count|env:default,resource:α,service:B,:":                           {value: 4, topLevel: 0},
		"B.bar.1|hits|env:default,resource:α,service:B":                                              {value: 1, topLevel: 0},
		"B.bar.1|errors|env:default,resource:α,service:B":                                            {value: 0, topLevel: 0},
		"B.bar.1|duration|env:default,resource:α,service:B":                                          {value: 5, topLevel: 0},
		"B.bar.2|_sublayers.duration.by_service|env:default,resource:α,service:B,sublayer_service:A": {value: 80, topLevel: 0},
		"B.bar.2|_sublayers.duration.by_service|env:default,resource:α,service:B,sublayer_service:B": {value: 20, topLevel: 0},
		"B.bar.2|_sublayers.duration.by_type|env:default,resource:α,service:B,sublayer_type:web":     {value: 100, topLevel: 0},
		"B.bar.2|_sublayers.span_count|env:default,resource:α,service:B,:":                           {value: 4, topLevel: 0},
		"B.bar.2|hits|env:default,resource:α,service:B":                                              {value: 1, topLevel: 0},
		"B.bar.2|errors|env:default,resource:α,service:B":                                            {value: 1, topLevel: 0},
		"B.bar.2|duration|env:default,resource:α,service:B":                                          {value: 3, topLevel: 0},
	}

	assert.Len(sb.Counts, len(expectedCounts), "Missing counts!")
	for ckey, c := range sb.Counts {
		val, ok := expectedCounts[ckey]
		if !ok {
			assert.Fail("Unexpected count %s", ckey)
		}
		assert.Equal(val.value, c.Value, "Count %s wrong value", ckey)
		assert.Equal(val.topLevel, c.TopLevel, "Count %s wrong topLevel", ckey)
		keyFields := strings.Split(ckey, "|")
		tags := NewTagSetFromString(keyFields[2])
		assert.Equal(tags, c.TagSet, "bad tagset for count %s", ckey)
	}

	expectedDistributions := map[string]expectedDistribution{
		"A.foo|duration|env:default,resource:α,service:A": {
			entries: []quantile.Entry{{V: 100, G: 1, Delta: 0}}, topLevel: 1},
		"B.bar|duration|env:default,resource:α,service:B": {
			entries: []quantile.Entry{{V: 20, G: 1, Delta: 0}}, topLevel: 1},
		// [TODO] the ultimate target is to *NOT* compute & store the counts below, which have topLevel == 0
		"B.bar.1|duration|env:default,resource:α,service:B": {
			entries: []quantile.Entry{{V: 5, G: 1, Delta: 0}}, topLevel: 0},
		"B.bar.2|duration|env:default,resource:α,service:B": {
			entries: []quantile.Entry{{V: 3, G: 1, Delta: 0}}, topLevel: 0},
	}

	assert.Len(sb.Distributions, len(expectedDistributions), "Missing distributions!")
	for dkey, d := range sb.Distributions {
		val, ok := expectedDistributions[dkey]
		if !ok {
			assert.Fail("Unexpected distribution %s", dkey)
		}
		assert.Equal(val.entries, d.Summary.Entries, "Distribution %s wrong value", dkey)
		assert.Equal(val.topLevel, d.TopLevel, "Distribution %s wrong topLevel", dkey)
		keyFields := strings.Split(dkey, "|")
		tags := NewTagSetFromString(keyFields[2])
		assert.Equal(tags, d.TagSet, "bad tagset for distribution %s", dkey)
	}
}

func TestTsRounding(t *testing.T) {
	assert := assert.New(t)

	durations := []int64{
		3 * 1e9,     // 10110010110100000101111000000000 -> 10110010110000000000000000000000 = 2998927360
		32432874923, // 11110001101001001100110010110101011 -> 11110001100000000000000000000000000 = 32413581312
		1000,        // Keep it with full precision
		45,          // Keep it with full precision
		41000234,    // 10011100011001110100101010 -> 10011100010000000000000000 = 40960000
	}

	type testcase struct {
		res time.Duration
		exp []float64
	}

	exp := []float64{2998927360, 32413581312, 1000, 45, 40960000}

	results := []float64{}
	for _, d := range durations {
		results = append(results, nsTimestampToFloat(d))
	}
	assert.Equal(exp, results, "Unproper rounding of timestamp")
}

func BenchmarkHandleSpan(b *testing.B) {

	srb := NewRawBucket(0, 1e9)
	aggr := []string{}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, s := range testWeightedSpans() {
			srb.HandleSpan(s, defaultEnv, aggr, nil)
		}
	}
}

func BenchmarkHandleSpanSublayers(b *testing.B) {
	srb := NewRawBucket(0, 1e9)
	aggr := []string{}

	tr := testTrace()
	sublayers := ComputeSublayers(tr)
	root := traceutil.GetRoot(tr)
	SetSublayersOnSpan(root, sublayers)

	wt := NewWeightedTrace(tr, root)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		for _, s := range wt {
			srb.HandleSpan(s, defaultEnv, aggr, sublayers)
		}
	}
}

// it's important to have these defined as var and not const/inline
// else compiler performs compile-time optimization when using + with strings
var grainName = "mysql.query"
var grainMeasure = "duration"
var grainAggr = "resource:SELECT * FROM stuff,service:mysql"

// testing out various way of doing string ops, to check which one is most efficient
func BenchmarkGrainKey(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = GrainKey(grainName, grainMeasure, grainAggr)
	}
}

func BenchmarkStringPlus(b *testing.B) {
	if testing.Short() {
		return
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = grainName + "|" + grainMeasure + "|" + grainAggr
	}
}

func BenchmarkSprintf(b *testing.B) {
	if testing.Short() {
		return
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = fmt.Sprintf("%s|%s|%s", grainName, grainMeasure, grainAggr)
	}
}

func BenchmarkBufferWriteByte(b *testing.B) {
	if testing.Short() {
		return
	}
	var buf bytes.Buffer
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		buf.WriteString(grainName)
		buf.WriteByte('|')
		buf.WriteString(grainMeasure)
		buf.WriteByte('|')
		buf.WriteString(grainAggr)
		_ = buf.String()
	}
}

func BenchmarkBufferWriteRune(b *testing.B) {
	if testing.Short() {
		return
	}
	var buf bytes.Buffer
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf.Reset()
		buf.WriteString(grainName)
		buf.WriteRune('|')
		buf.WriteString(grainMeasure)
		buf.WriteRune('|')
		buf.WriteString(grainAggr)
		_ = buf.String()
	}
}

func BenchmarkStringsJoin(b *testing.B) {
	if testing.Short() {
		return
	}
	a := []string{grainName, grainMeasure, grainAggr}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = strings.Join(a, "|")
	}
}
