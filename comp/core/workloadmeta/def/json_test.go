// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package workloadmeta

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestContainerJSONMarshaling(t *testing.T) {
	// Create a sample container
	container := &Container{
		EntityID: EntityID{
			Kind: KindContainer,
			ID:   "test-container-123",
		},
		EntityMeta: EntityMeta{
			Name:      "test-container",
			Namespace: "default",
			Labels: map[string]string{
				"app": "test",
			},
		},
		Hostname: "test-host",
		Image: ContainerImage{
			Name: "nginx",
			Tag:  "latest",
		},
		PID: 1234,
		State: ContainerState{
			Running:   true,
			Status:    ContainerStatusRunning,
			CreatedAt: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		},
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(container)
	require.NoError(t, err)

	// Unmarshal back to a map to check structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify the JSON structure uses Go default capitalized field names
	assert.Equal(t, "container", result["Kind"])
	assert.Equal(t, "test-container-123", result["ID"])
	assert.Equal(t, "test-container", result["Name"])
	assert.Equal(t, "default", result["Namespace"])
	assert.Equal(t, "test-host", result["Hostname"])
	assert.Equal(t, float64(1234), result["PID"])

	// Check nested structures
	state, ok := result["State"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, true, state["Running"])

	image, ok := result["Image"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "nginx", image["Name"])
	assert.Equal(t, "latest", image["Tag"])

	labels, ok := result["Labels"].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "test", labels["app"])
}

func TestKubernetesPodJSONMarshaling(t *testing.T) {
	// Create a sample pod
	pod := &KubernetesPod{
		EntityID: EntityID{
			Kind: KindKubernetesPod,
			ID:   "test-pod-123",
		},
		EntityMeta: EntityMeta{
			Name:      "test-pod",
			Namespace: "default",
		},
		Phase:         "Running",
		Ready:         true,
		IP:            "10.0.0.1",
		PriorityClass: "high",
		Containers: []OrchestratorContainer{
			{
				ID:   "container-1",
				Name: "nginx",
				Image: ContainerImage{
					Name: "nginx",
					Tag:  "latest",
				},
			},
		},
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(pod)
	require.NoError(t, err)

	// Unmarshal back to a map to check structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	// Verify the JSON structure uses Go default capitalized field names
	assert.Equal(t, "kubernetes_pod", result["Kind"])
	assert.Equal(t, "test-pod-123", result["ID"])
	assert.Equal(t, "test-pod", result["Name"])
	assert.Equal(t, "default", result["Namespace"])
	assert.Equal(t, "Running", result["Phase"])
	assert.Equal(t, true, result["Ready"])
	assert.Equal(t, "10.0.0.1", result["IP"])
	assert.Equal(t, "high", result["PriorityClass"])

	// Check containers array
	containers, ok := result["Containers"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, containers, 1)

	container, ok := containers[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "container-1", container["ID"])
	assert.Equal(t, "nginx", container["Name"])
}

func TestWorkloadDumpStructuredResponseJSONMarshaling(t *testing.T) {
	// Create a sample response
	response := WorkloadDumpStructuredResponse{
		Entities: map[string][]Entity{
			"container": {
				&Container{
					EntityID: EntityID{
						Kind: KindContainer,
						ID:   "test-123",
					},
					EntityMeta: EntityMeta{
						Name: "test-container",
					},
					Hostname: "test-host",
				},
			},
		},
	}

	// Marshal to JSON
	jsonBytes, err := json.Marshal(response)
	require.NoError(t, err)

	// Unmarshal back to a map to check structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonBytes, &result)
	require.NoError(t, err)

	entities, ok := result["Entities"].(map[string]interface{})
	assert.True(t, ok)

	containers, ok := entities["container"].([]interface{})
	assert.True(t, ok)
	assert.Len(t, containers, 1)

	container, ok := containers[0].(map[string]interface{})
	assert.True(t, ok)
	assert.Equal(t, "container", container["Kind"])
	assert.Equal(t, "test-123", container["ID"])
	assert.Equal(t, "test-container", container["Name"])
	assert.Equal(t, "test-host", container["Hostname"])
}
