// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build kubeapiserver && orchestrator && test

package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// CollectorTestConfig holds the configuration for running a collector test
type CollectorTestConfig struct {
	// Resources to create in the fake client
	Resources []runtime.Object
	// Expected metadata message type
	ExpectedMetadataType interface{}
	// Expected number of resources
	ExpectedResourcesListed int
	// Expected number of processed resources
	ExpectedResourcesProcessed int
	// Expected number of metadata messages
	ExpectedMetadataMessages int
	// Expected number of manifest messages
	ExpectedManifestMessages int
	// Additional setup function to run before the test
	SetupFn func(*collectors.CollectorRunConfig)
	// Additional assertions to run after the test
	AssertionsFn func(*testing.T, *collectors.CollectorRunConfig, *collectors.CollectorRunResult)
}

// RunCollectorTest is a generic test function that can be used by all collector tests
func RunCollectorTest(t *testing.T, config CollectorTestConfig, collector collectors.Collector) {
	// Create fake client
	client := fake.NewClientset(config.Resources...)

	// Create fake informer factory
	informerFactory := informers.NewSharedInformerFactoryWithOptions(client, 300*time.Second)

	// Create OrchestratorInformerFactory with fake informers
	orchestratorInformerFactory := &collectors.OrchestratorInformerFactory{
		InformerFactory: informerFactory,
	}

	apiClient := &apiserver.APIClient{Cl: client}

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

	// Run additional setup if provided
	if config.SetupFn != nil {
		config.SetupFn(runCfg)
	}

	// Initialize the collector
	collector.Init(runCfg)

	// Start the informer factory
	stopCh := make(chan struct{})
	defer close(stopCh)
	informerFactory.Start(stopCh)

	// Wait for the informer to sync
	if k8sCollector, ok := collector.(collectors.K8sCollector); ok {
		cache.WaitForCacheSync(stopCh, k8sCollector.Informer().HasSynced)
	}

	// Run the collector
	result, err := collector.Run(runCfg)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	// Assertions
	assert.Equal(t, config.ExpectedResourcesListed, result.ResourcesListed)
	assert.Equal(t, config.ExpectedResourcesProcessed, result.ResourcesProcessed)
	assert.Len(t, result.Result.MetadataMessages, config.ExpectedMetadataMessages)
	assert.Len(t, result.Result.ManifestMessages, config.ExpectedManifestMessages)

	if config.ExpectedMetadataType != nil && len(result.Result.MetadataMessages) > 0 {
		assert.IsType(t, config.ExpectedMetadataType, result.Result.MetadataMessages[0])
	}
	if len(result.Result.ManifestMessages) > 0 {
		assert.IsType(t, &model.CollectorManifest{}, result.Result.ManifestMessages[0])
	}

	// Run additional assertions if provided
	if config.AssertionsFn != nil {
		config.AssertionsFn(t, runCfg, result)
	}
}

// CreateTestTime returns a consistent test time for all tests
func CreateTestTime() metav1.Time {
	return metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
}
