// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the lookback component.
// Uses fx.Provide directly (not fxutil.ProvideComponentConstructor) so that
// the group:"hook" tag on Requires.MetricHooks is preserved by dig's reflection.
package fx

import (
	"go.uber.org/fx"

	lookbackimpl "github.com/DataDog/datadog-agent/comp/lookback/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the lookback component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(lookbackimpl.NewComponent),
	)
}
