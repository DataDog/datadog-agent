// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serializer

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/telemetrycapture"
)

// InventoryCapture is implemented only by the Agent, host, and system information inventory
// payloads. Other SendMetadata callers remain outside capture. Copy must own
// all fields and omit configuration and credentials before enqueueing.
type InventoryCapture interface {
	CaptureInventoryStream() telemetrycapture.Stream
	CaptureInventorySchedule() (time.Time, time.Duration)
	CaptureInventorySize() int64
	CopyCaptureInventory() *telemetrycapture.Inventory
}

func beginLiveInventory(manager *telemetrycapture.Manager, payload marshaler.JSONMarshaler) (capture *liveMetadataCapture) {
	if !manager.Enabled() {
		return nil
	}
	projection, ok := payload.(InventoryCapture)
	if !ok {
		return nil
	}
	var control telemetrycapture.Control
	var reservation *telemetrycapture.Reservation
	defer func() {
		if recover() != nil {
			_ = manager.Fail(control)
			reservation.Discard()
			capture = nil
		}
	}()
	stream := projection.CaptureInventoryStream()
	if stream != telemetrycapture.AgentInventory && stream != telemetrycapture.HostInventory && stream != telemetrycapture.HostSystemInfo {
		return nil
	}
	var selected bool
	control, selected = manager.Selected(stream)
	if !selected {
		return nil
	}
	at, cadence := projection.CaptureInventorySchedule()
	reservation = manager.BeginFor(control, stream, at, cadence, projection.CaptureInventorySize())
	if reservation == nil {
		return nil
	}
	owned := projection.CopyCaptureInventory()
	if owned == nil || (stream == telemetrycapture.AgentInventory && (owned.Agent == nil || owned.Host != nil || owned.SystemInfo != nil)) ||
		(stream == telemetrycapture.HostInventory && (owned.Host == nil || owned.Agent != nil || owned.SystemInfo != nil)) ||
		(stream == telemetrycapture.HostSystemInfo && (owned.SystemInfo == nil || owned.Agent != nil || owned.Host != nil)) {
		_ = manager.Fail(control)
		reservation.Discard()
		return nil
	}
	return &liveMetadataCapture{manager: manager, reservation: reservation, inventory: owned}
}
