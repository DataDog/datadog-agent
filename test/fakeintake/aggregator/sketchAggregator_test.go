// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package aggregator

import (
	"sort"
	"testing"
	"time"

	metricspb "github.com/DataDog/agent-payload/v5/gogen"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/fakeintake/api"
)

func TestParseSketches(t *testing.T) {
	pb := metricspb.SketchPayload{
		Sketches: []metricspb.SketchPayload_Sketch{
			{
				Metric: "test.dist",
				Host:   "h1",
				Tags:   []string{"env:prod", "service:api"},
			},
			{
				Metric: "test.dist",
				Host:   "h1",
				Tags:   []string{"env:dev"},
			},
			{
				Metric: "other.dist",
				Tags:   []string{"k:v"},
			},
		},
	}
	raw, err := pb.Marshal()
	assert.NoError(t, err)

	now := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	out, err := ParseSketches(api.Payload{
		Data:        raw,
		ContentType: "application/x-protobuf",
		Timestamp:   now,
	})
	assert.NoError(t, err)
	assert.Len(t, out, 3)
	assert.Equal(t, "test.dist", out[0].name())
	assert.Equal(t, now, out[0].GetCollectedTime())
	got := out[0].GetTags()
	sort.Strings(got)
	assert.Equal(t, []string{"env:prod", "service:api"}, got)
}

func TestSketchAggregatorByName(t *testing.T) {
	pb := metricspb.SketchPayload{
		Sketches: []metricspb.SketchPayload_Sketch{
			{Metric: "a"},
			{Metric: "b"},
			{Metric: "a"},
		},
	}
	raw, err := pb.Marshal()
	assert.NoError(t, err)

	agg := NewSketchAggregator()
	err = agg.UnmarshallPayloads([]api.Payload{{Data: raw, ContentType: "application/x-protobuf"}})
	assert.NoError(t, err)

	a := agg.GetPayloadsByName("a")
	assert.Len(t, a, 2)
	names := agg.GetNames()
	assert.Equal(t, []string{"a", "b"}, names)
}
