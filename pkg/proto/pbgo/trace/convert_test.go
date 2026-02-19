// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package trace

import (
	"testing"

	idx "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/stretchr/testify/assert"
)

func TestConvertedSpan(t *testing.T) {
	v4Span := &Span{
		Service:  "my-service",
		Name:     "span-name",
		Resource: "GET /res",
		SpanID:   12345678,
		ParentID: 1111,
		Duration: 234,
		Start:    171615,
		Metrics: map[string]float64{
			"someNum":               1.0,
			"_sampling_priority_v1": 2.0,
		},
		Meta: map[string]string{
			"someStr":            "bar",
			"_dd.p.tid":          "BABA",
			"env":                "production",
			"version":            "1.2.3",
			"component":          "http-client",
			"span.kind":          "client",
			"_dd.git.commit.sha": "abc123def456",
			"_dd.p.dm":           "-1",
			"_dd.hostname":       "my-hostname",
		},
		MetaStruct: map[string][]byte{
			"bts": []byte("bar"),
		},
		TraceID: 556677,
	}
	v4SpanBytes, err := v4Span.MarshalMsg(nil)
	assert.NoError(t, err)
	idxSpan := idx.NewInternalSpan(idx.NewStringTable(), &idx.Span{})
	convertedFields := idx.SpanConvertedFields{}
	o, err := idxSpan.UnmarshalMsgConverted(v4SpanBytes, &convertedFields)
	assert.NoError(t, err)
	assert.Empty(t, o)
	assert.Equal(t, "my-service", idxSpan.Service())
	assert.Equal(t, "span-name", idxSpan.Name())
	assert.Equal(t, "GET /res", idxSpan.Resource())
	assert.Equal(t, uint64(12345678), idxSpan.SpanID())
	assert.Equal(t, uint64(1111), idxSpan.ParentID())
	assert.Equal(t, uint64(234), idxSpan.Duration())
	assert.Equal(t, uint64(171615), idxSpan.Start())
	someNum, found := idxSpan.GetAttributeAsFloat64("someNum")
	assert.True(t, found)
	assert.Equal(t, float64(1.0), someNum)
	someStr, found := idxSpan.GetAttributeAsString("someStr")
	assert.True(t, found)
	assert.Equal(t, "bar", someStr)
	anyValue, found := idxSpan.GetAttribute("bts")
	assert.True(t, found)
	assert.Equal(t, &idx.AnyValue{
		Value: &idx.AnyValue_BytesValue{
			BytesValue: []byte("bar"),
		},
	}, anyValue)
	kindAttr, found := idxSpan.GetAttributeAsString("span.kind")
	assert.True(t, found)
	assert.Equal(t, "client", kindAttr)
	convertedV1, found := idxSpan.GetAttributeAsString("_dd.convertedv1")
	assert.True(t, found)
	assert.Equal(t, "v04", convertedV1)
	assert.Equal(t, "production", idxSpan.Env())
	assert.Equal(t, "1.2.3", idxSpan.Version())
	assert.Equal(t, "http-client", idxSpan.Component())
	assert.Equal(t, idx.SpanKind_SPAN_KIND_CLIENT, idxSpan.Kind())

	// Check for converted fields
	assert.Equal(t, uint64(556677), convertedFields.TraceIDLower)
	assert.Equal(t, uint64(0xBABA), convertedFields.TraceIDUpper)
	assert.Equal(t, "abc123def456", idxSpan.Strings.Get(convertedFields.GitCommitShaRef))
	assert.Equal(t, uint32(1), convertedFields.SamplingMechanism)
	assert.Equal(t, "my-hostname", idxSpan.Strings.Get(convertedFields.HostnameRef))
	assert.Equal(t, int32(2), convertedFields.SamplingPriority)
}

func TestConvertedSpanLinks(t *testing.T) {
	v4Span := &Span{
		SpanLinks: []*SpanLink{
			{
				TraceID:     0xAA_BB,
				TraceIDHigh: 0x12_34,
				SpanID:      1111,
				Tracestate:  "test=state",
				Flags:       1,
				Attributes: map[string]string{
					"link.attr": "link.value",
				},
			},
		},
	}
	v4SpanBytes, err := v4Span.MarshalMsg(nil)
	assert.NoError(t, err)
	idxSpan := idx.NewInternalSpan(idx.NewStringTable(), &idx.Span{})
	convertedFields := idx.SpanConvertedFields{}
	o, err := idxSpan.UnmarshalMsgConverted(v4SpanBytes, &convertedFields)
	assert.NoError(t, err)
	assert.Empty(t, o)
	assert.Len(t, idxSpan.Links(), 1)
	actualLink := idxSpan.Links()[0]
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x12, 0x34, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xaa, 0xbb}, actualLink.TraceID())
	assert.Equal(t, uint64(1111), actualLink.SpanID())
	assert.Equal(t, "test=state", actualLink.Tracestate())
	assert.Equal(t, uint32(1), actualLink.Flags())
	assert.Len(t, actualLink.Attributes(), 1)
	linkAttr, found := actualLink.GetAttributeAsString("link.attr")
	assert.True(t, found)
	assert.Equal(t, "link.value", linkAttr)
}

func TestConvertedSpanEvents(t *testing.T) {
	v4Span := &Span{
		SpanEvents: []*SpanEvent{
			{
				TimeUnixNano: 171615,
				Name:         "span-name",
				Attributes: map[string]*AttributeAnyValue{
					"event.attr": {
						Type:        AttributeAnyValue_STRING_VALUE,
						StringValue: "event.value",
					},
					"event.bool": {
						Type:      AttributeAnyValue_BOOL_VALUE,
						BoolValue: true,
					},
					"event.int": {
						Type:     AttributeAnyValue_INT_VALUE,
						IntValue: 42,
					},
					"event.double": {
						Type:        AttributeAnyValue_DOUBLE_VALUE,
						DoubleValue: 3.14,
					},
					"event.array": {
						Type: AttributeAnyValue_ARRAY_VALUE,
						ArrayValue: &AttributeArray{
							Values: []*AttributeArrayValue{
								{Type: AttributeArrayValue_STRING_VALUE, StringValue: "array.value"},
								{Type: AttributeArrayValue_INT_VALUE, IntValue: 100},
								{Type: AttributeArrayValue_BOOL_VALUE, BoolValue: true},
								{Type: AttributeArrayValue_DOUBLE_VALUE, DoubleValue: 3.14},
							},
						},
					},
				},
			},
		},
	}
	v4SpanBytes, err := v4Span.MarshalMsg(nil)
	assert.NoError(t, err)
	idxSpan := idx.NewInternalSpan(idx.NewStringTable(), &idx.Span{})
	convertedFields := idx.SpanConvertedFields{}
	o, err := idxSpan.UnmarshalMsgConverted(v4SpanBytes, &convertedFields)
	assert.NoError(t, err)
	assert.Empty(t, o)
	assert.Len(t, idxSpan.Events(), 1)
	actualEvent := idxSpan.Events()[0]
	assert.Equal(t, uint64(171615), actualEvent.Time())
	assert.Equal(t, "span-name", actualEvent.Name())
	assert.Len(t, actualEvent.Attributes(), len(v4Span.SpanEvents[0].Attributes))
	eventAttr, found := actualEvent.GetAttributeAsString("event.attr")
	assert.True(t, found)
	assert.Equal(t, "event.value", eventAttr)
	eventBool, found := actualEvent.GetAttribute("event.bool")
	assert.True(t, found)
	assert.Equal(t, true, eventBool.Value.(*idx.AnyValue_BoolValue).BoolValue)
	eventInt, found := actualEvent.GetAttribute("event.int")
	assert.True(t, found)
	assert.Equal(t, int64(42), eventInt.Value.(*idx.AnyValue_IntValue).IntValue)
	eventDouble, found := actualEvent.GetAttribute("event.double")
	assert.True(t, found)
	assert.Equal(t, float64(3.14), eventDouble.Value.(*idx.AnyValue_DoubleValue).DoubleValue)

	eventArrayAttr, found := actualEvent.GetAttribute("event.array")
	assert.True(t, found)
	attrArray := eventArrayAttr.Value.(*idx.AnyValue_ArrayValue).ArrayValue.Values
	assert.Len(t, attrArray, 4)
	assert.Equal(t, "array.value", idxSpan.Strings.Get(attrArray[0].Value.(*idx.AnyValue_StringValueRef).StringValueRef))
	assert.Equal(t, int64(100), attrArray[1].Value.(*idx.AnyValue_IntValue).IntValue)
	assert.Equal(t, true, attrArray[2].Value.(*idx.AnyValue_BoolValue).BoolValue)
	assert.Equal(t, float64(3.14), attrArray[3].Value.(*idx.AnyValue_DoubleValue).DoubleValue)
}

func TestConvertedTraceChunk(t *testing.T) {
	trace := Trace([]*Span{
		{
			Service:  "my-service",
			Name:     "span-name",
			Resource: "GET /res",
			SpanID:   12345678,
			ParentID: 1111,
			Duration: 234,
			Start:    171615,
			Metrics: map[string]float64{
				"someNum":               1.0,
				"_sampling_priority_v1": 2.0,
			},
			Meta: map[string]string{
				"someStr":            "bar",
				"_dd.p.tid":          "BABA",
				"env":                "production",
				"version":            "1.2.3",
				"component":          "http-client",
				"span.kind":          "client",
				"_dd.git.commit.sha": "abc123def456",
				"_dd.p.dm":           "-1",
				"_dd.hostname":       "my-hostname",
			},
			MetaStruct: map[string][]byte{
				"bts": []byte("bar"),
			},
			TraceID: 0xAA,
		},
		{
			Service:  "my-service2",
			Name:     "span-name2",
			Resource: "GET /res2",
			SpanID:   12345678,
			ParentID: 1111,
			Duration: 234,
			Start:    171615,
			Metrics: map[string]float64{
				"someNum":               1.0,
				"_sampling_priority_v1": 2.0,
			},
			Meta: map[string]string{
				"someStr":            "bar",
				"_dd.p.tid":          "BABA",
				"env":                "production",
				"version":            "1.2.3",
				"component":          "http-client",
				"span.kind":          "client",
				"_dd.git.commit.sha": "abc123def456",
				"_dd.p.dm":           "-1",
				"_dd.hostname":       "my-hostname",
				"_dd.origin":         "lambda",
			},
			MetaStruct: map[string][]byte{
				"bts": []byte("bar"),
			},
			TraceID: 0xAA,
		},
	})
	traceBytes, err := trace.MarshalMsg(nil)
	assert.NoError(t, err)
	st := idx.NewStringTable()
	chunk := idx.InternalTraceChunk{Strings: st}
	chunkConvertedFields := idx.ChunkConvertedFields{}
	chunk.UnmarshalMsgConverted(traceBytes, &chunkConvertedFields)
	assert.NoError(t, err)
	assert.Len(t, chunk.Spans, 2)
	assert.Equal(t, []byte{0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xba, 0xba, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0x0, 0xaa}, chunk.TraceID)
	assert.Equal(t, uint32(1), chunk.SamplingMechanism())
	assert.Equal(t, int32(2), chunk.Priority)
	assert.Equal(t, "lambda", chunk.Origin())
}

func TestConvertedTracePayload(t *testing.T) {
	traces := Traces([]Trace{
		Trace([]*Span{
			{
				Service:  "my-service",
				Name:     "span-name",
				Resource: "GET /res",
				SpanID:   12345678,
				ParentID: 1111,
				Duration: 234,
				Start:    171615,
				Metrics: map[string]float64{
					"someNum":               1.0,
					"_sampling_priority_v1": 2.0,
				},
				Meta: map[string]string{
					"someStr":            "bar",
					"_dd.p.tid":          "BABA",
					"env":                "production",
					"version":            "1.2.3",
					"component":          "http-client",
					"span.kind":          "client",
					"_dd.git.commit.sha": "abc123def456",
					"_dd.p.dm":           "-1",
				},
				MetaStruct: map[string][]byte{
					"bts": []byte("bar"),
				},
				TraceID: 556677,
			},
			{
				Service:  "my-service2",
				Name:     "span-name2",
				Resource: "GET /res2",
				SpanID:   12345678,
				ParentID: 1111,
				Duration: 234,
				Start:    171615,
				Metrics: map[string]float64{
					"someNum":               1.0,
					"_sampling_priority_v1": 2.0,
				},
				Meta: map[string]string{
					"someStr":            "bar",
					"_dd.p.tid":          "BABA",
					"env":                "production",
					"component":          "http-client",
					"span.kind":          "client",
					"_dd.git.commit.sha": "abc123def456",
					"_dd.p.dm":           "-1",
					"_dd.origin":         "lambda",
					"_dd.apm_mode":       "edge",
				},
				MetaStruct: map[string][]byte{
					"bts": []byte("bar"),
				},
				TraceID: 556677,
			},
		}),
		Trace([]*Span{
			{
				Service:  "my-service",
				Name:     "span-name",
				Resource: "GET /res",
				SpanID:   12345678,
				ParentID: 1111,
				Duration: 234,
				Start:    171615,
				Metrics: map[string]float64{
					"someNum":               1.0,
					"_sampling_priority_v1": 2.0,
				},
				Meta: map[string]string{
					"someStr":            "bar",
					"_dd.p.tid":          "BABA",
					"env":                "production",
					"component":          "http-client",
					"span.kind":          "client",
					"_dd.git.commit.sha": "abc123def456",
					"_dd.p.dm":           "-1",
					"_dd.hostname":       "my-hostname",
				},
				MetaStruct: map[string][]byte{
					"bts": []byte("bar"),
				},
				TraceID: 5566881,
			},
		})})
	tracesBytes, err := traces.MarshalMsg(nil)
	assert.NoError(t, err)
	st := idx.NewStringTable()
	tp := &idx.InternalTracerPayload{Strings: st}
	tp.UnmarshalMsgConverted(tracesBytes)
	assert.NoError(t, err)
	assert.Len(t, tp.Chunks, 2)
	assert.Equal(t, "production", tp.Env())
	assert.Equal(t, "my-hostname", tp.Hostname())
	assert.Equal(t, "1.2.3", tp.AppVersion())
	gitCommitSha, found := tp.GetAttributeAsString("_dd.git.commit.sha")
	assert.True(t, found)
	assert.Equal(t, "abc123def456", gitCommitSha)
	apmMode, found := tp.GetAttributeAsString("_dd.apm_mode")
	assert.True(t, found)
	assert.Equal(t, "edge", apmMode)
}
