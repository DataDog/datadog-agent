// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// The `python` build tag is used here as a proxy for "full agent, not IoT agent".
// See comp/anomalydetection/observer/fx/fx_noop.go for the stub used in IoT agent builds.

//go:build python

// Package fx defines the fx options for the observer component.
package fx

import (
	"go.uber.org/fx"

	observerdef "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/def"
	observerimpl "github.com/DataDog/datadog-agent/comp/anomalydetection/observer/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the observer component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			observerimpl.NewComponent,
		),
		fxutil.ProvideOptional[observerdef.Component](),
		fx.Invoke(func(_ observerdef.Component) {}),
	)
}
