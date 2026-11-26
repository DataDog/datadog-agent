// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a severity provider component mock.
package mock

import (
	"testing"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	severityprovider "github.com/DataDog/datadog-agent/comp/logs/severityprovider/def"
)

// Component is a configurable severity provider mock.
type Component struct {
	Level        severityeventsdef.SeverityLevel
	Available    bool
	ScopedLevels map[severityeventsdef.ScopeKey]severityeventsdef.SeverityLevel
}

// Current returns the configured severity level.
func (c *Component) Current() (severityeventsdef.SeverityLevel, bool) {
	return c.Level, c.Available
}

// CurrentForScope returns the configured severity level for getScope.
func (c *Component) CurrentForScope(getScope func() severityeventsdef.ScopeKey) (severityeventsdef.SeverityLevel, bool) {
	if c.ScopedLevels == nil {
		return c.Current()
	}
	if !c.Available {
		return severityeventsdef.SeverityLow, false
	}
	if len(c.ScopedLevels) == 0 || getScope == nil {
		return severityeventsdef.SeverityLow, true
	}
	key := getScope()
	if key == 0 {
		return severityeventsdef.SeverityLow, true
	}
	if level, ok := c.ScopedLevels[key]; ok {
		return level, true
	}
	return severityeventsdef.SeverityLow, true
}

// Provides defines the mock component output.
type Provides struct {
	Comp severityprovider.Component
}

// New creates a severity provider mock.
func New(t testing.TB) Provides {
	t.Helper()
	return Provides{Comp: &Component{}}
}

var _ severityprovider.Component = (*Component)(nil)
