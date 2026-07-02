// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package noopimpl provides a no-op fx module for the collector component.
// It is intentionally lightweight — it only imports the def package so that
// callers do not transitively depend on the full collector implementation.
package noopimpl

import (
	"go.uber.org/fx"

	collector "github.com/DataDog/datadog-agent/comp/collector/collector/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

// NoneModule returns a None optional type for collector.Component.
//
// This helper allows code that needs a disabled Optional type for the collector to get it.
// The helper is split from the implementation to avoid linking with the implementation.
func NoneModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() option.Option[collector.Component] {
			return option.None[collector.Component]()
		}),
	)
}
