// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In
	Lc          fx.Lifecycle
	EpForwarder eventplatform.Component
	Logger      log.Component
	AgentConfig config.Component
	Telemetry   telemetry.Component
}

type provides struct {
	fx.Out

	Comp npcollector.Component
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newNpCollector),
	)
}

func newNpCollector(deps dependencies) provides {
	var collector *npCollectorImpl

	configs := newConfig(deps.AgentConfig)
	if configs.networkPathCollectorEnabled() {
		deps.Logger.Debugf("Network Path Collector enabled")
		epForwarder, ok := deps.EpForwarder.Get()
		if !ok {
			deps.Logger.Errorf("Error getting EpForwarder")
			collector = newNoopNpCollectorImpl()
		} else {
			collector = newNpCollectorImpl(epForwarder, configs, deps.Logger, deps.Telemetry)
			deps.Lc.Append(fx.Hook{
				// No need for OnStart hook since NpCollector.Init() will be called by clients when needed.
				OnStart: func(context.Context) error {
					return collector.start()
				},
				OnStop: func(context.Context) error {
					collector.stop()
					return nil
				},
			})
		}
	} else {
		deps.Logger.Debugf("Network Path Collector disabled")
		collector = newNoopNpCollectorImpl()
	}

	return provides{
		Comp: collector,
	}
}
