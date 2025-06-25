// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package snmpscanimpl implements the snmpscan component interface
package snmpscanimpl

import (
	"github.com/DataDog/datadog-agent/comp/aggregator/demultiplexer"
	"github.com/DataDog/datadog-agent/comp/core/config"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	rcclienttypes "github.com/DataDog/datadog-agent/comp/remote-config/rcclient/types"
	snmpscan "github.com/DataDog/datadog-agent/comp/snmpscan/def"
	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	"github.com/DataDog/datadog-agent/pkg/snmp/snmpparse"
)

// Requires defines the dependencies for the snmpscan component
type Requires struct {
	compdef.In
	Logger        log.Component
	Config        config.Component
	Demultiplexer demultiplexer.Component
	Client        ipc.HTTPClient
}

// Provides defines the output of the snmpscan component
type Provides struct {
	Comp       snmpscan.Component
	RCListener rcclienttypes.TaskListenerProvider
}

// NewComponent creates a new snmpscan component
func NewComponent(reqs Requires) (Provides, error) {
	forwarder, err := reqs.Demultiplexer.GetEventPlatformForwarder()
	if err != nil {
		return Provides{}, err
	}
	scanner := snmpScannerImpl{
		log:         reqs.Logger,
		config:      reqs.Config,
		epforwarder: forwarder,
		client:      reqs.Client,
	}
	provides := Provides{
		Comp:       scanner,
		RCListener: rcclienttypes.NewTaskListener(scanner.handleAgentTask),
	}
	return provides, nil
}

type snmpScannerImpl struct {
	log         log.Component
	config      config.Component
	epforwarder eventplatform.Forwarder
	client      ipc.HTTPClient
}

func (s snmpScannerImpl) handleAgentTask(taskType rcclienttypes.TaskType, task rcclienttypes.AgentTaskConfig) (bool, error) {
	if taskType != rcclienttypes.TaskDeviceScan {
		return false, nil
	}
	return true, s.startDeviceScan(task)
}

func (s snmpScannerImpl) startDeviceScan(task rcclienttypes.AgentTaskConfig) error {
	deviceIP := task.Config.TaskArgs["ip_address"]
	ns, ok := task.Config.TaskArgs["namespace"]
	if !ok || ns == "" {
		ns = s.config.GetString("network_devices.namespace")
		if ns == "" {
			ns = "default"
		}
	}
	instance, err := snmpparse.GetParamsFromAgent(deviceIP, s.config, s.client)
	if err != nil {
		return err
	}
	return s.ScanDeviceAndSendData(instance, ns, metadata.RCTriggeredScan)

}
