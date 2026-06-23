// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a no-op mock for the health platform egress component.
package mock

import (
	"testing"

	egressdef "github.com/DataDog/datadog-agent/comp/healthplatform/egress/def"
)

// mockEgress satisfies egressdef.Component, which has no public methods.
// The real egress drives periodic store→intake flushes through its fx lifecycle
// hooks; in unit tests those hooks are not invoked, so no behaviour is needed.
type mockEgress struct {
	t testing.TB
}

// New returns a mock egress for testing.
func New(t testing.TB) egressdef.Component {
	return &mockEgress{t: t}
}
