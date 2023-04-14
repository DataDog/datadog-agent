// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test
// +build test

package defaultforwarder

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Create a new file for `Mock` because `newMockForwarder`` requires //go:build test

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// MockModule defines the fx options for the mock component.
var MockModule = fxutil.Component(
	fx.Provide(newMockForwarder),
)
