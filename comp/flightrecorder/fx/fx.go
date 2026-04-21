// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the flightrecorder component.
package fx

import (
	flightrecorder "github.com/DataDog/datadog-agent/comp/flightrecorder/def"
	flightrecorderimpl "github.com/DataDog/datadog-agent/comp/flightrecorder/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for the flightrecorder component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			flightrecorderimpl.NewComponent,
		),
		fxutil.ProvideOptional[flightrecorder.Component](),
		// Force instantiation: flightrecorder is self-contained and not consumed
		// by any other component, so we must invoke it explicitly.
		fx.Invoke(func(_ flightrecorder.Component) {}),
	)
}
