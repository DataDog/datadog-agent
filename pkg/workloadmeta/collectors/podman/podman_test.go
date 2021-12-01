// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build podman
// +build podman

package podman

import (
	"context"
	"testing"
	"time"

	"github.com/cri-o/ocicni/pkg/ocicni"
	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/util/podman"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta/collectors/util"
)

type fakeWorkloadmetaStore struct {
	workloadmeta.Store
	notifiedEvents []workloadmeta.CollectorEvent
}

func (store *fakeWorkloadmetaStore) Notify(events []workloadmeta.CollectorEvent) {
	store.notifiedEvents = append(store.notifiedEvents, events...)
}

type fakePodmanClient struct {
	mockGetAllContainers func() ([]podman.Container, error)
}

func (client *fakePodmanClient) GetAllContainers() ([]podman.Container, error) {
	return client.mockGetAllContainers()
}

func TestPull(t *testing.T) {
	startTime := time.Now()

	containers := []podman.Container{
		{
			Config: &podman.ContainerConfig{
				Spec: &specs.Spec{
					Process: &specs.Process{
						Env: []string{"TEST_ENV=TEST_VAL"},
					},
					Hostname: "agent",
					Annotations: map[string]string{
						"some-annotation":  "some-value",
						"other-annotation": "other-value",
					},
				},
				ID:           "123",
				Name:         "dd-agent",
				Namespace:    "default",
				RawImageName: "docker.io/datadog/agent:latest",
				ContainerNetworkConfig: podman.ContainerNetworkConfig{
					PortMappings: []ocicni.PortMapping{
						{
							HostPort:      1000,
							ContainerPort: 2000,
							Protocol:      "tcp",
						},
					},
				},
				ContainerMiscConfig: podman.ContainerMiscConfig{
					Labels: map[string]string{
						"label-a": "value-a",
						"label-b": "value-b",
					},
				},
			},
			State: &podman.ContainerState{
				State:       podman.ContainerStateRunning,
				StartedTime: startTime,
				PID:         10,
			},
		},
		{
			Config: &podman.ContainerConfig{
				Spec: &specs.Spec{
					Process: &specs.Process{
						Env: []string{"SOME_ENV=SOME_VAL"},
					},
					Hostname: "agent-dev",
					Annotations: map[string]string{
						"annotation-a": "value-a",
						"annotation-b": "value-b",
					},
				},
				ID:           "124",
				Name:         "dd-agent-dev",
				Namespace:    "dev",
				RawImageName: "docker.io/datadog/agent-dev:latest",
				ContainerNetworkConfig: podman.ContainerNetworkConfig{
					PortMappings: []ocicni.PortMapping{
						{
							HostPort:      2000,
							ContainerPort: 3000,
							Protocol:      "tcp",
						},
					},
				},
				ContainerMiscConfig: podman.ContainerMiscConfig{
					Labels: map[string]string{
						"label-a-dev": "value-a-dev",
						"label-b-dev": "value-b-dev",
					},
				},
			},
			State: &podman.ContainerState{
				State:       podman.ContainerStateRunning,
				StartedTime: startTime,
				PID:         11,
			},
		},
	}

	// Defined based on the containers above
	expectedEvents := []workloadmeta.CollectorEvent{
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourcePodman,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   "123",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "dd-agent",
					Namespace: "default",
					Annotations: map[string]string{
						"some-annotation":  "some-value",
						"other-annotation": "other-value",
					},
					Labels: map[string]string{
						"label-a": "value-a",
						"label-b": "value-b",
					},
				},
				EnvVars: map[string]string{
					"TEST_ENV": "TEST_VAL",
				},
				Hostname: "agent",
				Image: workloadmeta.ContainerImage{
					RawName:   "docker.io/datadog/agent:latest",
					Name:      "docker.io/datadog/agent",
					ShortName: "agent",
					Tag:       "latest",
				},
				NetworkIPs: make(map[string]string),
				PID:        10,
				Ports: []workloadmeta.ContainerPort{
					{
						Port:     2000,
						Protocol: "tcp",
					},
				},
				Runtime: workloadmeta.ContainerRuntimePodman,
				State: workloadmeta.ContainerState{
					Running:   true,
					StartedAt: startTime,
				},
			},
		},
		{
			Type:   workloadmeta.EventTypeSet,
			Source: workloadmeta.SourcePodman,
			Entity: &workloadmeta.Container{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   "124",
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      "dd-agent-dev",
					Namespace: "dev",
					Annotations: map[string]string{
						"annotation-a": "value-a",
						"annotation-b": "value-b",
					},
					Labels: map[string]string{
						"label-a-dev": "value-a-dev",
						"label-b-dev": "value-b-dev",
					},
				},
				EnvVars: map[string]string{
					"SOME_ENV": "SOME_VAL",
				},
				Hostname: "agent-dev",
				Image: workloadmeta.ContainerImage{
					RawName:   "docker.io/datadog/agent-dev:latest",
					Name:      "docker.io/datadog/agent-dev",
					ShortName: "agent-dev",
					Tag:       "latest",
				},
				NetworkIPs: make(map[string]string),
				PID:        11,
				Ports: []workloadmeta.ContainerPort{
					{
						Port:     3000,
						Protocol: "tcp",
					},
				},
				Runtime: workloadmeta.ContainerRuntimePodman,
				State: workloadmeta.ContainerState{
					Running:   true,
					StartedAt: startTime,
				},
			},
		},
	}

	client := fakePodmanClient{
		mockGetAllContainers: func() ([]podman.Container, error) {
			return containers, nil
		},
	}

	cacheWithExpired := util.NewExpire(1 * time.Second)
	expiredID := "1"
	cacheWithExpired.Update(workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   expiredID,
	}, time.Now().Add(-1*time.Hour)) // Expired because 1 hour > 10s defined above

	// The cache is initialized with a "lastExpire" that equals time.Now, and
	// the caller has no control over it. When calculating the expired entities,
	// it first checks that the last expire happen at least TTL seconds ago.
	// That's why here we need to sleep at least for the TTL defined (1 second
	// in this case).
	time.Sleep(1 * time.Second)

	tests := []struct {
		name           string
		client         podmanClient
		cache          *util.Expire
		expectedEvents []workloadmeta.CollectorEvent
	}{
		{
			name:           "without expired entities",
			client:         &client,
			cache:          util.NewExpire(10 * time.Second),
			expectedEvents: expectedEvents,
		},
		{
			name:   "with expired entities",
			client: &client,
			cache:  cacheWithExpired,
			expectedEvents: append(expectedEvents, workloadmeta.CollectorEvent{
				Type:   workloadmeta.EventTypeUnset,
				Source: workloadmeta.SourcePodman,
				Entity: workloadmeta.EntityID{
					Kind: workloadmeta.KindContainer,
					ID:   expiredID,
				},
			}),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			workloadmetaStore := fakeWorkloadmetaStore{}
			podmanCollector := collector{
				client: test.client,
				store:  &workloadmetaStore,
				expire: test.cache,
			}

			err := podmanCollector.Pull(context.TODO())

			assert.NoError(t, err)
			assert.Equal(t, test.expectedEvents, workloadmetaStore.notifiedEvents)
		})
	}
}
