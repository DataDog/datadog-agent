// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"context"

	"github.com/DataDog/datadog-go/v5/statsd"

	"github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	npcollector "github.com/DataDog/datadog-agent/comp/networkpath/npcollector/def"
	traceroute "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	nooprdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/impl-none"
)

type dependencies struct {
	compdef.In
	Lc          compdef.Lifecycle
	EpForwarder eventplatform.Component
	Traceroute  traceroute.Component
	Logger      log.Component
	AgentConfig config.Component
	RDNSQuerier rdnsquerier.Component
	Statsd      statsd.ClientInterface
}

// Provides defines the output of the npcollector component
type Provides struct {
	compdef.Out

	Comp npcollector.Component
}

// NewNpCollector creates a new npcollector component.
func NewNpCollector(deps dependencies) Provides {
	var collector *npCollectorImpl

	configs := newConfig(deps.AgentConfig, deps.Logger)
	deps.Logger.Debugf("Network Path Configs: %+v", configs)
	if configs.networkPathCollectorEnabled() {
		deps.Logger.Debug("Network Path Collector enabled")

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
			collector = newNpCollectorImpl(epForwarder, configs, deps.Traceroute, deps.Logger, rdnsQuerier, deps.Statsd)
			deps.Lc.Append(compdef.Hook{
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

	return Provides{
		Comp: collector,
	}
}
