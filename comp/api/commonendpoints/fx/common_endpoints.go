// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides Fx.Module with the common Agent API endpoints
package fx

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	"github.com/DataDog/datadog-agent/comp/api/commonendpoints/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type provider struct {
	fx.Out

	VersionEndpoint  api.AgentEndpointProvider
	HostnameEndpoint api.AgentEndpointProvider
	StopEndpoint     api.AgentEndpointProvider
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Supply(
			provider{
				VersionEndpoint:  api.NewAgentEndpointProvider(common.GetVersion, "/version", "GET"),
				HostnameEndpoint: api.NewAgentEndpointProvider(impl.GetHostname, "/hostname", "GET"),
				StopEndpoint:     api.NewAgentEndpointProvider(impl.StopAgent, "/stop", "POST"),
			}),
	)
}
