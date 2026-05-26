// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/config"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
)

func TestResolveTargetDetectsRuntime(t *testing.T) {
	tests := []struct {
		name       string
		setupStore func(t *testing.T) workloadmeta.Component
		config     integration.Config
		wantTarget target
		wantOK     bool
	}{
		{
			name: "host process",
			config: integration.Config{
				Name:      "redis",
				ServiceID: "process://1234",
				Instances: []integration.Data{
					[]byte("{}"),
				},
			},
			wantTarget: target{
				runtime:  RuntimeHost,
				entityID: "1234",
			},
			wantOK: true,
		},
		{
			name: "standalone docker container",
			config: integration.Config{
				Name:      "redis",
				ServiceID: "docker://abc123",
				Instances: []integration.Data{
					[]byte("{}"),
				},
			},
			wantTarget: target{
				runtime:  RuntimeDocker,
				entityID: "abc123",
			},
			wantOK: true,
		},
		{
			name: "container with kubernetes pod owner",
			setupStore: func(t *testing.T) workloadmeta.Component {
				store := newWorkloadMetaMock(t)
				store.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "abc123"},
					Runtime:  workloadmeta.ContainerRuntimeContainerd,
					Owner:    &workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid"},
				})
				store.Set(&workloadmeta.KubernetesPod{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid"},
					EntityMeta: workloadmeta.EntityMeta{
						Name:      "redis-0",
						Namespace: "default",
					},
				})
				return store
			},
			config: integration.Config{
				Name:      "redis",
				ServiceID: "containerd://abc123",
				Instances: []integration.Data{
					[]byte("{}"),
				},
			},
			wantTarget: target{
				runtime:  RuntimeKubernetes,
				entityID: "abc123",
			},
			wantOK: true,
		},
		{
			name: "unsupported standalone container runtime",
			config: integration.Config{
				Name:      "redis",
				ServiceID: "containerd://abc123",
				Instances: []integration.Data{
					[]byte("{}"),
				},
			},
			wantOK: false,
		},
		{
			name: "malformed service id",
			config: integration.Config{
				Name:      "redis",
				ServiceID: "not-an-ad-service-id",
				Instances: []integration.Data{
					[]byte("{}"),
				},
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var store workloadmeta.Component
			if tt.setupStore != nil {
				store = tt.setupStore(t)
			}
			resolver := targetResolver{store: store}

			got, ok := resolver.Resolve(tt.config)

			assert.Equal(t, tt.wantOK, ok)
			if tt.wantOK {
				assert.Equal(t, tt.wantTarget, got)
			}
		})
	}
}

func TestSchedulerDispatchesRegisteredIntegrationsOnly(t *testing.T) {
	collector := &recordingConfigCollector{}
	readerFactory := &recordingConfigReaderFactory{reader: fakeConfigReader{runtime: RuntimeHost}}
	withConfigCollectors(t, map[string]configCollector{"redis": collector})
	withConfigReaders(t, map[RuntimeType]configReaderFactory{RuntimeHost: readerFactory.Build})
	s := newADScheduler(targetResolver{})

	s.Schedule([]integration.Config{
		checkConfig("redis", "process://1234"),
		checkConfig("nginx", "process://5678"),
		{Name: "", ServiceID: "process://9999", Instances: []integration.Data{[]byte("{}")}},
		{Name: "redis", ServiceID: "process://9999", LogsConfig: []byte(`[{}]`)},
		{Name: "redis", ServiceID: "process://9999", ClusterCheck: true, Instances: []integration.Data{[]byte("{}")}},
	})

	require.Len(t, collector.runs, 1)
	assert.Equal(t, RuntimeHost, collector.runs[0].reader.Runtime())
	require.Len(t, readerFactory.targets, 1)
	assert.Equal(t, target{runtime: RuntimeHost, entityID: "1234"}, readerFactory.targets[0])
}

func TestSchedulerContinuesAfterInvalidConfigInBatch(t *testing.T) {
	collector := &recordingConfigCollector{}
	withConfigCollectors(t, map[string]configCollector{"redis": collector})
	withConfigReaders(t, map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})})
	s := newADScheduler(targetResolver{})

	s.Schedule([]integration.Config{
		checkConfig("redis", "not-an-ad-service-id"),
		checkConfig("redis", "docker://abc123"),
	})

	require.Len(t, collector.runs, 1)
	assert.Equal(t, RuntimeDocker, collector.runs[0].reader.Runtime())
}

func TestComponentRegistersAutodiscoverySchedulerOnStart(t *testing.T) {
	ac := &fakeAutodiscovery{}
	lifecycle := &recordingLifecycle{}

	NewComponent(Requires{
		Lifecycle:     lifecycle,
		Autodiscovery: ac,
	})

	require.NotNil(t, lifecycle.hook.OnStart)
	require.NoError(t, lifecycle.hook.OnStart(context.Background()))
	assert.Equal(t, schedulerName, ac.addedName)
	assert.True(t, ac.replay)
	require.Implements(t, (*scheduler.Scheduler)(nil), ac.scheduler)

	require.NotNil(t, lifecycle.hook.OnStop)
	require.NoError(t, lifecycle.hook.OnStop(context.Background()))
	assert.Equal(t, schedulerName, ac.removedName)
}

func checkConfig(name string, serviceID string) integration.Config {
	return integration.Config{
		Name:      name,
		ServiceID: serviceID,
		Instances: []integration.Data{
			[]byte("{}"),
		},
	}
}

type recordingLifecycle struct {
	hook compdef.Hook
}

func (l *recordingLifecycle) Append(hook compdef.Hook) {
	l.hook = hook
}

type recordingConfigCollector struct {
	runs []runCall
}

type runCall struct {
	reader ConfigReader
}

func (c *recordingConfigCollector) Run(_ context.Context, reader ConfigReader) error {
	c.runs = append(c.runs, runCall{
		reader: reader,
	})
	return nil
}

type fakeConfigReader struct {
	runtime RuntimeType
}

func (r fakeConfigReader) Runtime() RuntimeType {
	return r.runtime
}

func fakeConfigReaderFactory(reader ConfigReader) configReaderFactory {
	return func(target) (ConfigReader, error) {
		return reader, nil
	}
}

type recordingConfigReaderFactory struct {
	reader  ConfigReader
	targets []target
}

func (f *recordingConfigReaderFactory) Build(target target) (ConfigReader, error) {
	f.targets = append(f.targets, target)
	return f.reader, nil
}

func withConfigCollectors(t *testing.T, collectors map[string]configCollector) {
	t.Helper()

	oldCollectors := configCollectors
	configCollectors = collectors
	t.Cleanup(func() {
		configCollectors = oldCollectors
	})
}

func withConfigReaders(t *testing.T, readers map[RuntimeType]configReaderFactory) {
	t.Helper()

	oldReaders := configReaders
	configReaders = readers
	t.Cleanup(func() {
		configReaders = oldReaders
	})
}

func newWorkloadMetaMock(t *testing.T) workloadmetamock.Mock {
	t.Helper()

	return workloadmetaimpl.NewWorkloadMetaMock(workloadmetaimpl.Dependencies{
		Lc:     compdef.NewTestLifecycle(t),
		Log:    logmock.New(t),
		Config: config.NewMock(t),
		Params: workloadmeta.NewParams(),
	})
}

type fakeAutodiscovery struct {
	autodiscovery.Component

	addedName   string
	removedName string
	scheduler   scheduler.Scheduler
	replay      bool
}

func (a *fakeAutodiscovery) AddScheduler(name string, scheduler scheduler.Scheduler, replay bool) {
	a.addedName = name
	a.scheduler = scheduler
	a.replay = replay
}

func (a *fakeAutodiscovery) RemoveScheduler(name string) {
	a.removedName = name
}
