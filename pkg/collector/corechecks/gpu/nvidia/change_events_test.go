// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/metrics"
	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

func TestChangeEventProcessor(t *testing.T) {
	p := NewChangeEventProcessor(DefaultChangeEventRules)
	now := time.Unix(100, 0)

	run := func(gpuUUID string, port1, port2 float64) []*Event {
		samples := []Sample{
			&Metric{baseSample: baseSample{tags: []string{"nvlink_port:1"}}, Name: "nvlink.active", Value: port1, Type: metrics.GaugeType},
			&Metric{baseSample: baseSample{tags: []string{"nvlink_port:2"}}, Name: "nvlink.active", Value: port2, Type: metrics.GaugeType},
			&Metric{Name: "temperature", Value: port1, Type: metrics.GaugeType}, // not watched
		}
		out := p.ProcessSamples(samples, now, gpuUUID)
		require.Equal(t, samples, out[:len(samples)], "samples are kept as they are")
		var events []*Event
		for _, s := range out[len(samples):] {
			events = append(events, s.(*Event))
		}
		return events
	}

	require.Empty(t, run("GPU-1", 1, 1), "first values are only recorded")
	require.Empty(t, run("GPU-1", 1, 1), "unchanged values send nothing")
	require.Empty(t, run("GPU-2", 0, 1), "series are per GPU")

	events := run("GPU-1", 1, 0)
	require.Len(t, events, 1)
	require.Equal(t, "GPU GPU-1: nvlink.active changed from active to inactive (nvlink_port:2)", events[0].event.Title)
	require.Equal(t, event.AlertTypeError, events[0].event.AlertType)
	require.Equal(t, "gpu_state_change", events[0].event.EventType)
	require.Equal(t, "GPU-1", events[0].event.AggregationKey)
	require.Equal(t, now, events[0].occurredAt)
	require.ElementsMatch(t, []string{"nvlink_port:2", "nvlink_state:inactive", "previous_nvlink_state:active"}, events[0].Tags())

	events = run("GPU-1", 1, 1)
	require.Len(t, events, 1)
	require.Equal(t, event.AlertTypeSuccess, events[0].event.AlertType)
	require.ElementsMatch(t, []string{"nvlink_port:2", "nvlink_state:active", "previous_nvlink_state:inactive"}, events[0].Tags())
}

func TestChangeEventProcessorKeepsNoWorkloadTags(t *testing.T) {
	p := NewChangeEventProcessor(DefaultChangeEventRules)
	sample := func(value float64) []Sample {
		return []Sample{&Metric{
			baseSample: baseSample{tags: []string{"unavailable_reason:unhealthy"}, noWorkloadTags: true},
			Name:       "device.unavailable",
			Value:      value,
			Type:       metrics.GaugeType,
		}}
	}

	p.ProcessSamples(sample(0), time.Unix(100, 0), "GPU-1")
	out := p.ProcessSamples(sample(1), time.Unix(200, 0), "GPU-1")
	require.Len(t, out, 2)
	evt := out[1].(*Event)
	require.True(t, evt.NoWorkloadTags())
	require.ElementsMatch(t, []string{"unavailable_reason:unhealthy", "availability:unavailable", "previous_availability:available"}, evt.Tags())
}
