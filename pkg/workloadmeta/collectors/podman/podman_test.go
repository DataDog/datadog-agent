// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build podman
// +build podman

package podman

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/containernetworking/cni/pkg/types/current"
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
				NetworkStatus: []*current.Result{
					{
						IPs: []*current.IPConfig{
							{
								Address: net.IPNet{
									IP: net.ParseIP("10.88.0.13"),
								},
							},
						},
					},
				},
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
				NetworkStatus: []*current.Result{
					{
						IPs: []*current.IPConfig{
							{
								Address: net.IPNet{
									IP: net.ParseIP("10.88.0.14"),
								},
							},
						},
					},
				},
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
				NetworkIPs: map[string]string{
					"podman": "10.88.0.13",
				},
				PID: 10,
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
				NetworkIPs: map[string]string{
					"podman": "10.88.0.14",
				},
				PID: 11,
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

func TestNetworkIPS(t *testing.T) {
	tests := []struct {
		name               string
		container          podman.Container
		expectedNetworkIPs map[string]string
	}{
		{
			// This is the case where no --net is specified when running the
			// container. The default network "podman" is used in this case.
			name: "no network names, but one network status reported",
			container: podman.Container{
				Config: &podman.ContainerConfig{
					ContainerNetworkConfig: podman.ContainerNetworkConfig{
						Networks: []string{},
					},
				},
				State: &podman.ContainerState{
					NetworkStatus: []*current.Result{
						{
							IPs: []*current.IPConfig{
								{
									Address: net.IPNet{
										IP: net.ParseIP("10.88.0.14"),
									},
								},
							},
						},
					},
				},
			},
			expectedNetworkIPs: map[string]string{
				"podman": "10.88.0.14",
			},
		},
		{
			name: "same number of network names and statuses reported",
			container: podman.Container{
				Config: &podman.ContainerConfig{
					ContainerNetworkConfig: podman.ContainerNetworkConfig{
						// Sorted by the order they appear in the run command.
						Networks: []string{"network-b", "network-a", "network-c"},
					},
				},
				State: &podman.ContainerState{ // Sorted alphabetically by network name
					NetworkStatus: []*current.Result{
						{
							IPs: []*current.IPConfig{
								{
									Address: net.IPNet{
										IP: net.ParseIP("10.88.0.11"),
									},
								},
							},
						},
						{
							IPs: []*current.IPConfig{
								{
									Address: net.IPNet{
										IP: net.ParseIP("10.88.0.12"),
									},
								},
							},
						},
						{
							IPs: []*current.IPConfig{
								{
									Address: net.IPNet{
										IP: net.ParseIP("10.88.0.13"),
									},
								},
							},
						},
					},
				},
			},
			expectedNetworkIPs: map[string]string{
				"network-a": "10.88.0.11",
				"network-b": "10.88.0.12",
				"network-c": "10.88.0.13",
			},
		},
		{
			// If there's more than one network name and the number doesn't
			// match the number of statutes reported, it means that some
			// networks were attached or removed after the container was
			// started. This is a use case that we don't support, and we just
			// return and empty map.
			name: "different number of network names and statutes reported",
			container: podman.Container{
				Config: &podman.ContainerConfig{
					ContainerNetworkConfig: podman.ContainerNetworkConfig{
						Networks: []string{"network-a"},
					},
				},
				State: &podman.ContainerState{
					NetworkStatus: []*current.Result{
						{
							IPs: []*current.IPConfig{
								{
									Address: net.IPNet{
										IP: net.ParseIP("10.88.0.10"),
									},
								},
							},
						},
						{
							IPs: []*current.IPConfig{
								{
									Address: net.IPNet{
										IP: net.ParseIP("10.88.0.11"),
									},
								},
							},
						},
					},
				},
			},
			expectedNetworkIPs: map[string]string{},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.expectedNetworkIPs, networkIPs(&test.container))
		})
	}
}
