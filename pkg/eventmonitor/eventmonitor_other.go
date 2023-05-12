// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !linux && !windows
// +build !linux,!windows

package eventmonitor

import (
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/eventmonitor/config"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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
}

// EventTypeHandler event type based handler
type EventTypeHandler interface {
}

// AddEventTypeHandler registers an event handler
func (m *EventMonitor) AddEventTypeHandler(eventType model.EventType, handler EventTypeHandler) error {
	return fmt.Errorf("Not implemented on this platform")
}
