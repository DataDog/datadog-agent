// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package networkdevicesimpl implements the networkdevices component interface.
package networkdevicesimpl

import (
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkdevices "github.com/DataDog/datadog-agent/comp/networkdevices/def"
)

// Requires defines the dependencies for the networkdevices component.
type Requires struct {
	compdef.In
	Logger log.Component
}

// Provides defines the output of the networkdevices component.
type Provides struct {
	compdef.Out

	Comp                      networkdevices.Component
	ConnectivityCheckEndpoint api.EndpointProvider `group:"agent_endpoint"`
}

type networkDevicesImpl struct {
	logger log.Component
}

// NewComponent creates a new networkdevices component.
func NewComponent(reqs Requires) Provides {
	comp := &networkDevicesImpl{logger: reqs.Logger}
	return Provides{
		Comp: comp,
		ConnectivityCheckEndpoint: api.NewAgentEndpointProvider(
			comp.ConnectivityCheckEndpointHandler(),
			"/networkdevices/connectivity-check",
			"POST",
		).Provider,
	}
}
