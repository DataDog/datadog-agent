// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

// Package fx provides the fx module for the helmactions component.
package fx

import (
	uberfx "go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	helmactions "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/def"
	helmactionsimpl "github.com/DataDog/datadog-agent/comp/kubeactions/helmactions/impl"
	privateactionrunner "github.com/DataDog/datadog-agent/comp/privateactionrunner/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		// default; callers can override with their own fx.Supply higher up
		uberfx.Supply(helmactions.Params{}),
		// apiserver.GetAPIClient talks to the k8s API and fails outside a cluster;
		// only attempt it when the private action runner is actually enabled, so a
		// disabled PAR (or a unit test) never depends on apiserver reachability.
		uberfx.Provide(func(cfg config.Component) (*apiserver.APIClient, error) {
			if !cfg.GetBool(privateactionrunner.PAREnabled) {
				return nil, nil
			}
			return apiserver.GetAPIClient()
		}),

		fxutil.ProvideComponentConstructor(
			helmactionsimpl.NewComponent,
		),
		fxutil.ProvideOptional[helmactions.Component](),

		uberfx.Invoke(func(_ helmactions.Component) {}),
	)
}
