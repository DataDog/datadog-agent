// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package events

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// DelayedEvent describes a delayed event
type DelayedEvent struct {
	Event        model.Event
	ReevaluateAt time.Time
}

// NewDelayedEvent returns a new delayed event
func NewDelayedEvent(event *model.Event, delay time.Duration) *DelayedEvent {
	// resolve fields now to have them available with the right time context
	event.ResolveFields()
	event.Retain()

	dev := &DelayedEvent{
		Event:        *event,
		ReevaluateAt: time.Now().Add(delay),
	}

	return dev
}

// Release the underlying event
func (dev *DelayedEvent) Release() {
	dev.Event.Release()
}
