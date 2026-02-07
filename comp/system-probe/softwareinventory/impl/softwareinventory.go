// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

// Package softwareinventoryimpl implements the softwareinventory component interface
package softwareinventoryimpl

import (
	softwareinventory "github.com/DataDog/datadog-agent/comp/system-probe/softwareinventory/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
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
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			return &softwareInventoryModule{}, nil
		},
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
	return config.SoftwareInventoryModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return []string{"software_inventory"}
}

func (m *moduleFactory) Create() (types.SystemProbeModule, error) {
	return m.createFn()
}
