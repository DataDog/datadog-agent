// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package serializer

import (
	"strings"
	"time"
	"unsafe"

	"github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"
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
	payload     *transaction.BytesPayload
	routes      []telemetrycapture.Route
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

func (c *liveMetadataCapture) attach(payloads transaction.BytesPayloads) {
	defer c.recoverFailure()
	if len(payloads) != 1 || payloads[0] == nil || !c.reservation.Grow(512) {
		_ = c.manager.Fail(c.reservation.Control())
		return
	}
	c.payload = payloads[0]
	c.payload.SetCapture(&transaction.CaptureMetadata{
		SessionID: c.reservation.Control().SessionID, CycleID: c.reservation.CycleID(),
		PayloadID: 1, Ordinals: []uint64{1}, Observer: c,
	})
}

func (c *liveMetadataCapture) ObserveRoute(payloadID uint64, ordinals []uint64, endpoint, protocol, destination string, enqueuedAt time.Time) {
	defer func() {
		if recover() != nil {
			_ = c.manager.Fail(c.reservation.Control())
		}
	}()
	if payloadID != 1 || len(ordinals) != 1 || ordinals[0] != 1 ||
		!strings.HasPrefix(endpoint, "/") || strings.ContainsAny(endpoint, "?#") || protocol == "" || destination == "" {
		_ = c.manager.Fail(c.reservation.Control())
		return
	}
	if c.inventory != nil && (endpoint != "/api/v1/metadata" || protocol != "inventory-v1") {
		_ = c.manager.Fail(c.reservation.Control())
		return
	}
	bytes := 4*int64(unsafe.Sizeof(telemetrycapture.Route{})) + int64(len(endpoint)+len(protocol)+len(destination))
	if !c.reservation.Grow(bytes) {
		return
	}
	c.routes = append(c.routes, telemetrycapture.Route{
		PayloadID: 1, Endpoint: strings.Clone(endpoint), Protocol: strings.Clone(protocol),
		Destination: strings.Clone(destination), EnqueuedAt: enqueuedAt,
	})
}

func (c *liveMetadataCapture) finish(deliveryErr error) {
	defer c.recoverFailure()
	defer func() {
		if c.payload != nil {
			c.payload.ClearCapture()
		}
		c.payload, c.projection, c.inventory, c.routes = nil, nil, nil, nil
		c.reservation.Discard()
	}()
	if deliveryErr != nil || len(c.routes) == 0 {
		_ = c.manager.Fail(c.reservation.Control())
		return
	}
	_ = c.reservation.Commit(telemetrycapture.Payload{Metadata: c.projection, Inventory: c.inventory, Routes: c.routes})
}

func (c *liveMetadataCapture) recoverFailure() {
	if recover() != nil {
		_ = c.manager.Fail(c.reservation.Control())
	}
}
