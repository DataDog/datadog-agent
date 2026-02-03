// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf

// Package ebpfimpl implements the ebpf component interface
package ebpfimpl

import (
	"fmt"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	ebpf "github.com/DataDog/datadog-agent/comp/system-probe/ebpf/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/ebpfcheck"
	ddebpf "github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Requires defines the dependencies for the ebpf component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
	Log            log.Component
}

// Provides defines the output of the ebpf component
type Provides struct {
	Comp   ebpf.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new ebpf component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			reqs.Log.Infof("Starting the ebpf probe")
			okp, err := ebpfcheck.NewProbe(ddebpf.NewConfig())
			if err != nil {
				return nil, fmt.Errorf("unable to start the ebpf probe: %w", err)
			}
			return &ebpfModule{
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
	return config.EBPFModule
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
