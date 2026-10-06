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
	"go.uber.org/atomic"
	"go.yaml.in/yaml/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/fake"
	metafake "k8s.io/client-go/metadata/fake"
	"k8s.io/client-go/metadata/metadatainformer"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

const lastAppliedConfigAnnotation = "kubectl.kubernetes.io/last-applied-configuration"

func TestConfigMapCollector(t *testing.T) {
	configMap := &metav1.PartialObjectMetadata{
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

	scheme := runtime.NewScheme()
	require.NoError(t, metav1.AddMetaToScheme(scheme))
	metadataClient := metafake.NewSimpleMetadataClient(scheme, configMap)
	metadataInformerFactory := metadatainformer.NewSharedInformerFactory(metadataClient, 300*time.Second)

	orchestratorCfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	orchestratorCfg.KubeClusterName = "test-cluster"

	runCfg := &collectors.CollectorRunConfig{
		K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
			APIClient: &apiserver.APIClient{Cl: fake.NewClientset()},
			OrchestratorInformerFactory: &collectors.OrchestratorInformerFactory{
				MetadataInformerFactory: metadataInformerFactory,
			},
		},
		ClusterID:   "test-cluster",
		Config:      orchestratorCfg,
		MsgGroupRef: atomic.NewInt32(0),
	}

	collector := NewConfigMapCollector()
	collector.Init(runCfg)

	stopCh := make(chan struct{})
	defer close(stopCh)
	metadataInformerFactory.Start(stopCh)
	require.True(t, cache.WaitForCacheSync(stopCh, collector.Informer().HasSynced))

	// The transform trims objects before they are stored in the informer cache.
	cached, err := collector.lister.Namespace("namespace").Get("configmap")
	require.NoError(t, err)
	assert.Nil(t, cached.ManagedFields)
	assert.Equal(t, "-", cached.Annotations[lastAppliedConfigAnnotation])
	assert.Equal(t, "my-annotation", cached.Annotations["annotation"])

	result, err := collector.Run(runCfg)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.Equal(t, 1, result.ResourcesListed)
	assert.Equal(t, 1, result.ResourcesProcessed)
	require.Len(t, result.Result.ManifestMessages, 1)

	collectorManifest, ok := result.Result.ManifestMessages[0].(*model.CollectorManifest)
	require.True(t, ok)
	require.Len(t, collectorManifest.Manifests, 1)

	manifest := collectorManifest.Manifests[0]
	assert.Equal(t, string(configMap.UID), manifest.Uid)
	assert.Equal(t, configMap.ResourceVersion, manifest.ResourceVersion)

	var parsed map[string]interface{}
	require.NoError(t, yaml.Unmarshal(manifest.Content, &parsed))
	assert.Equal(t, "ConfigMap", parsed["kind"])
	assert.Equal(t, "v1", parsed["apiVersion"])
	assert.NotContains(t, parsed, "data")
	assert.NotContains(t, parsed, "binaryData")

	metadata, ok := parsed["metadata"].(map[string]interface{})
	require.True(t, ok)
	assert.Equal(t, "configmap", metadata["name"])
	assert.NotContains(t, metadata, "managedFields")
}
