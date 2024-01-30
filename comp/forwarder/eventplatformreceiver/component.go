// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package eventplatformreceiver implements the global receiver for the epforwarder package
package eventplatformreceiver

import (
	"github.com/DataDog/datadog-agent/pkg/logs/diagnostic"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// team: agent-metrics-logs

// Component is the component type.
type Component interface {
	SetEnabled(e bool) bool
	IsEnabled() bool
	HandleMessage(m *message.Message, rendered []byte, eventType string)
	Filter(filters *diagnostic.Filters, done <-chan struct{}) <-chan string
}
