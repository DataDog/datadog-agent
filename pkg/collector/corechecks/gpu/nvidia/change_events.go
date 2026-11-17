// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && nvml

package nvidia

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/DataDog/datadog-agent/pkg/metrics/event"
)

// ChangeEventState names one value of a watched metric, and the alert type of the event
// sent when the metric changes to it.
type ChangeEventState struct {
	Name      string
	AlertType event.AlertType
}

// ChangeEventRule turns changes of a metric value into events. The event carries the
// series tags plus <StateTag>:<new state> and previous_<StateTag>:<old state>.
type ChangeEventRule struct {
	StateTag string
	States   map[float64]ChangeEventState
}

// DefaultChangeEventRules are the metrics whose changes are sent as events, by metric name.
var DefaultChangeEventRules = map[string]ChangeEventRule{
	"nvlink.active": {
		StateTag: "nvlink_state",
		States: map[float64]ChangeEventState{
			1: {Name: "active", AlertType: event.AlertTypeSuccess},
			0: {Name: "inactive", AlertType: event.AlertTypeError},
		},
	},
	"device.unavailable": {
		StateTag: "availability",
		States: map[float64]ChangeEventState{
			0: {Name: "available", AlertType: event.AlertTypeSuccess},
			1: {Name: "unavailable", AlertType: event.AlertTypeError},
		},
	},
}

// ChangeEventProcessor sends an event when the value of a watched metric series changes.
// Series are identified by GPU UUID, metric name and tags. The first value of a series is
// only recorded.
type ChangeEventProcessor struct {
	rules map[string]ChangeEventRule
	last  map[string]float64
}

// NewChangeEventProcessor creates a processor for the given rules.
func NewChangeEventProcessor(rules map[string]ChangeEventRule) *ChangeEventProcessor {
	return &ChangeEventProcessor{rules: rules, last: make(map[string]float64)}
}

// ProcessSamples returns the samples of a device, plus an event for each watched series
// whose value changed since the previous run.
func (p *ChangeEventProcessor) ProcessSamples(samples []Sample, timestamp time.Time, gpuUUID string) []Sample {
	for _, sample := range samples {
		metric, ok := sample.(*Metric)
		if !ok {
			continue
		}
		rule, ok := p.rules[metric.Name]
		if !ok {
			continue
		}

		tags := slices.Sorted(slices.Values(metric.Tags()))
		key := gpuUUID + "|" + metric.Name + "|" + strings.Join(tags, ",")
		previous, seen := p.last[key]
		p.last[key] = metric.Value
		if !seen || previous == metric.Value {
			continue
		}

		previousState, okPrevious := rule.States[previous]
		currentState, okCurrent := rule.States[metric.Value]
		if !okPrevious || !okCurrent {
			continue
		}

		title := fmt.Sprintf("GPU %s: %s changed from %s to %s", gpuUUID, metric.Name, previousState.Name, currentState.Name)
		if len(tags) > 0 {
			title += " (" + strings.Join(tags, ", ") + ")"
		}
		evt := NewEvent(event.Event{
			Title:          title,
			Text:           title + ".",
			Priority:       event.PriorityNormal,
			AlertType:      currentState.AlertType,
			SourceTypeName: "gpu",
			EventType:      "gpu_state_change",
			AggregationKey: gpuUUID,
		}, timestamp, metric.Priority(), append(tags,
			rule.StateTag+":"+currentState.Name,
			"previous_"+rule.StateTag+":"+previousState.Name,
		), metric.AssociatedWorkloads())
		evt.noWorkloadTags = metric.NoWorkloadTags()
		samples = append(samples, evt)
	}
	return samples
}
