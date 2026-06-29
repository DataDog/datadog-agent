// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"time"

	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

const shadowIDSuffix = ":shadow"

type shadowMarker interface {
	isShadowCheck()
}

type checkUnwrapper interface {
	Unwrap() Check
}

// ShadowCheck wraps a normally loaded check so collector plumbing can route it
// through the shadow execution path without changing the base Check interface.
type ShadowCheck struct {
	Check

	id       checkid.ID
	interval time.Duration
}

// NewShadowCheck wraps inner with shadow identity and interval overrides.
func NewShadowCheck(inner Check, id checkid.ID, interval time.Duration) *ShadowCheck {
	return &ShadowCheck{
		Check:    inner,
		id:       id,
		interval: interval,
	}
}

// ShadowID returns the shadow check ID derived from sourceID.
func ShadowID(sourceID checkid.ID) checkid.ID {
	return checkid.ID(string(sourceID) + shadowIDSuffix)
}

// ID returns the shadow check ID.
func (c *ShadowCheck) ID() checkid.ID {
	return c.id
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
	for c != nil {
		if _, ok := c.(shadowMarker); ok {
			return true
		}

		unwrapper, ok := c.(checkUnwrapper)
		if !ok {
			return false
		}

		next := unwrapper.Unwrap()
		if next == nil || next == c {
			return false
		}
		c = next
	}
	return false
}
