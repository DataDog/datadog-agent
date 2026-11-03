// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.yaml.in/yaml/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	metafake "k8s.io/client-go/metadata/fake"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
)

const secretValue = "c3VwZXItc2VjcmV0" // base64("super-secret")

func newTestSecret() *metav1.PartialObjectMetadata {
	return &metav1.PartialObjectMetadata{
		// The fake metadata client derives the resource from the Kind.
		TypeMeta: metav1.TypeMeta{
			APIVersion: "v1",
			Kind:       "Secret",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "secret",
			Namespace:         "namespace",
			UID:               "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
			ResourceVersion:   "1220",
			CreationTimestamp: CreateTestTime(),
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation":                "my-annotation",
				lastAppliedConfigAnnotation: `{"apiVersion":"v1","data":{"password":"` + secretValue + `"},"kind":"Secret"}`,
			},
			ManagedFields: []metav1.ManagedFieldsEntry{
				{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply},
			},
		},
	}
}

func TestSecretCollector(t *testing.T) {
	secret := newTestSecret()
	collector := NewSecretCollector()

	config := CollectorTestConfig{
		MetadataResources:          []runtime.Object{secret},
		ExpectedResourcesListed:    1,
		ExpectedResourcesProcessed: 1,
		// Secret is manifest-only: the processor yields a nil metadata body, which is never sent.
		ExpectedMetadataMessages: 1,
		ExpectedManifestMessages: 1,
		AssertionsFn: func(t *testing.T, _ *collectors.CollectorRunConfig, result *collectors.CollectorRunResult) {
			assert.Nil(t, result.Result.MetadataMessages[0])

			// The transform deletes sensitive annotations before objects are stored in the informer cache.
			cached, err := collector.lister.Namespace("namespace").Get("secret")
			require.NoError(t, err)
			assert.Nil(t, cached.ManagedFields)
			assert.NotContains(t, cached.Annotations, lastAppliedConfigAnnotation)
			assert.Equal(t, "my-annotation", cached.Annotations["annotation"])
			assert.Equal(t, "my-app", cached.Labels["app"])

			require.Len(t, result.Result.ManifestMessages, 1)
			collectorManifest, ok := result.Result.ManifestMessages[0].(*model.CollectorManifest)
			require.True(t, ok)
			require.Len(t, collectorManifest.Manifests, 1)

			manifest := collectorManifest.Manifests[0]
			assert.Equal(t, string(secret.UID), manifest.Uid)
			assert.Equal(t, secret.ResourceVersion, manifest.ResourceVersion)
			assert.NotContains(t, string(manifest.Content), secretValue)

			var parsed map[string]interface{}
			require.NoError(t, yaml.Unmarshal(manifest.Content, &parsed))
			assert.Equal(t, "Secret", parsed["kind"])
			assert.Equal(t, "v1", parsed["apiVersion"])
			assert.NotContains(t, parsed, "data")
			assert.NotContains(t, parsed, "stringData")

			metadata, ok := parsed["metadata"].(map[string]interface{})
			require.True(t, ok)
			assert.Equal(t, "secret", metadata["name"])
			assert.NotContains(t, metadata, "managedFields")
			annotations, ok := metadata["annotations"].(map[string]interface{})
			require.True(t, ok)
			assert.NotContains(t, annotations, lastAppliedConfigAnnotation)
		},
	}

	RunCollectorTest(t, config, collector)
}

// TestSecretCollectorFailsClosed checks that the collector refuses to run when
// the cache transform cannot be registered, which happens when the shared
// informer was already started.
func TestSecretCollectorFailsClosed(t *testing.T) {
	metadataScheme := runtime.NewScheme()
	require.NoError(t, metav1.AddMetaToScheme(metadataScheme))
	metadataClient := metafake.NewSimpleMetadataClient(metadataScheme, newTestSecret())
	metadataInformerFactory := metadatainformer.NewSharedInformerFactory(metadataClient, 300*time.Second)

	// Start the informer before the collector gets to set its transform.
	stopCh := make(chan struct{})
	defer close(stopCh)
	informer := metadataInformerFactory.ForResource(secretGVR).Informer()
	metadataInformerFactory.Start(stopCh)
	require.True(t, cache.WaitForCacheSync(stopCh, informer.HasSynced))

	rcfg := &collectors.CollectorRunConfig{
		K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
			OrchestratorInformerFactory: &collectors.OrchestratorInformerFactory{
				SecretMetadataInformerFactory: metadataInformerFactory,
			},
		},
	}

	collector := NewSecretCollector()
	collector.Init(rcfg)

	result, err := collector.Run(rcfg)
	assert.Error(t, err)
	assert.Nil(t, result)

	result, err = collector.Process(rcfg, []*metav1.PartialObjectMetadata{newTestSecret()})
	assert.Error(t, err)
	assert.Nil(t, result)
}

func TestTrimSecretMetadata(t *testing.T) {
	secret := newTestSecret()
	secret.Annotations["consul.hashicorp.com/original-pod"] = "pod-spec"
	secret.Labels[lastAppliedConfigAnnotation] = "label-value"

	obj, err := trimSecretMetadata(secret)
	require.NoError(t, err)

	trimmed, ok := obj.(*metav1.PartialObjectMetadata)
	require.True(t, ok)
	assert.Nil(t, trimmed.ManagedFields)
	assert.Equal(t, map[string]string{"annotation": "my-annotation"}, trimmed.Annotations)
	assert.Equal(t, map[string]string{"app": "my-app"}, trimmed.Labels)
}
