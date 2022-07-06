// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"math/rand"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/client/products/apmsampling"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/stretchr/testify/assert"
)

func createSpanSamplingTestSpans(serviceName, operationName string, total int, mech apmsampling.SamplingMechanism, rate, limitRate float64) []*pb.Span {
	spans := make([]*pb.Span, total)
	for i := range spans {
		spans[i] = &pb.Span{TraceID: rand.Uint64(), Service: serviceName, Name: operationName, Metrics: map[string]float64{}}
		setSingleSpanTags(spans[i], mech, rate, limitRate)
	}
	return spans
}

func setSingleSpanTags(span *pb.Span, mech apmsampling.SamplingMechanism, rate, mps float64) {
	if span.Metrics == nil {
		span.Metrics = map[string]float64{}
	}
	span.Metrics[sampler.KeySpanSamplingMechanism] = float64(mech)
	span.Metrics[sampler.KeySpanSamplingRuleRate] = rate
	span.Metrics[sampler.KeySpanSamplingMPS] = mps
}

func TestSingleSpanExtractor(t *testing.T) {
	for _, test := range []struct {
		spans []*pb.Span
		rate  float64
		mech  apmsampling.SamplingMechanism
		out   float64
	}{
		// Name: <mechanism>/<rate>/<expected rate/>)
		{rate: 1, mech: 0, out: 0},
		{rate: -1, mech: 0, out: 0},
		{rate: -1, mech: apmsampling.SamplingMechanismSingleSpan, out: 0},
		{rate: 0, mech: apmsampling.SamplingMechanismSingleSpan, out: 0},
		{rate: 0.5, mech: apmsampling.SamplingMechanismSingleSpan, out: 1},
	} {
		t.Run("", func(t *testing.T) {
			spans := createSpanSamplingTestSpans("srv", "ops", 100, test.mech, test.rate, 100)
			test.spans = spans
			assert := assert.New(t)
			extracted := 0
			extractor := NewSingleSpanExtractor()
			for _, span := range test.spans {
				rate, ok := extractor.Extract(span, 0)
				if !ok {
					continue
				}
				extracted++
				assert.Equal(test.out, rate)
			}
		})
	}
}
