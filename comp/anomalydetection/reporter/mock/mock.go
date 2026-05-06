// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the reporter component
package mock

import (
	"testing"

	reporter "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
)

// Mock returns a mock for reporter component.
func Mock(t *testing.T) reporter.Component {
	// TODO: Implement the reporter mock
	return nil
}
