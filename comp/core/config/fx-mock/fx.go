// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package fx provides the fx mock module for the config component.
package fx

import (
	"testing"

	configdef "github.com/DataDog/datadog-agent/comp/core/config/def"
	configmock "github.com/DataDog/datadog-agent/comp/core/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// noopT satisfies testing.TB but ignores Cleanup calls.
// Used by MockModule when no real test context is available (e.g. fxutil.TestApp).
type noopT struct{ testing.TB }

func (noopT) Cleanup(func()) {}

// MockModule provides a mock config component via fx.
// Works with both fxutil.Test and fxutil.TestApp.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(
			func() configdef.Component {
				return configmock.NewWithTB(noopT{})
			},
		),
	)
}
