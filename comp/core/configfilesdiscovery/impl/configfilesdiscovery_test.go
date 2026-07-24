// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package configfilesdiscoveryimpl

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/v5/agentdiscovery"
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
			name:  "successful collection and report",
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

func TestSchedulerRetriesOnMatchingContainerProcess(t *testing.T) {
	tests := []struct {
		name      string
		runtime   RuntimeType
		serviceID string
		resolver  func(t *testing.T) targetResolver
	}{
		{
			name:      "docker",
			runtime:   RuntimeDocker,
			serviceID: "docker://abc123",
			resolver:  func(*testing.T) targetResolver { return targetResolver{} },
		},
		{
			name:      "kubernetes",
			runtime:   RuntimeKubernetes,
			serviceID: "containerd://abc123",
			resolver: func(t *testing.T) targetResolver {
				store := newWorkloadMetaMock(t)
				store.Set(&workloadmeta.Container{
					EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindContainer, ID: "abc123"},
					Runtime:  workloadmeta.ContainerRuntimeContainerd,
					Owner:    &workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid"},
				})
				store.Set(&workloadmeta.KubernetesPod{EntityID: workloadmeta.EntityID{Kind: workloadmeta.KindKubernetesPod, ID: "pod-uid"}})
				return targetResolver{store: store}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := &recordingCollectedConfigSender{}
			readerFactory := &recordingConfigReaderFactory{reader: fakeConfigReader{runtime: tt.runtime}}
			collector := &recordingConfigCollector{
				filesByRun: [][]ConfigFile{
					nil,
					{{Path: "/etc/redis/redis.conf", Content: []byte("port 6379\n")}},
				},
				matchCommandline: func(args []string) bool {
					return len(args) >= 2 && args[0] == "redis-server" && args[1] == "/etc/redis/redis.conf"
				},
			}
			s := newADScheduler(
				tt.resolver(t),
				map[RuntimeType]configReaderFactory{tt.runtime: readerFactory.Build},
				map[string]ConfigCollector{testRedisIntegrationName: collector},
				sender,
			)
			defer s.Stop()

			config := checkConfig(testRedisIntegrationName, tt.serviceID)
			s.Schedule([]integration.Config{config})
			collector.waitForRuns(t, 1)
			require.Eventually(t, func() bool {
				return s.isProcessRetryWaiting(config.Digest())
			}, time.Second, 10*time.Millisecond)

			s.handleProcessEventBundle(newProcessEventBundle(&workloadmeta.Process{
				ContainerID: "other-container",
				Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
			}))
			s.handleProcessEventBundle(newProcessEventBundle(&workloadmeta.Process{
				ContainerID: "abc123",
				Cmdline:     []string{"nginx", "-c", "/etc/nginx/nginx.conf"},
			}))
			assert.Len(t, collector.waitForRunsWithoutWaiting(), 1)

			bundle := newProcessEventBundle(&workloadmeta.Process{
				ContainerID: "abc123",
				Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
			})
			s.handleProcessEventBundle(bundle)
			select {
			case <-bundle.Ch:
			default:
				t.Fatal("process event bundle was not acknowledged")
			}

			collector.waitForRuns(t, 2)
			configs := sender.waitForCollectedConfigs(t, 1)
			require.Len(t, configs, 1)
			assert.Equal(t, tt.runtime, configs[0].Runtime)
			assert.Equal(t, "abc123", configs[0].RuntimeID)
			targets := readerFactory.recordedTargets()
			require.Len(t, targets, 2)
			assert.Equal(t, target{runtime: tt.runtime, entityID: "abc123"}, targets[1])
		})
	}
}

func TestSchedulerRetriesWhenProcessArrivesDuringInitialCollection(t *testing.T) {
	started := make(chan struct{})
	unblock := make(chan struct{})
	readerFactory := &recordingConfigReaderFactory{reader: fakeConfigReader{runtime: RuntimeDocker}}
	collector := &recordingConfigCollector{
		started:    started,
		unblock:    unblock,
		filesByRun: [][]ConfigFile{nil, {{Path: "/etc/redis/redis.conf"}}},
		matchCommandline: func(args []string) bool {
			return len(args) >= 2 && args[0] == "redis-server"
		},
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: readerFactory.Build},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		nil,
	)
	defer s.Stop()

	s.Schedule([]integration.Config{checkConfig(testRedisIntegrationName, "docker://abc123")})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial collection did not start")
	}
	s.handleProcessEventBundle(newProcessEventBundle(&workloadmeta.Process{
		ContainerID: "abc123",
		Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
	}))
	s.handleProcessEventBundle(newProcessEventBundle(&workloadmeta.Process{
		ContainerID: "abc123",
		Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
	}))
	close(unblock)

	collector.waitForRuns(t, 2)
	targets := readerFactory.recordedTargets()
	require.Len(t, targets, 2)
	assert.Equal(t, target{runtime: RuntimeDocker, entityID: "abc123"}, targets[1])
}

func TestSchedulerDiscardsContainerCollectionUnscheduledWhileRunning(t *testing.T) {
	started := make(chan struct{})
	unblock := make(chan struct{})
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		started: started,
		unblock: unblock,
		files:   []ConfigFile{{Path: "/etc/redis/redis.conf"}},
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		sender,
	)

	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	s.Schedule([]integration.Config{config})
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("collection did not start")
	}

	s.Unschedule([]integration.Config{config})
	close(unblock)
	collector.waitForRuns(t, 1)
	s.Stop()

	sender.mu.Lock()
	defer sender.mu.Unlock()
	assert.Empty(t, sender.batches)
}

func TestSchedulerRemovesEmptyRetryAfterMatchingProcessEvent(t *testing.T) {
	collector := &recordingConfigCollector{
		filesByRun:       [][]ConfigFile{nil, nil},
		matchCommandline: func([]string) bool { return true },
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		nil,
	)
	defer s.Stop()
	config := checkConfig(testRedisIntegrationName, "docker://abc123")

	s.Schedule([]integration.Config{config})
	collector.waitForRuns(t, 1)
	require.Eventually(t, func() bool {
		return s.isProcessRetryWaiting(config.Digest())
	}, time.Second, 10*time.Millisecond)

	process := &workloadmeta.Process{
		ContainerID: "abc123",
		Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
	}
	s.handleProcessEventBundle(newProcessEventBundle(process))
	collector.waitForRuns(t, 2)

	require.Eventually(t, func() bool {
		return !s.hasPendingProcessRetry(config.Digest())
	}, time.Second, 10*time.Millisecond)

	s.handleProcessEventBundle(newProcessEventBundle(process))
	assert.Len(t, collector.waitForRunsWithoutWaiting(), 2)
}

func TestSchedulerKeepsProcessRetryUntilUnschedule(t *testing.T) {
	collector := &recordingConfigCollector{
		matchCommandline: func([]string) bool { return true },
	}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
		nil,
	)
	defer s.Stop()
	config := checkConfig(testRedisIntegrationName, "docker://abc123")

	s.Schedule([]integration.Config{config})
	collector.waitForRuns(t, 1)
	require.Eventually(t, func() bool {
		return s.isProcessRetryWaiting(config.Digest())
	}, time.Second, 10*time.Millisecond)

	s.handleProcessEventBundle(newProcessEventBundle(&workloadmeta.Process{
		ContainerID: "other-container",
		Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
	}))
	require.True(t, s.isProcessRetryWaiting(config.Digest()))

	s.Unschedule([]integration.Config{config})
	require.False(t, s.hasPendingProcessRetry(config.Digest()))

	s.handleProcessEventBundle(newProcessEventBundle(&workloadmeta.Process{
		ContainerID: "abc123",
		Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
	}))
	assert.Len(t, collector.waitForRunsWithoutWaiting(), 1)
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
		envVars: []ConfigEnvVar{
			{Name: "REDIS_PORT", Value: "6379"},
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
	assert.Equal(t, CollectedConfig{
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
		EnvVars: []ConfigEnvVar{
			{Name: "REDIS_PORT", Value: "6379"},
		},
	}, collectedConfigs[0])
}

func TestSchedulerSendsEnvOnlyCollectedConfig(t *testing.T) {
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{
		envVars: []ConfigEnvVar{
			{Name: "REDIS_PORT", Value: "6379"},
			{Name: "REDIS_TLS_ENABLED", Value: "true"},
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
	assert.Equal(t, CollectedConfig{
		Integration: "redisdb",
		Runtime:     RuntimeDocker,
		RuntimeID:   "abc123",
		EnvVars: []ConfigEnvVar{
			{Name: "REDIS_PORT", Value: "6379"},
			{Name: "REDIS_TLS_ENABLED", Value: "true"},
		},
	}, collectedConfigs[0])
}

func TestSchedulerSkipsEmptyCollectedConfig(t *testing.T) {
	sender := &recordingCollectedConfigSender{}
	collector := &recordingConfigCollector{}
	s := newADScheduler(
		targetResolver{},
		map[RuntimeType]configReaderFactory{RuntimeDocker: fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})},
		map[string]ConfigCollector{"redisdb": collector},
		sender,
	)

	s.Schedule([]integration.Config{
		checkConfig("redisdb", "docker://abc123"),
	})

	collector.waitForRuns(t, 1)
	s.Stop()
	assert.Empty(t, sender.recordedBatches())
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

func TestSchedulerFlushesConfigCollectionBatchOnMaxCollectedConfigs(t *testing.T) {
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

	configs := make([]integration.Config, 0, configCollectionBatchMaxCollectedConfigs)
	for i := 0; i < configCollectionBatchMaxCollectedConfigs; i++ {
		configs = append(configs, checkConfig(testRedisIntegrationName, fmt.Sprintf("docker://container-%d", i)))
	}
	s.Schedule(configs)

	batches := sender.waitForBatches(t, 1)
	require.Len(t, batches[0], configCollectionBatchMaxCollectedConfigs)
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

func TestSchedulerCountsEnvVarsInCollectedConfigRawBytes(t *testing.T) {
	assert.Equal(t, len("REDIS_PORT")+len("6379")+len("REDIS_TLS_ENABLED")+len("true"), collectedConfigRawBytes(CollectedConfig{
		EnvVars: []ConfigEnvVar{
			{Name: "REDIS_PORT", Value: "6379"},
			{Name: "REDIS_TLS_ENABLED", Value: "true"},
		},
	}))
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

func TestComponentRetriesFromSubscribedProcessEventsAndStops(t *testing.T) {
	store := newWorkloadMetaMock(t)
	ac := &fakeAutodiscovery{}
	collector := &recordingConfigCollector{
		filesByRun: [][]ConfigFile{nil, {{Path: "/etc/redis/redis.conf"}}},
		matchCommandline: func(args []string) bool {
			return len(args) >= 2 && args[0] == "redis-server"
		},
	}
	c := newComponent(
		ac,
		targetResolver{store: store},
		noopCollectedConfigSender{},
		map[string]ConfigCollector{testRedisIntegrationName: collector},
	)
	c.scheduler.readers[RuntimeDocker] = fakeConfigReaderFactory(fakeConfigReader{runtime: RuntimeDocker})

	require.NoError(t, c.start(context.Background()))
	config := checkConfig(testRedisIntegrationName, "docker://abc123")
	c.scheduler.Schedule([]integration.Config{config})
	collector.waitForRuns(t, 1)

	store.Set(&workloadmeta.Process{
		EntityID:    workloadmeta.EntityID{Kind: workloadmeta.KindProcess, ID: "101"},
		ContainerID: "abc123",
		Cmdline:     []string{"redis-server", "/etc/redis/redis.conf"},
	})
	collector.waitForRuns(t, 2)

	require.NoError(t, c.stop(context.Background()))
	assert.Equal(t, schedulerName, ac.removedName)
	assert.False(t, c.scheduler.hasPendingProcessRetry(config.Digest()))
}

func TestComponentRegistersProvidedCollectors(t *testing.T) {
	collector := &recordingConfigCollector{}
	c := newComponent(nil, targetResolver{}, noopCollectedConfigSender{}, map[string]ConfigCollector{"custom": collector})
	assert.Same(t, collector, c.scheduler.collectors["custom"])
}

func TestComponentRegistersKubernetesConfigReader(t *testing.T) {
	c := newComponent(nil, targetResolver{}, noopCollectedConfigSender{}, nil)
	assert.Contains(t, c.scheduler.readers, RuntimeKubernetes)
}

func TestComponentUsesEventPlatformSenderWhenAvailable(t *testing.T) {
	forwarder := &recordingEventPlatformForwarder{}
	c := newComponent(nil, targetResolver{}, newEventPlatformCollectedConfigSender(recordingEventPlatformComponent{
		forwarder: forwarder,
		ok:        true,
	}, "test-host"), nil)
	_, ok := c.scheduler.sender.(*eventPlatformCollectedConfigSender)
	require.True(t, ok)
}

func TestEventPlatformSenderSendsAgentDiscoveryPayload(t *testing.T) {
	forwarder := &recordingEventPlatformForwarder{}
	sender := newEventPlatformCollectedConfigSender(recordingEventPlatformComponent{
		forwarder: forwarder,
		ok:        true,
	}, "test-host")

	beforeSend := time.Now()
	err := sender.SendCollectedConfigs([]CollectedConfig{
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
			EnvVars: []ConfigEnvVar{
				{Name: "REDIS_PORT", Value: "6379"},
				{Name: "REDIS_TLS_ENABLED", Value: "true"},
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
				EnvVars: []*agentdiscovery.AgentDiscoveryEnvVar{
					{
						Name:  "REDIS_PORT",
						Value: "6379",
					},
					{
						Name:  "REDIS_TLS_ENABLED",
						Value: "true",
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

	err := sender.SendCollectedConfigs([]CollectedConfig{
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

	err := sender.SendCollectedConfigs([]CollectedConfig{
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
	mu               sync.Mutex
	runs             []runCall
	started          chan struct{}
	startOnce        sync.Once
	unblock          chan struct{}
	files            []ConfigFile
	filesByRun       [][]ConfigFile
	envVars          []ConfigEnvVar
	err              error
	matchCommandline func([]string) bool
}

type runCall struct {
	reader ConfigReader
}

func (c *recordingConfigCollector) Collect(ctx context.Context, reader ConfigReader) (CollectedConfig, error) {
	if c.started != nil {
		c.startOnce.Do(func() { close(c.started) })
	}
	if c.unblock != nil {
		select {
		case <-ctx.Done():
			return CollectedConfig{}, ctx.Err()
		case <-c.unblock:
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	c.runs = append(c.runs, runCall{
		reader: reader,
	})
	if c.err != nil {
		return CollectedConfig{}, c.err
	}
	files := c.files
	if run := len(c.runs) - 1; run < len(c.filesByRun) {
		files = c.filesByRun[run]
	}
	return CollectedConfig{
		ConfigFiles: files,
		EnvVars:     c.envVars,
	}, nil
}

func (c *recordingConfigCollector) MatchesCommandline(args []string) bool {
	return c.matchCommandline != nil && c.matchCommandline(args)
}

type recordingCollectedConfigSender struct {
	mu      sync.Mutex
	batches [][]CollectedConfig
	err     error
}

func (r *recordingCollectedConfigSender) SendCollectedConfigs(configs []CollectedConfig) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	copiedConfigs := make([]CollectedConfig, len(configs))
	copy(copiedConfigs, configs)
	r.batches = append(r.batches, copiedConfigs)
	return r.err
}

func (r *recordingCollectedConfigSender) waitForCollectedConfigs(t *testing.T, count int) []CollectedConfig {
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

func (r *recordingCollectedConfigSender) waitForBatches(t *testing.T, count int) [][]CollectedConfig {
	t.Helper()

	require.Eventually(t, func() bool {
		r.mu.Lock()
		defer r.mu.Unlock()
		return len(r.batches) >= count
	}, 2*time.Second, 10*time.Millisecond)

	return r.recordedBatches()
}

func (r *recordingCollectedConfigSender) recordedBatches() [][]CollectedConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	batches := make([][]CollectedConfig, len(r.batches))
	for i, batch := range r.batches {
		batches[i] = make([]CollectedConfig, len(batch))
		copy(batches[i], batch)
	}
	return batches
}

func (r *recordingCollectedConfigSender) flattenCollectedConfigsLocked() []CollectedConfig {
	var configs []CollectedConfig
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

func (c *recordingConfigCollector) waitForRunsWithoutWaiting() []runCall {
	c.mu.Lock()
	defer c.mu.Unlock()
	runs := make([]runCall, len(c.runs))
	copy(runs, c.runs)
	return runs
}

func (s *adScheduler) hasPendingProcessRetry(digest string) bool {
	s.processRetryMu.Lock()
	defer s.processRetryMu.Unlock()
	return s.processRetries[digest] != nil
}

func (s *adScheduler) isProcessRetryWaiting(digest string) bool {
	s.processRetryMu.Lock()
	defer s.processRetryMu.Unlock()
	retry := s.processRetries[digest]
	return retry != nil && !retry.collecting
}

func newProcessEventBundle(process *workloadmeta.Process) workloadmeta.EventBundle {
	return workloadmeta.EventBundle{
		Events: []workloadmeta.Event{{Entity: process}},
		Ch:     make(chan struct{}),
	}
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

func (r fakeConfigReader) ReadRuntimeCommandline(context.Context) (TargetCommandline, error) {
	return TargetCommandline{}, errors.New("not implemented")
}

func (r fakeConfigReader) ReadLiveProcessCommandlines(context.Context) []TargetCommandline {
	return nil
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
