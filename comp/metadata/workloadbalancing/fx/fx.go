// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the workloadbalancing metadata component
package fx

import (
	workloadbalancing "github.com/DataDog/datadog-agent/comp/metadata/workloadbalancing/def"
	workloadbalancingimpl "github.com/DataDog/datadog-agent/comp/metadata/workloadbalancing/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			workloadbalancingimpl.NewComponent,
		),
		fxutil.ProvideOptional[workloadbalancing.Component](),
	)
}
