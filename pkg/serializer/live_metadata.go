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

// HostMetadataCapture is implemented by the host provider's actual payload.
// Size must include all owned allocations made by Copy. Neither method may
// serialize the original payload or include credentials in the projection.
type HostMetadataCapture interface {
	CaptureMetadataSchedule() (time.Time, time.Duration)
	CaptureMetadataSize() int64
	CopyCaptureMetadata() *telemetrycapture.HostMetadata
}

type liveMetadataCapture struct {
	manager     *telemetrycapture.Manager
	reservation *telemetrycapture.Reservation
	projection  *telemetrycapture.HostMetadata
	inventory   *telemetrycapture.Inventory
}

func beginLiveMetadata(manager *telemetrycapture.Manager, payload marshaler.JSONMarshaler) (capture *liveMetadataCapture) {
	control, selected := manager.Selected(telemetrycapture.Metadata)
	if !selected {
		return nil
	}
	var reservation *telemetrycapture.Reservation
	defer func() {
		if recover() != nil {
			_ = manager.Fail(control)
			reservation.Discard()
			capture = nil
		}
	}()
	projection, ok := payload.(HostMetadataCapture)
	if !ok {
		_ = manager.Fail(control)
		return nil
	}
	collectedAt, cadence := projection.CaptureMetadataSchedule()
	reservation = manager.BeginFor(control, telemetrycapture.Metadata, collectedAt, cadence, projection.CaptureMetadataSize())
	if reservation == nil {
		return nil
	}
	owned := projection.CopyCaptureMetadata()
	if owned == nil {
		_ = manager.Fail(reservation.Control())
		reservation.Discard()
		return nil
	}
	return &liveMetadataCapture{manager: manager, reservation: reservation, projection: owned}
}

func (c *liveMetadataCapture) finish(deliveryErr error) {
	defer c.recoverFailure()
	defer func() {
		c.projection, c.inventory = nil, nil
		c.reservation.Discard()
	}()
	if deliveryErr != nil {
		_ = c.manager.Fail(c.reservation.Control())
		return
	}
	_ = c.reservation.Commit(telemetrycapture.Payload{Metadata: c.projection, Inventory: c.inventory})
}

func (c *liveMetadataCapture) recoverFailure() {
	if recover() != nil {
		_ = c.manager.Fail(c.reservation.Control())
	}
}
