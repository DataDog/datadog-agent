// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows

// Package eventmonitor holds eventmonitor related files
package eventmonitor

import (
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	"github.com/DataDog/datadog-agent/pkg/security/probe"
	"github.com/DataDog/datadog-go/v5/statsd"
	"google.golang.org/grpc"
)

// Opts defines options that can be used for the eventmonitor
type Opts struct{}

// EventMonitor represents the system-probe module for kernel event monitoring
type EventMonitor struct {
	Config       *config.Config
	StatsdClient statsd.ClientInterface
	GRPCServer   *grpc.Server
	Probe        *probe.Probe
}

// AddEventConsumerHandler add a consumer
func (m *EventMonitor) AddEventConsumerHandler(_ EventConsumerHandler) error {
	return nil
}
