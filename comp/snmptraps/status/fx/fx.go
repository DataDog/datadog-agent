// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package fx provides the fx module for the status component.
package fx

import (
	"go.uber.org/fx"

	status "github.com/DataDog/datadog-agent/comp/snmptraps/status/def"
	statusimpl "github.com/DataDog/datadog-agent/comp/snmptraps/status/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(statusimpl.NewComponent),
		fxutil.ProvideOptional[status.Component](),
	)
}

// MockModule defines a fake Component for testing.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(statusimpl.NewMock),
	)
}
