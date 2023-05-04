// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows
// +build linux windows

package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	procevents "github.com/DataDog/datadog-agent/pkg/process/events"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventMonitor - Event monitor Factory
var EventMonitor = module.Factory{
	Name:             config.EventMonitorModule,
	ConfigNamespaces: []string{"event_monitoring_config", "runtime_security_config"},
	Fn: func(sysProbeConfig *config.Config) (module.Module, error) {
		emconfig := emconfig.NewConfig(sysProbeConfig)

		secconfig, err := secconfig.NewConfig()
		if err != nil {
			log.Infof("invalid probe configuration: %v", err)
			return nil, module.ErrNotEnabled
		}

		opts := eventmonitor.Opts{}

		// adapt options
		if secconfig.RuntimeSecurity.IsRuntimeEnabled() {
			secmodule.UpdateEventMonitorOpts(&opts)
		}

		evm, err := eventmonitor.NewEventMonitor(emconfig, secconfig, opts)
		if err != nil {
			log.Infof("error initializing event monitoring module: %v", err)
			return nil, module.ErrNotEnabled
		}

		if secconfig.RuntimeSecurity.IsRuntimeEnabled() {
			cws, err := secmodule.NewCWSConsumer(evm, secconfig.RuntimeSecurity)
			if err != nil {
				return nil, err
			}
			evm.RegisterEventConsumer(cws)
			log.Info("event monitoring cws consumer initialized")
		}

		if emconfig.NetworkConsumerEnabled {
			network, err := events.NewNetworkConsumer(evm)
			if err != nil {
				return nil, err
			}
			evm.RegisterEventConsumer(network)
			log.Info("event monitoring network consumer initialized")
		}

		if emconfig.ProcessConsumerEnabled {
			process, err := procevents.NewProcessConsumer(evm)
			if err != nil {
				return nil, err
			}
			evm.RegisterEventConsumer(process)
			log.Info("event monitoring process-agent consumer initialized")
		}

		return evm, err
	},
}
