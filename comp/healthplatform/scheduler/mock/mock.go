// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a no-op mock for the health platform scheduler.
package mock

import (
	"time"

	runnerdef "github.com/DataDog/datadog-agent/comp/healthplatform/runner/def"
	schedulerdef "github.com/DataDog/datadog-agent/comp/healthplatform/scheduler/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockScheduler struct{}

func (m *mockScheduler) Schedule(_ string, _ runnerdef.HealthCheckFunc, _ time.Duration, _ []string) error {
	return nil
}

// New returns a no-op mock scheduler for testing.
func New() schedulerdef.Component {
	return &mockScheduler{}
}

// MockModule provides a mock scheduler via fx.
func MockModule() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(New))
}
