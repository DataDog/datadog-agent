// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the par-executor component.
package fx

import (
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	executorimpl "github.com/DataDog/datadog-agent/comp/privateactionrunner/executor/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for the par-executor component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			executorimpl.NewComponent,
		),
		fxutil.ProvideOptional[privateactionrunner.Component](),
		fx.Invoke(func(_ privateactionrunner.Component) {}),
	)
}
