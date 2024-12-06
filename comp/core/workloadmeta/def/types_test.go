// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package workloadmeta

import (
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestNewContainerImage(t *testing.T) {
	tests := []struct {
		name                      string
		imageName                 string
		expectedWorkloadmetaImage ContainerImage
		expectsErr                bool
	}{
		{
			name:      "image with tag",
			imageName: "datadog/agent:7",
			expectedWorkloadmetaImage: ContainerImage{
				RawName:   "datadog/agent:7",
				Name:      "datadog/agent",
				ShortName: "agent",
				Tag:       "7",
				ID:        "0",
			},
		}, {
			name:      "image without tag",
			imageName: "datadog/agent",
			expectedWorkloadmetaImage: ContainerImage{
				RawName:   "datadog/agent",
				Name:      "datadog/agent",
				ShortName: "agent",
				Tag:       "latest", // Default to latest when there's no tag
				ID:        "1",
			},
		},
	}

	for i, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			image, err := NewContainerImage(strconv.Itoa(i), test.imageName)
			assert.NoError(t, err)
			assert.Equal(t, test.expectedWorkloadmetaImage, image)
		})
	}
}

func TestECSTaskString(t *testing.T) {
	task := ECSTask{
		EntityID: EntityID{
			Kind: KindECSTask,
			ID:   "task-1-id",
		},
		EntityMeta: EntityMeta{
			Name: "task-1",
		},
		Containers: []OrchestratorContainer{
			{
				ID:   "container-1-id",
				Name: "container-1",
				Image: ContainerImage{
					RawName:   "datadog/agent:7",
					Name:      "datadog/agent",
					ShortName: "agent",
					Tag:       "7",
					ID:        "0",
				},
			},
		},
		Family:  "family-1",
		Version: "revision-1",
		EphemeralStorageMetrics: map[string]int64{
			"memory": 100,
			"cpu":    200,
		},
	}
	expected := `----------- Entity ID -----------
Kind: ecs_task ID: task-1-id
----------- Entity Meta -----------
Name: task-1
Namespace:
Annotations:
Labels:
----------- Containers -----------
Name: container-1 ID: container-1-id
----------- Task Info -----------
Tags:
Container Instance Tags:
Cluster Name:
Region:
Availability Zone:
Family: family-1
Version: revision-1
Launch Type:
AWS Account ID: 0
Desired Status:
Known Status:
VPC ID:
Ephemeral Storage Metrics: map[cpu:200 memory:100]
Limits: map[]
`
	compareTestOutput(t, expected, task.String(true))
}

func compareTestOutput(t *testing.T, expected, actual string) {
	assert.Equal(t, strings.ReplaceAll(expected, " ", ""), strings.ReplaceAll(actual, " ", ""))
}

func TestMergeECSContainer(t *testing.T) {
	container1 := Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "container-1-id",
		},
		EntityMeta: EntityMeta{
			Name: "container-1",
		},
		PID: 123,
		ECSContainer: &ECSContainer{
			DisplayName: "ecs-container-1",
		},
	}
	container2 := Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "container-1-id",
		},
		EntityMeta: EntityMeta{
			Name: "container-1",
		},
	}

	err := container2.Merge(&container1)
	assert.NoError(t, err)
	assert.NotSame(t, container1.ECSContainer, container2.ECSContainer, "pointers of ECSContainer should not be equal")
	assert.EqualValues(t, container1.ECSContainer, container2.ECSContainer)

	container2.ECSContainer = nil
	err = container1.Merge(&container2)
	assert.NoError(t, err)
	assert.NotSame(t, container1.ECSContainer, container2.ECSContainer, "pointers of ECSContainer should not be equal")
	assert.Nil(t, container2.ECSContainer)
	assert.EqualValues(t, container1.ECSContainer.DisplayName, "ecs-container-1")
}
