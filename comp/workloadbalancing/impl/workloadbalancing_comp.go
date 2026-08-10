// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package workloadbalancingimpl implements the workloadbalancing component interface
package workloadbalancingimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	workloadbalancing "github.com/DataDog/datadog-agent/comp/workloadbalancing/def"
)

// Requires defines the dependencies for the workloadbalancing component
type Requires struct {
	Logger      log.Component
	AgentConfig config.Component
	Hostname    hostnameinterface.Component
}

// Provides defines the output of the workloadbalancing component
type Provides struct {
	Comp workloadbalancing.Component
}

// NewComponent creates a new workloadbalancing component
func NewComponent(reqs Requires) (Provides, error) {
	configs := newWorkloadBalancingConfigs(reqs.AgentConfig)

	return Provides{
		Comp: newWorkloadBalancingImpl(reqs.Logger, reqs.Hostname, configs),
	}, nil
}
