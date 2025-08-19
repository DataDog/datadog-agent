// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux || windows

// Package events handles process events
package events

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
)

// NetworkConsumer describes a process monitoring object
type NetworkConsumer struct{}

// Start starts the event consumer (noop)
func (n *NetworkConsumer) Start() error {
	return nil
}

// Stop stops the event consumer (noop)
func (n *NetworkConsumer) Stop() {
}

// ID returns id for process monitor
func (n *NetworkConsumer) ID() string {
	return "NETWORK"
}

// NewNetworkConsumer returns a new NetworkConsumer instance
func NewNetworkConsumer(evm *eventmonitor.EventMonitor) (*NetworkConsumer, error) {
	h := Consumer()
	if err := evm.AddEventConsumerHandler(h); err != nil {
		return nil, err
	}
	return &NetworkConsumer{}, nil
}
