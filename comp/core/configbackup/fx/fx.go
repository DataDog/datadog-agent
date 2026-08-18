// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the configbackup component
package fx

import (
	"go.uber.org/fx"

	configbackup "github.com/DataDog/datadog-agent/comp/core/configbackup/def"
	configbackupimpl "github.com/DataDog/datadog-agent/comp/core/configbackup/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(configbackupimpl.NewComponent),

		// configbackup has no public methods so nobody depends on it; this forces instantiation
		// whenever Module() is included — the expected behavior for a one-shot startup side effect.
		// fx.Invoke has no fxutil equivalent for this use case.
		fx.Invoke(func(_ configbackup.Component) {}),
	)
}
