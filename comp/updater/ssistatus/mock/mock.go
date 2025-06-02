// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the installerexec component
package mock

import (
	"context"
	"testing"

	ssiStatus "github.com/DataDog/datadog-agent/comp/updater/ssistatus/def"
)

type ssiStatusMock struct {
	instrumentationModes []string
}

func (m *ssiStatusMock) AutoInstrumentationStatus(_ context.Context) (bool, []string, error) {
	if len(m.instrumentationModes) == 0 {
		return false, nil, nil
	}
	return true, m.instrumentationModes, nil
}

// Mock returns a mock for installerexec component.
func Mock(_ *testing.T) ssiStatus.Component {
	return &ssiStatusMock{}
}

// WithInstrumentationModes returns a mock for installerexec component.
func WithInstrumentationModes(_ *testing.T, modes []string) ssiStatus.Component {
	return &ssiStatusMock{
		instrumentationModes: modes,
	}
}
