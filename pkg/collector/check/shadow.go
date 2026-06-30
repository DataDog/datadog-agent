// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package check

import (
	"time"

	healthplatformstore "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	checkid "github.com/DataDog/datadog-agent/pkg/collector/check/id"
)

const shadowIDSuffix = ":shadow"

// ShadowAware is an optional interface implemented by wrappers that preserve
// shadow identity.
type ShadowAware interface {
	IsShadow() bool
}

// ShadowCheck wraps a normally loaded check so collector plumbing can route it
// through the shadow execution path without changing the base Check interface.
type ShadowCheck struct {
	Check

	interval time.Duration
}

// NewShadowCheck wraps inner with shadow identity and interval overrides.
func NewShadowCheck(inner Check, interval time.Duration) *ShadowCheck {
	return &ShadowCheck{
		Check:    inner,
		interval: interval,
	}
}

// ShadowID returns the shadow check ID derived from sourceID.
func ShadowID(sourceID checkid.ID) checkid.ID {
	return checkid.ID(string(sourceID) + shadowIDSuffix)
}

// ID returns the shadow check ID.
func (c *ShadowCheck) ID() checkid.ID {
	return c.Check.ID() + shadowIDSuffix
}

// Interval returns the shadow collection interval.
func (c *ShadowCheck) Interval() time.Duration {
	return c.interval
}

// SetIssueReporter forwards issue reporter injection to issue-aware checks.
func (c *ShadowCheck) SetIssueReporter(reporter healthplatformstore.Component) {
	if aware, ok := c.Check.(IssueAwareCheck); ok {
		aware.SetIssueReporter(reporter)
	}
}

// IsShadow returns true for shadow checks.
func (*ShadowCheck) IsShadow() bool {
	return true
}

// IsShadow returns true when c is a shadow check wrapper.
func IsShadow(c Check) bool {
	shadow, ok := c.(ShadowAware)
	return ok && shadow.IsShadow()
}
