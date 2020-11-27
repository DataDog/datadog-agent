// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package probe

import (
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/security/secl/eval"
)

type CustomEvent struct {
	eventType   string
	tags        []string
	marshalFunc func() ([]byte, error)
}

func (ce *CustomEvent) GetTags() []string {
	return append(ce.tags, "type:"+ce.GetType())
}

func (ce *CustomEvent) GetType() string {
	return ce.eventType
}

func (ce *CustomEvent) MarshalJSON() ([]byte, error) {
	if ce.marshalFunc != nil {
		return ce.marshalFunc()
	}
	return nil, nil
}

func (ce *CustomEvent) String() string {
	d, err := json.Marshal(ce)
	if err != nil {
		return err.Error()
	}
	return string(d)
}

// NewEventLostReadEvent returns the rule and a populated custom event for a lost_events_read event
func NewEventLostReadEvent(mapName string, perCPU map[int]int64) (*eval.Rule, *CustomEvent) {
	return &eval.Rule{
			ID: "lost_events",
		}, &CustomEvent{
			eventType: "lost_events_read",
			marshalFunc: func() ([]byte, error) {
				return json.Marshal(struct {
					Name string        `json:"map"`
					Lost map[int]int64 `json:"per_cpu"`
				}{
					Name: mapName,
					Lost: perCPU,
				})
			},
		}
}

// NewEventLostWriteEvent returns the rule and a populated custom event for a lost_events_write event
func NewEventLostWriteEvent(mapName string, perEventPerCPU map[string]map[int]uint64) (*eval.Rule, *CustomEvent) {
	return &eval.Rule{
			ID: "lost_events",
		}, &CustomEvent{
			eventType: "lost_events_write",
			marshalFunc: func() ([]byte, error) {
				return json.Marshal(struct {
					Name string                    `json:"map"`
					Lost map[string]map[int]uint64 `json:"per_event_per_cpu"`
				}{
					Name: mapName,
					Lost: perEventPerCPU,
				})
			},
		}
}
