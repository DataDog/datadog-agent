// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux
// +build linux

package network

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	netevents "github.com/DataDog/datadog-agent/pkg/network/events"
	smodel "github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// NetworkConsumer describes a process monitoring object
type NetworkConsumer struct{}

func (n *NetworkConsumer) Start() error {
	return nil
}

func (n *NetworkConsumer) Stop() {
}

// ID returns id for process monitor
func (n *NetworkConsumer) ID() string {
	return "NETWORK"
}

// NewNetworkConsumer returns a new NetworkConsumer instance
func NewNetworkConsumer(evm *eventmonitor.EventMonitor) (*NetworkConsumer, error) {
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

	return &NetworkConsumer{}, nil
}
