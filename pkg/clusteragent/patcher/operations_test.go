// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patcher

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetMetadataAnnotations(t *testing.T) {
	op := SetMetadataAnnotations(map[string]interface{}{
		"key1": "val1",
		"key2": nil, // deletion
	})

	result := op.build()
	metadata := result["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})

	assert.Equal(t, "val1", annotations["key1"])
	assert.Nil(t, annotations["key2"])
}

func TestSetPodTemplateAnnotations(t *testing.T) {
	op := SetPodTemplateAnnotations(map[string]interface{}{
		"annot1": "value1",
	})

	result := op.build()
	spec := result["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	metadata := template["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})

	assert.Equal(t, "value1", annotations["annot1"])
}

func TestSetPodTemplateLabels(t *testing.T) {
	op := SetPodTemplateLabels(map[string]interface{}{
		"app": "myapp",
	})

	result := op.build()
	spec := result["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	metadata := template["metadata"].(map[string]interface{})
	labels := metadata["labels"].(map[string]interface{})

	assert.Equal(t, "myapp", labels["app"])
}

func TestDeletePodTemplateAnnotations(t *testing.T) {
	op := DeletePodTemplateAnnotations([]string{"remove-me", "also-remove"})

	result := op.build()
	spec := result["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	metadata := template["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})

	assert.Nil(t, annotations["remove-me"])
	assert.Nil(t, annotations["also-remove"])
	assert.Len(t, annotations, 2)
}

func TestSetMetadataLabels(t *testing.T) {
	op := SetMetadataLabels(map[string]interface{}{
		"label1": "val1",
		"label2": nil,
	})

	result := op.build()
	metadata := result["metadata"].(map[string]interface{})
	labels := metadata["labels"].(map[string]interface{})

	assert.Equal(t, "val1", labels["label1"])
	assert.Nil(t, labels["label2"])
}

func TestSetContainerResources(t *testing.T) {
	t.Run("single container with requests and limits", func(t *testing.T) {
		op := SetContainerResources([]ContainerResourcePatch{
			{
				Name:     "app",
				Requests: map[string]string{"cpu": "250m", "memory": "512Mi"},
				Limits:   map[string]string{"cpu": "500m", "memory": "1Gi"},
			},
		})

		result := op.build()
		spec := result["spec"].(map[string]interface{})
		containers := spec["containers"].([]interface{})
		require.Len(t, containers, 1)

		container := containers[0].(map[string]interface{})
		assert.Equal(t, "app", container["name"])
		resources := container["resources"].(map[string]interface{})

		requests := resources["requests"].(map[string]interface{})
		assert.Equal(t, "250m", requests["cpu"])
		assert.Equal(t, "512Mi", requests["memory"])

		limits := resources["limits"].(map[string]interface{})
		assert.Equal(t, "500m", limits["cpu"])
		assert.Equal(t, "1Gi", limits["memory"])
	})

	t.Run("multiple containers", func(t *testing.T) {
		op := SetContainerResources([]ContainerResourcePatch{
			{Name: "app", Requests: map[string]string{"cpu": "100m"}},
			{Name: "sidecar", Requests: map[string]string{"cpu": "50m"}},
		})

		result := op.build()
		spec := result["spec"].(map[string]interface{})
		containers := spec["containers"].([]interface{})
		require.Len(t, containers, 2)

		c0 := containers[0].(map[string]interface{})
		assert.Equal(t, "app", c0["name"])
		c0Requests := c0["resources"].(map[string]interface{})["requests"].(map[string]interface{})
		assert.Equal(t, "100m", c0Requests["cpu"])

		c1 := containers[1].(map[string]interface{})
		assert.Equal(t, "sidecar", c1["name"])
		c1Requests := c1["resources"].(map[string]interface{})["requests"].(map[string]interface{})
		assert.Equal(t, "50m", c1Requests["cpu"])
	})

	t.Run("requests only, no limits", func(t *testing.T) {
		op := SetContainerResources([]ContainerResourcePatch{
			{Name: "app", Requests: map[string]string{"memory": "256Mi"}},
		})

		result := op.build()
		spec := result["spec"].(map[string]interface{})
		containers := spec["containers"].([]interface{})
		container := containers[0].(map[string]interface{})
		resources := container["resources"].(map[string]interface{})

		_, hasRequests := resources["requests"]
		_, hasLimits := resources["limits"]
		assert.True(t, hasRequests)
		assert.False(t, hasLimits)
	})

	t.Run("limits only, no requests", func(t *testing.T) {
		op := SetContainerResources([]ContainerResourcePatch{
			{Name: "app", Limits: map[string]string{"cpu": "500m"}},
		})

		result := op.build()
		spec := result["spec"].(map[string]interface{})
		containers := spec["containers"].([]interface{})
		container := containers[0].(map[string]interface{})
		resources := container["resources"].(map[string]interface{})

		_, hasRequests := resources["requests"]
		_, hasLimits := resources["limits"]
		assert.False(t, hasRequests)
		assert.True(t, hasLimits)
		assert.Equal(t, "500m", resources["limits"].(map[string]interface{})["cpu"])
	})

	t.Run("empty container list", func(t *testing.T) {
		op := SetContainerResources([]ContainerResourcePatch{})
		result := op.build()
		spec := result["spec"].(map[string]interface{})
		containers := spec["containers"].([]interface{})
		assert.Empty(t, containers)
	})

	t.Run("empty Name is passed through without guard", func(t *testing.T) {
		// Name == "" is not rejected at construction time; the empty string becomes
		// the strategic-merge-patch key and will fail to match any real container.
		// Callers are responsible for providing a non-empty Name.
		op := SetContainerResources([]ContainerResourcePatch{
			{Name: "", Requests: map[string]string{"cpu": "100m"}},
		})
		result := op.build()
		spec := result["spec"].(map[string]interface{})
		containers := spec["containers"].([]interface{})
		require.Len(t, containers, 1)
		assert.Equal(t, "", containers[0].(map[string]interface{})["name"])
	})
}

func TestEmptyOperations(t *testing.T) {
	t.Run("empty metadata annotations", func(t *testing.T) {
		op := SetMetadataAnnotations(map[string]interface{}{})
		result := op.build()
		metadata := result["metadata"].(map[string]interface{})
		annotations := metadata["annotations"].(map[string]interface{})
		require.Empty(t, annotations)
	})

	t.Run("empty delete list", func(t *testing.T) {
		op := DeletePodTemplateAnnotations([]string{})
		result := op.build()
		spec := result["spec"].(map[string]interface{})
		template := spec["template"].(map[string]interface{})
		metadata := template["metadata"].(map[string]interface{})
		annotations := metadata["annotations"].(map[string]interface{})
		require.Empty(t, annotations)
	})
}
