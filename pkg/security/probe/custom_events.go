//go:generate go run github.com/mailru/easyjson/easyjson -gen_build_flags=-mod=mod -no_std_marshalers -build_tags linux $GOFILE

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package probe

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/resolvers/dentry"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
	easyjson "github.com/mailru/easyjson"
)

// EventLostRead is the event used to report lost events detected from user space
// easyjson:json
type EventLostRead struct {
	events.CustomEventCommonFields
	Name string  `json:"map"`
	Lost float64 `json:"lost"`
}

// NewEventLostReadEvent returns the rule and a populated custom event for a lost_events_read event
func NewEventLostReadEvent(mapName string, lost float64) (*rules.Rule, *events.CustomEvent) {
	evt := EventLostRead{
		Name: mapName,
		Lost: lost,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.LostEventsRuleID), events.NewCustomEvent(model.CustomLostReadEventType, func() easyjson.Marshaler { return evt })
}

// EventLostWrite is the event used to report lost events detected from kernel space
// easyjson:json
type EventLostWrite struct {
	events.CustomEventCommonFields
	Name string            `json:"map"`
	Lost map[string]uint64 `json:"per_event"`
}

// NewEventLostWriteEvent returns the rule and a populated custom event for a lost_events_write event
func NewEventLostWriteEvent(mapName string, perEventPerCPU map[string]uint64) (*rules.Rule, *events.CustomEvent) {
	evt := EventLostWrite{
		Name: mapName,
		Lost: perEventPerCPU,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.LostEventsRuleID), events.NewCustomEvent(model.CustomLostWriteEventType, func() easyjson.Marshaler { return evt })
}

// NoisyProcessEvent is used to report that a noisy process was temporarily discarded
// easyjson:json
type NoisyProcessEvent struct {
	events.CustomEventCommonFields
	Count          uint64        `json:"pid_count"`
	Threshold      int64         `json:"threshold"`
	ControlPeriod  time.Duration `json:"control_period"`
	DiscardedUntil time.Time     `json:"discarded_until"`
	Pid            uint32        `json:"pid"`
	Comm           string        `json:"comm"`
}

// NewNoisyProcessEvent returns the rule and a populated custom event for a noisy_process event
func NewNoisyProcessEvent(count uint64,
	threshold int64,
	controlPeriod time.Duration,
	discardedUntil time.Time,
	pid uint32,
	comm string,
	timestamp time.Time) (*rules.Rule, *events.CustomEvent) {

	evt := NoisyProcessEvent{
		Count:          count,
		Threshold:      threshold,
		ControlPeriod:  controlPeriod,
		DiscardedUntil: discardedUntil,
		Pid:            pid,
		Comm:           comm,
	}
	evt.FillCustomEventCommonFields()
	// Overwrite common timestamp
	evt.Timestamp = timestamp

	return events.NewCustomRule(events.NoisyProcessRuleID), events.NewCustomEvent(model.CustomNoisyProcessEventType, func() easyjson.Marshaler { return evt })
}

func resolutionErrorToEventType(err error) model.EventType {
	switch err.(type) {
	case dentry.ErrTruncatedParents, dentry.ErrTruncatedParentsERPC:
		return model.CustomTruncatedParentsEventType
	default:
		return model.UnknownEventType
	}
}

// AbnormalPathEvent is used to report that a path resolution failed for a suspicious reason
// easyjson:json
type AbnormalPathEvent struct {
	events.CustomEventCommonFields
	Event               *serializers.EventSerializer `json:"triggering_event"`
	PathResolutionError string                       `json:"path_resolution_error"`
}

// NewAbnormalPathEvent returns the rule and a populated custom event for a abnormal_path event
func NewAbnormalPathEvent(event *model.Event, probe *Probe, pathResolutionError error) (*rules.Rule, *events.CustomEvent) {
	marshalerCtor := func() easyjson.Marshaler {
		evt := AbnormalPathEvent{
			Event:               serializers.NewEventSerializer(event, probe.resolvers),
			PathResolutionError: pathResolutionError.Error(),
		}
		evt.FillCustomEventCommonFields()
		// Overwrite common timestamp with event timestamp
		evt.Timestamp = event.FieldHandlers.ResolveEventTimestamp(event)

		return evt
	}

	return events.NewCustomRule(events.AbnormalPathRuleID), events.NewCustomEvent(resolutionErrorToEventType(event.PathResolutionError), marshalerCtor)
}
