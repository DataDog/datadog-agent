// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package lookbackfx provides the fx module for the lookback component.
package lookbackfx

import (
	"go.uber.org/fx"

	lookbackimpl "github.com/DataDog/datadog-agent/comp/lookback/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module is the fx module for the lookback component.
func Module() fxutil.Module {
	return fxutil.Component(
		fx.Provide(lookbackimpl.NewComponent),
	)
}
