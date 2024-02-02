// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build test

// Package collectorimpl provides the implementation of the collector component.
package collectorimpl

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/collector"
	pkgcollector "github.com/DataDog/datadog-agent/pkg/collector"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(newMock),
		fx.Provide(func(collector collector.Component) optional.Option[collector.Component] {
			return optional.NewOption(collector)
		}),
	)
}

type mock struct {
	pkgcollector.Collector
}

func newMock() collector.Component {
	return &mock{}
}
