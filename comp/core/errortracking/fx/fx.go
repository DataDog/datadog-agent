// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package fx provides the fx module for the errortracking component.
package fx

import (
	errortracking "github.com/DataDog/datadog-agent/comp/core/errortracking/def"
	errortrackingimpl "github.com/DataDog/datadog-agent/comp/core/errortracking/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module registers the COAT error tracking sender component with Fx.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			errortrackingimpl.NewComponent,
		),
		fxutil.ProvideOptional[errortracking.Component](),
	)
}
