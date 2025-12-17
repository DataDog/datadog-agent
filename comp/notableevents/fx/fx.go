// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build windows

// Package fx provides fx wiring for the notable events component
package fx

import (
	notableevents "github.com/DataDog/datadog-agent/comp/notableevents/def"
	notableeventsimpl "github.com/DataDog/datadog-agent/comp/notableevents/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			notableeventsimpl.NewComponent,
		),
		// Force the instantiation of the component, uses fx.Lifecycle for start/stop
		fx.Invoke(func(_ notableevents.Component) {}),
	)
}
