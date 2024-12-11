// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"math/rand"
	"testing"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
)

func createTestSpansWithEventRate(eventRate float64) []*pb.Span {
	spans := make([]*pb.Span, 1000)
	for i := range spans {
		spans[i] = &pb.Span{TraceID: rand.Uint64(), Service: "test", Name: "test", Metrics: map[string]float64{}}
		if eventRate >= 0 {
			spans[i].Metrics[sampler.KeySamplingRateEventExtraction] = eventRate
		}
	}
	return spans
}

func TestMetricBasedExtractor(t *testing.T) {
	tests := []extractorTestCase{
		// Name: <priority>/<extraction rate>
		{"none/missing", createTestSpansWithEventRate(-1), 0, -1},
		{"none/0", createTestSpansWithEventRate(0), 0, 0},
		{"none/0.5", createTestSpansWithEventRate(0.5), 0, 0.5},
		{"none/1", createTestSpansWithEventRate(1), 0, 1},
		{"1/missing", createTestSpansWithEventRate(-1), 1, -1},
		{"1/0", createTestSpansWithEventRate(0), 1, 0},
		{"1/0.5", createTestSpansWithEventRate(0.5), 1, 0.5},
		{"1/1", createTestSpansWithEventRate(1), 1, 1},
		// Priority 2 should have extraction rate of 1 so long as any extraction rate is set and > 0
		{"2/missing", createTestSpansWithEventRate(-1), 2, -1},
		{"2/0", createTestSpansWithEventRate(0), 2, 0},
		{"2/0.5", createTestSpansWithEventRate(0.5), 2, 1},
		{"2/1", createTestSpansWithEventRate(1), 2, 1},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			testExtractor(t, NewMetricBasedExtractor(), test)
		})
	}
}
