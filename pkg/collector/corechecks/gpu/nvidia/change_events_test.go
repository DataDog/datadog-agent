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
	require.Equal(t, "GPU NVLink port down", events[0].event.Title)
	require.Equal(t, "nvlink.active on GPU GPU-1 changed from active to inactive.", events[0].event.Text)
	require.Equal(t, event.AlertTypeError, events[0].event.AlertType)
	require.Equal(t, "gpu_nvlink_state_change", events[0].event.EventType)
	require.Equal(t, "GPU-1", events[0].event.AggregationKey)
	require.Equal(t, now, events[0].occurredAt)
	require.Equal(t, []string{"nvlink_port:2"}, events[0].Tags())

	events = run("GPU-1", 1, 1)
	require.Len(t, events, 1)
	require.Equal(t, "GPU NVLink port up", events[0].event.Title)
	require.Equal(t, event.AlertTypeSuccess, events[0].event.AlertType)
	require.Equal(t, []string{"nvlink_port:2"}, events[0].Tags())
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
	require.Equal(t, "GPU device unavailable", evt.event.Title)
	require.Equal(t, "gpu_availability_change", evt.event.EventType)
	require.True(t, evt.NoWorkloadTags())
	require.Equal(t, []string{"unavailable_reason:unhealthy"}, evt.Tags())
}

func TestChangeEventProcessorUnschedulable(t *testing.T) {
	p := NewChangeEventProcessor(DefaultChangeEventRules)
	unschedulable := func(value float64, reason string) Sample {
		return &Metric{
			baseSample: baseSample{tags: []string{"unavailable_reason:unschedulable", "unschedulable_reason:" + reason}, noWorkloadTags: true},
			Name:       "device.unavailable",
			Value:      value,
			Type:       metrics.GaugeType,
		}
	}
	run := func(samples ...Sample) []*Event {
		var events []*Event
		for _, s := range p.ProcessSamples(samples, time.Unix(100, 0), "GPU-1")[len(samples):] {
			events = append(events, s.(*Event))
		}
		return events
	}

	// Cordon: the series appears with 1, a change from the not emitted value.
	events := run(unschedulable(1, "cordoned"))
	require.Len(t, events, 1)
	require.Equal(t, "GPU device unavailable", events[0].event.Title)
	require.Equal(t, []string{"unavailable_reason:unschedulable", "unschedulable_reason:cordoned"}, events[0].Tags())

	// The cause changes: the old cause reports 0 alongside the new one at 1.
	events = run(unschedulable(1, "not_ready"), unschedulable(0, "cordoned"))
	require.Len(t, events, 1)
	require.Equal(t, "GPU device unavailable", events[0].event.Title)
	require.Equal(t, []string{"unavailable_reason:unschedulable", "unschedulable_reason:not_ready", "previous_unschedulable_reason:cordoned"}, events[0].Tags())

	require.Empty(t, run(unschedulable(1, "not_ready"), unschedulable(0, "cordoned")))

	// Schedulable again.
	events = run(unschedulable(0, "not_ready"), unschedulable(0, "cordoned"))
	require.Len(t, events, 1)
	require.Equal(t, "GPU device available", events[0].event.Title)
	require.Contains(t, events[0].Tags(), "previous_unschedulable_reason:not_ready")

	// A series that appears with the not emitted value sends nothing.
	require.Empty(t, p.ProcessSamples([]Sample{unschedulable(0, "cordoned")}, time.Unix(100, 0), "GPU-2")[1:])
}
