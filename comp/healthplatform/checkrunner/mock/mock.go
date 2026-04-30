// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a no-op mock for the health platform check runner.
package mock

import (
	"time"

	checkrunner "github.com/DataDog/datadog-agent/comp/healthplatform/checkrunner/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockCheckRunner struct{}

func (m *mockCheckRunner) SetReporter(_ checkrunner.IssueReporter) {}
func (m *mockCheckRunner) RegisterCheck(_ string, _ string, _ checkrunner.HealthCheckFunc, _ time.Duration) error {
	return nil
}
func (m *mockCheckRunner) RunCheck(_ string, _ string, _ checkrunner.HealthCheckFunc) error {
	return nil
}

// New returns a no-op mock check runner for testing.
func New() checkrunner.Component {
	return &mockCheckRunner{}
}

// MockModule provides a mock check runner via fx.
func MockModule() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(New))
}
