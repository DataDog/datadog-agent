// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.
//go:build linux
// +build linux

package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	emconfig "github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	"github.com/DataDog/datadog-agent/pkg/network/events"
	"github.com/DataDog/datadog-agent/pkg/process/checks"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	secmodule "github.com/DataDog/datadog-agent/pkg/security/module"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventMonitor - Event monitor Factory
var EventMonitor = module.Factory{
	Name:             config.EventMonitorModule,
	ConfigNamespaces: []string{"event_monitoring_config", "runtime_security_config"},
	Fn: func(sysProbeConfig *config.Config) (module.Module, error) {
		seccfg := secconfig.NewConfig()

		emconfig, err := emconfig.NewConfig(sysProbeConfig, seccfg.IsRuntimeEnabled())
		if err != nil {
			log.Infof("invalid event monitoring configuration: %v", err)
			return nil, module.ErrNotEnabled
		}

		evm, err := eventmonitor.NewEventMonitor(emconfig)
		if err != nil {
			log.Infof("error initializing event monitoring module: %v", err)
			return nil, module.ErrNotEnabled
		}

		if seccfg.IsRuntimeEnabled() {
			cws, err := secmodule.NewCWSConsumer(evm)
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
			process, err := checks.NewProcessConsumer(evm)
			if err != nil {
				return nil, err
			}
			evm.RegisterEventConsumer(process)
			log.Info("event monitoring process-agent consumer initialized")
		}

		return evm, err
	},
}
