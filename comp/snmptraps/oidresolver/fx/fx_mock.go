// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package fx provides the fx module for the oidresolver component.
package fx

import (
	"go.uber.org/fx"

	oidresolverimpl "github.com/DataDog/datadog-agent/comp/snmptraps/oidresolver/impl"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockModule provides a dummy resolver with canned data for testing.
// Set your own data with fx.Replace(&oidresolver.TrapDBFileContent{...})
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(oidresolverimpl.NewMockResolver),
		fx.Supply(oidresolverimpl.DummyTrapDB()),
	)
}
