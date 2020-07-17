// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

package traceutil

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/traces"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/stretchr/testify/assert"
)

func TestTopLevelTypical(t *testing.T) {
	assert := assert.New(t)

	tr := traces.NewTrace([]traces.Span{
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "mcnulty", Type: "sql"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 3, ParentID: 2, Service: "master-db", Type: "sql"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Service: "redis", Type: "redis"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 5, ParentID: 1, Service: "mcnulty", Type: ""}),
	})

	ComputeTopLevel(tr)

	assert.True(HasTopLevel(tr.Spans[0]), "root span should be top-level")
	assert.False(HasTopLevel(tr.Spans[1]), "main service, and not a root span, not top-level")
	assert.True(HasTopLevel(tr.Spans[2]), "only 1 span for this service, should be top-level")
	assert.True(HasTopLevel(tr.Spans[3]), "only 1 span for this service, should be top-level")
	assert.False(HasTopLevel(tr.Spans[4]), "yet another sup span, not top-level")
}

// func TestSetMeta(t *testing.T) {
// 	for _, s := range []*pb.Span{
// 		{},
// 		{Meta: map[string]string{"A": "B", "C": "D"}},
// 	} {
// 		SetMeta(s, "X", "Y")
// 		assert.NotNil(t, s.Meta)
// 		assert.Equal(t, s.Meta["X"], "Y")
// 	}
// }

func TestTopLevelSingle(t *testing.T) {
	assert := assert.New(t)

	tr := traces.NewTrace([]traces.Span{
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web"}),
	})

	ComputeTopLevel(tr)

	assert.True(HasTopLevel(tr.Spans[0]), "root span should be top-level")
}

func TestTopLevelEmpty(t *testing.T) {
	assert := assert.New(t)

	tr := traces.NewTrace(nil)

	ComputeTopLevel(tr)

	assert.Equal(0, len(tr.Spans), "trace should still be empty")
}

func TestTopLevelOneService(t *testing.T) {
	assert := assert.New(t)

	tr := traces.NewTrace([]traces.Span{
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "mcnulty", Type: "web"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 3, ParentID: 2, Service: "mcnulty", Type: "web"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Service: "mcnulty", Type: "web"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 5, ParentID: 1, Service: "mcnulty", Type: "web"}),
	})

	ComputeTopLevel(tr)

	assert.False(HasTopLevel(tr.Spans[0]), "just a sub-span, not top-level")
	assert.False(HasTopLevel(tr.Spans[1]), "just a sub-span, not top-level")
	assert.True(HasTopLevel(tr.Spans[2]), "root span should be top-level")
	assert.False(HasTopLevel(tr.Spans[3]), "just a sub-span, not top-level")
	assert.False(HasTopLevel(tr.Spans[4]), "just a sub-span, not top-level")
}

func TestTopLevelLocalRoot(t *testing.T) {
	assert := assert.New(t)

	tr := traces.NewTrace([]traces.Span{
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "mcnulty", Type: "sql"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 3, ParentID: 2, Service: "master-db", Type: "sql"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Service: "redis", Type: "redis"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 5, ParentID: 1, Service: "mcnulty", Type: ""}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 6, ParentID: 4, Service: "redis", Type: "redis"}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 7, ParentID: 4, Service: "redis", Type: "redis"}),
	})

	ComputeTopLevel(tr)

	assert.True(HasTopLevel(tr.Spans[0]), "root span should be top-level")
	assert.False(HasTopLevel(tr.Spans[1]), "main service, and not a root span, not top-level")
	assert.True(HasTopLevel(tr.Spans[2]), "only 1 span for this service, should be top-level")
	assert.True(HasTopLevel(tr.Spans[3]), "top-level but not root")
	assert.False(HasTopLevel(tr.Spans[4]), "yet another sup span, not top-level")
	assert.False(HasTopLevel(tr.Spans[5]), "yet another sup span, not top-level")
	assert.False(HasTopLevel(tr.Spans[6]), "yet another sup span, not top-level")
}

func TestTopLevelWithTag(t *testing.T) {
	assert := assert.New(t)

	tr := traces.NewTrace([]traces.Span{
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web", Metrics: map[string]float64{"custom": 42}}),
		traces.NewEagerSpan(pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "mcnulty", Type: "web", Metrics: map[string]float64{"custom": 42}}),
	})

	ComputeTopLevel(tr)

	// TODO: Fix me.
	// t.Logf("%v\n", tr[1].Metrics)

	assert.True(HasTopLevel(tr.Spans[0]), "root span should be top-level")
	// TODO: Fix me.
	// assert.Equal(float64(42), tr.Spans[0].Metrics["custom"], "custom metric should still be here")
	assert.False(HasTopLevel(tr.Spans[1]), "not a top-level span")
	// TODO: Fix me.
	// assert.Equal(float64(42), tr.Spans[1].Metrics["custom"], "custom metric should still be here")
}

func TestTopLevelGetSetBlackBox(t *testing.T) {
	assert := assert.New(t)

	span := traces.NewEagerSpan(pb.Span{})

	assert.False(HasTopLevel(span), "by default, all spans are considered non top-level")
	SetTopLevel(span, true)
	assert.True(HasTopLevel(span), "marked as top-level")
	SetTopLevel(span, false)
	assert.False(HasTopLevel(span), "no more top-level")

	// TODO: Fix me.
	// span.Metrics = map[string]float64{"custom": 42}

	assert.False(HasTopLevel(span), "by default, all spans are considered non top-level")
	SetTopLevel(span, true)
	assert.True(HasTopLevel(span), "marked as top-level")
	SetTopLevel(span, false)
	assert.False(HasTopLevel(span), "no more top-level")
}

func TestTopLevelGetSetMetrics(t *testing.T) {
	// assert := assert.New(t)

	// span := traces.NewEagerSpan(pb.Span{})

	// TODO: Fix me.
	// assert.Nil(span.Metrics, "no meta at all")
	// SetTopLevel(span, true)
	// assert.Equal(float64(1), span.Metrics["_top_level"], "should have a _top_level:1 flag")
	// SetTopLevel(span, false)
	// assert.Equal(len(span.Metrics), 0, "no meta at all")

	// span.Metrics = map[string]float64{"custom": 42}

	// assert.False(HasTopLevel(span), "still non top-level")
	// SetTopLevel(span, true)
	// assert.Equal(float64(1), span.Metrics["_top_level"], "should have a _top_level:1 flag")
	// assert.Equal(float64(42), span.Metrics["custom"], "former metrics should still be here")
	// assert.True(HasTopLevel(span), "marked as top-level")
	// SetTopLevel(span, false)
	// assert.False(HasTopLevel(span), "non top-level any more")
	// assert.Equal(float64(0), span.Metrics["_top_level"], "should have no _top_level:1 flag")
	// assert.Equal(float64(42), span.Metrics["custom"], "former metrics should still be here")
}

func TestIsMeasured(t *testing.T) {
	// assert := assert.New(t)
	// span := traces.NewEagerSpan(pb.Span{})

	// assert.False(IsMeasured(span), "by default, metrics are not calculated for non top-level spans")

	// span.Metrics = map[string]float64{"_dd.measured": 1}
	// assert.True(IsMeasured(span), "the measured key is present, the span should be measured")

	// span.Metrics = map[string]float64{"_dd.measured": 0}
	// assert.False(IsMeasured(span), "the measured key is present but the value != 1, the span should not be measured")
}
