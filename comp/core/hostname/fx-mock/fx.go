// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

//go:build test

// Package fx provides an fx module for the hostname component in tests.
package fx

import (
	"context"

	"go.uber.org/fx"

	hostname "github.com/DataDog/datadog-agent/comp/core/hostname/def"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

// MockHostname is a type for injecting a mock hostname via fx.
// Usage: fx.Replace(fxmock.MockHostname("my-host"))
type MockHostname string

// MockModule provides the hostname component for testing via fx.
// Injecting MockModule will provide the hostname 'my-hostname';
// override with fx.Replace(MockHostname("whatever")).
func MockModule() fxutil.Module {
	return fxutil.Component(
		fxutil.ProvideComponentConstructor(newFxMock),
		fx.Supply(MockHostname("my-hostname")),
	)
}

type fxRequires struct {
	fx.In
	Name MockHostname
}

type fxMockService struct{ name string }

func (m *fxMockService) Get(_ context.Context) (string, error) { return m.name, nil }
func (m *fxMockService) GetSafe(_ context.Context) string      { return m.name }
func (m *fxMockService) GetWithProvider(_ context.Context) (hostname.Data, error) {
	return hostname.Data{Hostname: m.name, Provider: "fxmock"}, nil
}

type fxProvides struct {
	fx.Out
	Comp hostname.Component
}

func newFxMock(deps fxRequires) fxProvides {
	return fxProvides{Comp: &fxMockService{name: string(deps.Name)}}
}
