// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"math"
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
)

func fixedSpan() *pb.Span {
	return &pb.Span{
		Duration: 10000000,
		Error:    0,
		Resource: "GET /some/raclette",
		Service:  "django",
		Name:     "django.controller",
		SpanID:   42,
		Start:    1448466874000000000,
		TraceID:  424242,
		Meta: map[string]string{
			"user": "leo",
			"pool": "fondue",
		},
		Metrics: map[string]float64{
			"cheese_weight": 100000.0,
		},
		ParentID: 1111,
		Type:     "http",
	}
}

func TestSpanString(t *testing.T) {
	assert := assert.New(t)
	assert.NotEqual("", fixedSpan().String())
}

func TestSpanWeight(t *testing.T) {
	assert := assert.New(t)

	span := fixedSpan()
	assert.Equal(1., weight(span))

	span.Metrics[keySamplingRateGlobal] = -1.0
	assert.Equal(1., weight(span))

	span.Metrics[keySamplingRateGlobal] = 0.0
	assert.Equal(1., weight(span))

	span.Metrics[keySamplingRateGlobal] = 0.25
	assert.Equal(4., weight(span))

	span.Metrics[keySamplingRateGlobal] = 1.0
	assert.Equal(1., weight(span))

	span.Metrics[keySamplingRateGlobal] = 1.5
	assert.Equal(1., weight(span))
}

var sampleRateWeightCases = []struct {
	name string
	rate float64
	want float64
}{
	{"quarter", 0.25, 4},
	{"one", 1, 1},
	{"tiny rate", 0x1p-1000, 0x1p1000},
	{"negative", -1, 1},
	{"zero", 0, 1},
	{"above one", 1.5, 1},
	{"NaN", math.NaN(), 1},
	{"positive infinity", math.Inf(1), 1},
	{"negative infinity", math.Inf(-1), 1},
	{"subnormal rate with infinite inverse", math.SmallestNonzeroFloat64, 1},
}

func TestSpanWeightRejectsNonFiniteRates(t *testing.T) {
	for _, tc := range sampleRateWeightCases {
		t.Run(tc.name, func(t *testing.T) {
			span := fixedSpan()
			span.Metrics[keySamplingRateGlobal] = tc.rate
			assert.Equal(t, tc.want, weight(span))
		})
	}
}

func TestSpanWeightV1RejectsNonFiniteRates(t *testing.T) {
	for _, tc := range sampleRateWeightCases {
		t.Run(tc.name, func(t *testing.T) {
			strings := idx.NewStringTable()
			span := idx.NewInternalSpan(strings, &idx.Span{
				ServiceRef: strings.Add("django"),
				NameRef:    strings.Add("django.controller"),
				Attributes: map[uint32]*idx.AnyValue{
					strings.Add(keySamplingRateGlobal): {Value: &idx.AnyValue_DoubleValue{DoubleValue: tc.rate}},
				},
			})
			assert.Equal(t, tc.want, weightV1(span))
		})
	}
}

func TestSpanWeightNil(t *testing.T) {
	assert := assert.New(t)

	var span *pb.Span

	assert.Equal(1., weight(span), "Weight should be callable on nil and return a default value")
}
