// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the SBOM usage component.
package fx

import (
	"go.uber.org/fx"

	usage "github.com/DataDog/datadog-agent/comp/sbom/usage/def"
	usageimpl "github.com/DataDog/datadog-agent/comp/sbom/usage/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			usageimpl.NewComponent,
		),
		fx.Provide(usageimpl.NewTrivySource),
		fxutil.ProvideOptional[usage.Component](),
	)
}
