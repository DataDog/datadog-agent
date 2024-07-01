// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	procmon "github.com/DataDog/datadog-agent/pkg/process/monitor"
)

// EventMonitor - Event monitor Factory
var EventMonitor = module.Factory{
	Name:             config.EventMonitorModule,
	ConfigNamespaces: eventMonitorModuleConfigNamespaces,
	Fn:               createEventMonitorModule,
	NeedsEBPF: func() bool {
		return !coreconfig.SystemProbe.GetBool("runtime_security_config.ebpfless.enabled")
	},
}

func createProcessMonitorConsumer(evm *eventmonitor.EventMonitor) (eventmonitor.EventConsumerInterface, error) {
	return procmon.NewProcessMonitorEventConsumer(evm)
}
