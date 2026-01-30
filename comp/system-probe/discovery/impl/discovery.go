// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package discoveryimpl implements the discovery component interface
package discoveryimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/core/config"
	discovery "github.com/DataDog/datadog-agent/comp/system-probe/discovery/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the discovery component
type Requires struct {
	CoreConfig config.Component
}

// Provides defines the output of the discovery component
type Provides struct {
	Comp   discovery.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new discovery component
func NewComponent(_ Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.DiscoveryModule,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.DiscoveryModule.Fn(nil, sysmodule.FactoryDependencies{})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
