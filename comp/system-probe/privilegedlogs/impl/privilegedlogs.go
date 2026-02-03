// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package privilegedlogsimpl implements the privilegedlogs component interface
package privilegedlogsimpl

import (
	privilegedlogs "github.com/DataDog/datadog-agent/comp/system-probe/privilegedlogs/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	privilegedlogsmodule "github.com/DataDog/datadog-agent/pkg/privileged-logs/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Requires defines the dependencies for the privilegedlogs component
type Requires struct{}

// Provides defines the output of the privilegedlogs component
type Provides struct {
	Comp   privilegedlogs.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new privilegedlogs component
func NewComponent(_ Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			return privilegedlogsmodule.NewPrivilegedLogsModule(), nil
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
	return config.PrivilegedLogsModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return nil
}

func (m *moduleFactory) Create() (types.SystemProbeModule, error) {
	return m.createFn()
}

func (m *moduleFactory) NeedsEBPF() bool {
	return false
}

func (m *moduleFactory) OptionalEBPF() bool {
	return false
}
