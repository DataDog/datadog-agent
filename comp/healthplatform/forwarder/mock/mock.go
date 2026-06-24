// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a no-op mock for the health platform forwarder.
package mock

import (
	"context"

	healthplatformpayload "github.com/DataDog/agent-payload/v5/healthplatform"

	forwarder "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockForwarder struct{}

func (m *mockForwarder) Send(_ context.Context, _ *healthplatformpayload.HealthReport) error {
	return nil
}

// New returns a no-op mock forwarder for testing.
func New() forwarder.Component {
	return &mockForwarder{}
}

// MockModule provides a mock forwarder via fx.
func MockModule() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(New))
}
