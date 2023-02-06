// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package module

import (
	"github.com/DataDog/datadog-agent/cmd/system-probe/event_monitor"
	netevents "github.com/DataDog/datadog-agent/pkg/network/events"
	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NetworkModule describes a process monitoring object
type NetworkModule struct{}

func (n *NetworkModule) Start() error {
	return nil
}

func (n *NetworkModule) Stop() {
}

// ID returns id for process monitor
func (n *NetworkModule) ID() string {
	return "NETWORK_MODULE"
}

// NewNetworkModule returns a new NetworkModule instance
func NewNetworkModule(evm *event_monitor.EventMonitor) (*NetworkModule, error) {
	h := netevents.Handler()
	if err := evm.AddEventTypeHandler(smodel.ForkEventType, h); err != nil {
		return nil, err
	}
	if err := evm.AddEventTypeHandler(smodel.ExecEventType, h); err != nil {
		return nil, err
	}
	if err := evm.AddEventTypeHandler(smodel.ExitEventType, h); err != nil {
		return nil, err
	}

	log.Info("network process monitoring initialized")

	return &NetworkModule{}, nil
}
