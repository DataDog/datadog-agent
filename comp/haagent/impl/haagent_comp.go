// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagentimpl implements the haagent component interface
package haagentimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Requires defines the dependencies for the haagent component
type Requires struct {
	Logger      log.Component
	AgentConfig config.Component
	Hostname    hostnameinterface.Component
}

// Provides defines the output of the haagent component
type Provides struct {
	Comp       haagent.Component
	RCListener rctypes.ListenerProvider
}

// NewComponent creates a new haagent component
func NewComponent(reqs Requires) (Provides, error) {
	haAgentConf := newHaAgentConfigs(reqs.AgentConfig)
	haAgent := newHaAgentImpl(reqs.Logger, reqs.Hostname, haAgentConf)
	var rcListener rctypes.ListenerProvider
	if haAgent.Enabled() {
		reqs.Logger.Debug("Add HA Agent RCListener")
		rcListener.ListenerProvider = rctypes.RCListener{
			state.ProductHaAgent: haAgent.onHaAgentUpdate,
		}
	}

	provides := Provides{
		Comp:       haAgent,
		RCListener: rcListener,
	}
	return provides, nil
}
