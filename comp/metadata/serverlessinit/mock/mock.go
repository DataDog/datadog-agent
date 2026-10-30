// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the serverlessinit component.
package mock

import (
	"testing"

	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
)

// Provides exposes the mock component and its record of ReportNow calls.
type Provides struct {
	Comp serverlessdef.Component
	// Reported receives one value for each ReportNow call.
	Reported <-chan struct{}
}

type mock struct {
	onReport func()
	reports  chan struct{}
}

// New returns a mock that runs onReport, when it is not nil, on each ReportNow
// call. Reported buffers up to capacity calls.
func New(_ *testing.T, capacity int, onReport func()) Provides {
	m := &mock{onReport: onReport, reports: make(chan struct{}, capacity)}
	return Provides{Comp: m, Reported: m.reports}
}

// ReportNow runs the onReport callback and records the call.
func (m *mock) ReportNow() {
	if m.onReport != nil {
		m.onReport()
	}
	m.reports <- struct{}{}
}
