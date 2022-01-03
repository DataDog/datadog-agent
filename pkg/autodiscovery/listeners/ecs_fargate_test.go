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

	container := &workloadmeta.Container{
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
	}

	tests := []struct {
		name             string
		task             *workloadmeta.ECSTask
		container        *workloadmeta.Container
		expectedServices map[string]wlmListenerSvc
	}{
		{
			name:      "basic container setup",
			task:      task,
			container: container,
			expectedServices: map[string]wlmListenerSvc{
				"container://foobarquux": {
					parent: "ecs_task://foobar",
					service: &service{
						entity: container,
						adIdentifiers: []string{
							"docker://foobarquux",
							"gcr.io/foobar",
							"foobar",
						},
						hosts: map[string]string{
							"awsvpc": "127.0.0.1",
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
			listener, wlm := newECSFargateListener(t)

			listener.createContainerService(tt.task, tt.container, integration.After)

			wlm.assertServices(tt.expectedServices)
		})
	}
}

func newECSFargateListener(t *testing.T) (*ECSFargateListener, *testWorkloadmetaListener) {
	wlm := newTestWorkloadmetaListener(t)

	return &ECSFargateListener{workloadmetaListener: wlm}, wlm
}
