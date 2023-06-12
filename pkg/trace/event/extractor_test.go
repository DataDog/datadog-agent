// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

type extractorTestCase struct {
	name     string
	spans    []*pb.Span
	priority sampler.SamplingPriority
	out      float64
}

func testExtractor(t *testing.T, extractor Extractor, testCase extractorTestCase) {
	t.Run(testCase.name, func(t *testing.T) {
		assert := assert.New(t)
		for _, span := range testCase.spans {
			rate, ok := extractor.Extract(span, testCase.priority)
			if !ok {
				rate = -1
			}
			assert.EqualValues(testCase.out, rate)
		}
	})
}
