// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the example task manager component.
package fx

import (
	"go.uber.org/fx"

	exampletaskmanager "github.com/DataDog/datadog-agent/comp/exampletaskmanager/def"
	exampletaskmanagerimpl "github.com/DataDog/datadog-agent/comp/exampletaskmanager/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the example task manager component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			exampletaskmanagerimpl.NewComponent,
		),
		fx.Invoke(func(_ exampletaskmanager.Component) {}),
	)
}
