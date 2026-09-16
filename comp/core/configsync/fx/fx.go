// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package fx provides the fx module for the configsync component
package fx

import (
	"go.uber.org/fx"

	configsync "github.com/DataDog/datadog-agent/comp/core/configsync/def"
	configsyncimpl "github.com/DataDog/datadog-agent/comp/core/configsync/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module(params configsync.Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(configsyncimpl.NewComponent),
		fx.Supply(params),

		// configSync has no public methods so nobody depends on it; this forces instantiation
		// whenever Module() is included — the expected behavior for a background sync component.
		// fx.Invoke has no fxutil equivalent for this use case.
		fx.Invoke(func(_ configsync.Component) {}),
	)
}
