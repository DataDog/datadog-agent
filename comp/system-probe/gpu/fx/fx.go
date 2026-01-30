// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf && nvml

// Package fx provides the fx module for the gpu component
package fx

import (
	gpu "github.com/DataDog/datadog-agent/comp/system-probe/gpu/def"
	gpuimpl "github.com/DataDog/datadog-agent/comp/system-probe/gpu/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Module defines the fx options for this component
func Module() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			gpuimpl.NewComponent,
		),
		fxutil.ProvideOptional[gpu.Component](),
	)
}
