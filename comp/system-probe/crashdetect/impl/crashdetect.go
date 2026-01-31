// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package crashdetectimpl implements the crashdetect component interface
package crashdetectimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	crashdetect "github.com/DataDog/datadog-agent/comp/system-probe/crashdetect/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the crashdetect component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
}

// Provides defines the output of the crashdetect component
type Provides struct {
	Comp   crashdetect.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new crashdetect component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.WinCrashProbe,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.WinCrashProbe.Fn(reqs.SysprobeConfig.SysProbeObject(), sysmodule.FactoryDependencies{})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
