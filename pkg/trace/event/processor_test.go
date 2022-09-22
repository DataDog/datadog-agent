// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/trace/pb"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
)

func TestProcessor(t *testing.T) {
	tests := []struct {
		name                 string
		extractorRates       []float64
		samplerRate          float64
		priority             sampler.SamplingPriority
		expectedExtractedPct float64
		expectedSampledPct   float64
		deltaPct             float64
		droppedTrace         bool
	}{
		// droppedTrace - true
		// Name: <extraction rates>/<maxEPSSampler rate>/<priority>
		{"none/1/none", nil, 1, sampler.PriorityNone, 0, 0, 0, true},

		// Test Extractors
		{"0/1/none", []float64{0}, 1, sampler.PriorityNone, 0, 0, 0, true},
		{"0.5/1/none", []float64{0.5}, 1, sampler.PriorityNone, 0.5, 1, 0.15, true},
		{"-1,0.8/1/none", []float64{-1, 0.8}, 1, sampler.PriorityNone, 0.8, 1, 0.1, true},
		{"-1,-1,-0.8/1/none", []float64{-1, -1, 0.8}, 1, sampler.PriorityNone, 0.8, 1, 0.1, true},

		// Test MaxEPS sampler
		{"1/0/none", []float64{1}, 0, sampler.PriorityNone, 1, 0, 0, true},
		{"1/0.5/none", []float64{1}, 0.5, sampler.PriorityNone, 1, 0.5, 0.1, true},
		{"1/1/none", []float64{1}, 1, sampler.PriorityNone, 1, 1, 0, true},

		// Test Extractor and Sampler combinations
		{"-1,0.8/0.8/none", []float64{-1, 0.8}, 0.8, sampler.PriorityNone, 0.8, 0.8, 0.1, true},
		{"-1,0.8/0.8/autokeep", []float64{-1, 0.8}, 0.8, sampler.PriorityAutoKeep, 0.8, 0.8, 0.1, true},
		// Test userkeep bypass of max eps
		{"-1,0.8/0.8/userkeep", []float64{-1, 0.8}, 0.8, sampler.PriorityUserKeep, 0.8, 1, 0.1, true},

		// droppedTrace - false
		// Name: <extraction rates>/<maxEPSSampler rate>/<priority>
		{"none/1/none", nil, 1, sampler.PriorityNone, 0, 0, 0, false},

		// Test Extractors
		{"0/1/none", []float64{0}, 1, sampler.PriorityNone, 0, 0, 0, false},
		{"0.5/1/none", []float64{0.5}, 1, sampler.PriorityNone, 0.5, 1, 0.15, false},
		{"-1,0.8/1/none", []float64{-1, 0.8}, 1, sampler.PriorityNone, 0.8, 1, 0.1, false},
		{"-1,-1,-0.8/1/none", []float64{-1, -1, 0.8}, 1, sampler.PriorityNone, 0.8, 1, 0.1, false},

		// Test MaxEPS sampler
		{"1/0/none", []float64{1}, 0, sampler.PriorityNone, 1, 0, 0, false},
		{"1/0.5/none", []float64{1}, 0.5, sampler.PriorityNone, 1, 0.5, 0.1, false},
		{"1/1/none", []float64{1}, 1, sampler.PriorityNone, 1, 1, 0, false},

		// Test Extractor and Sampler combinations
		{"-1,0.8/0.8/none", []float64{-1, 0.8}, 0.8, sampler.PriorityNone, 0.8, 0.8, 0.1, false},
		{"-1,0.8/0.8/autokeep", []float64{-1, 0.8}, 0.8, sampler.PriorityAutoKeep, 0.8, 0.8, 0.1, false},
		// Test userkeep bypass of max eps
		{"-1,0.8/0.8/userkeep", []float64{-1, 0.8}, 0.8, sampler.PriorityUserKeep, 0.8, 1, 0.1, false},
	}

	testClientSampleRate := 0.3
	testPreSampleRate := 0.5

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert := assert.New(t)

			extractors := make([]Extractor, len(test.extractorRates))
			for i, rate := range test.extractorRates {
				extractors[i] = &MockExtractor{Rate: rate}
			}

			testSampler := &MockEventSampler{Rate: test.samplerRate}
			p := newProcessor(extractors, testSampler)

			testSpans := createTestSpans("test", "test")
			numSpans := len(testSpans)
			testChunk := testutil.TraceChunkWithSpans(testSpans)
			root := testSpans[0]
			sampler.SetPreSampleRate(root, testPreSampleRate)
			sampler.SetClientRate(root, testClientSampleRate)
			testChunk.Priority = int32(test.priority)
			testChunk.DroppedTrace = test.droppedTrace

			p.Start()
			numEvents, extracted := p.Process(root, testChunk)
			p.Stop()

			expectedExtracted := float64(numSpans) * test.expectedExtractedPct
			assert.InDelta(expectedExtracted, extracted, expectedExtracted*test.deltaPct)

			expectedReturned := expectedExtracted * test.expectedSampledPct
			assert.InDelta(expectedReturned, numEvents, expectedReturned*test.deltaPct)

			assert.EqualValues(1, testSampler.StartCalls)
			assert.EqualValues(1, testSampler.StopCalls)

			expectedSampleCalls := extracted
			if test.priority == sampler.PriorityUserKeep {
				expectedSampleCalls = 0
			}
			assert.EqualValues(expectedSampleCalls, testSampler.SampleCalls)

			if !test.droppedTrace {
				assert.EqualValues(numSpans, len(testChunk.Spans))
			} else {
				assert.EqualValues(numEvents, len(testChunk.Spans))
			}

			for _, event := range testChunk.Spans {
				if !sampler.IsAnalyzedSpan(event) {
					continue
				}

				numEvents--
				assert.EqualValues(test.expectedExtractedPct, sampler.GetEventExtractionRate(event))
				assert.EqualValues(test.expectedSampledPct, sampler.GetMaxEPSRate(event))
				assert.EqualValues(testClientSampleRate, sampler.GetClientRate(event))
				assert.EqualValues(testPreSampleRate, sampler.GetPreSampleRate(event))

				priority, ok := sampler.GetSamplingPriority(testChunk)
				if !ok {
					priority = sampler.PriorityNone
				}
				assert.EqualValues(test.priority, priority)
			}

			assert.EqualValues(0, numEvents)
		})
	}
}

type MockExtractor struct {
	Rate float64
}

func (e *MockExtractor) Extract(s *pb.Span, priority sampler.SamplingPriority) (float64, bool) {
	if e.Rate < 0 {
		return 0, false
	}
	return e.Rate, true
}

type MockEventSampler struct {
	Rate float64

	StartCalls  int
	StopCalls   int
	SampleCalls int
}

func (s *MockEventSampler) Start() {
	s.StartCalls++
}

func (s *MockEventSampler) Stop() {
	s.StopCalls++
}

func (s *MockEventSampler) Sample(event *pb.Span) (bool, float64) {
	s.SampleCalls++

	return rand.Float64() < s.Rate, s.Rate
}
