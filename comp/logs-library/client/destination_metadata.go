// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package client

import (
	"fmt"
)

// DestinationMetadata contains metadata about a destination
type DestinationMetadata struct {
	componentName    string
	instanceID       string
	kind             string
	endpointID       string
	evpCategory      string
	ReportingEnabled bool
}

// NewDestinationMetadata returns a new DestinationMetadata
func NewDestinationMetadata(componentName, instanceID, kind, endpointID, evpCategory string) *DestinationMetadata {
	return &DestinationMetadata{
		componentName:    componentName,
		instanceID:       instanceID,
		kind:             kind,
		endpointID:       endpointID,
		evpCategory:      evpCategory,
		ReportingEnabled: true,
	}
}

// NewNoopDestinationMetadata returns a new DestinationMetadata with reporting disabled
func NewNoopDestinationMetadata() *DestinationMetadata {
	return &DestinationMetadata{
		ReportingEnabled: false,
	}
}

// TelemetryName returns the telemetry name for the destination
func (d *DestinationMetadata) TelemetryName() string {
	if !d.ReportingEnabled {
		return ""
	}
	return fmt.Sprintf("%s_%s_%s_%s", d.componentName, d.instanceID, d.kind, d.endpointID)
}

// MonitorTag returns the monitor tag for the destination
func (d *DestinationMetadata) MonitorTag() string {
	if !d.ReportingEnabled {
		return ""
	}
	return fmt.Sprintf("destination_%s_%s", d.kind, d.endpointID)
}

// EvpCategory returns the EvP category for the destination
func (d *DestinationMetadata) EvpCategory() string {
	return d.evpCategory
}
