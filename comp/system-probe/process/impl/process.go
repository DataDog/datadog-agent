// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package processimpl implements the process component interface
package processimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	process "github.com/DataDog/datadog-agent/comp/system-probe/process/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the process component
type Requires struct {
}

// Provides defines the output of the process component
type Provides struct {
	Comp   process.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new process component
func NewComponent(_ Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.Process,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.Process.Fn(nil, sysmodule.FactoryDependencies{})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
