// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build python

// Package fx provides the FX module for smart adaptive sampling.
package fx

import (
	"go.uber.org/fx"

	smartadaptivesampling "github.com/DataDog/datadog-agent/comp/logs/smartadaptivesampling/def"
	smartadaptivesamplingimpl "github.com/DataDog/datadog-agent/comp/logs/smartadaptivesampling/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the FX options for smart adaptive sampling.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(smartadaptivesamplingimpl.NewComponent),
		fx.Invoke(func(_ smartadaptivesampling.Component) {}),
	)
}
