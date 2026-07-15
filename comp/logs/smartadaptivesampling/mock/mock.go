// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a smart adaptive sampling component mock.
package mock

import (
	"testing"

	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	smartadaptivesampling "github.com/DataDog/datadog-agent/comp/logs/smartadaptivesampling/def"
)

// Component is a configurable smart adaptive sampling mock.
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
	Comp smartadaptivesampling.Component
}

// New creates a smart adaptive sampling mock.
func New(t testing.TB) Provides {
	t.Helper()
	return Provides{Comp: &Component{}}
}

var _ smartadaptivesampling.Component = (*Component)(nil)
