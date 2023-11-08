// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !ebpfless

// Package probe holds probe related files
package probe

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// EventLostRead is the event used to report lost events detected from user space
type EventLostRead struct {
	events.CustomEventCommonFields
	Name string  `json:"map"`
	Lost float64 `json:"lost"`
}

// ToJSON marshal using json format
func (e EventLostRead) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// NewEventLostReadEvent returns the rule and a populated custom event for a lost_events_read event
func NewEventLostReadEvent(mapName string, lost float64) (*rules.Rule, *events.CustomEvent) {
	evt := EventLostRead{
		Name: mapName,
		Lost: lost,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.LostEventsRuleID, events.LostEventsRuleDesc), events.NewCustomEvent(model.CustomLostReadEventType, evt)
}

// EventLostWrite is the event used to report lost events detected from kernel space
type EventLostWrite struct {
	events.CustomEventCommonFields
	Name string            `json:"map"`
	Lost map[string]uint64 `json:"per_event"`
}

// ToJSON marshal using json format
func (e EventLostWrite) ToJSON() ([]byte, error) {
	return json.Marshal(e)
}

// NewEventLostWriteEvent returns the rule and a populated custom event for a lost_events_write event
func NewEventLostWriteEvent(mapName string, perEventPerCPU map[string]uint64) (*rules.Rule, *events.CustomEvent) {
	evt := EventLostWrite{
		Name: mapName,
		Lost: perEventPerCPU,
	}
	evt.FillCustomEventCommonFields()

	return events.NewCustomRule(events.LostEventsRuleID, events.LostEventsRuleDesc), events.NewCustomEvent(model.CustomLostWriteEventType, evt)
}
