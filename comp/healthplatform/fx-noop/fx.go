// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx exposes the healthplatform noop component to FX
package fx

import (
	healthplatformnoopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module specifies the healthplatform noop module.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			healthplatformnoopimpl.NewComponent,
		),
	)
}
