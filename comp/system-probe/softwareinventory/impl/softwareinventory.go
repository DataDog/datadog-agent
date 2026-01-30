// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package softwareinventoryimpl implements the softwareinventory component interface
package softwareinventoryimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	softwareinventory "github.com/DataDog/datadog-agent/comp/system-probe/softwareinventory/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the softwareinventory component
type Requires struct {
}

// Provides defines the output of the softwareinventory component
type Provides struct {
	Comp   softwareinventory.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new softwareinventory component
func NewComponent(_ Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.SoftwareInventory,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.SoftwareInventory.Fn(nil, sysmodule.FactoryDependencies{})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
