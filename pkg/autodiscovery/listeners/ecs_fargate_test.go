// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2017-present Datadog, Inc.

// +build !serverless

package listeners

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/workloadmeta"
)

const (
	taskID   = "foobar"
	taskName = "datadog-agent-foobar"
)

func TestECSFargateCreateContainerService(t *testing.T) {
	task := &workloadmeta.ECSTask{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindECSTask,
			ID:   taskID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: taskName,
		},
	}

	containerEntityID := workloadmeta.EntityID{
		Kind: workloadmeta.KindContainer,
		ID:   containerID,
	}

	containerEntityMeta := workloadmeta.EntityMeta{
		Name: containerName,
	}

	tests := []struct {
		name             string
		task             *workloadmeta.ECSTask
		container        *workloadmeta.Container
		expectedServices map[string]Service
	}{
		{
			name: "basic container setup",
			task: task,
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
				NetworkIPs: map[string]string{
					"awsvpc": "127.0.0.1",
				},
				Runtime: workloadmeta.ContainerRuntimeDocker,
			},
			expectedServices: map[string]Service{
				"docker://foobarquux": &ECSService{
					cID:     containerID,
					runtime: "docker",
					ADIdentifiers: []string{
						"docker://foobarquux",
						"gcr.io/foobar",
						"foobar",
					},
					hosts: map[string]string{
						"awsvpc": "127.0.0.1",
					},
					creationTime: integration.After,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			newCh := make(chan Service)
			delCh := make(chan Service)
			listener := newECSFargateListener(t, newCh, delCh)
			actualServices, doneCh := consumeServiceCh(t, newCh, delCh)

			listener.createContainerService(tt.task, tt.container, false)

			close(newCh)
			close(delCh)
			<-doneCh

			assertExpectedServices(t, tt.expectedServices, actualServices)
		})
	}
}

func TestECSFargateRemoveTaskService(t *testing.T) {
	newCh := make(chan Service)
	delCh := make(chan Service)
	listener := newECSFargateListener(t, newCh, delCh)
	actualServices, doneCh := consumeServiceCh(t, newCh, delCh)

	task := &workloadmeta.ECSTask{
		EntityID: workloadmeta.EntityID{
			Kind: workloadmeta.KindECSTask,
			ID:   taskID,
		},
		EntityMeta: workloadmeta.EntityMeta{
			Name: taskName,
		},
		Containers: []workloadmeta.OrchestratorContainer{
			{ID: "foo"},
			{ID: "bar"},
		},
	}

	containers := []*workloadmeta.Container{
		{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "foo",
			},
			Runtime: workloadmeta.ContainerRuntimeDocker,
		},
		{
			EntityID: workloadmeta.EntityID{
				Kind: workloadmeta.KindContainer,
				ID:   "bar",
			},
			Runtime: workloadmeta.ContainerRuntimeDocker,
		},
	}

	for _, c := range containers {
		listener.createContainerService(task, c, false)
	}

	listener.removeTaskService(task.GetID())

	close(newCh)
	close(delCh)
	<-doneCh

	assertExpectedServices(t, map[string]Service{}, actualServices)
}

func newECSFargateListener(t *testing.T, newCh, delCh chan Service) *ECSFargateListener {
	filters, err := newContainerFilters()
	if err != nil {
		t.Fatalf("cannot initialize container filters: %s", err)
	}

	return &ECSFargateListener{
		services:       make(map[string]Service),
		taskContainers: make(map[string]map[string]struct{}),
		newService:     newCh,
		delService:     delCh,
		filters:        filters,
	}
}
