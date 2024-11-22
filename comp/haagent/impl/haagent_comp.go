// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package haagentimpl implements the haagent component interface
package haagentimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	haagent "github.com/DataDog/datadog-agent/comp/haagent/def"
)

// Requires defines the dependencies for the haagent component
type Requires struct {
	Logger      log.Component
	AgentConfig config.Component
}

// Provides defines the output of the haagent component
type Provides struct {
	Comp haagent.Component
}

// NewComponent creates a new haagent component
func NewComponent(reqs Requires) (Provides, error) {
	haAgentConfigs := newHaAgentConfigs(reqs.AgentConfig)
	provides := Provides{
		Comp: newHaAgentImpl(reqs.Logger, haAgentConfigs),
	}
	return provides, nil
}
