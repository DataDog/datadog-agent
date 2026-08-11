// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package fx provides the fx module for the installer status api component.
package fx

import (
	uberfx "go.uber.org/fx"

	statusapi "github.com/DataDog/datadog-agent/comp/updater/statusapi/def"
	statusapiimpl "github.com/DataDog/datadog-agent/comp/updater/statusapi/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			statusapiimpl.NewComponent,
		),

		// Nothing depends on statusapi and fx only instantiates components when they're
		// needed, so a dummy invoke is required to force its instantiation. Without it,
		// adding 'statusapifx.Module()' to a main would silently do nothing.
		uberfx.Invoke(func(_ statusapi.Component) {}),
	)
}
