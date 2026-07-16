// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	autodiscovery "github.com/DataDog/datadog-agent/comp/core/autodiscovery/def"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/scheduler"
	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/hostname"
	hostnameinterface "github.com/DataDog/datadog-agent/comp/core/hostname/hostnameinterface/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetaimpl "github.com/DataDog/datadog-agent/comp/core/workloadmeta/impl"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	compdef "github.com/DataDog/datadog-agent/comp/def"
	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
)

const testRedisIntegrationName = "redisdb"
const testRedisConfigPayloadFormat = agentdiscovery.AgentDiscoveryConfigFilePayloadFormat_PAYLOAD_FORMAT_REDIS_CONF

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
			name: "standalone container service with docker runtime",
			setupStore: func(t *testing.T) workloadmeta.Component {
				store := newWorkloadMetaMock(t)
				store.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "abc123"},
					Runtime:  workloadmeta.ContainerRuntimeDocker,
				})
				return store
			},
			config: integration.Config{
				Name:      "redis",
				ServiceID: "container://abc123",
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
			name: "kubernetes pod service is unsupported",
			config: integration.Config{
				Name:      "redis",
				ServiceID: "kubernetes_pod://pod-uid",
				Instances: []integration.Data{
					[]byte("{}"),
				},
			},
			wantOK: false,
		},
		{
			name: "container service with kubernetes pod owner",
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
				ServiceID: "container://abc123",
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
			name: "containerd service with kubernetes pod owner",
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
			name: "container service with kubernetes pod owner and docker runtime",
			setupStore: func(t *testing.T) workloadmeta.Component {
				store := newWorkloadMetaMock(t)
				store.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "abc123"},
					Runtime:  workloadmeta.ContainerRuntimeDocker,
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
				ServiceID: "container://abc123",
				Instances: []integration.Data{
					[]byte("{}"),
				},
			},
			wantOK: false,
		},
		{
			name: "unsupported standalone container service runtime",
			setupStore: func(t *testing.T) workloadmeta.Component {
				store := newWorkloadMetaMock(t)
				store.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "abc123"},
					Runtime:  workloadmeta.ContainerRuntimeContainerd,
				})
				return store
			},
			config: integration.Config{
				Name:      "redis",
				ServiceID: "container://abc123",
				Instances: []integration.Data{
					[]byte("{}"),
				},
			},
			wantOK: false,
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

func TestSchedulerDispatchesKubernetesOwnedContainerToKubernetesReader(t *testing.T) {
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

	collector := &recordingConfigCollector{}
	readerFactory := &recordingConfigReaderFactory{reader: fakeConfigReader{runtime: RuntimeKubernetes}}
	s := newADScheduler(
		targetResolver{store: store},
		map[RuntimeType]configReaderFactory{RuntimeKubernetes: readerFactory.Build},
		map[string]ConfigCollector{"redis": collector},
		nil,
	)
	defer s.Stop()

	s.Schedule([]integration.Config{
		checkConfig("redis", "containerd://abc123"),
		checkConfig("redis", "containerd://standalone"),
	})

	collector.waitForRuns(t, 1)
	targets := readerFactory.recordedTargets()
	require.Len(t, targets, 1)
	assert.Equal(t, target{runtime: RuntimeKubernetes, entityID: "abc123"}, targets[0])
}

func TestSchedulerClosesReaderAfterCollection(t *testing.T) {
	tests := []struct {
		name         string
		files        []ConfigFile
		collectorErr error
	}{
		{
			name:  "successful collection and send",
			files: []ConfigFile{{Path: "/etc/redis/redis.conf"}},
		},
		{
			name:         "collector error",
			collectorErr: errors.New("collection failed"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			closed := make(chan struct{})
			collector := &recordingConfigCollector{files: tt.files, err: tt.collectorErr}
			readerFactory := fakeConfigReaderFactory(fakeConfigReader{
				runtime: RuntimeDocker,
				closeFunc: func() {
					close(closed)
				},
			})
			s := newADScheduler(
				targetResolver{},
				map[RuntimeType]configReaderFactory{RuntimeDocker: readerFactory},
				map[string]ConfigCollector{"redis": collector},
				nil,
			)
			defer s.Stop()

			s.Schedule([]integration.Config{
				checkConfig("redis", "docker://abc123"),
			})

			collector.waitForRuns(t, 1)
			require.Eventually(t, func() bool {
				select {
				case <-closed:
					return true
				default:
					return false
				}
			}, time.Second, 10*time.Millisecond)
		})
	}
}

func TestSchedulerDispatchesRegisteredIntegrationsOnly(t *testing.T) {
	collector := &recordingConfigCollector{}
	readerFactory := &recordingConfigReaderFactory{reader: fakeConfigReader{runtime: RuntimeHost}}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeHost: readerFactory.Build},
		map[string]ConfigCollector{"redis": collector},
		nil,
	)
	defer s.Stop()

	s.Schedule([]integration.Config{
		checkConfig("redis", "process://1234"),
		checkConfig("nginx", "process://5678"),
		{Name: "", ServiceID: "process://9999", Instances: []integration.Data{[]byte("{}")}},
		{Name: "redis", ServiceID: "process://9999", LogsConfig: []byte(`[{}]`)},
		{Name: "redis", ServiceID: "process://9999", ClusterCheck: true, Instances: []integration.Data{[]byte("{}")}},
	})

	collector.waitForRuns(t, 1)
	targets := readerFactory.recordedTargets()
	require.Len(t, targets, 1)
	assert.Equal(t, target{runtime: RuntimeHost, entityID: "1234"}, targets[0])
}

func TestSchedulerContinuesAfterInvalidConfigInBatch(t *testing.T) {
	collector := &recordingConfigCollector{}
	readerFactory := &recordingConfigReaderFactory{reader: fakeConfigReader{runtime: RuntimeDocker}}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: readerFactory.Build},
		map[string]ConfigCollector{"redis": collector},
		nil,
	)
	defer s.Stop()

	s.Schedule([]integration.Config{
		checkConfig("redis", "not-an-ad-service-id"),
		checkConfig("redis", "docker://abc123"),
	})

	collector.waitForRuns(t, 1)
	targets := readerFactory.recordedTargets()
	require.Len(t, targets, 1)
	assert.Equal(t, target{runtime: RuntimeDocker, entityID: "abc123"}, targets[0])
}

func TestSchedulerRunsCollectorOutsideScheduleCallback(t *testing.T) {
	collector := &recordingConfigCollector{
		unblock: make(chan struct{}),
	}
	readerFactory := &recordingConfigReaderFactory{reader: fakeConfigReader{runtime: RuntimeHost}}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeHost: readerFactory.Build},
		map[string]ConfigCollector{"redis": collector},
		nil,
	)
	defer s.Stop()

	returned := make(chan struct{})
	go func() {
		s.Schedule([]integration.Config{
			checkConfig("redis", "process://1234"),
		})
		close(returned)
	}()

	select {
	case <-returned:
	case <-time.After(time.Second):
		close(collector.unblock)
		t.Fatal("Schedule blocked while collector was running")
	}

	close(collector.unblock)
	collector.waitForRuns(t, 1)
}

func TestSchedulerSendsCollectedConfig(t *testing.T) {
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:      "/etc/redis/redis.conf",
				Content:   []byte("port 6379\n"),
				Truncated: true,
			},
		},
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{"redisdb": collector},
		sender,
	)
	defer s.Stop()

	s.Schedule([]integration.Config{
		checkConfig("redisdb", "docker://abc123"),
	})

	collectedConfigs := sender.waitForCollectedConfigs(t, 1)
	assert.Equal(t, collectedConfig{
		Integration: "redisdb",
		Runtime:     RuntimeDocker,
		RuntimeID:   "abc123",
		ConfigFiles: []ConfigFile{
			{
				Path:      "/etc/redis/redis.conf",
				Content:   []byte("port 6379\n"),
				Truncated: true,
			},
		},
	}, collectedConfigs[0])
}

func TestSchedulerBatchesMultipleCompletedConfigCollections(t *testing.T) {
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: []byte("port 6379\n"),
			},
		},
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
	)
	defer s.Stop()

	s.Schedule([]integration.Config{checkConfig(testRedisIntegrationName, "docker://abc123")})
	s.Schedule([]integration.Config{checkConfig(testRedisIntegrationName, "docker://def456")})

	batches := sender.waitForBatches(t, 1)
	require.Len(t, batches[0], 2)
	assert.Equal(t, "abc123", batches[0][0].RuntimeID)
	assert.Equal(t, "def456", batches[0][1].RuntimeID)
}

func TestSchedulerFlushesConfigCollectionBatchOnTimeout(t *testing.T) {
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: []byte("port 6379\n"),
			},
		},
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
	)
	defer s.Stop()

	s.Schedule([]integration.Config{checkConfig(testRedisIntegrationName, "docker://abc123")})

	batches := sender.waitForBatches(t, 1)
	require.Len(t, batches[0], 1)
	assert.Equal(t, "abc123", batches[0][0].RuntimeID)
}

func TestConfigCollectionBatchFlushesOnMaxCollectedConfigs(t *testing.T) {
	var batch collectedConfigBatch
	for range configCollectionBatchMaxCollectedConfigs - 1 {
		batch.add(pendingCollectedConfig{})
	}
	assert.False(t, batch.shouldFlush())

	batch.add(pendingCollectedConfig{})
	assert.True(t, batch.shouldFlush())
}

func TestSchedulerFlushesOversizedConfigCollectionAlone(t *testing.T) {
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: make([]byte, configCollectionBatchMaxRawConfigBytes+1),
			},
		},
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
	)
	defer s.Stop()

	s.Schedule([]integration.Config{checkConfig(testRedisIntegrationName, "docker://abc123")})

	batches := sender.waitForBatches(t, 1)
	require.Len(t, batches[0], 1)
	assert.Equal(t, "abc123", batches[0][0].RuntimeID)
	assert.Greater(t, collectedConfigRawBytes(batches[0][0]), configCollectionBatchMaxRawConfigBytes)
}

func TestSchedulerFlushesPendingConfigCollectionBatchOnStop(t *testing.T) {
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: []byte("port 6379\n"),
			},
		},
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
	)

	s.Schedule([]integration.Config{checkConfig(testRedisIntegrationName, "docker://abc123")})
	collector.waitForRuns(t, 1)
	s.Stop()

	batches := sender.waitForBatches(t, 1)
	require.Len(t, batches[0], 1)
	assert.Equal(t, "abc123", batches[0][0].RuntimeID)
}

func TestSchedulerHeartbeatsCollectedFilesWithJitter(t *testing.T) {
	mockClock := clock.NewMock()
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: []byte("port 6379\n"),
			},
		},
	}
	s := newADSchedulerWithConfig(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
		adSchedulerConfig{
			heartbeatInterval:      time.Hour,
			heartbeatJitter:        10 * time.Minute,
			heartbeatRetryInterval: time.Minute,
			heartbeatCheckInterval: time.Minute,
			clock:                  mockClock,
			jitter:                 fixedJitter(10 * time.Minute),
		},
	)
	defer s.Stop()

	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	s.Schedule([]integration.Config{
		config,
	})

	sender.waitForCollectedConfigs(t, 1)
	waitForWatchScheduled(t, s, config)

	mockClock.Add(69 * time.Minute)
	require.Never(t, func() bool {
		return len(sender.recordedCollectedConfigs()) > 1
	}, 50*time.Millisecond, 5*time.Millisecond)

	mockClock.Add(time.Minute)
	collectedConfigs := sender.waitForCollectedConfigs(t, 2)
	assert.Equal(t, collectedConfigs[0], collectedConfigs[1])
}

func TestSchedulerCollectsChangedFilesAtNextHeartbeat(t *testing.T) {
	mockClock := clock.NewMock()
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: []byte("port 6379\n"),
			},
		},
	}
	s := newADSchedulerWithConfig(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
		adSchedulerConfig{
			heartbeatInterval:      time.Hour,
			heartbeatJitter:        0,
			heartbeatRetryInterval: time.Minute,
			heartbeatCheckInterval: time.Minute,
			clock:                  mockClock,
			jitter:                 fixedJitter(0),
		},
	)
	defer s.Stop()

	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	s.Schedule([]integration.Config{
		config,
	})
	sender.waitForCollectedConfigs(t, 1)
	waitForWatchScheduled(t, s, config)

	collector.setFiles([]ConfigFile{
		{
			Path:    "/etc/redis/redis.conf",
			Content: []byte("port 6380\n"),
		},
	})
	s.Schedule([]integration.Config{
		config,
	})

	require.Never(t, func() bool {
		return len(collector.recordedRuns()) > 1
	}, 50*time.Millisecond, 5*time.Millisecond)

	mockClock.Add(time.Hour)
	collectedConfigs := sender.waitForCollectedConfigs(t, 2)
	assert.Equal(t, []byte("port 6380\n"), collectedConfigs[1].ConfigFiles[0].Content)
}

func TestSchedulerIgnoresRescheduleReceivedInFlight(t *testing.T) {
	sender := newBlockingCollectedConfigSender()
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: []byte("port 6379\n"),
			},
		},
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
	)
	defer s.Stop()
	defer sender.release()

	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	s.Schedule([]integration.Config{config})
	sender.waitUntilBlocked(t)

	collector.setFiles([]ConfigFile{
		{
			Path:    "/etc/redis/redis.conf",
			Content: []byte("port 6380\n"),
		},
	})
	s.Schedule([]integration.Config{config})
	s.Schedule([]integration.Config{config})

	nextCollection, inFlight, ok := watchedConfigState(s, config)
	require.True(t, ok)
	assert.True(t, inFlight)
	assert.True(t, nextCollection.IsZero())

	sender.release()
	collectedConfigs := sender.waitForCollectedConfigs(t, 1)
	require.Len(t, collectedConfigs, 1)
	assert.Equal(t, []byte("port 6379\n"), collectedConfigs[0].ConfigFiles[0].Content)
	waitForWatchScheduled(t, s, config)
	require.Never(t, func() bool {
		return len(collector.recordedRuns()) > 1
	}, 50*time.Millisecond, 5*time.Millisecond)
}

func TestSchedulerUnscheduleStopsHeartbeats(t *testing.T) {
	mockClock := clock.NewMock()
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: []byte("port 6379\n"),
			},
		},
	}
	s := newADSchedulerWithConfig(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
		adSchedulerConfig{
			heartbeatInterval:      time.Hour,
			heartbeatJitter:        10 * time.Minute,
			heartbeatRetryInterval: time.Minute,
			heartbeatCheckInterval: time.Minute,
			clock:                  mockClock,
			jitter:                 fixedJitter(0),
		},
	)
	defer s.Stop()

	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	s.Schedule([]integration.Config{config})
	sender.waitForCollectedConfigs(t, 1)
	waitForWatchScheduled(t, s, config)

	s.Unschedule([]integration.Config{config})
	mockClock.Add(2 * time.Hour)

	require.Never(t, func() bool {
		return len(sender.recordedCollectedConfigs()) > 1
	}, 50*time.Millisecond, 5*time.Millisecond)
}

func TestSchedulerContinuesHeartbeatingAfterEmptyCollection(t *testing.T) {
	mockClock := clock.NewMock()
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: []byte("port 6379\n"),
			},
		},
	}
	s := newADSchedulerWithConfig(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
		adSchedulerConfig{
			heartbeatInterval:      time.Hour,
			heartbeatJitter:        0,
			heartbeatRetryInterval: time.Minute,
			heartbeatCheckInterval: time.Minute,
			clock:                  mockClock,
			jitter:                 fixedJitter(0),
		},
	)
	defer s.Stop()

	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	s.Schedule([]integration.Config{
		config,
	})

	sender.waitForCollectedConfigs(t, 1)
	waitForWatchScheduled(t, s, config)
	collector.setFiles(nil)

	mockClock.Add(time.Hour)
	collector.waitForRuns(t, 2)
	assert.Len(t, sender.recordedCollectedConfigs(), 1)
	waitForWatchScheduled(t, s, config)

	collector.setFiles([]ConfigFile{
		{
			Path:    "/etc/redis/redis.conf",
			Content: []byte("port 6380\n"),
		},
	})

	mockClock.Add(time.Hour)
	collectedConfigs := sender.waitForCollectedConfigs(t, 2)
	assert.Equal(t, []byte("port 6380\n"), collectedConfigs[1].ConfigFiles[0].Content)
}

func TestSchedulerRetriesAfterFailedSend(t *testing.T) {
	mockClock := clock.NewMock()
	sender := &recordingCollectedConfigSender{err: errors.New("send failed")}
	collector := &recordingConfigCollector{
		files: []ConfigFile{
			{
				Path:    "/etc/redis/redis.conf",
				Content: []byte("port 6379\n"),
			},
		},
	}
	s := newADSchedulerWithConfig(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
		adSchedulerConfig{
			heartbeatInterval:      time.Hour,
			heartbeatJitter:        0,
			heartbeatRetryInterval: time.Minute,
			heartbeatCheckInterval: time.Minute,
			clock:                  mockClock,
			jitter:                 fixedJitter(0),
		},
	)
	defer s.Stop()

	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	s.Schedule([]integration.Config{config})
	sender.waitForCollectedConfigs(t, 1)
	require.Eventually(t, func() bool {
		nextCollection, inFlight, ok := watchedConfigState(s, config)
		return ok && !inFlight && nextCollection.Equal(mockClock.Now().Add(time.Minute))
	}, time.Second, 10*time.Millisecond)

	sender.setErr(nil)
	mockClock.Add(time.Minute)

	collectedConfigs := sender.waitForCollectedConfigs(t, 2)
	assert.Equal(t, collectedConfigs[0], collectedConfigs[1])
	waitForWatchScheduled(t, s, config)
}

func TestSchedulerRetriesFailedSendBeforeExistingHeartbeat(t *testing.T) {
	mockClock := clock.NewMock()
	s := newADSchedulerWithConfig(
		targetResolver{},
		nil,
		nil,
		nil,
		adSchedulerConfig{
			heartbeatInterval:      time.Hour,
			heartbeatJitter:        0,
			heartbeatRetryInterval: time.Minute,
			heartbeatCheckInterval: time.Minute,
			clock:                  mockClock,
			jitter:                 fixedJitter(0),
		},
	)
	defer s.Stop()

	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	oldHeartbeat := mockClock.Now().Add(time.Hour)
	watch := &watchedConfig{
		key:            watchKey(config),
		integration:    config.Name,
		serviceID:      config.ServiceID,
		nextCollection: oldHeartbeat,
		inFlight:       true,
	}
	s.mu.Lock()
	s.watches[watchKey(config)] = watch
	s.mu.Unlock()

	s.finishSend([]pendingCollectedConfig{
		{
			watch: watch,
		},
	}, false)

	nextCollection, inFlight, ok := watchedConfigState(s, config)
	require.True(t, ok)
	assert.False(t, inFlight)
	assert.Equal(t, mockClock.Now().Add(time.Minute), nextCollection)
	assert.True(t, nextCollection.Before(oldHeartbeat))
}

func TestSchedulerRejectsCollectionFromReplacedWatch(t *testing.T) {
	s := newADScheduler(targetResolver{}, nil, nil, nil)
	defer s.Stop()

	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	key := watchKey(config)
	staleWatch := &watchedConfig{
		key:         key,
		integration: config.Name,
		serviceID:   config.ServiceID,
	}
	replacementDeadline := time.Now().Add(time.Hour)
	replacementWatch := &watchedConfig{
		key:            key,
		integration:    config.Name,
		serviceID:      config.ServiceID,
		nextCollection: replacementDeadline,
		inFlight:       true,
	}
	s.mu.Lock()
	s.watches[key] = replacementWatch
	s.mu.Unlock()

	_, _, _, ok := s.snapshotWatch(staleWatch)
	assert.False(t, ok)

	s.finishSend([]pendingCollectedConfig{
		{
			watch: staleWatch,
		},
	}, true)
	s.finishCollection(staleWatch, time.Time{})

	s.mu.Lock()
	activeWatch := s.watches[key]
	activeDeadline := activeWatch.nextCollection
	activeInFlight := activeWatch.inFlight
	s.mu.Unlock()
	assert.Same(t, replacementWatch, activeWatch)
	assert.Equal(t, replacementDeadline, activeDeadline)
	assert.True(t, activeInFlight)
}

func TestADSchedulerConfigFromAgentConfigDefaultsAndClampsJitter(t *testing.T) {
	cfg := adSchedulerConfigFromAgentConfig(config.NewMock(t))
	assert.Equal(t, time.Hour, cfg.heartbeatInterval)
	assert.Equal(t, 10*time.Minute, cfg.heartbeatJitter)

	cfg = adSchedulerConfigFromAgentConfig(config.NewMockWithOverrides(t, map[string]interface{}{
		heartbeatIntervalConfigKey: 4 * time.Hour,
		heartbeatJitterConfigKey:   2 * time.Hour,
	}))
	assert.Equal(t, time.Hour, cfg.heartbeatJitter)

	cfg = adSchedulerConfigFromAgentConfig(config.NewMockWithOverrides(t, map[string]interface{}{
		heartbeatIntervalConfigKey: 5 * time.Minute,
		heartbeatJitterConfigKey:   10 * time.Minute,
	}))
	assert.Equal(t, 5*time.Minute, cfg.heartbeatInterval)
	assert.Equal(t, 2*time.Minute+30*time.Second, cfg.heartbeatJitter)
}

func TestComponentRegistersAutodiscoverySchedulerOnStart(t *testing.T) {
	ac := &fakeAutodiscovery{}
	lifecycle := &recordingLifecycle{}

	NewComponent(Requires{
		Lifecycle:     lifecycle,
		Autodiscovery: ac,
		Hostname:      fakeHostname{hostname: "test-host"},
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

func TestComponentRegistersProvidedCollectors(t *testing.T) {
	collector := &recordingConfigCollector{}
	c := newComponent(nil, targetResolver{}, noopCollectedConfigSender{}, map[string]ConfigCollector{"custom": collector})
	adScheduler, ok := c.scheduler.(*adScheduler)
	require.True(t, ok)

	assert.Same(t, collector, adScheduler.collectors["custom"])
}

func TestComponentRegistersKubernetesConfigReader(t *testing.T) {
	c := newComponent(nil, targetResolver{}, noopCollectedConfigSender{}, nil)
	adScheduler, ok := c.scheduler.(*adScheduler)
	require.True(t, ok)

	assert.Contains(t, adScheduler.readers, RuntimeKubernetes)
}

func TestComponentUsesEventPlatformSenderWhenAvailable(t *testing.T) {
	forwarder := &recordingEventPlatformForwarder{}
	c := newComponent(nil, targetResolver{}, newEventPlatformCollectedConfigSender(recordingEventPlatformComponent{
		forwarder: forwarder,
		ok:        true,
	}, "test-host"), nil)
	adScheduler, ok := c.scheduler.(*adScheduler)
	require.True(t, ok)

	_, ok = adScheduler.sender.(*eventPlatformCollectedConfigSender)
	require.True(t, ok)
}

func TestEventPlatformSenderSendsAgentDiscoveryPayload(t *testing.T) {
	forwarder := &recordingEventPlatformForwarder{}
	sender := newEventPlatformCollectedConfigSender(recordingEventPlatformComponent{
		forwarder: forwarder,
		ok:        true,
	}, "test-host")

	beforeSend := time.Now()
	err := sender.SendCollectedConfigs([]collectedConfig{
		{
			Integration: testRedisIntegrationName,
			Runtime:     RuntimeDocker,
			RuntimeID:   "abc123",
			ConfigFiles: []ConfigFile{
				{
					Path:          "/etc/redis/redis.conf",
					Content:       []byte("port 6379\n"),
					Truncated:     true,
					PayloadFormat: testRedisConfigPayloadFormat,
				},
				{
					Path:          "/etc/redis/redis-extra.conf",
					Content:       []byte("appendonly no\n"),
					Truncated:     false,
					PayloadFormat: testRedisConfigPayloadFormat,
				},
			},
		},
		{
			Integration: testRedisIntegrationName,
			Runtime:     RuntimeDocker,
			RuntimeID:   "def456",
			ConfigFiles: []ConfigFile{
				{
					Path:          "/etc/redis/redis.conf",
					Content:       []byte("port 6380\n"),
					PayloadFormat: testRedisConfigPayloadFormat,
				},
			},
		},
	})
	afterSend := time.Now()

	require.NoError(t, err)
	sent := forwarder.recordedMessages()
	require.Len(t, sent, 1)
	assert.Equal(t, eventplatform.EventTypeAgentDiscovery, sent[0].eventType)

	var batch agentdiscovery.AgentDiscoveryPayloadBatch
	require.NoError(t, proto.Unmarshal(sent[0].message.GetContent(), &batch))
	assert.Equal(t, "test-host", batch.GetHostId())
	require.Len(t, batch.Payloads, 2)
	ingestionTimestamps := make([]*timestamppb.Timestamp, 0, len(batch.Payloads))
	for _, payload := range batch.Payloads {
		ingestionTimestamp := payload.GetIngestionTimestamp()
		require.NotNil(t, ingestionTimestamp)
		require.NoError(t, ingestionTimestamp.CheckValid())
		ingestionTime := ingestionTimestamp.AsTime()
		assert.False(t, ingestionTime.Before(beforeSend), "ingestion timestamp %s before send start %s", ingestionTime, beforeSend)
		assert.False(t, ingestionTime.After(afterSend), "ingestion timestamp %s after send end %s", ingestionTime, afterSend)
		ingestionTimestamps = append(ingestionTimestamps, ingestionTimestamp)
	}

	want := &agentdiscovery.AgentDiscoveryPayloadBatch{
		HostId: "test-host",
		Payloads: []*agentdiscovery.AgentDiscoveryPayload{
			{
				Integration:        testRedisIntegrationName,
				Runtime:            string(RuntimeDocker),
				RuntimeId:          "abc123",
				IngestionTimestamp: ingestionTimestamps[0],
				ConfigFiles: []*agentdiscovery.AgentDiscoveryConfigFile{
					{
						Path:          "/etc/redis/redis.conf",
						Content:       []byte("port 6379\n"),
						Truncated:     true,
						PayloadFormat: testRedisConfigPayloadFormat,
					},
					{
						Path:          "/etc/redis/redis-extra.conf",
						Content:       []byte("appendonly no\n"),
						Truncated:     false,
						PayloadFormat: testRedisConfigPayloadFormat,
					},
				},
			},
			{
				Integration:        testRedisIntegrationName,
				Runtime:            string(RuntimeDocker),
				RuntimeId:          "def456",
				IngestionTimestamp: ingestionTimestamps[1],
				ConfigFiles: []*agentdiscovery.AgentDiscoveryConfigFile{
					{
						Path:          "/etc/redis/redis.conf",
						Content:       []byte("port 6380\n"),
						PayloadFormat: testRedisConfigPayloadFormat,
					},
				},
			},
		},
	}
	assert.True(t, proto.Equal(want, &batch), "payload mismatch: got %v", &batch)
}

func TestEventPlatformSenderSkipsEmptyCollections(t *testing.T) {
	forwarder := &recordingEventPlatformForwarder{}
	sender := newEventPlatformCollectedConfigSender(recordingEventPlatformComponent{
		forwarder: forwarder,
		ok:        true,
	}, "test-host")

	err := sender.SendCollectedConfigs([]collectedConfig{
		{
			Integration: testRedisIntegrationName,
			Runtime:     RuntimeDocker,
		},
	})

	require.NoError(t, err)
	assert.Empty(t, forwarder.recordedMessages())
}

func TestEventPlatformSenderReturnsSendError(t *testing.T) {
	forwarder := &recordingEventPlatformForwarder{err: errors.New("queue unavailable")}
	sender := newEventPlatformCollectedConfigSender(recordingEventPlatformComponent{
		forwarder: forwarder,
		ok:        true,
	}, "test-host")

	err := sender.SendCollectedConfigs([]collectedConfig{
		{
			Integration: testRedisIntegrationName,
			Runtime:     RuntimeDocker,
			RuntimeID:   "abc123",
			ConfigFiles: []ConfigFile{
				{Path: "/etc/redis/redis.conf", Content: []byte("port 6379\n")},
			},
		},
	})

	require.ErrorContains(t, err, "send agent discovery payload to event platform")
	require.ErrorContains(t, err, "queue unavailable")
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

type fakeHostname struct {
	hostname string
}

var _ hostname.Component = fakeHostname{}

func (h fakeHostname) Get(context.Context) (string, error) {
	return h.hostname, nil
}

func (h fakeHostname) GetWithProvider(context.Context) (hostnameinterface.Data, error) {
	return hostnameinterface.Data{Hostname: h.hostname, Provider: "test"}, nil
}

func (h fakeHostname) GetSafe(context.Context) string {
	return h.hostname
}

type recordingConfigCollector struct {
	mu      sync.Mutex
	runs    []runCall
	unblock chan struct{}
	files   []ConfigFile
	err     error
}

type runCall struct {
	reader ConfigReader
}

func (c *recordingConfigCollector) Collect(ctx context.Context, reader ConfigReader) ([]ConfigFile, error) {
	if c.unblock != nil {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-c.unblock:
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs = append(c.runs, runCall{
		reader: reader,
	})
	return c.files, c.err
}

type recordingCollectedConfigSender struct {
	mu      sync.Mutex
	batches [][]collectedConfig
	err     error
}

type blockingCollectedConfigSender struct {
	recordingCollectedConfigSender
	started     chan struct{}
	unblock     chan struct{}
	blockOnce   sync.Once
	releaseOnce sync.Once
}

func newBlockingCollectedConfigSender() *blockingCollectedConfigSender {
	return &blockingCollectedConfigSender{
		started: make(chan struct{}),
		unblock: make(chan struct{}),
	}
}

func (s *blockingCollectedConfigSender) SendCollectedConfigs(configs []collectedConfig) error {
	s.blockOnce.Do(func() {
		close(s.started)
		<-s.unblock
	})
	return s.recordingCollectedConfigSender.SendCollectedConfigs(configs)
}

func (s *blockingCollectedConfigSender) waitUntilBlocked(t *testing.T) {
	t.Helper()

	select {
	case <-s.started:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for collected config send")
	}
}

func (s *blockingCollectedConfigSender) release() {
	s.releaseOnce.Do(func() {
		close(s.unblock)
	})
}

func (r *recordingCollectedConfigSender) SendCollectedConfigs(configs []collectedConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copiedConfigs := make([]collectedConfig, len(configs))
	copy(copiedConfigs, configs)
	r.batches = append(r.batches, copiedConfigs)
	return r.err
}

func (c *recordingConfigCollector) setFiles(files []ConfigFile) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.files = files
}

func (c *recordingConfigCollector) recordedRuns() []runCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	runs := make([]runCall, len(c.runs))
	copy(runs, c.runs)
	return runs
}

func (r *recordingCollectedConfigSender) setErr(err error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.err = err
}

func (r *recordingCollectedConfigSender) waitForCollectedConfigs(t *testing.T, count int) []collectedConfig {
	t.Helper()

	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.flattenCollectedConfigsLocked()) >= count
	}, 2*time.Second, 10*time.Millisecond)

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flattenCollectedConfigsLocked()
}

func (r *recordingCollectedConfigSender) recordedCollectedConfigs() []collectedConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.flattenCollectedConfigsLocked()
}

func (r *recordingCollectedConfigSender) waitForBatches(t *testing.T, count int) [][]collectedConfig {
	t.Helper()

	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.batches) >= count
	}, 2*time.Second, 10*time.Millisecond)

	r.mu.Lock()
	defer r.mu.Unlock()
	batches := make([][]collectedConfig, len(r.batches))
	for i, batch := range r.batches {
		batches[i] = make([]collectedConfig, len(batch))
		copy(batches[i], batch)
	}
	return batches
}

func (r *recordingCollectedConfigSender) flattenCollectedConfigsLocked() []collectedConfig {
	var configs []collectedConfig
	for _, batch := range r.batches {
		configs = append(configs, batch...)
	}
	return configs
}

type recordingEventPlatformComponent struct {
	forwarder eventplatform.Forwarder
	ok        bool
}

func (c recordingEventPlatformComponent) Get() (eventplatform.Forwarder, bool) {
	return c.forwarder, c.ok
}

type eventPlatformSendCall struct {
	message   *message.Message
	eventType string
}

type recordingEventPlatformForwarder struct {
	mu       sync.Mutex
	messages []eventPlatformSendCall
	err      error
}

func (f *recordingEventPlatformForwarder) SendEventPlatformEvent(msg *message.Message, eventType string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.messages = append(f.messages, eventPlatformSendCall{
		message:   msg,
		eventType: eventType,
	})
	return f.err
}

func (f *recordingEventPlatformForwarder) SendEventPlatformEventBlocking(_ *message.Message, _ string) error {
	return errors.New("unexpected blocking Event Platform send")
}

func (f *recordingEventPlatformForwarder) Purge() map[string][]*message.Message {
	return nil
}

func (f *recordingEventPlatformForwarder) recordedMessages() []eventPlatformSendCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	messages := make([]eventPlatformSendCall, len(f.messages))
	copy(messages, f.messages)
	return messages
}

func fixedJitter(d time.Duration) func(time.Duration) time.Duration {
	return func(time.Duration) time.Duration {
		return d
	}
}

func watchedConfigState(s *adScheduler, config integration.Config) (time.Time, bool, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	watch, ok := s.watches[watchKey(config)]
	if !ok {
		return time.Time{}, false, false
	}
	return watch.nextCollection, watch.inFlight, true
}

func waitForWatchScheduled(t *testing.T, s *adScheduler, config integration.Config) {
	t.Helper()

	require.Eventually(t, func() bool {
		nextCollection, inFlight, ok := watchedConfigState(s, config)
		return ok && !inFlight && !nextCollection.IsZero()
	}, time.Second, 10*time.Millisecond)
}

func (c *recordingConfigCollector) waitForRuns(t *testing.T, count int) []runCall {
	t.Helper()

	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return len(c.runs) >= count
	}, time.Second, 10*time.Millisecond)

	c.mu.Lock()
	defer c.mu.Unlock()
	runs := make([]runCall, len(c.runs))
	copy(runs, c.runs)
	return runs
}

type fakeConfigReader struct {
	runtime   RuntimeType
	closeFunc func()
}

func (r fakeConfigReader) Runtime() RuntimeType {
	return r.runtime
}

func (r fakeConfigReader) Close() {
	if r.closeFunc != nil {
		r.closeFunc()
	}
}

func (r fakeConfigReader) ReadFile(context.Context, string) (ConfigFile, error) {
	return ConfigFile{}, errors.New("not implemented")
}

func (r fakeConfigReader) ReadEnvVars(context.Context, []string) (map[string]string, error) {
	return nil, errors.New("not implemented")
}

func (r fakeConfigReader) ReadCommandline(context.Context) (TargetCommandline, error) {
	return TargetCommandline{}, errors.New("not implemented")
}

func fakeConfigReaderFactory(reader ConfigReader) configReaderFactory {
	return func(target) (ConfigReader, error) {
		return reader, nil
	}
}

type recordingConfigReaderFactory struct {
	mu      sync.Mutex
	reader  ConfigReader
	targets []target
}

func (f *recordingConfigReaderFactory) Build(target target) (ConfigReader, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.targets = append(f.targets, target)
	return f.reader, nil
}

func (f *recordingConfigReaderFactory) recordedTargets() []target {
	f.mu.Lock()
	defer f.mu.Unlock()
	targets := make([]target, len(f.targets))
	copy(targets, f.targets)
	return targets
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
