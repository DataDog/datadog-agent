// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the remote health-platform issue loader component.
package fx

import (
	remoteimpl "github.com/DataDog/datadog-agent/comp/healthplatform/store/remote/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the Fx options for the remote issue loader.
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(remoteimpl.NewComponent),
	)
}
