// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package oidresolver resolves OIDs
package oidresolver

import (
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"go.uber.org/fx"
)

// team: network-device-monitoring

// Component is a interface to get Trap and Variable metadata from OIDs
type Component interface {
	GetTrapMetadata(trapOID string) (TrapMetadata, error)
	GetVariableMetadata(trapOID string, varOID string) (VariableMetadata, error)
}

// Module defines the fx options for this component.
var Module = fxutil.Component(
	fx.Provide(newResolver),
)

// MockModule provides a dummy resolver with canned data.
// Set your own data with fx.Replace(&TrapDBFileContent{...})
var MockModule = fxutil.Component(
	fx.Provide(NewMockResolver),
	fx.Supply(&dummyTrapDB),
)
