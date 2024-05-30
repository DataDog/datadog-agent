// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatformreceiverimpl implements the event platform receiver component.
package eventplatformreceiverimpl

import (
	"net/http"

	"go.uber.org/fx"

	apihelper "github.com/DataDog/datadog-agent/comp/api/api/helpers"
	apiutils "github.com/DataDog/datadog-agent/comp/api/api/utils/stream"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatformreceiver"
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(NewReceiver),
	)
}

type provides struct {
	fx.Out

	Comp     eventplatformreceiver.Component
	Endpoint apihelper.AgentEndpointProvider
}

func streamEventPlatform(eventPlatformReceiver eventplatformreceiver.Component) func(w http.ResponseWriter, r *http.Request) {
	return apiutils.GetStreamFunc(func() apiutils.MessageReceiver { return eventPlatformReceiver }, "event platform payloads", "agent")
}

// NewReceiver returns a new event platform receiver.
func NewReceiver(hostname hostnameinterface.Component) provides { // nolint:revive
	epr := diagnostic.NewBufferedMessageReceiver(&epFormatter{}, hostname)
	return provides{
		Comp:     epr,
		Endpoint: apihelper.NewAgentEndpointProvider(streamEventPlatform(epr), "/stream-event-platform", "POST"),
	}
}
