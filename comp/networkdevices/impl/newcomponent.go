// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package networkdevicesimpl implements the networkdevices component interface.
package networkdevicesimpl

import (
	"context"
	"sync"

	"golang.org/x/sync/semaphore"

	api "github.com/DataDog/datadog-agent/comp/api/api/def"
	config "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	networkdevices "github.com/DataDog/datadog-agent/comp/networkdevices/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/probe/pingprobe"
)

// workersConfigKey sizes the interactive probe budget, separate from the
// discovery sweep budget.
const workersConfigKey = "network_devices.connectivity_check.workers"

// Requires defines the dependencies for the networkdevices component.
type Requires struct {
	compdef.In

	Lifecycle compdef.Lifecycle
	Logger    log.Component
	Config    config.Component
}

// Provides defines the output of the networkdevices component.
type Provides struct {
	compdef.Out

	Comp                      networkdevices.Component
	ConnectivityCheckEndpoint api.EndpointProvider `group:"agent_endpoint"`
}

type networkDevicesImpl struct {
	logger     log.Component
	maxWorkers int
	sem        *semaphore.Weighted

	pingOnce sync.Once
	ping     pingprobe.Capability
}

func newImpl(logger log.Component, maxWorkers int) *networkDevicesImpl {
	if maxWorkers < 1 {
		maxWorkers = 1
	}
	return &networkDevicesImpl{
		logger:     logger,
		maxWorkers: maxWorkers,
		sem:        semaphore.NewWeighted(int64(maxWorkers)),
	}
}

// NewComponent creates a new networkdevices component.
func NewComponent(reqs Requires) Provides {
	comp := newImpl(reqs.Logger, reqs.Config.GetInt(workersConfigKey))
	reqs.Lifecycle.Append(compdef.Hook{OnStart: func(context.Context) error {
		comp.pingCapability()
		return nil
	}})

	return Provides{
		Comp: comp,
		ConnectivityCheckEndpoint: api.NewAgentEndpointProvider(
			comp.ConnectivityCheckEndpointHandler(),
			"/networkdevices/connectivity-check",
			"POST",
		).Provider,
	}
}
