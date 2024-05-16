// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package npcollectorimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

type dependencies struct {
	fx.In
	Lc          fx.Lifecycle
	EpForwarder eventplatform.Component
	Logger      log.Component
	AgentConfig config.Component
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
	var scheduler *npCollectorImpl
	configs := newConfig(deps.AgentConfig)
	if configs.networkPathCollectorEnabled() {
		deps.Logger.Debugf("Network Path Scheduler enabled")
		scheduler = newNpCollectorImpl(deps.EpForwarder, configs)
	} else {
		deps.Logger.Debugf("Network Path Scheduler disabled")
		scheduler = newNoopNpCollectorImpl()
	}

	return provides{
		Comp: scheduler,
	}
}
