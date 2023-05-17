// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package events

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
)

// NetworkConsumer describes a process monitoring object
type NetworkConsumer struct{}

func (n *NetworkConsumer) Start() error {
	return fmt.Errorf("network consumer is only supported on linux")
}

func (n *NetworkConsumer) Stop() {}

// ID returns id for process monitor
func (n *NetworkConsumer) ID() string {
	return "NETWORK"
}

// NewNetworkConsumer returns a new NetworkConsumer instance
func NewNetworkConsumer(_ *eventmonitor.EventMonitor) (*NetworkConsumer, error) {
	return nil, fmt.Errorf("network consumer is only supported on linux")
}
