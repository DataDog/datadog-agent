// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

package serverlessinitimpl

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/comp/core/config"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	inventoryagent "github.com/DataDog/datadog-agent/comp/metadata/inventoryagent/impl"
	runner "github.com/DataDog/datadog-agent/comp/metadata/runner/def"
	runnerfx "github.com/DataDog/datadog-agent/comp/metadata/runner/fx"
	serverlessdef "github.com/DataDog/datadog-agent/comp/metadata/serverlessinit/def"
	"github.com/DataDog/datadog-agent/pkg/config/model"
	"github.com/DataDog/datadog-agent/pkg/serializer"
	"github.com/DataDog/datadog-agent/pkg/serializer/marshaler"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	"github.com/DataDog/datadog-agent/pkg/version"
)

type recordingSerializer struct {
	serializer.MetricSerializer
	payloads []*inventoryagent.Payload
	err      error
	onSend   func()
}

func (s *recordingSerializer) SendMetadata(p marshaler.JSONMarshaler) error {
	s.payloads = append(s.payloads, p.(*inventoryagent.Payload))
	if s.onSend != nil {
		s.onSend()
	}
	return s.err
}

type fakeProvider struct {
	mu       sync.Mutex
	data     serverlessdef.ServerlessInitData
	eligible bool
	reads    int
}

func (p *fakeProvider) CanCollectInventory() bool { return p.eligible }
func (p *fakeProvider) GetInventoryData() serverlessdef.ServerlessInitData {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.reads++
	return p.data
}

func (p *fakeProvider) setData(data serverlessdef.ServerlessInitData) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.data = data
}

type fakeHostname struct{ hostnameinterface.Component }

func (fakeHostname) Get(context.Context) (string, error) { return "test-host", nil }
func (fakeHostname) GetWithProvider(context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{Hostname: "test-host", Provider: "configuration"}, nil
}

type hookRecorder struct{ hooks []compdef.Hook }

func (h *hookRecorder) Append(hook compdef.Hook) { h.hooks = append(h.hooks, hook) }

func newTestComponent(t *testing.T, overrides map[string]any, provider *fakeProvider, params serverlessdef.Params) (Provides, *recordingSerializer, *hookRecorder) {
	t.Helper()
	conf := config.NewMockWithOverrides(t, overrides)
	s := &recordingSerializer{}
	lc := &hookRecorder{}
	p := NewComponent(Requires{Lc: lc, Log: logmock.New(t), Config: conf, Params: params, Provider: provider, Serializer: s, Hostname: fakeHostname{}})
	return p, s, lc
}

func enabledConfig() map[string]any {
	return map[string]any{
		"serverless.inventory_enabled":      true,
		"inventories_first_run_delay":       3600,
		"inventories_min_interval":          2,
		"inventories_max_interval":          6,
		"inventories_configuration_enabled": false,
	}
}

func TestFirstCollectionAndPayloadCompatibility(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: "resource", WorkloadType: "azure_app_service"}}
		tags := map[string]string{"env": "test-env", "service": "test-service"}
		p, s, lc := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{Tags: tags, WrappedCommand: []string{"python", "app.py"}})
		require.Zero(t, provider.reads, "construction must not collect")
		require.Empty(t, s.payloads)
		require.Len(t, lc.hooks, 1)
		assert.Nil(t, lc.hooks[0].OnStart)
		tags["service"] = "mutated-after-construction"
		start := time.Now()
		assert.Equal(t, 2*time.Second, p.Provider.Callback(t.Context()))
		assert.Equal(t, start, time.Now(), "ready identity needs no delay")
		require.Len(t, s.payloads, 1)
		payload := s.payloads[0]
		assert.NotEmpty(t, payload.UUID)
		assert.Equal(t, "test-host", payload.Hostname)
		assert.Equal(t, start.UnixNano(), payload.Timestamp)
		for key, want := range map[string]any{
			"resource_id": "resource", "flavor": "serverless-init", "report_reason": "startup",
			"workload_type": "azure_app_service", "dd_env": "test-env", "dd_service": "test-service",
			"deployment_model": "in-container", "runtime": "Python", "wrapped_command": nil,
			"region": nil, "agent_version": version.AgentVersion, "package_version": version.AgentPackageVersion,
			"infrastructure_mode": "full", "hostname_source": "configuration",
		} {
			assert.Equal(t, want, payload.Metadata[key], key)
		}
		assert.Contains(t, payload.Metadata, "agent_startup_time_ms")
		assert.Contains(t, payload.Metadata, "install_method_tool")
		assert.NotContains(t, payload.Metadata, "feature_logs_enabled")
		assert.NotContains(t, payload.Metadata, "full_configuration")
		raw, err := payload.MarshalJSON()
		require.NoError(t, err)
		var wire map[string]any
		require.NoError(t, json.Unmarshal(raw, &wire))
		assert.Len(t, wire, 4)
		assert.Equal(t, "resource", wire["agent_metadata"].(map[string]any)["resource_id"])
		assert.Equal(t, 3600*time.Second, p.Comp.(*serverlessinitComp).conf.GetDuration("inventories_first_run_delay")*time.Second)
	})
}

func TestCollectionIntervalsAndIdentityUUID(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: "instance-a"}}
		p, s, _ := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{})
		interval := p.Provider.Callback(t.Context())
		require.Len(t, s.payloads, 1)
		firstUUID := s.payloads[0].UUID
		for range 2 {
			time.Sleep(interval)
			interval = p.Provider.Callback(t.Context())
			assert.Len(t, s.payloads, 1, "unchanged data waits for max interval")
		}
		time.Sleep(interval)
		p.Provider.Callback(t.Context())
		require.Len(t, s.payloads, 2)
		assert.Equal(t, firstUUID, s.payloads[1].UUID)
		assert.Equal(t, "periodic", s.payloads[1].Metadata["report_reason"])
		provider.data.ResourceName = "updated"
		time.Sleep(interval)
		p.Provider.Callback(t.Context())
		require.Len(t, s.payloads, 3)
		assert.Equal(t, firstUUID, s.payloads[2].UUID)
		assert.Equal(t, "updated", s.payloads[2].Metadata["resource_name"])
		provider.data = serverlessdef.ServerlessInitData{ResourceID: "instance-b"}
		time.Sleep(interval)
		p.Provider.Callback(t.Context())
		require.Len(t, s.payloads, 4)
		assert.NotEqual(t, firstUUID, s.payloads[3].UUID)
		assert.Equal(t, "startup", s.payloads[3].Metadata["report_reason"])
		assert.Nil(t, s.payloads[3].Metadata["resource_name"])
		assert.Equal(t, "instance-a", s.payloads[0].Metadata["resource_id"], "previous payload stays immutable")
	})
}

func TestLateIdentityAndCanceledWait(t *testing.T) {
	for _, late := range []bool{false, true} {
		t.Run(map[bool]string{false: "canceled", true: "late identity"}[late], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: " \t"}}
				p, s, _ := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{})
				ctx, cancel := context.WithCancel(t.Context())
				defer cancel()
				done := make(chan struct{})
				go func() { p.Provider.Callback(ctx); close(done) }()
				synctest.Wait()
				assert.Empty(t, s.payloads)
				assert.Positive(t, provider.reads)
				if late {
					provider.setData(serverlessdef.ServerlessInitData{ResourceID: "late-resource"})
					time.Sleep(identityRetryInterval)
				} else {
					cancel()
				}
				<-done
				if late {
					require.Len(t, s.payloads, 1)
					assert.Equal(t, "late-resource", s.payloads[0].Metadata["resource_id"])
				} else {
					assert.Empty(t, s.payloads)
				}
			})
		})
	}
}

func TestCollectionDisabled(t *testing.T) {
	for _, key := range []string{"serverless.inventory_enabled", "inventories_enabled", "enable_metadata_collection", "ineligible provider"} {
		t.Run(key, func(t *testing.T) {
			conf := enabledConfig()
			provider := &fakeProvider{eligible: key != "ineligible provider"}
			if key == "serverless.inventory_enabled" {
				delete(conf, key)
			} else if key != "ineligible provider" {
				conf[key] = false
			}
			p, s, lc := newTestComponent(t, conf, provider, serverlessdef.Params{})
			assert.Nil(t, p.Provider.Callback)
			assert.Empty(t, lc.hooks)
			p.Comp.(*serverlessinitComp).collect(t.Context())
			require.NoError(t, p.Comp.(*serverlessinitComp).stop(t.Context()))
			assert.Zero(t, provider.reads)
			assert.Empty(t, s.payloads)
		})
	}
}

func TestShutdownBeforeFirstCallback(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: "ready"}}
		p, s, lc := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{})
		start := time.Now()
		require.NoError(t, lc.hooks[0].OnStop(t.Context()))
		assert.Equal(t, start, time.Now())
		require.Len(t, s.payloads, 1)
		assert.Equal(t, "startup", s.payloads[0].Metadata["report_reason"])
		p.Provider.Callback(t.Context())
		require.NoError(t, lc.hooks[0].OnStop(t.Context()))
		assert.Len(t, s.payloads, 1, "a delayed callback or repeated stop must not duplicate the final payload")
	})
}

func TestShutdownSkipsSubmittedIdentity(t *testing.T) {
	for _, shutdownResourceID := range []string{"ready", ""} {
		synctest.Test(t, func(t *testing.T) {
			provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: "ready"}}
			p, s, lc := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{})
			p.Provider.Callback(t.Context())
			provider.setData(serverlessdef.ServerlessInitData{ResourceID: shutdownResourceID})
			start := time.Now()
			require.NoError(t, lc.hooks[0].OnStop(t.Context()))
			assert.Equal(t, start, time.Now())
			assert.Len(t, s.payloads, 1)
		})
	}
}

func TestShutdownSubmitsChangedIdentity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: "first"}}
		p, s, lc := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{})
		p.Provider.Callback(t.Context())
		require.Len(t, s.payloads, 1)
		provider.setData(serverlessdef.ServerlessInitData{ResourceID: "second"})
		require.NoError(t, lc.hooks[0].OnStop(t.Context()))
		require.Len(t, s.payloads, 2)
		assert.Equal(t, "second", s.payloads[1].Metadata["resource_id"])
		assert.Equal(t, "startup", s.payloads[1].Metadata["report_reason"])
		assert.NotEqual(t, s.payloads[0].UUID, s.payloads[1].UUID)
	})
}

func TestShutdownGraceAndDeadline(t *testing.T) {
	for _, tt := range []struct {
		name     string
		deadline time.Duration
		late     bool
	}{
		{name: "never ready", deadline: time.Second},
		{name: "earlier deadline", deadline: 20 * time.Millisecond},
		{name: "already expired"},
		{name: "identity during grace", deadline: time.Second, late: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				provider := &fakeProvider{eligible: true}
				_, s, lc := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{})
				ctx, cancel := context.WithTimeout(t.Context(), tt.deadline)
				defer cancel()
				start := time.Now()
				if tt.late {
					done := make(chan struct{})
					go func() { assert.NoError(t, lc.hooks[0].OnStop(ctx)); close(done) }()
					synctest.Wait()
					provider.setData(serverlessdef.ServerlessInitData{ResourceID: "late"})
					<-done
					require.Len(t, s.payloads, 1)
					assert.Equal(t, identityRetryInterval, time.Since(start))
				} else {
					require.NoError(t, lc.hooks[0].OnStop(ctx))
					assert.Equal(t, min(shutdownGrace, tt.deadline), time.Since(start))
					assert.Empty(t, s.payloads)
				}
			})
		})
	}
}

func TestShutdownSerializerError(t *testing.T) {
	for _, finalError := range []error{nil, errors.New("still unavailable")} {
		synctest.Test(t, func(t *testing.T) {
			provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: "ready"}}
			p, s, lc := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{})
			s.err = errors.New("enqueue failed")
			p.Provider.Callback(t.Context())
			require.Len(t, s.payloads, 1)
			assert.False(t, p.Comp.(*serverlessinitComp).submittedCurrentIdentity)
			s.err = finalError
			require.NoError(t, lc.hooks[0].OnStop(t.Context()))
			require.Len(t, s.payloads, 2)
			assert.Equal(t, "startup", s.payloads[1].Metadata["report_reason"])
			assert.Equal(t, s.payloads[0].UUID, s.payloads[1].UUID)
			assert.Equal(t, finalError == nil, p.Comp.(*serverlessinitComp).submittedCurrentIdentity)
		})
	}
}

func TestShutdownConcurrentSubmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: "ready"}}
		p, s, lc := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{})
		entered, release := make(chan struct{}), make(chan struct{})
		s.onSend = func() { close(entered); <-release }
		collectionDone := make(chan struct{})
		go func() { p.Provider.Callback(t.Context()); close(collectionDone) }()
		<-entered
		stopDone := make(chan struct{})
		go func() { assert.NoError(t, lc.hooks[0].OnStop(t.Context())); close(stopDone) }()
		synctest.Wait()
		close(release)
		<-collectionDone
		<-stopDone
		assert.Len(t, s.payloads, 1)
	})
}

func TestShutdownDoesNotWaitIndefinitelyForSerializer(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: "ready"}}
		p, s, lc := newTestComponent(t, enabledConfig(), provider, serverlessdef.Params{})
		entered, release := make(chan struct{}), make(chan struct{})
		s.onSend = func() { close(entered); <-release }
		done := make(chan struct{})
		go func() { p.Provider.Callback(t.Context()); close(done) }()
		<-entered
		start := time.Now()
		require.NoError(t, lc.hooks[0].OnStop(t.Context()))
		assert.Equal(t, shutdownGrace, time.Since(start))
		close(release)
		<-done
		assert.Len(t, s.payloads, 1)
	})
}

// These stand in for the serializer adapter's real dependency chain in main.
type forwarderDrain struct{}
type demuxDrain struct{}

func TestRunnerShutdownOrdering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		conf := config.NewMockWithOverrides(t, enabledConfig())
		conf.Set("metadata_provider_stop_timeout", time.Second, model.SourceAgentRuntime)
		provider := &fakeProvider{eligible: true}
		var order []string
		s := &recordingSerializer{onSend: func() { order = append(order, "enqueue") }}
		var comp serverlessdef.Component
		app := fx.New(
			fx.NopLogger,
			fxutil.FxLifecycleAdapter(),
			fx.Supply(serverlessdef.Params{}),
			fx.Provide(func() config.Component { return conf }),
			fx.Provide(func() log.Component { return logmock.New(t) }),
			fx.Provide(func() hostnameinterface.Component { return fakeHostname{} }),
			fx.Provide(func() serverlessdef.InventoryProvider { return provider }),
			fx.Provide(func(lc fx.Lifecycle) forwarderDrain {
				lc.Append(fx.Hook{OnStop: func(context.Context) error { order = append(order, "forwarder drain"); return nil }})
				return forwarderDrain{}
			}),
			fx.Provide(func(lc fx.Lifecycle, _ forwarderDrain) demuxDrain {
				lc.Append(fx.Hook{OnStop: func(context.Context) error { order = append(order, "demux drain"); return nil }})
				return demuxDrain{}
			}),
			fx.Provide(func(_ demuxDrain) serializer.MetricSerializer { return s }),
			fxutil.ProvideComponentConstructor(NewComponent),
			runnerfx.Module(),
			fx.Populate(&comp),
			fx.Invoke(func(_ runner.Component) {}),
		)
		require.NoError(t, app.Err())
		require.Zero(t, provider.reads)
		require.NoError(t, app.Start(t.Context()))
		synctest.Wait()
		require.Positive(t, provider.reads)
		require.Empty(t, s.payloads)
		// The runner is waiting for identity. Its cancellation must release
		// collection before the provider's shutdown attempt can enqueue.
		stopped := make(chan error, 1)
		go func() { stopped <- app.Stop(t.Context()) }()
		synctest.Wait()
		require.True(t, comp.(*serverlessinitComp).stopped)
		provider.setData(serverlessdef.ServerlessInitData{ResourceID: "shutdown-identity"})
		require.NoError(t, <-stopped)
		assert.Equal(t, []string{"enqueue", "demux drain", "forwarder drain"}, order)
		require.Len(t, s.payloads, 1)
		assert.Equal(t, "shutdown-identity", s.payloads[0].Metadata["resource_id"])
	})
}

func TestCollectionConfigurationAndWrappedCommand(t *testing.T) {
	conf := enabledConfig()
	conf["inventories_configuration_enabled"] = true
	conf["serverless.inventory_wrapped_command_enabled"] = true
	conf["api_key"] = "secret-api-key"
	provider := &fakeProvider{eligible: true, data: serverlessdef.ServerlessInitData{ResourceID: "resource"}}
	p, s, _ := newTestComponent(t, conf, provider, serverlessdef.Params{WrappedCommand: []string{"python", "app.py"}})
	p.Provider.Callback(t.Context())
	require.Len(t, s.payloads, 1)
	assert.Equal(t, "python app.py", s.payloads[0].Metadata["wrapped_command"])
	assert.Contains(t, s.payloads[0].Metadata, "full_configuration")
	raw, err := s.payloads[0].MarshalJSON()
	require.NoError(t, err)
	assert.NotContains(t, string(raw), "secret-api-key")
}
