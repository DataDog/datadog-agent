// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build test

// Package mock provides a mock for the health platform status component.
package mock

import (
	"testing"

	statusdef "github.com/DataDog/datadog-agent/comp/healthplatform/status/def"
)

// Mock is a no-op mock for the health platform status component. The
// component has no public methods to fake: it exists solely to register an
// `agent status` information provider as a side effect of construction.
type Mock struct{}

// New returns a no-op mock health platform status component.
func New(*testing.T) statusdef.Component {
	return &Mock{}
}
