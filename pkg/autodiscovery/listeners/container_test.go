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

func TestCreateContainerService(t *testing.T) {
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

	basicContainer := &workloadmeta.Container{
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
	}

	multiplePortsContainer := &workloadmeta.Container{
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
	}

	tests := []struct {
		name             string
		container        *workloadmeta.Container
		expectedServices map[string]wlmListenerSvc
	}{
		{
			name:      "basic container setup",
			container: basicContainer,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					service: &service{
						entity: basicContainer,
						adIdentifiers: []string{
							"docker://foobarquux",
							"gcr.io/foobar",
							"foobar",
						},
						hosts:        map[string]string{},
						creationTime: integration.After,
						ports:        []ContainerPort{},
						ready:        true,
					},
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
			expectedServices: map[string]wlmListenerSvc{},
		},
		{
			name:      "container with multiple ports collects them in ascending order",
			container: multiplePortsContainer,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					service: &service{
						entity: multiplePortsContainer,
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
						ready:        true,
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listener, wlm := newContainerListener(t)

			listener.createContainerService(tt.container, integration.After)

			wlm.assertServices(tt.expectedServices)
		})
	}
}

func newContainerListener(t *testing.T) (*ContainerListener, *testWorkloadmetaListener) {
	wlm := newTestWorkloadmetaListener(t)

	return &ContainerListener{workloadmetaListener: wlm}, wlm
}
