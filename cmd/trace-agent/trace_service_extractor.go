package main

import (
	"github.com/DataDog/datadog-agent/pkg/trace/agent"
	"github.com/DataDog/datadog-agent/pkg/trace/pb"
)

// appType is one of the pieces of information embedded in ServiceMetadata
const appType = "app_type"

// TraceServiceExtractor extracts service metadata from top-level spans
type TraceServiceExtractor struct {
	outServices chan<- pb.ServicesMetadata
}

// NewTraceServiceExtractor returns a new TraceServiceExtractor
func NewTraceServiceExtractor(out chan<- pb.ServicesMetadata) *TraceServiceExtractor {
	return &TraceServiceExtractor{out}
}

// Process extracts service metadata from top-level spans and sends it downstream
func (ts *TraceServiceExtractor) Process(t agent.WeightedTrace) {
	meta := make(pb.ServicesMetadata)

	for _, s := range t {
		if !s.TopLevel {
			continue
		}

		if _, ok := meta[s.Service]; ok {
			continue
		}

		if v := s.Type; len(v) > 0 {
			meta[s.Service] = map[string]string{appType: v}
		}
	}

	if len(meta) > 0 {
		ts.outServices <- meta
	}
}
