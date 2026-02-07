// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package eventmonitorimpl

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/consumers"
	netconfig "github.com/DataDog/datadog-agent/pkg/network/config"
	usmconfig "github.com/DataDog/datadog-agent/pkg/network/usm/config"
	usmstate "github.com/DataDog/datadog-agent/pkg/network/usm/state"
	"github.com/DataDog/datadog-agent/pkg/process/monitor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

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
