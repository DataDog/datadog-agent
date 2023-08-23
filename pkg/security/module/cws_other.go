// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

package module

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

// CWSConsumer is a no-op struct when CWS is unsupported
type CWSConsumer struct{}

// NewCWSConsumer returns an error because CWS is unsupported
func NewCWSConsumer(evm *eventmonitor.EventMonitor, config *config.RuntimeSecurityConfig, opts ...Opts) (*CWSConsumer, error) {
	return nil, fmt.Errorf("CWS is only supported on linux")
}

// ID returns the ID of this consumer
func (c *CWSConsumer) ID() string {
	return "CWS"
}

// Start starts this unsupported consumer
func (c *CWSConsumer) Start() error {
	return fmt.Errorf("CWS is only supported on linux")
}

// Stop stops this CWS consumer
func (c *CWSConsumer) Stop() {}

// UpdateEventMonitorOpts adapt the event monitor options
func UpdateEventMonitorOpts(opts *eventmonitor.Opts) {}
