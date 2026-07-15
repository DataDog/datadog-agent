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
	Level     severityeventsdef.SeverityLevel
	Available bool
}

// Current returns the configured severity level.
func (c *Component) Current() (severityeventsdef.SeverityLevel, bool) {
	return c.Level, c.Available
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
