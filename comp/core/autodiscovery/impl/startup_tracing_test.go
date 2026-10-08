// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package autodiscoveryimpl

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/listeners"
	providerTypes "github.com/DataDog/datadog-agent/comp/core/autodiscovery/providers/types"
	acTelemetry "github.com/DataDog/datadog-agent/comp/core/autodiscovery/telemetry"
	tagger "github.com/DataDog/datadog-agent/comp/core/tagger/def"
	workloadfilter "github.com/DataDog/datadog-agent/comp/core/workloadfilter/def"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	healthplatform "github.com/DataDog/datadog-agent/comp/healthplatform/store/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup" //nolint:depguard // Legacy listener configuration type only; no global config access.
	"github.com/DataDog/datadog-agent/pkg/config/setup/constants"      //nolint:depguard // Legacy provider factory signature only; no global config access.
	"github.com/DataDog/datadog-agent/pkg/util/fxutil/startup"
)

type startupTraceListener struct{ calls int }

func (l *startupTraceListener) Listen(_, _ chan<- listeners.Service) { l.calls++ }
func (*startupTraceListener) Stop()                                  {}

func TestStartupListenerTracing(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "enabled"}[enabled], func(t *testing.T) {
			recorder := startup.NewRecorder(enabled)
			recorder.BeginHook(42)
			listener := &startupTraceListener{}
			ac := &AutoConfig{
				startupTracing:     recorder,
				listenerCandidates: make(map[string]*listenerCandidate),
				serviceListenerFactories: map[string]listeners.ServiceListenerFactory{
					"test": func(listeners.ServiceListernerDeps) (listeners.ServiceListener, error) { return listener, nil },
				},
			}
			ac.AddListeners([]pkgconfigsetup.Listeners{{Name: "test"}})
			require.Equal(t, 1, listener.calls, "Listen must still execute synchronously")
			require.Empty(t, ac.listenerCandidates)
			require.Len(t, ac.listeners, 1)
			recorder.EndHook()
			events, dropped := recorder.Drain()
			require.Zero(t, dropped)
			if !enabled {
				require.Empty(t, events)
				return
			}
			require.Len(t, events, 5)
			parent := events[0]
			require.Equal(t, "autodiscovery.listeners.initialize", parent.Name)
			require.EqualValues(t, 42, parent.ParentID)
			for i, name := range []string{
				"autodiscovery.listeners.register", "autodiscovery.listeners.lock",
				"autodiscovery.listener.initialize", "autodiscovery.listener.listen",
			} {
				require.Equal(t, name, events[i+1].Name)
				require.Equal(t, parent.SpanID, events[i+1].ParentID)
				require.False(t, events[i+1].Failed)
				require.False(t, events[i+1].Incomplete)
			}
			require.Equal(t, "test", events[3].Resource)
		})
	}
}

func TestStartupProviderFactoryErrorTracing(t *testing.T) {
	recorder := startup.NewRecorder(true)
	recorder.BeginHook(42)
	factoryError := errors.New("sensitive factory error")
	ac := &AutoConfig{
		startupTracing: recorder,
		providerCatalog: map[string]providerTypes.ConfigProviderFactory{
			"test": func(*constants.ConfigurationProviders, workloadmeta.Component, tagger.Component, workloadfilter.Component, healthplatform.Component, *acTelemetry.Store) (providerTypes.ConfigProvider, error) {
				return nil, factoryError
			},
		},
	}
	err := ac.AddConfigProviderFromCatalog(constants.ConfigurationProviders{Name: "test"})
	require.ErrorIs(t, err, factoryError)
	require.Empty(t, ac.configPollers)
	// Unrecognized user-supplied provider names must not become trace resources.
	require.Error(t, ac.AddConfigProviderFromCatalog(constants.ConfigurationProviders{Name: "unknown"}))
	events, _ := recorder.Drain()
	require.Len(t, events, 1)
	require.Equal(t, "autodiscovery.provider.initialize", events[0].Name)
	require.Equal(t, "test", events[0].Resource)
	require.True(t, events[0].Failed)
}

func TestListenerRetriesDoNotTraceOtherHooks(t *testing.T) {
	recorder := startup.NewRecorder(true)
	recorder.BeginHook(42)
	listener := &startupTraceListener{}
	ac := &AutoConfig{
		startupTracing: recorder,
		listenerCandidates: map[string]*listenerCandidate{
			"test": {factory: func(listeners.ServiceListernerDeps) (listeners.ServiceListener, error) { return listener, nil }},
		},
	}
	require.False(t, ac.initListenerCandidates())
	require.Equal(t, 1, listener.calls)
	events, _ := recorder.Drain()
	require.Empty(t, events, "background listener retries must not attach to another startup hook")
}
