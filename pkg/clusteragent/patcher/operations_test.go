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
