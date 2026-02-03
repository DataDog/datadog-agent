// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build linux && linux_bpf

// Package dynamicinstrumentationimpl implements the dynamicinstrumentation component interface
package dynamicinstrumentationimpl

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"

	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/sysprobeconfig"
	dynamicinstrumentation "github.com/DataDog/datadog-agent/comp/system-probe/dynamicinstrumentation/def"
	"github.com/DataDog/datadog-agent/comp/system-probe/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	dimod "github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	sysconfig "github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

// Requires defines the dependencies for the dynamicinstrumentation component
type Requires struct {
	CoreConfig     config.Component
	SysprobeConfig sysprobeconfig.Component
	Ipc            ipc.Component
}

// Provides defines the output of the dynamicinstrumentation component
type Provides struct {
	Comp   dynamicinstrumentation.Component
	Module types.ProvidesSystemProbeModule
}

// NewComponent creates a new dynamicinstrumentation component
func NewComponent(reqs Requires) (Provides, error) {
	mc := &moduleFactory{
		createFn: func() (types.SystemProbeModule, error) {
			config, err := dimod.NewConfig(reqs.SysprobeConfig.SysProbeObject())
			if err != nil {
				return nil, fmt.Errorf("invalid dynamic instrumentation module configuration: %w", err)
			}
			ipcAddress, err := pkgconfigsetup.GetIPCAddress(reqs.CoreConfig)
			if err != nil {
				return nil, fmt.Errorf("failed to get ipc address: %w", err)
			}
			client, err := ddgrpc.GetDDAgentSecureClient(
				context.Background(),
				ipcAddress,
				pkgconfigsetup.GetIPCPort(),
				reqs.Ipc.GetTLSClientConfig().Clone(),
				grpc.WithPerRPCCredentials(ddgrpc.NewBearerTokenAuth(reqs.Ipc.GetAuthToken())),
			)
			if err != nil {
				return nil, fmt.Errorf("failed to create gRPC client for RC subscription: %w", err)
			}

			m, err := dimod.NewModule(config, client)
			if err != nil {
				if errors.Is(err, ebpf.ErrNotImplemented) {
					return nil, types.ErrNotEnabled
				}
				return nil, err
			}

			return m, nil
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
	return sysconfig.DynamicInstrumentationModule
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
