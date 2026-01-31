// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux

// Package pingimpl implements the ping component interface
package pingimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	ping "github.com/DataDog/datadog-agent/comp/system-probe/ping/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the ping component
type Requires struct{}

// Provides defines the output of the ping component
type Provides struct {
	Comp   ping.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new ping component
func NewComponent(_ Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.Pinger,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.Pinger.Fn(nil, sysmodule.FactoryDependencies{})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
