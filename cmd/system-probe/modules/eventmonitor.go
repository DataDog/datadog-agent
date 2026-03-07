// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"context"
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/network/sender"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var eventMonitorModuleConfigNamespaces = []string{"event_monitoring_config", "runtime_security_config"}

func createEventMonitorModule(_ *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
	emconfig := emconfig.NewConfig()

	secconfig, err := secconfig.NewConfig()
	if err != nil {
		log.Errorf("invalid probe configuration: %v", err)
		return nil, module.ErrNotEnabled
	}

	opts := eventmonitor.Opts{}
	opts.StatsdClient = deps.Statsd
	opts.ProbeOpts.EnvsVarResolutionEnabled = emconfig.EnvVarsResolutionEnabled
	opts.ProbeOpts.Tagger = deps.Tagger

	if secconfig.Probe != nil {
		opts.ProbeOpts.GenerateEventProcessingTimeMetrics = secconfig.Probe.GenerateEventProcessingTimeMetrics
	}
	secmoduleOpts := secmodule.Opts{}

	// adapt options
	if secconfig.RuntimeSecurity.IsRuntimeEnabled() {
		secmodule.UpdateEventMonitorOpts(&opts, secconfig)
	} else {
		secmodule.DisableRuntimeSecurity(secconfig)
	}

	hostname, err := deps.Hostname.Get(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to get hostname: %w", err)
	}
	if hostname == "" {
		return nil, errors.New("hostname from core agent is empty")
	}

	evm, err := eventmonitor.NewEventMonitor(emconfig, secconfig, hostname, opts)
	if err != nil {
		log.Errorf("error initializing event monitoring module: %v", err)
		return nil, module.ErrNotEnabled
	}

	if secconfig.RuntimeSecurity.IsRuntimeEnabled() {
		cws, err := secmodule.NewCWSConsumer(evm, secconfig.RuntimeSecurity, deps.WMeta, deps.FilterStore, secmoduleOpts, deps.Compression, deps.Ipc, hostname)
		if err != nil {
			return nil, err
		}
		evm.RegisterEventConsumer(cws)
		evm.SetCWSStatusProvider(cws)
		log.Info("event monitoring cws consumer initialized")
	}

	netconfig := netconfig.New()
	// only add the network consumer if the pkg/network/events
	// module was initialized by the network tracer module
	// (this will happen only if the network consumer is enabled
	// in config and the network tracer module is loaded successfully)
	if events.Initialized() {
		network, err := events.NewNetworkConsumer(evm)
		if err != nil {
			return nil, err
		}
		evm.RegisterEventConsumer(network)
		log.Info("event monitoring network consumer initialized")

		if netconfig.DirectSend {
			ds, err := sender.NewDirectSenderConsumer(evm, deps.Log, deps.SysprobeConfig)
			if err != nil {
				return nil, err
			}
			if ds != nil {
				evm.RegisterEventConsumer(ds)
				log.Info("event monitoring direct sender consumer initialized")
			}
		}
	}

	if netconfig.EnableUSMEventStream {
		if err := createProcessMonitorConsumer(evm, netconfig); err != nil {
			return nil, err
		}
	}

	gpucfg := gpuconfig.New()
	if gpucfg.Enabled {
		err := createGPUProcessEventConsumer(evm)
		if err != nil {
			return nil, fmt.Errorf("cannot create event consumer for GPU: %w", err)
		}
	}

	return evm, err
}
