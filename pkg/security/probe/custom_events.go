// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package probe holds probe related files
package probe

import (
	"encoding/json"

	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
	"github.com/DataDog/datadog-agent/pkg/security/serializers"
)

// AbnormalEvent is used to report that a path resolution failed for a suspicious reason
type AbnormalEvent struct {
	events.CustomEventCommonFields
	Event *serializers.EventSerializer `json:"triggering_event"`
	Error string                       `json:"error"`
}

// ToJSON marshal using json format
func (a AbnormalEvent) ToJSON() ([]byte, error) {
	return json.Marshal(a)
}

// NewAbnormalEvent returns the rule and a populated custom event for a abnormal event
func NewAbnormalEvent(id string, description string, event *model.Event, probe *Probe, eventType model.EventType, err error) (*rules.Rule, *events.CustomEvent) {
	marshalerCtor := func() events.EventMarshaler {
		evt := AbnormalEvent{
			Event: serializers.NewEventSerializer(event, probe.resolvers),
			Error: err.Error(),
		}
		evt.FillCustomEventCommonFields()
		// Overwrite common timestamp with event timestamp
		evt.Timestamp = event.FieldHandlers.ResolveEventTime(event)

		return evt
	}

	return events.NewCustomRule(id, description), events.NewCustomEventLazy(eventType, marshalerCtor)
}
