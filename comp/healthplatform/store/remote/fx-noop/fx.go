// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the no-op remote health-platform issue loader component.
package fx

import (
	remotenoopimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/remote/noop-impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the Fx options for the no-op remote issue loader.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(remotenoopimpl.NewComponent),
	)
}
