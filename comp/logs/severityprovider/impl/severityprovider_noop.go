// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build !python

// Package severityproviderimpl implements the severity provider component.
package severityproviderimpl

import (
	severityeventsdef "github.com/DataDog/datadog-agent/comp/anomalydetection/severityevents/def"
	severityprovider "github.com/DataDog/datadog-agent/comp/logs/severityprovider/def"
)

// Provides defines the severity provider component output.
type Provides struct {
	Comp severityprovider.Component
}

type component struct{}

// NewComponent creates an unavailable component for builds without Python.
func NewComponent() Provides {
	return Provides{Comp: component{}}
}

// Current reports that severity profiles are unavailable in this build.
func (component) Current() (severityeventsdef.SeverityLevel, bool) {
	return severityeventsdef.SeverityLow, false
}

var _ severityprovider.Component = component{}
