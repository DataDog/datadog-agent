// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package marshaler

import "github.com/DataDog/datadog-agent/comp/forwarder/defaultforwarder/transaction"

// PayloadCapture is optional local correlation for a single serializer pass.
// Implementations reserve memory before copying and never return delivery errors.
type PayloadCapture interface {
	CurrentCaptureOrdinal() uint64
	ReserveCaptureBytes(int64) bool
	CapturePayload(*transaction.BytesPayload, []uint64)
}

// CaptureMembership tracks only successfully encoded, selected source ordinals.
// The serializer calls Accepted after committing an item to its compressor and
// Finished when that compressor produces a payload. Rejected/retried items do
// not enter the membership list until they are actually accepted.
type CaptureMembership struct {
	Observer PayloadCapture
	ordinals []uint64
}

// Accepted records membership in the current serialized payload.
func (c *CaptureMembership) Accepted() {
	if c.Observer == nil {
		return
	}
	ordinal := c.Observer.CurrentCaptureOrdinal()
	// Four words cover slice growth, including the old backing allocation.
	if ordinal != 0 && c.Observer.ReserveCaptureBytes(32) {
		c.ordinals = append(c.ordinals, ordinal)
	}
}

// Finished attaches local-only correlation and begins an empty membership list.
func (c *CaptureMembership) Finished(payload *transaction.BytesPayload) {
	if c.Observer != nil && len(c.ordinals) != 0 {
		c.Observer.CapturePayload(payload, c.ordinals)
	}
	c.ordinals = nil
}
