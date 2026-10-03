// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package observer

import "testing"

type contextReader struct {
	StorageReader
	context *MetricContext
	calls   int
}

func (r *contextReader) GetContext(SeriesRef) (MetricContext, bool) {
	r.calls++
	if r.context == nil {
		return MetricContext{}, false
	}
	return *r.context, true
}

func TestMaterializeAnomalyResolvesCurrentContextWithoutChangingInput(t *testing.T) {
	reader := &contextReader{context: &MetricContext{Pattern: "first"}}
	a := Anomaly{SourceRef: &QueryHandle{Ref: 7}}

	materialized := MaterializeAnomaly(a, reader)
	if reader.calls != 1 || a.Context != nil || materialized.Anomaly.Context == nil || materialized.Anomaly.Context.Pattern != "first" {
		t.Fatalf("unexpected materialization: calls=%d, input=%+v, output=%+v", reader.calls, a, materialized)
	}
	reader.context.Pattern = "second"
	if materialized.Anomaly.Context.Pattern != "first" {
		t.Fatal("prepared anomaly changed after storage update")
	}
	if got := MaterializeAnomaly(a, reader).Anomaly.Context.Pattern; got != "second" {
		t.Fatalf("new output got %q, want current context", got)
	}
}

func TestResolveAnomalyContextFallbackAndExplicitContext(t *testing.T) {
	reader := &contextReader{}
	if got := ResolveAnomalyContext(Anomaly{}, reader); got.Context != nil || reader.calls != 0 {
		t.Fatal("anomaly without a series should not resolve")
	}
	a := Anomaly{SourceRef: &QueryHandle{Ref: 3}}
	if got := ResolveAnomalyContext(a, reader); got.Context != nil || reader.calls != 1 {
		t.Fatal("missing context should leave anomaly unenriched")
	}
	explicit := &MetricContext{Pattern: "caller supplied"}
	a.Context = explicit
	if got := ResolveAnomalyContext(a, reader); got.Context != explicit || reader.calls != 1 {
		t.Fatal("explicit context should be preserved without a lookup")
	}
}

func BenchmarkMaterializeAnomaly(b *testing.B) {
	a := Anomaly{SourceRef: &QueryHandle{Ref: 7}, DetectorName: "scanmw", DebugInfo: &AnomalyDebugInfo{CurrentValue: 5}}
	for _, tc := range []struct {
		name    string
		context *MetricContext
	}{
		{name: "context", context: &MetricContext{Pattern: "error <*> timeout", Example: "error db timeout"}},
		{name: "missing"},
	} {
		b.Run(tc.name, func(b *testing.B) {
			reader := &contextReader{context: tc.context}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				_ = MaterializeAnomaly(a, reader)
			}
		})
	}
}
