// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build test

// Package mock provides a test mock for the installinfo component.
package mock

import (
	"go.uber.org/fx"

	installinfo "github.com/DataDog/datadog-agent/comp/agent/installinfo/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// Mock implements installinfo.Component for testing.
// Set Info and/or Err before calling Get to control the response.
type Mock struct {
	Info *installinfo.InstallInfo
	Err  error
}

var _ installinfo.Component = (*Mock)(nil)

// Get returns Info and Err as configured, or an empty InstallInfo by default.
func (m *Mock) Get() (*installinfo.InstallInfo, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	if m.Info != nil {
		return m.Info, nil
	}
	return &installinfo.InstallInfo{}, nil
}

// New returns a new Mock with default (empty, no-error) behaviour.
func New() *Mock {
	return &Mock{}
}

// MockModule provides a default mock installinfo component via FX.
func MockModule() fxutil.Module {
	return fxutil.Component(
		fx.Provide(func() installinfo.Component { return New() }),
	)
}
