// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf && nvml

// Package gpuimpl implements the gpu component interface
package gpuimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	gpu "github.com/DataDog/datadog-agent/comp/system-probe/gpu/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the gpu component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
	Telemetry      telemetry.Component
	WMeta          workloadmeta.Component
}

// Provides defines the output of the gpu component
type Provides struct {
	Comp   gpu.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new gpu component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.GPUMonitoring,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.GPUMonitoring.Fn(nil, sysmodule.FactoryDependencies{
				Telemetry: reqs.Telemetry,
				WMeta:     reqs.WMeta,
			})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
