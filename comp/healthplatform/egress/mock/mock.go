// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the health platform egress component.
package mock

import (
	"testing"

	forwarderdef "github.com/DataDog/datadog-agent/comp/healthplatform/forwarder/def"
	storedef "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
)

// Mock satisfies egressdef.Component, which has no public methods.
// The real egress drives periodic store→intake flushes through its fx lifecycle
// hooks; in unit tests those hooks are not invoked, so no behaviour is needed.
// store and forwarder are accepted to mirror the real egress dependency graph
// and make the composition explicit at the call site.
type Mock struct {
	t         testing.TB
	store     storedef.Component
	forwarder forwarderdef.Component
}

// New returns a mock egress for testing.
func New(t testing.TB, store storedef.Component, forwarder forwarderdef.Component) *Mock {
	return &Mock{t: t, store: store, forwarder: forwarder}
}
