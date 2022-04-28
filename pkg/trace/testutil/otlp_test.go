package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.opentelemetry.io/collector/pdata/ptrace"
)

func TestNewOTLPSpan(t *testing.T) {
	t.Run("bare", func(t *testing.T) {
		span := NewOTLPSpan(&OTLPSpan{})
		assert := assert.New(t)
		assert.Equal(OTLPFixedTraceID, span.TraceID())
		assert.Equal(OTLPFixedSpanID, span.SpanID())
		assert.NotZero(span.StartTimestamp())
		assert.NotZero(span.EndTimestamp())
		assert.True(span.EndTimestamp() > span.StartTimestamp())
	})

	t.Run("common", func(t *testing.T) {
		testSpanID := [8]byte{1, 2, 3, 4, 5, 6, 7, 8}
		testParentID := [8]byte{1, 2, 3, 4, 5, 6, 7, 9}
		testTraceID := [16]byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16}
		span := NewOTLPSpan(&OTLPSpan{
			TraceID:    testTraceID,
			SpanID:     testSpanID,
			TraceState: "state",
			ParentID:   testParentID,
			Name:       "name",
			Kind:       ptrace.SpanKindInternal,
			Start:      11,
			End:        55,
			Attributes: map[string]interface{}{
				"A": "B",
				"C": 1,
			},
			Events: []OTLPSpanEvent{
				{
					Timestamp:  66,
					Name:       "event-name",
					Attributes: map[string]interface{}{"u": "v"},
					Dropped:    4,
				},
				{
					Timestamp:  67,
					Name:       "event-name2",
					Attributes: map[string]interface{}{"i": "b"},
					Dropped:    1,
				},
			},
			StatusMsg:  "status-msg",
			StatusCode: ptrace.StatusCodeOk,
		})
		assert := assert.New(t)
		assert.Equal(testTraceID, span.TraceID().Bytes())
		assert.Equal(testSpanID, span.SpanID().Bytes())
		assert.Equal("state", string(span.TraceState()))
		assert.Equal(testParentID, span.ParentSpanID().Bytes())
		assert.Equal("name", span.Name())
		assert.Equal(ptrace.SpanKindInternal, span.Kind())
		assert.Equal(uint64(11), uint64(span.StartTimestamp()))
		assert.Equal(uint64(55), uint64(span.EndTimestamp()))
		v, ok := span.Attributes().Get("A")
		assert.True(ok)
		assert.Equal("B", v.StringVal())
		v, ok = span.Attributes().Get("C")
		assert.True(ok)
		assert.Equal(int64(1), v.IntVal())
		assert.Equal(2, span.Events().Len())
		assert.Equal(uint64(66), uint64(span.Events().At(0).Timestamp()))
		assert.Equal("event-name", span.Events().At(0).Name())
		v, ok = span.Events().At(0).Attributes().Get("u")
		assert.True(ok)
		assert.Equal("v", v.StringVal())
		assert.Equal(uint64(67), uint64(span.Events().At(1).Timestamp()))
		assert.Equal("event-name2", span.Events().At(1).Name())
		v, ok = span.Events().At(1).Attributes().Get("i")
		assert.True(ok)
		assert.Equal("b", v.StringVal())
		assert.Equal(uint32(1), span.Events().At(1).DroppedAttributesCount())
		assert.Equal("status-msg", span.Status().Message())
		assert.Equal(ptrace.StatusCodeOk, span.Status().Code())
	})
}
