// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	usmstate "github.com/DataDog/datadog-agent/pkg/network/usm/state"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// EventMonitor - Event monitor Factory
var EventMonitor = module.Factory{
	Name:             config.EventMonitorModule,
	ConfigNamespaces: eventMonitorModuleConfigNamespaces,
	Fn:               createEventMonitorModule,
	NeedsEBPF: func() bool {
		return !secconfig.IsEBPFLessModeEnabled()
	},
}

const (
	eventMonitorID          = "PROCESS_MONITOR"
	eventMonitorChannelSize = 500
)

var (
	eventTypes = []consumers.ProcessConsumerEventTypes{
		consumers.ExecEventType,
		consumers.ExitEventType,
	}
)

func createProcessMonitorConsumer(evm *eventmonitor.EventMonitor, config *netconfig.Config) error {
	if !usmconfig.IsUSMSupportedAndEnabled(config) || !usmconfig.NeedProcessMonitor(config) || usmstate.Get() != usmstate.Running {
		return nil
	}

	consumer, err := consumers.NewProcessConsumer(eventMonitorID, eventMonitorChannelSize, eventTypes, evm)
	if err != nil {
		return err
	}
	monitor.InitializeEventConsumer(consumer)
	log.Info("USM process monitoring consumer initialized")
	return nil
}
