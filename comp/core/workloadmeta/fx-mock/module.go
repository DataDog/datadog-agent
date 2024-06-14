// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package fxmock provides the workloadmeta fx mock component for the Datadog Agent
package fxmock

import (
	"go.uber.org/fx"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	wmimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	wmmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// team: container-platform

// MockModule defines the fx options for the mock component.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(wmimpl.NewWorkloadMetaMock),
		fx.Provide(func(mock wmmock.Mock) wmdef.Component { return mock }),
		fx.Provide(func(mock wmmock.Mock) optional.Option[wmdef.Component] {
			return optional.NewOption[wmdef.Component](mock)
		}),
	)
}

// TODO(components): For consistency, let's add an isV2 field to
//                   Params, and leverage that in the constructor
//                   to return the right implementation.

// MockModuleV2 defines the fx options for the mock component.
func MockModuleV2() fxutil.Module {
	return fxutil.Component(
		fx.Provide(wmimpl.NewWorkloadMetaMockV2),
		fx.Provide(func(mock wmmock.Mock) wmdef.Component { return mock }),
		fx.Provide(func(mock wmmock.Mock) optional.Option[wmdef.Component] {
			return optional.NewOption[wmdef.Component](mock)
		}),
	)
}
