// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package stats

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
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

func TestSpanWeightNil(t *testing.T) {
	assert := assert.New(t)

	var span *pb.Span

	assert.Equal(1., weight(span), "Weight should be callable on nil and return a default value")
}
