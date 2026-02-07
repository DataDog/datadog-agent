// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf

// Package tcpqueuelengthimpl implements the tcpqueuelength component interface
package tcpqueuelengthimpl

import (
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	tcpqueuelengthdef "github.com/DataDog/datadog-agent/comp/system-probe/tcpqueuelength/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/ebpf/probe/tcpqueuelength"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Requires defines the dependencies for the tcpqueuelength component
type Requires struct {
	SysprobeConfig sysprobeconfig.Component
}

// Provides defines the output of the tcpqueuelength component
type Provides struct {
	Comp   tcpqueuelengthdef.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new tcpqueuelength component
func NewComponent(_ Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			t, err := tcpqueuelength.NewTracer(ebpf.NewConfig())
			if err != nil {
				return nil, fmt.Errorf("unable to start the TCP queue length tracer: %w", err)
			}
			return &tcpQueueLengthModule{
				Tracer: t,
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
	return config.TCPQueueLengthTracerModule
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
