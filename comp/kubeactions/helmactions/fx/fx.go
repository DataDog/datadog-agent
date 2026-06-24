// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the helmactions component.
package fx

import (
	uberfx "go.uber.org/fx"

	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	helmactionsimpl "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			helmactionsimpl.NewComponent,
		),
		fxutil.ProvideOptional[helmactions.Component](),

		uberfx.Invoke(func(_ helmactions.Component) {}),
	)
}
