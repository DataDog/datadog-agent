// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"math"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/semantics"
)

func newTestSpanV1(attrs map[string]*idx.AnyValue) *idx.InternalSpan {
	st := idx.NewStringTable()
	m := make(map[uint32]*idx.AnyValue, len(attrs))
	for k, v := range attrs {
		m[st.Add(k)] = v
	}
	return idx.NewInternalSpan(st, &idx.Span{Attributes: m})
}

func doubleValue(v float64) *idx.AnyValue {
	return &idx.AnyValue{Value: &idx.AnyValue_DoubleValue{DoubleValue: v}}
}

func intValue(v int64) *idx.AnyValue {
	return &idx.AnyValue{Value: &idx.AnyValue_IntValue{IntValue: v}}
}

func TestNewStatSpanFlags(t *testing.T) {
	sc := NewSpanConcentrator(&SpanConcentratorConfig{BucketInterval: 10e9}, time.Now())
	for _, tc := range []struct {
		name     string
		metrics  map[string]float64
		eligible bool
	}{
		{"no flags", map[string]float64{}, false},
		{"measured", map[string]float64{"_dd.measured": 1}, true},
		{"measured 0", map[string]float64{"_dd.measured": 0}, false},
		{"tracer top-level", map[string]float64{"_dd.top_level": 1}, true},
		{"agent top-level", map[string]float64{"_top_level": 1}, true},
		// First present key wins.
		{"conflicting top-level keys", map[string]float64{"_dd.top_level": 0, "_top_level": 1}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, ok := sc.NewStatSpanWithConfig(StatSpanConfig{Metrics: tc.metrics})
			assert.Equal(t, tc.eligible, ok, "v0")
			double := make(map[string]*idx.AnyValue, len(tc.metrics))
			integer := make(map[string]*idx.AnyValue, len(tc.metrics))
			for k, v := range tc.metrics {
				double[k] = doubleValue(v)
				integer[k] = intValue(int64(v))
			}
			_, ok = sc.NewStatSpanFromV1(newTestSpanV1(double), nil, nil)
			assert.Equal(t, tc.eligible, ok, "v1 double")
			_, ok = sc.NewStatSpanFromV1(newTestSpanV1(integer), nil, nil)
			assert.Equal(t, tc.eligible, ok, "v1 int")
		})
	}
}

func TestNewStatSpanIgnoresNonNumericFlagsV1(t *testing.T) {
	sc := NewSpanConcentrator(&SpanConcentratorConfig{BucketInterval: 10e9}, time.Now())
	st := idx.NewStringTable()
	for _, v := range []*idx.AnyValue{
		{Value: &idx.AnyValue_BoolValue{BoolValue: true}},
		{Value: &idx.AnyValue_StringValueRef{StringValueRef: st.Add("1")}},
	} {
		s := idx.NewInternalSpan(st, &idx.Span{Attributes: map[uint32]*idx.AnyValue{st.Add("_dd.measured"): v}})
		_, ok := sc.NewStatSpanFromV1(s, nil, nil)
		assert.False(t, ok, "%T", v.Value)
	}
}

func TestNewStatSpanSkipsPartialSnapshots(t *testing.T) {
	sc := NewSpanConcentrator(&SpanConcentratorConfig{BucketInterval: 10e9}, time.Now())
	for _, tc := range []struct {
		name    string
		version float64
		set     bool
		partial bool
	}{
		{"no version", 0, false, false},
		{"version 0", 0, true, true},
		{"version 5", 5, true, true},
		{"negative version", -1, true, false},
		{"fractional version", 0.5, true, true},
		{"+Inf version", math.Inf(1), true, true},
		{"NaN version", math.NaN(), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			metrics := map[string]float64{"_top_level": 1}
			double := map[string]*idx.AnyValue{"_top_level": doubleValue(1)}
			integer := map[string]*idx.AnyValue{"_top_level": intValue(1)}
			if tc.set {
				metrics["_dd.partial_version"] = tc.version
				double["_dd.partial_version"] = doubleValue(tc.version)
				integer["_dd.partial_version"] = intValue(int64(tc.version))
			}
			_, ok := sc.NewStatSpanWithConfig(StatSpanConfig{Metrics: metrics})
			assert.Equal(t, !tc.partial, ok, "v0")
			_, ok = sc.NewStatSpanFromV1(newTestSpanV1(double), nil, nil)
			assert.Equal(t, !tc.partial, ok, "v1 double")
			if tc.version == math.Trunc(tc.version) && !math.IsInf(tc.version, 0) {
				_, ok = sc.NewStatSpanFromV1(newTestSpanV1(integer), nil, nil)
				assert.Equal(t, !tc.partial, ok, "v1 int")
			}
		})
	}
}

// TestNewStatSpanFollowsRegistry checks that a measured alias added by a registry
// swap (e.g. Remote Config) makes spans eligible for stats.
func TestNewStatSpanFollowsRegistry(t *testing.T) {
	original := semantics.DefaultRegistry()
	t.Cleanup(func() { semantics.UpdateRegistry(original) })

	const alias = "_dd.measured.alias"
	sc := NewSpanConcentrator(&SpanConcentratorConfig{BucketInterval: 10e9}, time.Now())
	cfg := StatSpanConfig{Metrics: map[string]float64{alias: 1}}
	v1 := newTestSpanV1(map[string]*idx.AnyValue{alias: doubleValue(1)})
	_, ok := sc.NewStatSpanWithConfig(cfg)
	require.False(t, ok, "v0 before swap")
	_, ok = sc.NewStatSpanFromV1(v1, nil, nil)
	require.False(t, ok, "v1 before swap")

	custom, err := semantics.NewRegistryFromJSON([]byte(`{"metadata":{"content_hash":"test"},"concepts":{"_dd.measured":{"fallbacks":[` +
		`{"name":"_dd.measured","provider":"datadog","type":"int64"},{"name":"` + alias + `","provider":"datadog","type":"int64"}]}}}`))
	require.NoError(t, err)
	semantics.UpdateRegistry(custom)

	_, ok = sc.NewStatSpanWithConfig(cfg)
	assert.True(t, ok, "v0 after swap")
	_, ok = sc.NewStatSpanFromV1(v1, nil, nil)
	assert.True(t, ok, "v1 after swap")
}
