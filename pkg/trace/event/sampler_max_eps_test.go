// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func TestMaxEPSSampler(t *testing.T) {
	for _, testCase := range []struct {
		name               string
		events             []*pb.Span
		maxEPS             float64
		pastEPS            float64
		expectedSampleRate float64
		deltaPct           float64
	}{
		{"low", generateTestEvents(1000), 100, 50, 1., 0},
		{"limit", generateTestEvents(1000), 100, 100, 1., 0},
		{"overload", generateTestEvents(1000), 100, 150, 100. / 150., 0.2},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			assert := assert.New(t)

			counter := &MockRateCounter{
				GetRateResult: testCase.pastEPS,
			}
			testSampler := newMaxEPSSampler(testCase.maxEPS)
			testSampler.rateCounter = counter
			testSampler.Start()

			sampled := 0
			for _, event := range testCase.events {
				sample, rate := testSampler.Sample(event)
				if sample {
					sampled++
				}
				assert.EqualValues(testCase.expectedSampleRate, rate)
			}

			testSampler.Stop()

			assert.InDelta(testCase.expectedSampleRate, float64(sampled)/float64(len(testCase.events)), testCase.expectedSampleRate*testCase.deltaPct)
		})
	}
}

func generateTestEvents(numEvents int) []*pb.Span {
	testEvents := make([]*pb.Span, numEvents)
	for i := range testEvents {
		testEvents[i] = testutil.RandomSpan()
	}
	return testEvents
}

type MockRateCounter struct {
	CountCalls    int
	GetRateCalls  int
	GetRateResult float64
}

func (mc *MockRateCounter) Start() {}
func (mc *MockRateCounter) Stop()  {}

func (mc *MockRateCounter) Count() {
	mc.CountCalls++
}

func (mc *MockRateCounter) GetRate() float64 {
	mc.GetRateCalls++
	return mc.GetRateResult
}
