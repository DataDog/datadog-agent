// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package syntheticstestschedulerimpl

import (
	"context"
	"time"

	agentconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	rctypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	syntheticstestscheduler "github.com/DataDog/datadog-agent/comp/syntheticstestscheduler/def"
	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
)

// Requires defines the dependencies for the syntheticstestscheduler component
type Requires struct {
	Lifecycle   compdef.Lifecycle
	EpForwarder eventplatform.Component
	Logger      log.Component
	Telemetry   telemetry.Component
	AgentConfig agentconfig.Component
}

// Provides defines the output of the syntheticstestscheduler component
type Provides struct {
	Comp                    syntheticstestscheduler.Component
	RCListener              rctypes.ListenerProvider
	SyntheticsTestScheduler *SyntheticsTestScheduler
}

// NewComponent creates a new syntheticstestscheduler component
func NewComponent(reqs Requires) (Provides, error) {
	configs := newSchedulerConfigs(reqs.AgentConfig)
	if !configs.syntheticsSchedulerEnabled {
		reqs.Logger.Debugf("Synthetics scheduler disabled")
		return Provides{}, nil
	}

	epForwarder, ok := reqs.EpForwarder.Get()
	if !ok {
		return Provides{}, reqs.Logger.Errorf("error getting EpForwarder")
	}

	scheduler, err := newSyntheticsTestScheduler(
		configs,
		epForwarder,
		reqs.Logger,
		reqs.AgentConfig.GetString("config_id"),
		time.Now)
	if err != nil {
		return Provides{}, err
	}

	var rcListener rctypes.ListenerProvider
	rcListener.ListenerProvider = rctypes.RCListener{
		state.ProductSyntheticsTest: scheduler.onConfigUpdate,
	}

	reqs.Lifecycle.Append(compdef.Hook{
		OnStart: func(ctx context.Context) error {
			return scheduler.start()
		},
		OnStop: func(context.Context) error {
			scheduler.stop()
			return nil
		},
	})

	return Provides{
		RCListener:              rcListener,
		SyntheticsTestScheduler: scheduler,
	}, nil
}
