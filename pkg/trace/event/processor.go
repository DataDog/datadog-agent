// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package event

import (
	"github.com/DataDog/datadog-agent/pkg/proto/pbgo/trace/idx"
	"github.com/DataDog/datadog-agent/pkg/trace/sampler"
	"github.com/DataDog/datadog-agent/pkg/trace/traceutil"

	"github.com/DataDog/datadog-go/v5/statsd"
)

// Processor is responsible for all the logic surrounding extraction and sampling of APM events from processed traces.
type Processor struct {
	extractors    []Extractor
	maxEPSSampler eventSampler
}

// NewProcessor returns a new instance of Processor configured with the provided extractors and max eps limitation.
//
// Extractors will look at each span in the trace and decide whether it should be converted to an APM event or not. They
// will be tried in the provided order, with the first one returning an event stopping the chain.
//
// All extracted APM events are then submitted to sampling. This sampling is 2-fold:
//   - A first sampling step is done based on the extraction sampling rate returned by an Extractor. If an Extractor
//     returns an event accompanied with a 0.1 extraction rate, then there's a 90% chance that this event will get
//     discarded.
//   - A max events per second maxEPSSampler is applied to all non-PriorityUserKeep events that survived the first step
//     and will ensure that, in average, the total rate of events returned by the processor is not bigger than maxEPS.
func NewProcessor(extractors []Extractor, maxEPS float64, statsd statsd.ClientInterface) *Processor {
	return newProcessor(extractors, newMaxEPSSampler(maxEPS, statsd))
}

func newProcessor(extractors []Extractor, maxEPSSampler eventSampler) *Processor {
	return &Processor{
		extractors:    extractors,
		maxEPSSampler: maxEPSSampler,
	}
}

// Start starts the processor.
func (p *Processor) Start() {
	p.maxEPSSampler.Start()
}

// Stop stops the processor.
func (p *Processor) Stop() {
	p.maxEPSSampler.Stop()
}

// ProcessV1 takes a processed trace, extracts events from it and samples them, returning a collection of
// sampled events along with the total count of events.
// numEvents is the number of sampled events found in the trace
// numExtracted is the number of events found in the trace
// events is the slice of sampled analytics events to keep (only has values if pt will be dropped)
func (p *Processor) ProcessV1(pt *traceutil.ProcessedTraceV1) (numEvents, numExtracted int64, events []*idx.InternalSpan) {
	clientSampleRate := sampler.GetClientRateV1(pt.Root)
	preSampleRate := sampler.GetPreSampleRateV1(pt.Root)
	priority := sampler.SamplingPriority(pt.TraceChunk.Priority)

	for _, span := range pt.TraceChunk.Spans {
		extractionRate, ok := p.extractV1(span, priority)
		if !ok {
			continue
		}
		if !sampler.SampleByRate(pt.TraceChunk.LegacyTraceID(), extractionRate) {
			continue
		}

		numExtracted++

		sampled, epsRate := p.maxEPSSampleV1(pt.TraceChunk.LegacyTraceID(), priority)
		if !sampled {
			continue
		}
		// event analytics tags shouldn't be set on sampled single spans
		sampler.SetMaxEPSRateV1(span, epsRate)
		sampler.SetClientRateV1(span, clientSampleRate)
		sampler.SetPreSampleRateV1(span, preSampleRate)
		sampler.SetEventExtractionRateV1(span, extractionRate)
		sampler.SetAnalyzedSpanV1(span)
		span.SetFloat64Attribute(sampler.KeyAnalyzedSpans, 1)
		if pt.TraceChunk.DroppedTrace {
			events = append(events, span)
		}
		numEvents++
	}
	return numEvents, numExtracted, events
}

func (p *Processor) extractV1(span *idx.InternalSpan, priority sampler.SamplingPriority) (float64, bool) {
	for _, extractor := range p.extractors {
		if rate, ok := extractor.ExtractV1(span, priority); ok {
			return rate, ok
		}
	}
	return 0, false
}

func (p *Processor) maxEPSSampleV1(traceID uint64, priority sampler.SamplingPriority) (sampled bool, rate float64) {
	if priority == sampler.PriorityUserKeep {
		return true, 1
	}
	return p.maxEPSSampler.SampleV1(traceID)
}

type eventSampler interface {
	Start()
	SampleV1(traceID uint64) (sampled bool, rate float64)
	Stop()
}
