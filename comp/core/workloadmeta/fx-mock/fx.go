// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the workloadmeta fx mock component for the Datadog Agent
package fx

import (
	"go.uber.org/fx"

	wmdef "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	wmimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	wmmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// team: container-platform

// MockModule defines the fx options for the mock component.
func MockModule(params wmdef.Params) fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(wmimpl.NewWorkloadMetaMock),
		fx.Provide(func(mock wmmock.Mock) wmdef.Component { return mock }),
		fx.Provide(func(mock wmmock.Mock) option.Option[wmdef.Component] {
			return option.New[wmdef.Component](mock)
		}),
		fx.Supply(params),
	)
}
