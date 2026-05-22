// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the health-platform component
package fx

import (
	"go.uber.org/fx"

	healthplatformdef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	coreimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			coreimpl.NewComponent,
		),
		fx.Provide(func(hp healthplatformdef.Component) option.Option[healthplatformdef.Component] {
			return option.New(hp)
		}),
	)
}
