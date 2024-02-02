// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package collectorimpl provides the implementation of the collector component.
package collectorimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/cmd/agent/common"
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/collector/collector"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/log"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

type dependencies struct {
	fx.In

	Lc     fx.Lifecycle
	Config config.Component
	Log    log.Component

	Demultiplexer demultiplexer.Component
}

type collectorImpl struct {
	pkgcollector.Collector
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newCollector),
		fx.Provide(func(collector collector.Component) optional.Option[collector.Component] {
			return optional.NewOption(collector)
		}),
	)
}

func newCollector(deps dependencies) collector.Component {
	return &collectorImpl{
		Collector: pkgcollector.NewCollector(deps.Demultiplexer, deps.Config.GetDuration("check_cancel_timeout"), common.GetPythonPaths()...),
	}
}
