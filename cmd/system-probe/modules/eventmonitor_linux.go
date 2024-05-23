// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	"github.com/DataDog/datadog-agent/comp/core/workloadmeta"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	procmon "github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

// EventMonitor - Event monitor Factory
var EventMonitor = module.Factory{
	Name:             config.EventMonitorModule,
	ConfigNamespaces: eventMonitorModuleConfigNamespaces,
	Fn:               createEventMonitorModuleLinux,
	NeedsEBPF: func() bool {
		return !coreconfig.SystemProbe.GetBool("runtime_security_config.ebpfless.enabled")
	},
}

func createEventMonitorModuleLinux(config *sysconfigtypes.Config, wmeta optional.Option[workloadmeta.Component]) (module.Module, error) {
	evm, err := createEventMonitor(config, wmeta)
	if err != nil {
		return evm, err
	}

	netconfig := netconfig.New()
	if netconfig.EnableUSMEventStream {
		procmonconsumer, err := procmon.NewProcessMonitorEventConsumer(evm)
		if err != nil {
			return nil, err
		}
		evm.RegisterEventConsumer(procmonconsumer)
		log.Info("USM process monitoring consumer initialized")
	}

	return evm, err
}
