// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package fx provides the fx module for the ipc component
package fx

import (
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	noneimpl "github.com/DataDog/datadog-agent/comp/core/ipc/impl-none"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			noneimpl.NewNoopIPC,
		),
		fxutil.ProvideOptional[ipc.Component](),
	)
}
