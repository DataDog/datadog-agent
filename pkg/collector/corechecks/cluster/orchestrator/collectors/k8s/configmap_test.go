// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
)

const lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

func TestConfigMapCollector(t *testing.T) {
	configMap := &metav1.PartialObjectMetadata{
		// The fake metadata client derives the resource from the Kind.
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "configmap",
			Namespace:         "namespace",
			UID:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
			ResourceVersion:   "1220",
			CreationTimestamp: CreateTestTime(),
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation":                "my-annotation",
				lastAppliedConfigAnnotation: `{"apiVersion":"v1","data":{"key":"secret-value"},"kind":"ConfigMap"}`,
			},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply},
			},
		},
	}

	collector := NewConfigMapCollector()

	config := CollectorTestConfig{
		MetadataResources:          []runtime.Object{configMap},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		// ConfigMap is manifest-only: the processor yields a nil metadata body, which is never sent.
		ExpectedMetadataMessages: 1,
		ExpectedManifestMessages: 1,
		AssertionsFn: func(t *testing.T, _ *collectors.CollectorRunConfig, result *collectors.CollectorRunResult) {
			assert.Nil(t, result.Result.MetadataMessages[0])

			// The transform trims objects before they are stored in the informer cache.
			cached, err := collector.lister.Namespace("namespace").Get("configmap")
			assert.NoError(t, err)
			assert.Nil(t, cached.ManagedFields)
			assert.Equal(t, "-", cached.Annotations[lastAppliedConfigAnnotation])
			assert.Equal(t, "my-annotation", cached.Annotations["annotation"])

			assert.Len(t, result.Result.ManifestMessages, 1)
			collectorManifest, ok := result.Result.ManifestMessages[0].(*model.CollectorManifest)
			assert.True(t, ok)
			assert.Len(t, collectorManifest.Manifests, 1)

			manifest := collectorManifest.Manifests[0]
			assert.Equal(t, string(configMap.UID), manifest.Uid)
			assert.Equal(t, configMap.ResourceVersion, manifest.ResourceVersion)

			var parsed map[string]interface{}
			assert.NoError(t, yaml.Unmarshal(manifest.Content, &parsed))
			assert.Equal(t, "ConfigMap", parsed["kind"])
			assert.Equal(t, "v1", parsed["apiVersion"])
			assert.NotContains(t, parsed, "data")
			assert.NotContains(t, parsed, "binaryData")

			metadata, ok := parsed["metadata"].(map[string]interface{})
			assert.True(t, ok)
			assert.Equal(t, "configmap", metadata["name"])
			assert.NotContains(t, metadata, "managedFields")
		},
	}

	RunCollectorTest(t, config, collector)
}
