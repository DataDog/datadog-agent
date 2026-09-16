// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package remoteagentregistry

import "go.uber.org/fx"

// EventCallback is registered by a component to receive Remote Agent events.
//
// It is invoked synchronously on the event-reporting path, once per report, with the events reported
// together by a single remote agent. Implementations MUST return quickly and MUST NOT block: offload
// any slow work (network calls, refreshes, etc.) to their own goroutine.
type EventCallback func(agent RegisteredAgent, events []RemoteAgentEvent)

// EventSubscriber wraps an EventCallback for registration via an fx value group. It is a struct rather
// than a bare func so that fields can be added later without breaking existing subscribers.
type EventSubscriber struct {
	// Name identifies the subscriber in logs (for example, if its callback panics). Optional.
	Name string
	// Callback receives the reported events. Required.
	Callback EventCallback
}

// EventSubscriberProvider is provided by components to register themselves as a subscriber of Remote
// Agent events. The Remote Agent Registry collects every provided subscriber through the
// "remoteAgentEventSubscriber" fx value group and invokes them whenever a remote agent reports events.
type EventSubscriberProvider struct {
	fx.Out

	Subscriber *EventSubscriber `group:"remoteAgentEventSubscriber"`
}

// NewEventSubscriber returns an EventSubscriberProvider wrapping the given callback.
func NewEventSubscriber(name string, callback EventCallback) EventSubscriberProvider {
	return EventSubscriberProvider{
		Subscriber: &EventSubscriber{
			Name:     name,
			Callback: callback,
		},
	}
}
