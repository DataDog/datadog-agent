// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"math/rand"
	"testing"

	"github.com/stretchr/testify/assert"

	pb "github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace"
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/testutil"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"
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

			numSpans := 10000
			var totalEvents, totalExtracted int64
			var allEvents []*idx.InternalSpan
			var allSpans []*idx.InternalSpan
			var lastChunk *idx.InternalTraceChunk

			p.Start()
			// Process each span in its own chunk for per-span sampling (matching old behavior)
			for i := 0; i < numSpans; i++ {
				strings := idx.NewStringTable()
				span := idx.NewInternalSpan(strings, &idx.Span{
					SpanID:     rand.Uint64(),
					ServiceRef: strings.Add("test"),
					NameRef:    strings.Add("test"),
					Attributes: make(map[uint32]*idx.AnyValue),
				})
				// Set rates on all spans since each span is its own root
				sampler.SetPreSampleRateV1(span, testPreSampleRate)
				sampler.SetClientRateV1(span, testClientSampleRate)
				chunk := idx.NewInternalTraceChunk(
					strings,
					int32(test.priority),
					"",
					make(map[uint32]*idx.AnyValue),
					[]*idx.InternalSpan{span},
					test.droppedTrace,
					make([]byte, 16),
					0,
				)
				chunk.SetLegacyTraceID(testutil.RandomSpanTraceID())
				lastChunk = chunk
				allSpans = append(allSpans, span)

				pt := &traceutil.ProcessedTraceV1{
					TraceChunk: chunk,
					Root:       span,
				}
				numEventsResult, numExtractedResult, eventsResult := p.ProcessV1(pt)
				totalEvents += numEventsResult
				totalExtracted += numExtractedResult
				allEvents = append(allEvents, eventsResult...)
			}
			p.Stop()

			numEvents := totalEvents
			numExtracted := totalExtracted
			events := allEvents
			testChunk := lastChunk
			_ = testChunk // Used for priority check

			expectedExtracted := float64(numSpans) * test.expectedExtractedPct
			assert.InDelta(expectedExtracted, numExtracted, expectedExtracted*test.deltaPct)

			expectedReturned := expectedExtracted * test.expectedSampledPct
			assert.InDelta(expectedReturned, numEvents, expectedReturned*test.deltaPct)

			assert.EqualValues(1, testSampler.StartCalls)
			assert.EqualValues(1, testSampler.StopCalls)

			expectedSampleCalls := numExtracted
			if test.priority == sampler.PriorityUserKeep {
				expectedSampleCalls = 0
			}
			assert.EqualValues(expectedSampleCalls, testSampler.SampleCalls)

			if !test.droppedTrace {
				// For non-dropped traces, check all spans for analyzed events
				events = allSpans
				assert.EqualValues(numSpans, len(allSpans))
			} else {
				assert.EqualValues(numEvents, len(events))
			}

			for _, event := range events {
				if !sampler.IsAnalyzedSpanV1(event) {
					continue
				}

				numEvents--
				// Default to 1.0 if attribute not found (bandwidth optimization)
				extractionRate, ok := event.GetAttributeAsFloat64(sampler.KeySamplingRateEventExtraction)
				if !ok {
					extractionRate = 1.0
				}
				assert.EqualValues(test.expectedExtractedPct, extractionRate)
				maxEPSRate, ok := event.GetAttributeAsFloat64(sampler.KeySamplingRateMaxEPSSampler)
				if !ok {
					maxEPSRate = 1.0
				}
				assert.EqualValues(test.expectedSampledPct, maxEPSRate)
				clientRate := sampler.GetClientRateV1(event)
				assert.EqualValues(testClientSampleRate, clientRate)
				preSampleRate := sampler.GetPreSampleRateV1(event)
				assert.EqualValues(testPreSampleRate, preSampleRate)

				priority, ok := sampler.GetSamplingPriorityV1(testChunk)
				if !ok {
					priority = sampler.PriorityNone
				}
				assert.EqualValues(test.priority, priority)
			}

			assert.EqualValues(0, numEvents)
		})
	}
}

// MockExtractor is a mock implementation of the Extractor interface
type MockExtractor struct {
	Rate float64
}

func (e *MockExtractor) Extract(_ *pb.Span, _ sampler.SamplingPriority) (float64, bool) {
	if e.Rate < 0 {
		return 0, false
	}
	return e.Rate, true
}

func (e *MockExtractor) ExtractV1(_ *idx.InternalSpan, _ sampler.SamplingPriority) (float64, bool) {
	if e.Rate < 0 {
		return 0, false
	}
	return e.Rate, true
}

// MockEventSampler is a mock implementation of the EventSampler interface
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

func (s *MockEventSampler) SampleV1(_ uint64) (bool, float64) {
	s.SampleCalls++

	return rand.Float64() < s.Rate, s.Rate
}
