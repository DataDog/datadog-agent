// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf

// Package oomkillimpl implements the oomkill component interface
package oomkillimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	oomkill "github.com/DataDog/datadog-agent/comp/system-probe/oomkill/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the oomkill component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
}

// Provides defines the output of the oomkill component
type Provides struct {
	Comp   oomkill.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new oomkill component
func NewComponent(_ Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.OOMKillProbe,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.OOMKillProbe.Fn(nil, sysmodule.FactoryDependencies{})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
