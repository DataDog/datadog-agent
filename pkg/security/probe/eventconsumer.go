// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package probe

import (
	"github.com/DataDog/datadog-agent/pkg/security/events"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/rules"
)

// this file contain common platform-agnostic interfaces (no go:build tags)

// EventHandler represents a handler for events sent by the probe that needs access to all the fields in the SECL model
type EventHandler interface {
	HandleEvent(event *model.Event)
}

// EventConsumerHandler represents a handler for events sent by the probe. This handler makes a copy of the event upon receipt
type EventConsumerHandler interface {
	IDer
	ChanSize() int
	HandleEvent(_ any)
	Copy(_ *model.Event) any
	EventTypes() []model.EventType
}

// IDer provides unique ID for each event consumer
type IDer interface {
	// ID returns the ID of the event consumer
	ID() string
}

// CustomEventHandler represents an handler for the custom events sent by the probe
type CustomEventHandler interface {
	HandleCustomEvent(rule *rules.Rule, event *events.CustomEvent)
}
