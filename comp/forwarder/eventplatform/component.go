// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains the logic for forwarding events to the event platform
package eventplatform

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

// team: agent-metrics-logs

const (
	// EventTypeNetworkDevicesMetadata is the event type for network devices metadata
	EventTypeNetworkDevicesMetadata = "network-devices-metadata"

	// EventTypeSnmpTraps is the event type for snmp traps
	EventTypeSnmpTraps = "network-devices-snmp-traps"

	// EventTypeNetworkDevicesNetFlow is the event type for network devices NetFlow data
	EventTypeNetworkDevicesNetFlow = "network-devices-netflow"

	// EventTypeNetworkPath is the event type for network devices Network Path data
	EventTypeNetworkPath = "network-path"

	// EventTypeContainerLifecycle represents a container lifecycle event
	EventTypeContainerLifecycle = "container-lifecycle"
	// EventTypeContainerImages represents a container images event
	EventTypeContainerImages = "container-images"
	// EventTypeContainerSBOM represents a container SBOM event
	EventTypeContainerSBOM = "container-sbom"
)

// Component is the interface of the event platform forwarder component.
type Component interface {
	// Get the forwarder instance if it exists.
	Get() (Forwarder, bool)
}

// Forwarder is the interface of the event platform forwarder.
type Forwarder interface {
	SendEventPlatformEvent(e *message.Message, eventType string) error
	SendEventPlatformEventBlocking(e *message.Message, eventType string) error
	Purge() map[string][]*message.Message
}
