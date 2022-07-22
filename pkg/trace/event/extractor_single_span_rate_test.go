// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/stretchr/testify/assert"
)

func createSpanSamplingTestSpans(serviceName, operationName string, total int, mech float64) []*pb.Span {
	spans := make([]*pb.Span, total)
	for i := range spans {
		spans[i] = &pb.Span{TraceID: rand.Uint64(), Service: serviceName, Name: operationName, Metrics: map[string]float64{}}
		spans[i].Metrics[sampler.KeySpanSamplingMechanism] = mech
	}
	return spans
}

func TestSingleSpanExtractor(t *testing.T) {
	for _, test := range []struct {
		span *pb.Span
		rate float64
		ok   bool
	}{
		// Name: <mechanism>/<rate>/<expected rate/>)
		{span: &pb.Span{TraceID: rand.Uint64(), Service: "serviceName", Name: "operationName"}, rate: 0, ok: false},
		{span: &pb.Span{TraceID: rand.Uint64(), Service: "serviceName", Name: "operationName",
			Metrics: map[string]float64{sampler.KeySpanSamplingMechanism: 8}}, rate: 1, ok: true},
	} {
		extractor := NewSingleSpanExtractor()
		rate, ok := extractor.Extract(test.span, 0)
		assert.Equal(t, test.rate, rate)
		assert.Equal(t, test.ok, ok)
	}
}
