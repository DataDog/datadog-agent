// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	procconsumer "github.com/DataDog/datadog-agent/pkg/process/events/consumer"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
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
	opts.ProbeOpts.EnvsVarResolutionEnabled = emconfig.EnvVarsResolutionEnabled
	secmoduleOpts := secmodule.Opts{}

	// adapt options
	if secconfig.RuntimeSecurity.IsRuntimeEnabled() {
		secmodule.UpdateEventMonitorOpts(&opts, secconfig)
	} else {
		secmodule.DisableRuntimeSecurity(secconfig)
	}

	evm, err := eventmonitor.NewEventMonitor(emconfig, secconfig, opts, deps.Telemetry)
	if err != nil {
		log.Errorf("error initializing event monitoring module: %v", err)
		return nil, module.ErrNotEnabled
	}

	if secconfig.RuntimeSecurity.IsRuntimeEnabled() {
		cws, err := secmodule.NewCWSConsumer(evm, secconfig.RuntimeSecurity, deps.WMeta, secmoduleOpts)
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
		procmonconsumer, err := createProcessMonitorConsumer(evm, netconfig)
		if err != nil {
			return nil, err
		}
		if procmonconsumer != nil {
			evm.RegisterEventConsumer(procmonconsumer)
			log.Info("USM process monitoring consumer initialized")
		}
	}

	return evm, err
}
