// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"context"

	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	nooprdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/impl-none"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type dependencies struct {
	fx.In
	Lc          fx.Lifecycle
	EpForwarder eventplatform.Component
	Logger      log.Component
	AgentConfig config.Component
	Telemetry   telemetry.Component
	RDNSQuerier rdnsquerier.Component
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

		// Note that multiple components can share the same rdnsQuerier instance.  If any of them have
		// reverse DNS enrichment enabled then the deps.RDNSQuerier component passed here will be an
		// active instance.  However, we also need to check here whether the network path component has
		// reverse DNS enrichment enabled to decide whether to use the passed instance or to override
		// it with a noop implementation.
		rdnsQuerier := deps.RDNSQuerier
		if !configs.reverseDNSEnabled {
			deps.Logger.Infof("Reverse DNS enrichment is disabled for Network Path Collector")
			rdnsQuerier = nooprdnsquerier.NewNone().Comp
		}

		epForwarder, ok := deps.EpForwarder.Get()
		if !ok {
			deps.Logger.Errorf("Error getting EpForwarder")
			collector = newNoopNpCollectorImpl()
		} else {
			collector = newNpCollectorImpl(epForwarder, configs, deps.Logger, deps.Telemetry, rdnsQuerier)
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
