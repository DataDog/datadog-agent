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
	assert.Equal(t, uint32(2), convertedFields.SamplingPriority)
}
