// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the loopback component.
// Uses fx.Provide directly (not fxutil.ProvideComponentConstructor) so that
// the group:"hook" tag on Requires.MetricHooks is preserved by dig's reflection.
package fx

import (
	"go.uber.org/fx"

	loopbackimpl "github.com/DataDog/datadog-agent/comp/loopback/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the loopback component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(loopbackimpl.NewComponent),
	)
}
