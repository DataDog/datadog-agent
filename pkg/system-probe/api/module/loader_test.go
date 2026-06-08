// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"errors"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
)

type fakeModule struct {
	registered int
	closed     int
}

func (m *fakeModule) GetStats() map[string]interface{} { return nil }
func (m *fakeModule) Register(_ *Router) error         { m.registered++; return nil }
func (m *fakeModule) Close()                           { m.closed++ }

func fakeFactory(name sysconfigtypes.ModuleName, mod Module, err error) *Factory {
	return &Factory{
		Name: name,
		Fn: func(_ *sysconfigtypes.Config, _ FactoryDependencies) (Module, error) {
			return mod, err
		},
	}
}

// resetLoader replaces the package-level loader with a fresh one so each test is
// independent. httpMux is set so EnableModule can mount routers.
func resetLoader() {
	l = &loader{
		modules:    make(map[sysconfigtypes.ModuleName]Module),
		errors:     make(map[sysconfigtypes.ModuleName]error),
		routers:    make(map[sysconfigtypes.ModuleName]*Router),
		moduleStop: make(map[sysconfigtypes.ModuleName]chan struct{}),
		httpMux:    http.NewServeMux(),
	}
}

func TestEnableModuleLoadsWhenAbsent(t *testing.T) {
	resetLoader()
	mod := &fakeModule{}
	require.NoError(t, EnableModule(fakeFactory("test", mod, nil), FactoryDependencies{}))
	assert.True(t, IsLoaded("test"))
	assert.Equal(t, 1, mod.registered)
}

func TestEnableModuleNoopWhenPresent(t *testing.T) {
	resetLoader()
	mod := &fakeModule{}
	f := fakeFactory("test", mod, nil)
	require.NoError(t, EnableModule(f, FactoryDependencies{}))
	require.NoError(t, EnableModule(f, FactoryDependencies{}))
	assert.Equal(t, 1, mod.registered)
}

func TestEnableModulePropagatesFactoryError(t *testing.T) {
	resetLoader()
	require.Error(t, EnableModule(fakeFactory("test", nil, errors.New("boom")), FactoryDependencies{}))
	assert.False(t, IsLoaded("test"))
}

func TestDisableModuleClosesWhenPresent(t *testing.T) {
	resetLoader()
	mod := &fakeModule{}
	require.NoError(t, EnableModule(fakeFactory("test", mod, nil), FactoryDependencies{}))
	require.NoError(t, DisableModule("test"))
	assert.False(t, IsLoaded("test"))
	assert.Equal(t, 1, mod.closed)
}

func TestDisableModuleNoopWhenAbsent(t *testing.T) {
	resetLoader()
	assert.NoError(t, DisableModule("missing"))
}

func TestEnableDisableCycleReusesRouter(t *testing.T) {
	resetLoader()
	mod := &fakeModule{}
	f := fakeFactory("test", mod, nil)
	require.NoError(t, EnableModule(f, FactoryDependencies{}))
	require.NoError(t, DisableModule("test"))
	// Re-enabling must not panic by re-registering the prefix on the mux.
	require.NotPanics(t, func() { _ = EnableModule(f, FactoryDependencies{}) })
	assert.True(t, IsLoaded("test"))
}

func TestEnableModuleRefusedWhenClosed(t *testing.T) {
	resetLoader()
	l.closed = true
	assert.Error(t, EnableModule(fakeFactory("test", &fakeModule{}, nil), FactoryDependencies{}))
}

func TestDisableModuleRefusedWhenClosed(t *testing.T) {
	resetLoader()
	require.NoError(t, EnableModule(fakeFactory("test", &fakeModule{}, nil), FactoryDependencies{}))
	l.closed = true
	assert.Error(t, DisableModule("test"))
}
