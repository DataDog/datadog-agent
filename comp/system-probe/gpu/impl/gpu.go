// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf && nvml

// Package gpuimpl implements the gpu component interface
package gpuimpl

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	gpudef "github.com/DataDog/datadog-agent/comp/system-probe/gpu/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/processeventconsumer"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	"github.com/DataDog/datadog-agent/pkg/gpu"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	gpuconfigconsts "github.com/DataDog/datadog-agent/pkg/gpu/config/consts"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

// Requires defines the dependencies for the gpu component
type Requires struct {
	SysprobeConfig  sysprobeconfig.Component
	Telemetry       telemetry.Component
	WMeta           workloadmeta.Component
	ProcessConsumer processeventconsumer.ProcessEventConsumer `name:"gpu"`
}

// Provides defines the output of the gpu component
type Provides struct {
	Comp   gpudef.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new gpu component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			if reqs.ProcessConsumer.Get() == nil {
				return nil, errors.New("process event consumer not initialized")
			}

			c := gpuconfig.New()

			ctx, cancel := context.WithCancel(context.Background())

			if c.ConfigureCgroupPerms {
				configureCgroupPermissions(ctx, c.CgroupReapplyInterval, c.CgroupReapplyInfinitely)
			}

			probeDeps := gpu.ProbeDependencies{
				Telemetry:            reqs.Telemetry,
				WorkloadMeta:         reqs.WMeta,
				ProcessEventConsumer: reqs.ProcessConsumer,
			}
			p, err := gpu.NewProbe(c, probeDeps)
			if err != nil {
				cancel()
				return nil, fmt.Errorf("unable to start %s: %w", config.GPUMonitoringModule, err)
			}

			return &GPUMonitoringModule{
				Probe:         p,
				contextCancel: cancel,
				context:       ctx,
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
	return config.GPUMonitoringModule
}

func (m *moduleFactory) ConfigNamespaces() []string {
	return []string{gpuconfigconsts.GPUNS}
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
