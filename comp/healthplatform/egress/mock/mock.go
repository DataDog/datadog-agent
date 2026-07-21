// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a no-op mock for the health platform egress component.
package mock

import (
	egressdef "github.com/DataDog/datadog-agent/comp/healthplatform/egress/def"
)

// Mock is a no-op implementation of egressdef.Component. The interface has no
// methods — egress behaviour is entirely lifecycle-driven (on each tick it
// POSTs store.GetAllIssues() to the forwarder) — so there is nothing to fake.
// This exists so tests can supply an egress component without pulling in the
// real implementation's networking and ticker. To test egress's own tick
// logic, construct the real (unexported) type directly, as
// egress/impl/egress_test.go does.
type Mock struct{}

// New returns a no-op mock egress for testing.
func New() egressdef.Component {
	return &Mock{}
}
