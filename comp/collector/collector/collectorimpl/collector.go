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
	"github.com/DataDog/datadog-agent/comp/core/status"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	collectorStatus "github.com/DataDog/datadog-agent/pkg/status/collector"
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

type provides struct {
	fx.Out

	Comp         collector.Component
	OptionalComp optional.Option[collector.Component]
	Provider     status.InformationProvider
}

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newCollector),
	)
}

// ModuleNoneCollector defines a module with a none collector and a status provider.
func ModuleNoneCollector() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newNoneCollector),
	)
}

func newCollector(deps dependencies) provides {
	c := &collectorImpl{
		Collector: pkgcollector.NewCollector(deps.Demultiplexer, deps.Config.GetDuration("check_cancel_timeout"), common.GetPythonPaths()...),
	}

	return provides{
		Comp:         c,
		OptionalComp: optional.NewOption[collector.Component](c),
		Provider:     status.NewInformationProvider(collectorStatus.Provider{}),
	}
}

func newNoneCollector() provides {
	return provides{
		OptionalComp: optional.NewNoneOption[collector.Component](),
		Provider:     status.NewInformationProvider(collectorStatus.Provider{}),
	}
}
