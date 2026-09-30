// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package modules

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers/yara"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	usmstate "github.com/DataDog/datadog-agent/pkg/network/usm/state"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	secconfig "github.com/DataDog/datadog-agent/pkg/security/config"
	"github.com/DataDog/datadog-agent/pkg/system-probe/api/module"
	"github.com/DataDog/datadog-agent/pkg/system-probe/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

func init() { registerModule(EventMonitor) }

// EventMonitor - Event monitor Factory
var EventMonitor = &module.Factory{
	Name: config.EventMonitorModule,
	Fn:   createEventMonitorModule,
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
	if !usmconfig.IsUSMSupportedAndEnabled(config) || !usmconfig.NeedProcessMonitor(config) || !usmstate.IsActive() {
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

// createYaraExecConsumer registers the YARA exec scanner when it is enabled. On error nothing is
// registered, and the caller must carry on without it.
func createYaraExecConsumer(evm *eventmonitor.EventMonitor) error {
	cfg := yara.NewConfig()
	if !cfg.Enabled {
		return nil
	}

	p, err := yara.NewExecScanner(evm, cfg)
	if err != nil {
		return err
	}
	log.Infof("event monitoring yara exec consumer initialized: rules_dir=%s rules_version=%s workers=%d queue_size=%d max_file_size=%d",
		cfg.RulesDir, p.RulesVersion(), cfg.Workers, cfg.QueueSize, cfg.MaxFileSize)
	return nil
}
