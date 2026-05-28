// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eventplatform contains the logic for forwarding events to the event platform
package eventplatform

// team: agent-log-pipelines

import (
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const (
	// EventTypeNetworkDevicesMetadata is the event type for network devices metadata
	EventTypeNetworkDevicesMetadata = "network-devices-metadata"

	// EventTypeSnmpTraps is the event type for snmp traps
	EventTypeSnmpTraps = "network-devices-snmp-traps"

	// EventTypeNetworkDevicesNetFlow is the event type for network devices NetFlow data
	EventTypeNetworkDevicesNetFlow = "network-devices-netflow"

	// EventTypeNetworkPath is the event type for network devices Network Path data
	EventTypeNetworkPath = "network-path"

	// EventTypeSynthetics is the event type for Synthetics test results
	EventTypeSynthetics = "synthetics"

	// EventTypeNetworkConfigManagement is the event type for network device configuration management
	EventTypeNetworkConfigManagement = "ndmconfig"

	// EventTypeContainerLifecycle represents a container lifecycle event
	EventTypeContainerLifecycle = "container-lifecycle"
	// EventTypeContainerImages represents a container images event
	EventTypeContainerImages = "container-images"
	// EventTypeContainerSBOM represents a container SBOM event
	EventTypeContainerSBOM = "container-sbom"
	// EventTypeGenResources represents a generic resources event
	EventTypeGenResources = "genresources"
	// EventTypeSoftwareInventory represents a software inventory event
	EventTypeSoftwareInventory = "software-inventory"
	// EventTypeEventManagement represents an event for the Event Management API
	EventTypeEventManagement = "event-management"
	// EventTypeKubeActions represents a kubernetes action result event
	EventTypeKubeActions = "kube-actions"
	// EventTypeDataStreamsMessage is the event type for Data Streams monitoring messages
	EventTypeDataStreamsMessage = "data-streams-message"
	// EventTypeDoQueryResults is the event type for Data Observability query results
	EventTypeDoQueryResults = "do-query-results"
)

// PipelineDesc describes a passthrough pipeline that forwards a specific event type
// to its corresponding intake.
//
// Pipeline descriptions are contributed to the event platform forwarder via the
// "ep_pipeline_descs" fx group, so that each product team can own the configuration
// for their own event types in their team-owned packages, rather than having every
// app-specific pipeline live inside the event platform forwarder itself.
type PipelineDesc struct {
	EventType   string
	Category    string
	ContentType string
	// IntakeTrackType is the track type to use for the v2 intake API. When blank, v1 is used instead.
	IntakeTrackType               string
	EndpointsConfigPrefix         string
	HostnameEndpointPrefix        string
	DefaultBatchMaxConcurrentSend int
	DefaultBatchMaxContentSize    int
	DefaultBatchMaxSize           int
	DefaultInputChanSize          int
	ForceCompressionKind          string
	ForceCompressionLevel         int
	UseStreamStrategy             bool
}

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
