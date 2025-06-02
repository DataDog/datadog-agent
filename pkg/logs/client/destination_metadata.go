// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(AML) Fix revive linter
package client

import (
	"fmt"
)

// DestinationMetadata contains metadata about a destination
type DestinationMetadata struct {
	componentName    string
	instanceID       string
	kind             string
	endpointId       string
	ReportingEnabled bool
}

// NewDestinationMetadata returns a new DestinationMetadata
func NewDestinationMetadata(componentName, instanceID, kind, endpointId string) *DestinationMetadata {
	return &DestinationMetadata{
		componentName:    componentName,
		instanceID:       instanceID,
		kind:             kind,
		endpointId:       endpointId,
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
	return fmt.Sprintf("%s_%s_%s_%s", d.componentName, d.instanceID, d.kind, d.endpointId)
}

// MonitorTag returns the monitor tag for the destination
func (d *DestinationMetadata) MonitorTag() string {
	if !d.ReportingEnabled {
		return ""
	}
	return fmt.Sprintf("destination_%s_%s", d.kind, d.endpointId)
}
