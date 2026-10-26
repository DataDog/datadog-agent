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

// Mock is a serverlessinit component that records ReportNow calls.
type Mock struct {
	onReport func()
	reports  chan struct{}
}

var _ serverlessdef.Component = (*Mock)(nil)

// New returns a mock that runs onReport, when it is not nil, on each ReportNow
// call. Reported receives one value per call; it buffers up to capacity calls.
func New(_ *testing.T, capacity int, onReport func()) *Mock {
	return &Mock{onReport: onReport, reports: make(chan struct{}, capacity)}
}

// ReportNow runs the onReport callback and records the call.
func (m *Mock) ReportNow() {
	if m.onReport != nil {
		m.onReport()
	}
	m.reports <- struct{}{}
}

// Reported receives one value for each ReportNow call.
func (m *Mock) Reported() <-chan struct{} {
	return m.reports
}
