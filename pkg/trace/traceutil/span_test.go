// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package traceutil

import (
	"math/rand"
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"

	"github.com/stretchr/testify/assert"
)

func TestTopLevelTypical(t *testing.T) {
	assert := assert.New(t)

	tr := pb.Trace{
		&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web"},
		&pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "mcnulty", Type: "sql"},
		&pb.Span{TraceID: 1, SpanID: 3, ParentID: 2, Service: "master-db", Type: "sql"},
		&pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Service: "redis", Type: "redis"},
		&pb.Span{TraceID: 1, SpanID: 5, ParentID: 1, Service: "mcnulty", Type: ""},
	}

	ComputeTopLevel(tr)

	assert.True(HasTopLevel(tr[0]), "root span should be top-level")
	assert.False(HasTopLevel(tr[1]), "main service, and not a root span, not top-level")
	assert.True(HasTopLevel(tr[2]), "only 1 span for this service, should be top-level")
	assert.True(HasTopLevel(tr[3]), "only 1 span for this service, should be top-level")
	assert.False(HasTopLevel(tr[4]), "yet another sup span, not top-level")
}

func TestSetMeta(t *testing.T) {
	for _, s := range []*pb.Span{
		{},
		{Meta: map[string]string{"A": "B", "C": "D"}},
	} {
		SetMeta(s, "X", "Y")
		assert.NotNil(t, s.Meta)
		assert.Equal(t, s.Meta["X"], "Y")
	}
}

func TestGetSetMetaStruct(t *testing.T) {
	for _, s := range []*pb.Span{
		{},
		{MetaStruct: map[string][]byte{"A": []byte(``)}},
	} {
		assert.Nil(t, SetMetaStruct(s, "Z", []int{1, 2, 3}))
		assert.NotNil(t, s.MetaStruct)
		assert.Equal(t, []byte{0x93, 0x1, 0x2, 0x3}, s.MetaStruct["Z"])
		val, ok := GetMetaStruct(s, "Z")
		assert.True(t, ok)
		assert.Equal(t, []interface{}{int64(1), int64(2), int64(3)}, val)
		assert.NotNil(t, SetMetaStruct(s, "cannot-marshal", struct{}{}))
		_, ok = GetMetaStruct(s, "cannot-marshal")
		assert.False(t, ok)
	}
}

func TestTopLevelSingle(t *testing.T) {
	assert := assert.New(t)

	tr := pb.Trace{
		&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web"},
	}

	ComputeTopLevel(tr)

	assert.True(HasTopLevel(tr[0]), "root span should be top-level")
}

func TestTopLevelEmpty(t *testing.T) {
	assert := assert.New(t)

	tr := pb.Trace{}

	ComputeTopLevel(tr)

	assert.Equal(0, len(tr), "trace should still be empty")
}

func TestTopLevelOneService(t *testing.T) {
	assert := assert.New(t)

	tr := pb.Trace{
		&pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "mcnulty", Type: "web"},
		&pb.Span{TraceID: 1, SpanID: 3, ParentID: 2, Service: "mcnulty", Type: "web"},
		&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web"},
		&pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Service: "mcnulty", Type: "web"},
		&pb.Span{TraceID: 1, SpanID: 5, ParentID: 1, Service: "mcnulty", Type: "web"},
	}

	ComputeTopLevel(tr)

	assert.False(HasTopLevel(tr[0]), "just a sub-span, not top-level")
	assert.False(HasTopLevel(tr[1]), "just a sub-span, not top-level")
	assert.True(HasTopLevel(tr[2]), "root span should be top-level")
	assert.False(HasTopLevel(tr[3]), "just a sub-span, not top-level")
	assert.False(HasTopLevel(tr[4]), "just a sub-span, not top-level")
}

func TestTopLevelLocalRoot(t *testing.T) {
	assert := assert.New(t)

	tr := pb.Trace{
		&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web"},
		&pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "mcnulty", Type: "sql"},
		&pb.Span{TraceID: 1, SpanID: 3, ParentID: 2, Service: "master-db", Type: "sql"},
		&pb.Span{TraceID: 1, SpanID: 4, ParentID: 1, Service: "redis", Type: "redis"},
		&pb.Span{TraceID: 1, SpanID: 5, ParentID: 1, Service: "mcnulty", Type: ""},
		&pb.Span{TraceID: 1, SpanID: 6, ParentID: 4, Service: "redis", Type: "redis"},
		&pb.Span{TraceID: 1, SpanID: 7, ParentID: 4, Service: "redis", Type: "redis"},
	}

	ComputeTopLevel(tr)

	assert.True(HasTopLevel(tr[0]), "root span should be top-level")
	assert.False(HasTopLevel(tr[1]), "main service, and not a root span, not top-level")
	assert.True(HasTopLevel(tr[2]), "only 1 span for this service, should be top-level")
	assert.True(HasTopLevel(tr[3]), "top-level but not root")
	assert.False(HasTopLevel(tr[4]), "yet another sup span, not top-level")
	assert.False(HasTopLevel(tr[5]), "yet another sup span, not top-level")
	assert.False(HasTopLevel(tr[6]), "yet another sup span, not top-level")
}

func TestTopLevelWithTag(t *testing.T) {
	assert := assert.New(t)

	tr := pb.Trace{
		&pb.Span{TraceID: 1, SpanID: 1, ParentID: 0, Service: "mcnulty", Type: "web", Metrics: map[string]float64{"custom": 42}},
		&pb.Span{TraceID: 1, SpanID: 2, ParentID: 1, Service: "mcnulty", Type: "web", Metrics: map[string]float64{"custom": 42}},
	}

	ComputeTopLevel(tr)

	t.Logf("%v\n", tr[1].Metrics)

	assert.True(HasTopLevel(tr[0]), "root span should be top-level")
	assert.Equal(float64(42), tr[0].Metrics["custom"], "custom metric should still be here")
	assert.False(HasTopLevel(tr[1]), "not a top-level span")
	assert.Equal(float64(42), tr[1].Metrics["custom"], "custom metric should still be here")
}

func TestTopLevelGetSetBlackBox(t *testing.T) {
	assert := assert.New(t)

	span := &pb.Span{}

	assert.False(HasTopLevel(span), "by default, all spans are considered non top-level")
	SetTopLevel(span, true)
	assert.True(HasTopLevel(span), "marked as top-level")
	SetTopLevel(span, false)
	assert.False(HasTopLevel(span), "no more top-level")

	span.Metrics = map[string]float64{"custom": 42}

	assert.False(HasTopLevel(span), "by default, all spans are considered non top-level")
	SetTopLevel(span, true)
	assert.True(HasTopLevel(span), "marked as top-level")
	SetTopLevel(span, false)
	assert.False(HasTopLevel(span), "no more top-level")
}

func TestTopLevelGetSetMetrics(t *testing.T) {
	assert := assert.New(t)

	span := &pb.Span{}

	assert.Nil(span.Metrics, "no meta at all")
	SetTopLevel(span, true)
	assert.Equal(float64(1), span.Metrics["_top_level"], "should have a _top_level:1 flag")
	SetTopLevel(span, false)
	assert.Equal(len(span.Metrics), 0, "no meta at all")

	span.Metrics = map[string]float64{"custom": 42}

	assert.False(HasTopLevel(span), "still non top-level")
	SetTopLevel(span, true)
	assert.Equal(float64(1), span.Metrics["_top_level"], "should have a _top_level:1 flag")
	assert.Equal(float64(42), span.Metrics["custom"], "former metrics should still be here")
	assert.True(HasTopLevel(span), "marked as top-level")
	SetTopLevel(span, false)
	assert.False(HasTopLevel(span), "non top-level any more")
	assert.Equal(float64(0), span.Metrics["_top_level"], "should have no _top_level:1 flag")
	assert.Equal(float64(42), span.Metrics["custom"], "former metrics should still be here")
}

func TestIsMeasured(t *testing.T) {
	assert := assert.New(t)
	span := &pb.Span{}

	assert.False(IsMeasured(span), "by default, metrics are not calculated for non top-level spans")

	span.Metrics = map[string]float64{"_dd.measured": 1}
	assert.True(IsMeasured(span), "the measured key is present, the span should be measured")

	span.Metrics = map[string]float64{"_dd.measured": 0}
	assert.False(IsMeasured(span), "the measured key is present but the value != 1, the span should not be measured")
}

func TestIsPartialSnapshot(t *testing.T) {
	assert := assert.New(t)
	span := &pb.Span{}

	assert.False(IsPartialSnapshot(span), "by default, a span is considered as complete")

	span.Metrics = map[string]float64{"_dd.partial_version": -10}
	assert.False(IsPartialSnapshot(span), "Negative versions do not mark the span as incomplete")

	span.Metrics = map[string]float64{"_dd.partial_version": float64(rand.Uint32())}
	assert.True(IsPartialSnapshot(span), "Any value in partialVersion key will mark the span as incomplete")
}
