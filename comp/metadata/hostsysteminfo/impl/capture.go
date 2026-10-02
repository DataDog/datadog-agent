// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package hostsysteminfoimpl

import (
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

var _ serializer.InventoryCapture = (*Payload)(nil)

func (*Payload) CaptureInventoryStream() telemetrycapture.Stream {
	return telemetrycapture.HostSystemInfo
}

func (p *Payload) captureMetadata() telemetrycapture.HostSystemInfoMetadata {
	h := p.Metadata
	return telemetrycapture.HostSystemInfoMetadata{
		Manufacturer: h.Manufacturer, ModelNumber: h.ModelNumber, SerialNumber: h.SerialNumber,
		ModelName: h.ModelName, ChassisType: h.ChassisType, Identifier: h.Identifier,
	}
}

// CaptureInventorySize measures borrowed strings before any owned copy exists.
func (p *Payload) CaptureInventorySize() int64 {
	if p.Metadata == nil {
		return 256
	}
	h := p.captureMetadata()
	return 256 + telemetrycapture.InventorySize(&telemetrycapture.Inventory{Hostname: p.Hostname, UUID: p.UUID, SystemInfo: &h})
}

// CopyCaptureInventory owns only six native hardware fields and the common
// envelope. The coordinator sanitizes the native identity fields in memory.
func (p *Payload) CopyCaptureInventory() *telemetrycapture.Inventory {
	if p.Metadata == nil {
		return nil
	}
	h := p.captureMetadata()
	return telemetrycapture.CloneInventory(&telemetrycapture.Inventory{Hostname: p.Hostname, UUID: p.UUID, Timestamp: p.Timestamp, SystemInfo: &h})
}
