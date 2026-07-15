// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"time"

	"github.com/DataDog/datadog-agent/pkg/aggregator/sender"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

const shadowIDSuffix = ":shadow"

type shadowMarker interface {
	isShadowCheck()
}

// ShadowCheck wraps a normally loaded check so collector plumbing can route it
// through the shadow execution path without changing the base Check interface.
type ShadowCheck struct {
	Check

	// sourceCheckID preserves the normal check ID when the wrapped check was
	// loaded from shadow-mutated config bytes. In that case, the inner check's
	// own ID may be derived from the shadow config instead of the source config.
	sourceCheckID checkid.ID
	interval      time.Duration
	senderManager sender.SenderManager
}

// NewShadowCheck wraps inner with shadow identity and interval overrides.
func NewShadowCheck(inner Check, interval time.Duration) *ShadowCheck {
	return NewShadowCheckWithSenderManagerOverride(inner, interval, nil)
}

// NewShadowCheckWithSenderManagerOverride wraps inner with shadow identity,
// interval, and sender manager overrides.
func NewShadowCheckWithSenderManagerOverride(inner Check, interval time.Duration, senderManager sender.SenderManager) *ShadowCheck {
	return newShadowCheck(inner, "", interval, senderManager)
}

// NewShadowCheckForSource wraps inner as a shadow of sourceCheckID.
func NewShadowCheckForSource(inner Check, sourceCheckID checkid.ID, interval time.Duration, senderManager sender.SenderManager) *ShadowCheck {
	return newShadowCheck(inner, sourceCheckID, interval, senderManager)
}

func newShadowCheck(inner Check, sourceCheckID checkid.ID, interval time.Duration, senderManager sender.SenderManager) *ShadowCheck {
	return &ShadowCheck{
		Check:         inner,
		sourceCheckID: sourceCheckID,
		interval:      interval,
		senderManager: senderManager,
	}
}

// ShadowID returns the shadow check ID derived from sourceID.
func ShadowID(sourceID checkid.ID) checkid.ID {
	return checkid.ID(string(sourceID) + shadowIDSuffix)
}

// ID returns the shadow check ID.
func (c *ShadowCheck) ID() checkid.ID {
	if c.sourceCheckID != "" {
		return ShadowID(c.sourceCheckID)
	}
	return ShadowID(c.Check.ID())
}

// Interval returns the shadow collection interval.
func (c *ShadowCheck) Interval() time.Duration {
	return c.interval
}

// Unwrap returns the wrapped check.
func (c *ShadowCheck) Unwrap() Check {
	return c.Check
}

func (*ShadowCheck) isShadowCheck() {}

// IsShadow returns true when c is a shadow check wrapper.
func IsShadow(c Check) bool {
	_, ok := As[shadowMarker](c)
	return ok
}

// SenderManagerOverride returns the sender manager that should be used for c
// when it differs from the collector's normal sender manager.
func SenderManagerOverride(c Check) (sender.SenderManager, bool) {
	shadow, ok := As[*ShadowCheck](c)
	if !ok || shadow.senderManager == nil {
		return nil, false
	}
	return shadow.senderManager, true
}
