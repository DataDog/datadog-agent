// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux
// +build !linux

package module

import (
	"fmt"

	"github.com/DataDog/datadog-agent/pkg/eventmonitor"
	"github.com/DataDog/datadog-agent/pkg/security/config"
)

type CWSConsumer struct{}

func NewCWSConsumer(evm *eventmonitor.EventMonitor, config *config.RuntimeSecurityConfig, opts ...Opts) (*CWSConsumer, error) {
	return nil, fmt.Errorf("CWS is only supported on linux")
}

func (c *CWSConsumer) ID() string {
	return "CWS"
}

func (c *CWSConsumer) Start() error {
	return fmt.Errorf("CWS is only supported on linux")
}

func (c *CWSConsumer) Stop() {}

// UpdateEventMonitorOpts adapt the event monitor options
func UpdateEventMonitorOpts(opts *eventmonitor.Opts) {}
