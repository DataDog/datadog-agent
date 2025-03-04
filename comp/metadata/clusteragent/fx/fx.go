// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the cluster-agent metadata component
package fx

import (
	clusteragent "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/def"
	clusteragentimpl "github.com/DataDog/datadog-agent/comp/metadata/clusteragent/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			clusteragentimpl.NewComponent,
		),
		fxutil.ProvideOptional[clusteragent.Component](),
		fx.Invoke(func(_ clusteragent.Component) {}),
	)
}
