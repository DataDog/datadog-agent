// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package fx provides the fx module for the kubeactions component.
package fx

import (
	uberfx "go.uber.org/fx"

	kubeactions "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/def"
	kubeactionsimpl "github.com/DataDog/datadog-agent/comp/kubeactions/kubeactions/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		uberfx.Supply(kubeactions.Params{}), // default; callers can override with their own fx.Supply higher up
		// *apiserver.APIClient is provided by the sibling helmactions module in the
		// same kubeactions bundle (comp/kubeactions/bundle.go). We consume that
		// shared provider instead of registering our own, because fx rejects two
		// providers of the same type. That provider yields nil when the private
		// action runner is disabled, so NewComponent tolerates a nil APIClient.

		fxutil.ProvideComponentConstructor(
			kubeactionsimpl.NewComponent,
		),
		fxutil.ProvideOptional[kubeactions.Component](),

		uberfx.Invoke(func(kubeactions.Component) {}),
	)
}
