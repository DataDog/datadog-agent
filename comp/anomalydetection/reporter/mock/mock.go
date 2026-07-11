// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the reporter component.
package mock

import (
	"testing"

	reporter "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
)

type noopReporter struct{}

func (r *noopReporter) Name() string                        { return "mock_reporter" }
func (r *noopReporter) Report(_ reporter.ReportOutput) bool { return false }

// Mock returns a no-op reporter for use in tests.
func Mock(_ *testing.T) reporter.Reporter {
	return &noopReporter{}
}
