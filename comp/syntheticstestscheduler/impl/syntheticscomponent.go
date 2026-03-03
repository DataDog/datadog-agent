// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package syntheticstestschedulerimpl implements the syntheticstestsscheduler component interface
package syntheticstestschedulerimpl

// team: synthetics-executing

import (
	"context"
	"time"

	"github.com/DataDog/datadog-go/v5/statsd"

	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	syntheticstestscheduler "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Requires defines the dependencies for the syntheticstestscheduler component
type Requires struct {
	Lifecycle       compdef.Lifecycle
	EpForwarder     eventplatform.Component
	Logger          log.Component
	Traceroute      traceroute.Component
	AgentConfig     agentconfig.Component
	HostnameService hostname.Component
	Statsd          statsd.ClientInterface
}

// Provides defines the output of the syntheticstestscheduler component
type Provides struct {
	Comp       syntheticstestscheduler.Component
	RCListener rctypes.ListenerProvider
}

// NewComponent creates a new syntheticstestscheduler component
func NewComponent(reqs Requires) (Provides, error) {
	configs := newSchedulerConfigs(reqs.AgentConfig)
	if !configs.syntheticsSchedulerEnabled {
		reqs.Logger.Debugf("Synthetics scheduler disabled")
		var empty interface{}
		return Provides{
			RCListener: rctypes.ListenerProvider{ListenerProvider: rctypes.RCListener{}},
			Comp:       empty,
		}, nil
	}

	epForwarder, ok := reqs.EpForwarder.Get()
	if !ok {
		return Provides{}, reqs.Logger.Errorf("error getting EpForwarder")
	}

	scheduler := newSyntheticsTestScheduler(configs, epForwarder, reqs.Logger, reqs.HostnameService, time.Now, reqs.Statsd, reqs.Traceroute)

	var rcListener rctypes.ListenerProvider
	rcListener.ListenerProvider = rctypes.RCListener{
		state.ProductSyntheticsTest: scheduler.onConfigUpdate,
	}

	ctx := context.Background()
	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(context.Context) error {
			return scheduler.start(ctx)
		},
		OnStop: func(context.Context) error {
			scheduler.stop()
			return nil
		},
	})

	return Provides{
		RCListener: rcListener,
		Comp:       scheduler,
	}, nil
}
