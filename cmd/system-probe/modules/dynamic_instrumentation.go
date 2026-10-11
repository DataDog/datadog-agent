// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package modules

import (
	"context"
	"errors"
	"fmt"

	ddgostatsd "github.com/DataDog/datadog-go/v5/statsd"
	"google.golang.org/grpc"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	statsdimpl "github.com/DataDog/datadog-agent/comp/dogstatsd/statsd/impl"
	pkgconfighelper "github.com/DataDog/datadog-agent/pkg/config/helper"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	configutils "github.com/DataDog/datadog-agent/pkg/config/utils"
	dimod "github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	ddgrpc "github.com/DataDog/datadog-agent/pkg/util/grpc"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(DynamicInstrumentation) }

// DynamicInstrumentation is a system probe module which allows you to add instrumentation into
// running Go services without restarts.
var DynamicInstrumentation = &module.Factory{
	Name: config.DynamicInstrumentationModule,
	Fn: func(agentConfiguration *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
		config, err := dimod.NewConfig(agentConfiguration)
		if err != nil {
			return nil, fmt.Errorf("invalid dynamic instrumentation module configuration: %w", err)
		}
		ipcAddress, err := pkgconfighelper.GetIPCAddress(pkgconfigsetup.Datadog())
		if err != nil {
			return nil, fmt.Errorf("failed to get ipc address: %w", err)
		}
		client, err := ddgrpc.GetDDAgentSecureClient(
			context.Background(),
			ipcAddress,
			pkgconfighelper.GetIPCPort(pkgconfigsetup.Datadog()),
			deps.Ipc.GetTLSClientConfig().Clone(),
			grpc.WithPerRPCCredentials(ddgrpc.NewBearerTokenAuth(deps.Ipc.GetAuthToken())),
		)
		if err != nil {
			return nil, fmt.Errorf("failed to create gRPC client for RC subscription: %w", err)
		}

		opts := []dimod.Option{dimod.WithStatsd(deps.Statsd)}
		metricProbeStatsd, err := newMetricProbeStatsdClient(deps.CoreConfig)
		if err != nil {
			log.Warnf("metric probes will not report metrics: failed to create statsd client: %v", err)
		} else {
			opts = append(opts, dimod.WithMetricProbeStatsd(metricProbeStatsd))
		}
		m, err := dimod.NewModule(config, client, opts...)
		if err != nil {
			if metricProbeStatsd != nil {
				_ = metricProbeStatsd.Close()
			}
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

// newMetricProbeStatsdClient creates the DogStatsD client that metric probes
// report to. It is configured like system-probe's own client, except that it
// doesn't attach system-probe's container ID to the metrics: they describe
// other processes.
func newMetricProbeStatsdClient(cfg coreconfig.Component) (ddgostatsd.ClientInterface, error) {
	provides, err := statsdimpl.NewComponent()
	if err != nil {
		return nil, err
	}
	return provides.Comp.CreateForHostPort(
		configutils.GetBindHost(cfg),
		cfg.GetInt("dogstatsd_port"),
		ddgostatsd.WithoutOriginDetection(),
	)
}
