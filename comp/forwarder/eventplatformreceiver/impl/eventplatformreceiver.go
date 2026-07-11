// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatformreceiverimpl implements the event platform receiver component.
package eventplatformreceiverimpl

import (
	"net/http"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	apiutils "github.com/DataDog/datadog-agent/comp/api/api/utils/stream"
	configComponent "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatformreceiver "github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver/def"
	"github.com/DataDog/datadog-agent/comp/logs-library/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(NewComponent),
	)
}

// Requires defines the component dependencies.
type Requires struct {
	compdef.In

	Hostname hostnameinterface.Component
	Config   configComponent.Component
}

// Provides defines the component outputs.
type Provides struct {
	compdef.Out

	Comp     eventplatformreceiver.Component
	Endpoint api.AgentEndpointProvider
}

func streamEventPlatform(eventPlatformReceiver eventplatformreceiver.Component) func(w http.ResponseWriter, r *http.Request) {
	return apiutils.GetStreamFunc(func() apiutils.MessageReceiver { return eventPlatformReceiver }, "event platform payloads", "agent")
}

// NewComponent returns a new event platform receiver component.
func NewComponent(reqs Requires) Provides {
	return NewReceiver(reqs.Hostname, reqs.Config)
}

// NewReceiver returns a new event platform receiver.
// Prefer NewComponent for fx-based construction; this function exists for
// callers that construct the receiver directly without an fx container.
func NewReceiver(hostname hostnameinterface.Component, config configComponent.Component) Provides {
	epr := diagnostic.NewBufferedMessageReceiver(&epFormatter{}, hostname, config)
	return Provides{
		Comp:     epr,
		Endpoint: api.NewAgentEndpointProvider(streamEventPlatform(epr), "/stream-event-platform", "POST"),
	}
}
