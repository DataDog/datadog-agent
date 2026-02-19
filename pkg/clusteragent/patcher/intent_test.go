// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package patcher

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPatchIntentBuildEmpty(t *testing.T) {
	intent := NewPatchIntent(DeploymentTarget("default", "test"))
	data, err := intent.Build()
	require.NoError(t, err)
	assert.Nil(t, data, "empty intent should return nil patch data")
}

func TestPatchIntentBuildSingleOperation(t *testing.T) {
	intent := NewPatchIntent(DeploymentTarget("default", "test")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"key": "value",
		}))

	data, err := intent.Build()
	require.NoError(t, err)
	require.NotNil(t, data)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	metadata := result["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, "value", annotations["key"])
}

func TestPatchIntentBuildMergesOperations(t *testing.T) {
	intent := NewPatchIntent(DeploymentTarget("default", "test")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"meta-annot": "meta-value",
		})).
		With(SetPodTemplateAnnotations(map[string]interface{}{
			"template-annot": "template-value",
		})).
		With(SetPodTemplateLabels(map[string]interface{}{
			"app": "myapp",
		}))

	data, err := intent.Build()
	require.NoError(t, err)
	require.NotNil(t, data)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	// Check metadata.annotations
	metadata := result["metadata"].(map[string]interface{})
	metaAnnotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, "meta-value", metaAnnotations["meta-annot"])

	// Check spec.template.metadata.annotations
	spec := result["spec"].(map[string]interface{})
	template := spec["template"].(map[string]interface{})
	templateMeta := template["metadata"].(map[string]interface{})
	templateAnnotations := templateMeta["annotations"].(map[string]interface{})
	assert.Equal(t, "template-value", templateAnnotations["template-annot"])

	// Check spec.template.metadata.labels
	templateLabels := templateMeta["labels"].(map[string]interface{})
	assert.Equal(t, "myapp", templateLabels["app"])
}

func TestPatchIntentBuildMergesOperationsConflicts(t *testing.T) {
	// Operations that set metadata annotations which should be merged
	intent := NewPatchIntent(DeploymentTarget("default", "test")).
		With(SetMetadataAnnotations(map[string]interface{}{
			"key1": "val1",
		})).
		With(SetMetadataAnnotations(map[string]interface{}{
			"key2": "val2",
		})).
		With(SetMetadataAnnotations(map[string]interface{}{
			"key1": "val3",
		}))

	data, err := intent.Build()
	require.NoError(t, err)

	var result map[string]interface{}
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	metadata := result["metadata"].(map[string]interface{})
	annotations := metadata["annotations"].(map[string]interface{})
	assert.Equal(t, "val3", annotations["key1"])
	assert.Equal(t, "val2", annotations["key2"])
}

func TestMergeMaps(t *testing.T) {
	t.Run("simple merge", func(t *testing.T) {
		dst := map[string]interface{}{"a": "1"}
		src := map[string]interface{}{"b": "2"}
		mergeMaps(dst, src)
		assert.Equal(t, "1", dst["a"])
		assert.Equal(t, "2", dst["b"])
	})

	t.Run("nested merge", func(t *testing.T) {
		dst := map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{"existing": "val"},
			},
		}
		src := map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{"new": "val2"},
			},
		}
		mergeMaps(dst, src)

		metadata := dst["metadata"].(map[string]interface{})
		annotations := metadata["annotations"].(map[string]interface{})
		assert.Equal(t, "val", annotations["existing"])
		assert.Equal(t, "val2", annotations["new"])
	})

	t.Run("overwrite non-map value", func(t *testing.T) {
		dst := map[string]interface{}{"key": "old"}
		src := map[string]interface{}{"key": "new"}
		mergeMaps(dst, src)
		assert.Equal(t, "new", dst["key"])
	})

	t.Run("nil values preserved", func(t *testing.T) {
		dst := map[string]interface{}{}
		src := map[string]interface{}{
			"metadata": map[string]interface{}{
				"annotations": map[string]interface{}{"delete-me": nil},
			},
		}
		mergeMaps(dst, src)
		metadata := dst["metadata"].(map[string]interface{})
		annotations := metadata["annotations"].(map[string]interface{})
		v, exists := annotations["delete-me"]
		assert.True(t, exists)
		assert.Nil(t, v)
	})
}
