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

func TestCopyTraceID(t *testing.T) {
	t.Run("64-bit trace ID", func(t *testing.T) {
		src := &pb.Span{
			TraceID: 12345,
		}
		dst := &pb.Span{
			TraceID: 99999,
		}

		CopyTraceID(dst, src)

		assert.Equal(t, uint64(12345), dst.TraceID, "TraceID should be copied")
		assert.Nil(t, dst.Meta, "Meta should remain nil when source has no high bits")
	})

	t.Run("128-bit trace ID", func(t *testing.T) {
		src := &pb.Span{
			TraceID: 12345,
			Meta: map[string]string{
				"_dd.p.tid": "6958127700000000",
			},
		}
		dst := &pb.Span{
			TraceID: 99999,
		}

		CopyTraceID(dst, src)

		assert.Equal(t, uint64(12345), dst.TraceID, "TraceID (low 64 bits) should be copied")
		assert.NotNil(t, dst.Meta, "Meta should be initialized")
		assert.Equal(t, "6958127700000000", dst.Meta["_dd.p.tid"], "High 64 bits should be copied")
	})

	t.Run("128-bit trace ID with existing destination meta", func(t *testing.T) {
		src := &pb.Span{
			TraceID: 12345,
			Meta: map[string]string{
				"_dd.p.tid": "6958127700000000",
			},
		}
		dst := &pb.Span{
			TraceID: 99999,
			Meta: map[string]string{
				"existing": "value",
			},
		}

		CopyTraceID(dst, src)

		assert.Equal(t, uint64(12345), dst.TraceID, "TraceID should be copied")
		assert.Equal(t, "6958127700000000", dst.Meta["_dd.p.tid"], "High 64 bits should be copied")
		assert.Equal(t, "value", dst.Meta["existing"], "Existing meta should be preserved")
	})

	t.Run("Source with other meta fields", func(t *testing.T) {
		src := &pb.Span{
			TraceID: 12345,
			Meta: map[string]string{
				"_dd.p.tid": "6958127700000000",
				"other":     "field",
			},
		}
		dst := &pb.Span{
			TraceID: 99999,
		}

		CopyTraceID(dst, src)

		assert.Equal(t, uint64(12345), dst.TraceID, "TraceID should be copied")
		assert.Equal(t, "6958127700000000", dst.Meta["_dd.p.tid"], "High 64 bits should be copied")
		assert.NotContains(t, dst.Meta, "other", "Other meta fields should not be copied")
	})

	t.Run("Nil source meta", func(t *testing.T) {
		src := &pb.Span{
			TraceID: 12345,
			Meta:    nil,
		}
		dst := &pb.Span{
			TraceID: 99999,
		}

		CopyTraceID(dst, src)

		assert.Equal(t, uint64(12345), dst.TraceID, "TraceID should be copied")
		assert.Nil(t, dst.Meta, "Meta should remain nil when source has nil meta")
	})

	t.Run("Overwrite existing high bits", func(t *testing.T) {
		src := &pb.Span{
			TraceID: 12345,
			Meta: map[string]string{
				"_dd.p.tid": "6958127700000000",
			},
		}
		dst := &pb.Span{
			TraceID: 99999,
			Meta: map[string]string{
				"_dd.p.tid": "0000000000000000",
			},
		}

		CopyTraceID(dst, src)

		assert.Equal(t, uint64(12345), dst.TraceID, "TraceID should be copied")
		assert.Equal(t, "6958127700000000", dst.Meta["_dd.p.tid"], "High 64 bits should be overwritten")
	})
}

func TestSameTraceID(t *testing.T) {
	t.Run("Same 64-bit trace ID", func(t *testing.T) {
		a := &pb.Span{TraceID: 12345}
		b := &pb.Span{TraceID: 12345}
		assert.True(t, SameTraceID(a, b))
	})

	t.Run("Different 64-bit trace ID", func(t *testing.T) {
		a := &pb.Span{TraceID: 12345}
		b := &pb.Span{TraceID: 99999}
		assert.False(t, SameTraceID(a, b))
	})

	t.Run("Same 128-bit trace ID", func(t *testing.T) {
		a := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "6958127700000000"},
		}
		b := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "6958127700000000"},
		}
		assert.True(t, SameTraceID(a, b))
	})

	t.Run("Same low bits, different high bits", func(t *testing.T) {
		a := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "6958127700000000"},
		}
		b := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "1111111111111111"},
		}
		assert.False(t, SameTraceID(a, b))
	})

	t.Run("One has high bits, other doesn't", func(t *testing.T) {
		// Per Datadog documentation, spans with 128-bit trace IDs and spans with
		// 64-bit trace IDs should be treated as matching if the low 64 bits match.
		// https://docs.datadoghq.com/tracing/guide/span_and_trace_id_format/
		a := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "6958127700000000"},
		}
		b := &pb.Span{
			TraceID: 12345,
		}
		assert.True(t, SameTraceID(a, b))
	})

	t.Run("Neither has high bits", func(t *testing.T) {
		a := &pb.Span{TraceID: 12345}
		b := &pb.Span{TraceID: 12345}
		assert.True(t, SameTraceID(a, b))
	})

	t.Run("Other meta fields don't affect comparison", func(t *testing.T) {
		a := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "6958127700000000", "other": "a"},
		}
		b := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "6958127700000000", "other": "b"},
		}
		assert.True(t, SameTraceID(a, b))
	})
}

func TestGetTraceIDHigh(t *testing.T) {
	t.Run("Has high bits", func(t *testing.T) {
		s := &pb.Span{Meta: map[string]string{"_dd.p.tid": "6958127700000000"}}
		val, ok := GetTraceIDHigh(s)
		assert.True(t, ok)
		assert.Equal(t, "6958127700000000", val)
	})

	t.Run("No high bits", func(t *testing.T) {
		s := &pb.Span{}
		val, ok := GetTraceIDHigh(s)
		assert.False(t, ok)
		assert.Equal(t, "", val)
	})

	t.Run("Nil meta", func(t *testing.T) {
		s := &pb.Span{Meta: nil}
		val, ok := GetTraceIDHigh(s)
		assert.False(t, ok)
		assert.Equal(t, "", val)
	})
}

func TestSetTraceIDHigh(t *testing.T) {
	t.Run("Nil meta", func(t *testing.T) {
		s := &pb.Span{}
		SetTraceIDHigh(s, "6958127700000000")
		assert.Equal(t, "6958127700000000", s.Meta["_dd.p.tid"])
	})

	t.Run("Existing meta", func(t *testing.T) {
		s := &pb.Span{Meta: map[string]string{"other": "value"}}
		SetTraceIDHigh(s, "6958127700000000")
		assert.Equal(t, "6958127700000000", s.Meta["_dd.p.tid"])
		assert.Equal(t, "value", s.Meta["other"])
	})

	t.Run("Overwrite existing", func(t *testing.T) {
		s := &pb.Span{Meta: map[string]string{"_dd.p.tid": "1111111111111111"}}
		SetTraceIDHigh(s, "6958127700000000")
		assert.Equal(t, "6958127700000000", s.Meta["_dd.p.tid"])
	})
}

func TestHasTraceIDHigh(t *testing.T) {
	t.Run("Has high bits", func(t *testing.T) {
		s := &pb.Span{Meta: map[string]string{"_dd.p.tid": "6958127700000000"}}
		assert.True(t, HasTraceIDHigh(s))
	})

	t.Run("No high bits", func(t *testing.T) {
		s := &pb.Span{}
		assert.False(t, HasTraceIDHigh(s))
	})
}

func TestUpgradeTraceID(t *testing.T) {
	t.Run("Upgrade 64-bit to 128-bit", func(t *testing.T) {
		dst := &pb.Span{TraceID: 12345}
		src := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "6958127700000000"},
		}
		upgraded := UpgradeTraceID(dst, src)
		assert.True(t, upgraded)
		assert.Equal(t, "6958127700000000", dst.Meta["_dd.p.tid"])
	})

	t.Run("Different trace IDs", func(t *testing.T) {
		dst := &pb.Span{TraceID: 12345}
		src := &pb.Span{
			TraceID: 99999, // Different low 64 bits
			Meta:    map[string]string{"_dd.p.tid": "6958127700000000"},
		}
		upgraded := UpgradeTraceID(dst, src)
		assert.False(t, upgraded)
		assert.Nil(t, dst.Meta) // Should NOT copy high bits from different trace
	})

	t.Run("Already has high bits", func(t *testing.T) {
		dst := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "1111111111111111"},
		}
		src := &pb.Span{
			TraceID: 12345,
			Meta:    map[string]string{"_dd.p.tid": "6958127700000000"},
		}
		upgraded := UpgradeTraceID(dst, src)
		assert.False(t, upgraded)
		assert.Equal(t, "1111111111111111", dst.Meta["_dd.p.tid"]) // Unchanged
	})

	t.Run("Source has no high bits", func(t *testing.T) {
		dst := &pb.Span{TraceID: 12345}
		src := &pb.Span{TraceID: 12345}
		upgraded := UpgradeTraceID(dst, src)
		assert.False(t, upgraded)
		assert.Nil(t, dst.Meta) // Still nil
	})

	t.Run("Both have no high bits", func(t *testing.T) {
		dst := &pb.Span{TraceID: 12345}
		src := &pb.Span{TraceID: 12345}
		upgraded := UpgradeTraceID(dst, src)
		assert.False(t, upgraded)
	})
}
