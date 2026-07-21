// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package networkconfigmanagementimpl implements the networkconfigmanagement component interface
package networkconfigmanagementimpl

import (
	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkconfigmanagement "github.com/DataDog/datadog-agent/comp/networkconfigmanagement/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// CheckName is the name of the check
const CheckName = "network_config_management"

// Provides defines the output of the networkconfigmanagement component
type Provides struct {
	compdef.Out

	Comp              option.Option[networkconfigmanagement.Component]
	GetConfigEndpoint api.EndpointProvider `group:"agent_endpoint"`
	RollbackEndpoint  api.EndpointProvider `group:"agent_endpoint"`
}

// NewProvides populates a Provides from a component
func NewProvides(comp networkconfigmanagement.Component) Provides {
	return Provides{
		Comp:              option.New(comp),
		GetConfigEndpoint: api.NewAgentEndpointProvider(comp.GetConfigEndpointHandler(), "/ncm/config", "GET").Provider,
		RollbackEndpoint:  api.NewAgentEndpointProvider(comp.RollbackEndpointHandler(), "/ncm/rollback", "POST").Provider,
	}
}
