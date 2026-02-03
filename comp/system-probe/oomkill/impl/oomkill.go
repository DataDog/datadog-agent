// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf

// Package oomkillimpl implements the oomkill component interface
package oomkillimpl

import (
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	oomkilldef "github.com/DataDog/datadog-agent/comp/system-probe/oomkill/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/oomkill"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Requires defines the dependencies for the oomkill component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
	Log            log.Component
}

// Provides defines the output of the oomkill component
type Provides struct {
	Comp   oomkilldef.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new oomkill component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			reqs.Log.Infof("Starting the OOM Kill probe")
			okp, err := oomkill.NewProbe(ebpf.NewConfig())
			if err != nil {
				return nil, fmt.Errorf("unable to start the OOM kill probe: %w", err)
			}
			return &oomKillModule{
				Probe: okp,
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
	return config.OOMKillProbeModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return nil
}

func (m *moduleFactory) Create() (types.SystemProbeModule, error) {
	return m.createFn()
}

func (m *moduleFactory) NeedsEBPF() bool {
	return true
}

func (m *moduleFactory) OptionalEBPF() bool {
	return false
}
