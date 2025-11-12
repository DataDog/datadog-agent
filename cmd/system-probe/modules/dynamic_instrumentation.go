// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"context"
	"errors"
	"fmt"

	"google.golang.org/grpc"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	dimod "github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
)

func init() { registerModule(DynamicInstrumentation) }

// DynamicInstrumentation is a system probe module which allows you to add instrumentation into
// running Go services without restarts.
var DynamicInstrumentation = &module.Factory{
	Name:             config.DynamicInstrumentationModule,
	ConfigNamespaces: []string{},
	Fn: func(agentConfiguration *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
		config, err := dimod.NewConfig(agentConfiguration)
		if err != nil {
			return nil, fmt.Errorf("invalid dynamic instrumentation module configuration: %w", err)
		}
		ipcAddress, err := pkgconfigsetup.GetIPCAddress(pkgconfigsetup.Datadog())
		if err != nil {
			return nil, fmt.Errorf("failed to get ipc address: %w", err)
		}
		client, err := ddgrpc.GetDDAgentSecureClient(
			context.Background(),
			ipcAddress,
			pkgconfigsetup.GetIPCPort(),
			deps.Ipc.GetTLSClientConfig().Clone(),
			grpc.WithPerRPCCredentials(ddgrpc.NewBearerTokenAuth(deps.Ipc.GetAuthToken())),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC client for RC subscription: %w", err)
		}

		m, err := dimod.NewModule(config, client)
		if err != nil {
			if errors.Is(err, ebpf.ErrNotImplemented) {
				return nil, module.ErrNotEnabled
			}
			return nil, err
		}

		return m, nil
	},
	NeedsEBPF: func() bool {
		return true
	},
}
