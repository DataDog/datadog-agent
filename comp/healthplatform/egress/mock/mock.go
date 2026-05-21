// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a no-op mock for the health platform egress component.
package mock

import (
	egressdef "github.com/DataDog/datadog-agent/comp/healthplatform/egress/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

type mockEgress struct{}

// New returns a no-op mock egress for testing.
func New() egressdef.Component {
	return &mockEgress{}
}

// MockModule provides a mock egress via fx.
func MockModule() fxutil.Module {
	return fxutil.Component(fxutil.ProvideComponentConstructor(New))
}
