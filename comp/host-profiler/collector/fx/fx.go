// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build hostprofiler

// Package fx provides the fx module for the collector component.
//
// This package defines the dependency injection module for the host profiler
// collector component using the fx framework.
package fx

import (
	collector "github.com/DataDog/datadog-agent/comp/host-profiler/collector/def"
	collectorimpl "github.com/DataDog/datadog-agent/comp/host-profiler/collector/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module(params collectorimpl.Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			collectorimpl.NewComponent,
		),
		fx.Supply(params),
		fxutil.ProvideOptional[collector.Component](),
	)
}
