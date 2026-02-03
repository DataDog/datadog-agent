// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Package tracerouteimpl implements the traceroute component interface
package tracerouteimpl

import (
	traceroutecomp "github.com/DataDog/datadog-agent/comp/networkpath/traceroute/def"
	traceroute "github.com/DataDog/datadog-agent/comp/system-probe/traceroute/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
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
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			return &tracerouteImpl{
				runner: reqs.Traceroute,
			}, nil
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
	return config.TracerouteModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return []string{"traceroute"}
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
