// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf

// Package ebpfimpl implements the ebpf component interface
package ebpfimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	ebpf "github.com/DataDog/datadog-agent/comp/system-probe/ebpf/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the ebpf component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
}

// Provides defines the output of the ebpf component
type Provides struct {
	Comp   ebpf.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new ebpf component
func NewComponent(_ Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.EBPFProbe,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.EBPFProbe.Fn(nil, sysmodule.FactoryDependencies{})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
