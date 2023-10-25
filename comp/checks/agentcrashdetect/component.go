// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build windows

// Package agentcrashdetect ... /* TODO: detailed doc comment for the component */
package agentcrashdetect

import (
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// team: windows-kernel-integrations

// Component is the component type.
type Component interface {
	/* TODO: define Component interface */
}

// Mock implements mock-specific methods.
type Mock interface {
	Component
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newAgentCrashComponent),
)

// MockModule defines the fx options for the mock component.
//var MockModule = fxutil.Component(
//	fx.Provide(newMock),
//)
