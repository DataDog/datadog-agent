// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !serverless

package listeners

import (
	"fmt"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
	"github.com/stretchr/testify/assert"
)

const (
	containerID   = "foobarquux"
	containerName = "agent"
	podID         = "foobar"
	podName       = "datadog-agent-foobar"
	podNamespace  = "default"
)

func TestCreatePodService(t *testing.T) {
	tests := []struct {
		name             string
		pod              workloadmeta.KubernetesPod
		containers       []workloadmeta.Container
		expectedServices map[string]Service
	}{
		{
			name: "pod with several containers collects ports in ascending order",
			pod: workloadmeta.KubernetesPod{
				EntityID: workloadmeta.EntityID{
					Kind: workloadmeta.KindKubernetesPod,
					ID:   podID,
				},
				EntityMeta: workloadmeta.EntityMeta{
					Name:      podName,
					Namespace: podNamespace,
				},
				IP: "127.0.0.1",
			},
			containers: []workloadmeta.Container{
				{
					Ports: []workloadmeta.ContainerPort{
						{
							Name: "http",
							Port: 80,
						},
					},
				},
				{
					Ports: []workloadmeta.ContainerPort{
						{
							Name: "ssh",
							Port: 22,
						},
					},
				},
			},
			expectedServices: map[string]Service{
				"kubernetes_pod://foobar": &KubePodService{
					entity:        "kubernetes_pod://foobar",
					adIdentifiers: []string{"kubernetes_pod://foobar"},
					ports: []ContainerPort{
						{
							Port: 22,
							Name: "ssh",
						},
						{
							Port: 80,
							Name: "http",
						},
					},
					hosts: map[string]string{
						"pod": "127.0.0.1",
					},
					creationTime: integration.After,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan Service)
			listener := newListener(t, ch)
			actualServices, doneCh := consumeServiceCh(ch)

			listener.createPodService(tt.pod, tt.containers, false)

			close(ch)
			<-doneCh

			assertExpectedServices(t, tt.expectedServices, actualServices)
		})
	}
}

func TestCreateContainerService(t *testing.T) {
	pod := workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      podName,
			Namespace: podNamespace,
		},
		IP: "127.0.0.1",
	}

	podWithAnnotations := workloadmeta.KubernetesPod{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindKubernetesPod,
			ID:   podID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name:      podName,
			Namespace: podNamespace,
			Annotations: map[string]string{
				fmt.Sprintf("ad.datadoghq.com/%s.check.id", containerName): `customid`,
				fmt.Sprintf("ad.datadoghq.com/%s.instances", "customid"):   `[{}]`,
				fmt.Sprintf("ad.datadoghq.com/%s.check_names", "customid"): `["customcheck"]`,
			},
		},
		IP: "127.0.0.1",
	}

	containerEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	containerEntityMeta := workloadmeta.EntityMeta{
		Name: containerName,
	}

	basicImage := workloadmeta.ContainerImage{
		RawName:   "foobar",
		ShortName: "foobar",
	}

	tests := []struct {
		name             string
		pod              workloadmeta.KubernetesPod
		container        workloadmeta.Container
		expectedServices map[string]Service
	}{
		{
			name: "basic container setup",
			pod:  pod,
			container: workloadmeta.Container{
				EntityID:   containerEntityID,
				EntityMeta: containerEntityMeta,
				Image: workloadmeta.ContainerImage{
					RawName:   "gcr.io/foobar:latest",
					ShortName: "foobar",
				},
				State: workloadmeta.ContainerState{
					Running: true,
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
			expectedServices: map[string]Service{
				"docker://foobarquux": &KubeContainerService{
					entity: "docker://foobarquux",
					adIdentifiers: []string{
						"docker://foobarquux",
						"gcr.io/foobar:latest",
						"foobar",
					},
					hosts: map[string]string{
						"pod": "127.0.0.1",
					},
					ports:        []ContainerPort{},
					creationTime: integration.After,
					extraConfig: map[string]string{
						"namespace": podNamespace,
						"pod_name":  podName,
						"pod_uid":   podID,
					},
				},
			},
		},
		{
			name: "recently stopped container excludes metrics but not logs",
			pod:  pod,
			container: workloadmeta.Container{
				EntityID:   containerEntityID,
				EntityMeta: containerEntityMeta,
				Image:      basicImage,
				State: workloadmeta.ContainerState{
					Running: false,
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
			expectedServices: map[string]Service{
				"docker://foobarquux": &KubeContainerService{
					entity: "docker://foobarquux",
					adIdentifiers: []string{
						"docker://foobarquux",
						"foobar",
					},
					hosts: map[string]string{
						"pod": "127.0.0.1",
					},
					ports:           []ContainerPort{},
					creationTime:    integration.After,
					metricsExcluded: true,
					extraConfig: map[string]string{
						"namespace": podNamespace,
						"pod_name":  podName,
						"pod_uid":   podID,
					},
				},
			},
		},
		{
			name: "old stopped container does not get collected",
			pod:  pod,
			container: workloadmeta.Container{
				EntityID:   containerEntityID,
				EntityMeta: containerEntityMeta,
				Image:      basicImage,
				State: workloadmeta.ContainerState{
					FinishedAt: time.Now().Add(-48 * time.Hour),
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
			expectedServices: map[string]Service{},
		},
		{
			name: "container with multiple ports collects them in ascending order",
			pod:  pod,
			container: workloadmeta.Container{
				EntityID:   containerEntityID,
				EntityMeta: containerEntityMeta,
				Image:      basicImage,
				Ports: []workloadmeta.ContainerPort{
					{
						Name: "http",
						Port: 80,
					},
					{
						Name: "ssh",
						Port: 22,
					},
				},
				State: workloadmeta.ContainerState{
					Running: true,
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
			expectedServices: map[string]Service{
				"docker://foobarquux": &KubeContainerService{
					entity: "docker://foobarquux",
					adIdentifiers: []string{
						"docker://foobarquux",
						"foobar",
					},
					hosts: map[string]string{
						"pod": "127.0.0.1",
					},
					ports: []ContainerPort{
						{
							Port: 22,
							Name: "ssh",
						},
						{
							Port: 80,
							Name: "http",
						},
					},
					creationTime: integration.After,
					extraConfig: map[string]string{
						"namespace": podNamespace,
						"pod_name":  podName,
						"pod_uid":   podID,
					},
				},
			},
		},
		{
			name: "pod with custom check names and identifiers",
			pod:  podWithAnnotations,
			container: workloadmeta.Container{
				EntityID:   containerEntityID,
				EntityMeta: containerEntityMeta,
				Image:      basicImage,
				State: workloadmeta.ContainerState{
					Running: true,
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
			expectedServices: map[string]Service{
				"docker://foobarquux": &KubeContainerService{
					entity: "docker://foobarquux",
					adIdentifiers: []string{
						"customid",
						"docker://foobarquux",
						"foobar",
					},
					hosts: map[string]string{
						"pod": "127.0.0.1",
					},
					ports:        []ContainerPort{},
					creationTime: integration.After,
					checkNames:   []string{"customcheck"},
					extraConfig: map[string]string{
						"namespace": podNamespace,
						"pod_name":  podName,
						"pod_uid":   podID,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan Service)
			listener := newListener(t, ch)
			actualServices, doneCh := consumeServiceCh(ch)

			listener.createContainerService(tt.pod, tt.container, false)

			close(ch)
			<-doneCh

			assertExpectedServices(t, tt.expectedServices, actualServices)
		})
	}
}

func newListener(t *testing.T, ch chan Service) *KubeletListener {
	filters, err := newContainerFilters()
	if err != nil {
		t.Fatalf("cannot initialize container filters: %s", err)
	}

	return &KubeletListener{
		services:   make(map[string]Service),
		newService: ch,
		filters:    filters,
	}
}

func consumeServiceCh(ch chan Service) (map[string]Service, chan struct{}) {
	doneCh := make(chan struct{})
	services := make(map[string]Service)

	go func() {
		for svc := range ch {
			if svc == nil {
				break
			}

			services[svc.GetEntity()] = svc
		}

		close(doneCh)
	}()

	return services, doneCh
}

func assertExpectedServices(t *testing.T, expectedServices, actualServices map[string]Service) {
	for entity, expectedSvc := range expectedServices {
		actualSvc, ok := actualServices[entity]
		if !ok {
			t.Errorf("expected to find service %q, but it was not generated", entity)
			continue
		}

		assert.Equal(t, expectedSvc, actualSvc)

		delete(actualServices, entity)
	}

	if len(actualServices) > 0 {
		t.Errorf("got unexpected services: %+v", actualServices)
	}
}
