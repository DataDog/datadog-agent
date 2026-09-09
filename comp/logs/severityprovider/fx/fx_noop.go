// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !python

// Package fx provides the FX module for the severity provider.
package fx

import (
	"go.uber.org/fx"

	severityprovider "github.com/DataDog/datadog-agent/comp/logs/severityprovider/def"
	severityproviderimpl "github.com/DataDog/datadog-agent/comp/logs/severityprovider/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the unavailable severity provider component for builds without Python.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(severityproviderimpl.NewComponent),
		fx.Invoke(func(provider severityprovider.Component) {
			severityprovider.SetSeverityProvider(provider.Current)
		}),
	)
}
