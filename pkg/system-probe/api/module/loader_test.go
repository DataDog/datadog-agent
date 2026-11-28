// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package module

import (
	"errors"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	sysconfigtypes "github.com/DataDog/datadog-agent/pkg/system-probe/config/types"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/logging"
)

type startupTestModule struct {
	registerErr    error
	registerCalled bool
}

type shutdownTestModule struct {
	startupTestModule
	closed int
}

func (m *shutdownTestModule) Close() { m.closed++ }

type tracedShutdownTestModule struct {
	shutdownTestModule
	phase *logging.StartupPhase
}

func (m *tracedShutdownTestModule) CloseWithTrace(phase *logging.StartupPhase) {
	m.phase = phase
	m.closed++
}

func TestCloseWithTraceDispatchAndIdempotence(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			previous := l
			t.Cleanup(func() { l = previous })
			plain := &shutdownTestModule{}
			traced := &tracedShutdownTestModule{}
			l = &loader{modules: map[sysconfigtypes.ModuleName]Module{"plain": plain, "traced": traced}}
			var phase *logging.StartupPhase
			if enabled {
				logger := &logging.FxTracingLogger{}
				logger.EnableSpansSending("8126")
				phase = logging.StartStartupTrace(logger, "stopSystemProbe", "test").StartPhase("module.Close")
			}
			CloseWithTrace(phase)
			CloseWithTrace(phase)
			require.Equal(t, 1, plain.closed)
			require.Equal(t, 1, traced.closed, "the traced method must replace, rather than duplicate, Close")
			require.Equal(t, enabled, traced.phase != nil)
			require.Empty(t, l.modules)
			require.True(t, l.closed)
		})
	}
}

func (*startupTestModule) GetStats() map[string]interface{} { return nil }
func (*startupTestModule) Close()                           {}
func (m *startupTestModule) Register(*Router) error {
	m.registerCalled = true
	return m.registerErr
}

func TestRegisterFactory(t *testing.T) {
	factoryErr := errors.New("factory failed")
	registerErr := errors.New("registration failed")
	for _, test := range []struct {
		name        string
		factoryErr  error
		registerErr error
	}{
		{name: "success"},
		{name: "factory failure", factoryErr: factoryErr},
		{name: "registration failure", registerErr: registerErr},
	} {
		t.Run(test.name, func(t *testing.T) {
			loader := &loader{
				modules: make(map[sysconfigtypes.ModuleName]Module),
				errors:  make(map[sysconfigtypes.ModuleName]error),
				routers: make(map[sysconfigtypes.ModuleName]*Router),
			}
			mod := &startupTestModule{registerErr: test.registerErr}
			factory := &Factory{
				Name: sysconfigtypes.ModuleName("test"),
				Fn: func(*sysconfigtypes.Config, FactoryDependencies) (Module, error) {
					return mod, test.factoryErr
				},
			}
			loader.registerFactory(nil, http.NewServeMux(), factory, FactoryDependencies{}, nil)
			if test.factoryErr != nil {
				require.False(t, mod.registerCalled)
				require.ErrorIs(t, loader.errors[factory.Name], test.factoryErr)
			} else if test.registerErr != nil {
				require.True(t, mod.registerCalled)
				require.ErrorIs(t, loader.errors[factory.Name], test.registerErr)
			} else {
				require.True(t, mod.registerCalled)
				require.Same(t, mod, loader.modules[factory.Name])
				require.NotNil(t, loader.routers[factory.Name])
				require.Empty(t, loader.errors)
				return
			}
			require.Empty(t, loader.modules, "failed modules must not be registered")
			require.Empty(t, loader.routers)
		})
	}
}
