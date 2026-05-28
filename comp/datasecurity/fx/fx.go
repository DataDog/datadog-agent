// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the data security component.
package fx

import (
	"go.uber.org/fx"

	datasecurity "github.com/DataDog/datadog-agent/comp/datasecurity/def"
	datasecurityimpl "github.com/DataDog/datadog-agent/comp/datasecurity/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for the data security component.
//
// Nothing depends on the component yet, so we force-instantiate it with
// fx.Invoke; otherwise the constructor (and therefore the RC subscription)
// would never run.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			datasecurityimpl.NewComponent,
		),
		fx.Invoke(func(_ datasecurity.Component) {}),
	)
}
