// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	procconsumer "github.com/DataDog/datadog-agent/pkg/process/events/consumer"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var eventMonitorModuleConfigNamespaces = []string{"event_monitoring_config", "runtime_security_config"}

func createEventMonitorModule(sysconfig *sysconfigtypes.Config, deps module.FactoryDependencies) (module.Module, error) {
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
	secmoduleOpts := secmodule.Opts{}

	// adapt options
	if secconfig.RuntimeSecurity.IsRuntimeEnabled() {
		secmodule.UpdateEventMonitorOpts(&opts, secconfig)
	} else {
		secmodule.DisableRuntimeSecurity(secconfig)
	}

	evm, err := eventmonitor.NewEventMonitor(emconfig, secconfig, deps.Ipc, opts)
	if err != nil {
		log.Errorf("error initializing event monitoring module: %v", err)
		return nil, module.ErrNotEnabled
	}

	if secconfig.RuntimeSecurity.IsRuntimeEnabled() {
		cws, err := secmodule.NewCWSConsumer(evm, secconfig.RuntimeSecurity, deps.WMeta, secmoduleOpts, deps.Compression, deps.Ipc)
		if err != nil {
			return nil, err
		}
		evm.RegisterEventConsumer(cws)
		log.Info("event monitoring cws consumer initialized")
	}

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
	}

	if emconfig.ProcessConsumerEnabled {
		process, err := procconsumer.NewProcessConsumer(evm)
		if err != nil {
			return nil, err
		}
		evm.RegisterEventConsumer(process)
		log.Info("event monitoring process-agent consumer initialized")
	}

	netconfig := netconfig.New()
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

	if sysconfig.ModuleIsEnabled(config.DynamicInstrumentationModule) {
		err := createGoDIProcessEventConsumer(evm)
		if err != nil {
			return nil, fmt.Errorf("cannot create event consumer for dynamic instrumentation: %w", err)
		}
	}

	return evm, err
}
