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

// ChangeEventState names one value of a watched metric, with the title and alert type of
// the event sent when the metric changes to it.
type ChangeEventState struct {
	Name      string
	Title     string
	AlertType event.AlertType
}

// ChangeEventRule turns changes of a metric value into events of EventType, which carry
// the series tags.
type ChangeEventRule struct {
	EventType string
	States    map[float64]ChangeEventState
	// PreviousTags are series tags reported with their previous value: they are left out of
	// the series identity, a change of their value is a change of the series, and the event
	// also carries previous_<tag>:<old value>.
	PreviousTags []string
	// NotEmittedValue, when set, is the value of a series that hasn't been emitted yet, so
	// that its first value is a change from it. When nil, the first value is only recorded.
	NotEmittedValue *float64
}

// DefaultChangeEventRules are the metrics whose changes are sent as events, by metric name.
var DefaultChangeEventRules = map[string]ChangeEventRule{
	"nvlink.active": {
		EventType: "gpu_nvlink_state_change",
		States: map[float64]ChangeEventState{
			1: {Name: "active", Title: "GPU NVLink port up", AlertType: event.AlertTypeSuccess},
			0: {Name: "inactive", Title: "GPU NVLink port down", AlertType: event.AlertTypeError},
		},
	},
	"device.unavailable": {
		EventType:    "gpu_availability_change",
		PreviousTags: []string{"unschedulable_reason"},
		// The unschedulable series is only emitted while the node is unschedulable.
		NotEmittedValue: new(0.0),
		States: map[float64]ChangeEventState{
			0: {Name: "available", Title: "GPU device available", AlertType: event.AlertTypeSuccess},
			1: {Name: "unavailable", Title: "GPU device unavailable", AlertType: event.AlertTypeError},
		},
	},
}

// changeEventSeries is the last value of a watched series and of its PreviousTags.
type changeEventSeries struct {
	value        float64
	previousTags []string
}

// ChangeEventProcessor sends an event when the value of a watched metric series changes.
// Series are identified by GPU UUID, metric name and tags other than the rule's
// PreviousTags; if a run has several samples of a series, the highest value is used. The
// first value of a series is only recorded, unless its rule sets NotEmittedValue.
type ChangeEventProcessor struct {
	rules map[string]ChangeEventRule
	last  map[string]changeEventSeries
}

// NewChangeEventProcessor creates a processor for the given rules.
func NewChangeEventProcessor(rules map[string]ChangeEventRule) *ChangeEventProcessor {
	return &ChangeEventProcessor{rules: rules, last: make(map[string]changeEventSeries)}
}

// ProcessSamples returns the samples of a device, plus an event for each watched series
// whose value or PreviousTags changed since the previous run.
func (p *ChangeEventProcessor) ProcessSamples(samples []Sample, timestamp time.Time, gpuUUID string) []Sample {
	type observation struct {
		metric       *Metric
		rule         ChangeEventRule
		tags         []string
		previousTags []string
	}
	var keys []string
	current := make(map[string]observation)
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
		var identity, previousTags []string
		for _, tag := range tags {
			name, _, _ := strings.Cut(tag, ":")
			if slices.Contains(rule.PreviousTags, name) {
				previousTags = append(previousTags, tag)
			} else {
				identity = append(identity, tag)
			}
		}
		key := gpuUUID + "|" + metric.Name + "|" + strings.Join(identity, ",")
		if existing, ok := current[key]; ok {
			if existing.metric.Value >= metric.Value {
				continue
			}
		} else {
			keys = append(keys, key)
		}
		current[key] = observation{metric: metric, rule: rule, tags: tags, previousTags: previousTags}
	}

	for _, key := range keys {
		obs := current[key]
		previous, seen := p.last[key]
		p.last[key] = changeEventSeries{value: obs.metric.Value, previousTags: obs.previousTags}
		switch {
		case seen:
			if previous.value == obs.metric.Value && slices.Equal(previous.previousTags, obs.previousTags) {
				continue
			}
		case obs.rule.NotEmittedValue != nil:
			// Only the value can change from a series that wasn't emitted.
			if *obs.rule.NotEmittedValue == obs.metric.Value {
				continue
			}
			previous = changeEventSeries{value: *obs.rule.NotEmittedValue}
		default:
			continue
		}

		previousState, okPrevious := obs.rule.States[previous.value]
		currentState, okCurrent := obs.rule.States[obs.metric.Value]
		if !okPrevious || !okCurrent {
			continue
		}

		tags := slices.Clone(obs.tags)
		for _, tag := range previous.previousTags {
			tags = append(tags, "previous_"+tag)
		}
		evt := NewEvent(event.Event{
			Title:          currentState.Title,
			Text:           fmt.Sprintf("%s on GPU %s changed from %s to %s.", obs.metric.Name, gpuUUID, previousState.Name, currentState.Name),
			Priority:       event.PriorityNormal,
			AlertType:      currentState.AlertType,
			SourceTypeName: "gpu",
			EventType:      obs.rule.EventType,
			AggregationKey: gpuUUID,
		}, timestamp, obs.metric.Priority(), tags, obs.metric.AssociatedWorkloads())
		evt.noWorkloadTags = obs.metric.NoWorkloadTags()
		samples = append(samples, evt)
	}
	return samples
}
