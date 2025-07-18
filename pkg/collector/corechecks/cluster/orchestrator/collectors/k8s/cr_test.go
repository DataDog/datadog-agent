// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic/dynamicinformer"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestCRCollector(t *testing.T) {
	// Create a custom resource instance
	customResource := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "example.com/v1",
			"kind":       "MyCustomResource",
			"metadata": map[string]interface{}{
				"name":              "test-cr",
				"namespace":         "default",
				"uid":               "test-uid-123",
				"resourceVersion":   "1204",
				"creationTimestamp": metav1.Now().Format(time.RFC3339),
				"labels": map[string]interface{}{
					"app": "my-app",
				},
				"annotations": map[string]interface{}{
					"annotation": "my-annotation",
				},
			},
			"spec": map[string]interface{}{
				"replicas": float64(3),
				"image":    "nginx:latest",
			},
			"status": map[string]interface{}{
				"ready": true,
			},
		},
	}

	// Create a fake dynamic client
	scheme := runtime.NewScheme()
	dynamicClient := fake.NewSimpleDynamicClient(scheme, customResource)

	// Create fake dynamic informer factory
	dynamicInformerFactory := dynamicinformer.NewDynamicSharedInformerFactory(dynamicClient, 300*time.Second)

	// Create OrchestratorInformerFactory with dynamic informers
	orchestratorInformerFactory := &collectors.OrchestratorInformerFactory{
		DynamicInformerFactory: dynamicInformerFactory,
	}

	apiClient := &apiserver.APIClient{DynamicCl: dynamicClient}

	orchestratorCfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	orchestratorCfg.KubeClusterName = "test-cluster"

	runCfg := &collectors.CollectorRunConfig{
		K8sCollectorRunConfig: collectors.K8sCollectorRunConfig{
			APIClient:                   apiClient,
			OrchestratorInformerFactory: orchestratorInformerFactory,
		},
		ClusterID:   "test-cluster",
		Config:      orchestratorCfg,
		MsgGroupRef: atomic.NewInt32(0),
	}

	// Create CR collector
	collector, err := NewCRCollector("mycustomresources", "example.com/v1")
	assert.NoError(t, err)

	// Initialize the collector
	collector.Init(runCfg)

	// Start the informer factory
	stopCh := make(chan struct{})
	defer close(stopCh)
	dynamicInformerFactory.Start(stopCh)

	// Wait for the informer to sync
	cache.WaitForCacheSync(stopCh, collector.Informer().HasSynced)

	// Run the collector
	result, err := collector.Run(runCfg)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.ResourcesListed)
	assert.Equal(t, 1, result.ResourcesProcessed)
	// CRs produce manifest messages but metadata messages are nil
	assert.Nil(t, result.Result.MetadataMessages[0])
	assert.Len(t, result.Result.ManifestMessages, 1)
	assert.IsType(t, &model.CollectorManifestCR{}, result.Result.ManifestMessages[0])

}
