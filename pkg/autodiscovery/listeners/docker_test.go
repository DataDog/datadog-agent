// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// +build !serverless

package listeners

import (
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

func TestDockerCreateContainerService(t *testing.T) {
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
		container        *workloadmeta.Container
		expectedServices map[string]Service
	}{
		{
			name: "basic container setup",
			container: &workloadmeta.Container{
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
				"docker://foobarquux": &DockerService{
					containerID: "foobarquux",
					adIdentifiers: []string{
						"docker://foobarquux",
						"gcr.io/foobar",
						"foobar",
					},
					hosts:        map[string]string{},
					creationTime: integration.After,
				},
			},
		},
		{
			name: "old stopped container does not get collected",
			container: &workloadmeta.Container{
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
			container: &workloadmeta.Container{
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
				"docker://foobarquux": &DockerService{
					containerID: "foobarquux",
					adIdentifiers: []string{
						"docker://foobarquux",
						"foobar",
					},
					hosts: map[string]string{},
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
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch := make(chan Service)
			listener := newDockerListener(t, ch)
			actualServices, doneCh := consumeServiceCh(ch)

			listener.createContainerService(tt.container)

			close(ch)
			<-doneCh

			assertExpectedServices(t, tt.expectedServices, actualServices)
		})
	}
}

func newDockerListener(t *testing.T, ch chan Service) *DockerListener {
	filters, err := newContainerFilters()
	if err != nil {
		t.Fatalf("cannot initialize container filters: %s", err)
	}

	return &DockerListener{
		services:   make(map[string]Service),
		newService: ch,
		filters:    filters,
	}
}
