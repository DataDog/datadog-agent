// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package k8s

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
)

func TestHelmReleaseConfigMapCollector(t *testing.T) {
	const releasePayload = "H4sIAAAAAAAA/encoded-release"

	helmRelease := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "sh.helm.release.v1.my-app.v2",
			Namespace:         "namespace",
			UID:               "1a7d6c4e-0749-11e8-a2b8-000c29dea4f6",
			ResourceVersion:   "1220",
			CreationTimestamp: CreateTestTime(),
			Labels: map[string]string{
				"owner":   "helm",
				"name":    "my-app",
				"status":  "superseded",
				"version": "2",
			},
			Annotations: map[string]string{
				lastAppliedConfigAnnotation: `{"apiVersion":"v1","data":{"key":"value"},"kind":"ConfigMap"}`,
			},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "helm", Operation: metav1.ManagedFieldsOperationUpdate},
			},
		},
		Data: map[string]string{
			"release": releasePayload,
			"other":   "not-a-release",
		},
		BinaryData: map[string][]byte{"blob": []byte("binary")},
	}
	otherConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "not-helm",
			Namespace:       "namespace",
			UID:             "2b8e7d5f-0749-11e8-a2b8-000c29dea4f6",
			ResourceVersion: "1221",
		},
		Data: map[string]string{"key": "value"},
	}

	orchestrator.KubernetesResourceCache.Flush()
	t.Cleanup(orchestrator.KubernetesResourceCache.Flush)

	collector := NewHelmReleaseConfigMapCollector()

	config := CollectorTestConfig{
		Resources:                  []runtime.Object{helmRelease, otherConfigMap},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		// Helm releases are manifest-only: the processor yields a nil metadata body, which is never sent.
		ExpectedMetadataMessages: 1,
		ExpectedManifestMessages: 1,
		AssertionsFn: func(t *testing.T, _ *collectors.CollectorRunConfig, result *collectors.CollectorRunResult) {
			assert.Nil(t, result.Result.MetadataMessages[0])

			// Only Helm ConfigMaps reach the informer cache, trimmed like regular ConfigMaps.
			cached, err := collector.lister.List(labels.Everything())
			require.NoError(t, err)
			require.Len(t, cached, 1)
			assert.Equal(t, helmRelease.Data, cached[0].Data)
			assert.Equal(t, helmRelease.BinaryData, cached[0].BinaryData)
			assert.Nil(t, cached[0].ManagedFields)
			assert.Equal(t, "-", cached[0].Annotations[lastAppliedConfigAnnotation])

			collectorManifest, ok := result.Result.ManifestMessages[0].(*model.CollectorManifest)
			require.True(t, ok)
			require.Len(t, collectorManifest.Manifests, 1)

			manifest := collectorManifest.Manifests[0]
			assert.Equal(t, int32(orchestrator.K8sConfigMap), manifest.Type)
			assert.Equal(t, string(helmRelease.UID), manifest.Uid)
			assert.Equal(t, helmRelease.ResourceVersion, manifest.ResourceVersion)

			var parsed map[string]interface{}
			require.NoError(t, yaml.Unmarshal(manifest.Content, &parsed))
			assert.Equal(t, "ConfigMap", parsed["kind"])
			assert.Equal(t, "v1", parsed["apiVersion"])
			assert.Equal(t, map[string]interface{}{"release": releasePayload, "other": "not-a-release"}, parsed["data"])
			assert.Contains(t, parsed, "binaryData")

			metadata, ok := parsed["metadata"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "sh.helm.release.v1.my-app.v2", metadata["name"])
			assert.NotContains(t, metadata, "managedFields")
			annotations, ok := metadata["annotations"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "-", annotations[lastAppliedConfigAnnotation])
		},
	}

	RunCollectorTest(t, config, collector)
}
