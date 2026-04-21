// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides a no-op fx module for the health platform component
package fx

import (
	healthplatformnoopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/core/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module provides a no-op health platform component (for processes that don't use it)
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(healthplatformnoopimpl.NewNoopComponent),
	)
}
