// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && linux_bpf

package modules

import (
	"errors"
	"fmt"

	dimod "github.com/DataDog/datadog-agent/pkg/dyninst/module"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	ebpf_process "github.com/DataDog/datadog-agent/pkg/ebpf/process"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

func init() { registerModule(DynamicInstrumentation) }

var godiProcessEventConsumer *consumers.ProcessConsumer

// DynamicInstrumentation is a system probe module which allows you to add instrumentation into
// running Go services without restarts.
var DynamicInstrumentation = &module.Factory{
	Name:             config.DynamicInstrumentationModule,
	ConfigNamespaces: []string{},
	Fn: func(agentConfiguration *sysconfigtypes.Config, _ module.FactoryDependencies) (module.Module, error) {
		if godiProcessEventConsumer == nil {
			return nil, errors.New("process event consumer not initialized")
		}
		godiSubscriber, err := ebpf_process.NewMonitor(godiProcessEventConsumer)
		if err != nil {
			return nil, err
		}

		config, err := dimod.NewConfig(agentConfiguration)
		if err != nil {
			return nil, fmt.Errorf("invalid dynamic instrumentation module configuration: %w", err)
		}
		m, err := dimod.NewModule(config, godiSubscriber)
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

// createGoDIProcessEventConsumer creates the process event consumer for the GoDI module. Should be called from the event monitor module
func createGoDIProcessEventConsumer(evm *eventmonitor.EventMonitor) (err error) {
	eventTypes := []consumers.ProcessConsumerEventTypes{consumers.ExecEventType, consumers.ExitEventType}
	godiProcessEventConsumer, err = consumers.NewProcessConsumer("dynamicinstrumentation", 100, eventTypes, evm)
	return err
}
