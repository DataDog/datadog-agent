// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"time"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

// Sketch wraps a single sketch entry from a SketchPayload (one per metric +
// tagset combination posted by the agent to /api/beta/sketches).
type Sketch struct {
	metricspb.SketchPayload_Sketch

	collectedTime time.Time
}

func (s *Sketch) name() string {
	return s.Metric
}

// GetTags returns the tags attached to this sketch by the agent, after any
// agent-side tag filtering (filterlist) has been applied.
func (s *Sketch) GetTags() []string {
	return s.Tags
}

// GetCollectedTime returns when fakeintake received the payload.
func (s *Sketch) GetCollectedTime() time.Time {
	return s.collectedTime
}

// ParseSketches decodes a /api/beta/sketches payload (compressed protobuf) into
// individual Sketch entries, one per (metric, host, tagset) sketch in the
// payload's sketches array.
func ParseSketches(payload api.Payload) ([]*Sketch, error) {
	inflated, err := inflate(payload.Data, payload.Encoding)
	if err != nil {
		return nil, err
	}
	pb := new(metricspb.SketchPayload)
	if err := pb.Unmarshal(inflated); err != nil {
		return nil, err
	}
	out := make([]*Sketch, 0, len(pb.Sketches))
	for i := range pb.Sketches {
		out = append(out, &Sketch{
			SketchPayload_Sketch: pb.Sketches[i],
			collectedTime:        payload.Timestamp,
		})
	}
	return out, nil
}

// SketchAggregator stores sketch payloads received on /api/beta/sketches.
type SketchAggregator struct {
	Aggregator[*Sketch]
}

// NewSketchAggregator returns a new SketchAggregator.
func NewSketchAggregator() SketchAggregator {
	return SketchAggregator{
		Aggregator: newAggregator(ParseSketches),
	}
}
