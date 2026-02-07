// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package discoveryimpl implements the discovery component interface
package discoveryimpl

import (
	"github.com/DataDog/datadog-agent/comp/core/config"
	discovery "github.com/DataDog/datadog-agent/comp/system-probe/discovery/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	discoverymodule "github.com/DataDog/datadog-agent/pkg/discovery/module"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
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
	mc := &moduleFactory{
		createFn: discoverymodule.NewDiscoveryModule,
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}

type moduleFactory struct {
	createFn func() (types.SystemProbeModule, error)
}

func (m *moduleFactory) Name() sysconfigtypes.ModuleName {
	return sysconfig.DiscoveryModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return []string{"discovery"}
}

func (m *moduleFactory) Create() (types.SystemProbeModule, error) {
	return m.createFn()
}

func (m *moduleFactory) NeedsEBPF() bool {
	return false
}

func (m *moduleFactory) OptionalEBPF() bool {
	return true
}
