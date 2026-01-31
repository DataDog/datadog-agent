// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package tracerouteimpl implements the traceroute component interface
package tracerouteimpl

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/modules"
	traceroutecomp "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/module"
	traceroute "github.com/DataDog/datadog-agent/comp/system-probe/traceroute/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	sysmodule "github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
)

// Requires defines the dependencies for the traceroute component
type Requires struct {
	Traceroute traceroutecomp.Component
}

// Provides defines the output of the traceroute component
type Provides struct {
	Comp   traceroute.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new traceroute component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &module.Component{
		Factory: modules.Traceroute,
		CreateFn: func() (types.SystemProbeModule, error) {
			return modules.Traceroute.Fn(nil, sysmodule.FactoryDependencies{
				Traceroute: reqs.Traceroute,
			})
		},
	}
	provides := Provides{
		Module: types.ProvidesSystemProbeModule{Component: mc},
		Comp:   mc,
	}
	return provides, nil
}
