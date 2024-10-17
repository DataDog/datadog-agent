// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package modules

import (
	"errors"
	"fmt"

	"github.com/DataDog/datadog-agent/cmd/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/cmd/system-probe/config/types"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/cmd/system-probe/api/module"
)

func isEventStreamEnabled() bool {
	eventStreamUSM := pkgconfigsetup.SystemProbe().GetBool("service_monitoring_config.enable_event_stream")
	eventStreamProcMon := pkgconfigsetup.SystemProbe().GetBool("process_monitoring_config.enable_event_stream")

	return eventStreamUSM || eventStreamProcMon
}

// ProcessMonitor - Event monitor Factory
var ProcessMonitor = module.Factory{
	Name:             config.ProcessMonitorModule,
	ConfigNamespaces: processMonitorModuleConfigNamespaces,
	Fn:               createProcessMonitorModule,
	NeedsEBPF: func() bool {
		return isEventStreamEnabled()
	},
}

var processMonitorModuleConfigNamespaces = []string{"process_monitoring_config", "service_monitoring_config"}

func createProcessMonitorModule(_ *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
	log.Infof("Initializing process monitor...")
	processMonitor := monitor.GetProcessMonitor()
	if processMonitor == nil {
		return nil, errors.New("could not get process monitor")
	}

	err := processMonitor.Initialize(isEventStreamEnabled())
	if err != nil {
		return nil, fmt.Errorf("cannot initialize process monitor: %w", err)
	}

	return &processMonitorModule{}, nil
}

type processMonitorModule struct {
}

func (m *processMonitorModule) GetStats() map[string]interface{} {
	return nil
}

func (m *processMonitorModule) Register(_ *module.Router) error {
	return nil
}

func (m *processMonitorModule) Close() {
}
