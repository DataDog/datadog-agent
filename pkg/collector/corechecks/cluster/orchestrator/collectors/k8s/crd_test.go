// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && orchestrator

package k8s

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	v1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiextensionsfake "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/fake"
	apiextensionsinformers "k8s.io/apiextensions-apiserver/pkg/client/informers/externalversions"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestCRDCollector(t *testing.T) {
	customResourceDefinition := &v1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "mycustomresources.example.com",
			UID:             "test-crd-uid-123",
			ResourceVersion: "1205",
			CreationTimestamp: metav1.Time{
				Time: time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC),
			},
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
		},
		Spec: v1.CustomResourceDefinitionSpec{
			Group: "example.com",
			Names: v1.CustomResourceDefinitionNames{
				Kind:     "MyCustomResource",
				ListKind: "MyCustomResourceList",
				Plural:   "mycustomresources",
				Singular: "mycustomresource",
			},
			Scope: v1.NamespaceScoped,
			Versions: []v1.CustomResourceDefinitionVersion{
				{
					Name:    "v1",
					Served:  true,
					Storage: true,
					Schema: &v1.CustomResourceValidation{
						OpenAPIV3Schema: &v1.JSONSchemaProps{
							Type: "object",
							Properties: map[string]v1.JSONSchemaProps{
								"spec": {
									Type: "object",
									Properties: map[string]v1.JSONSchemaProps{
										"replicas": {Type: "integer"},
										"image":    {Type: "string"},
									},
								},
							},
						},
					},
				},
			},
		},
		Status: v1.CustomResourceDefinitionStatus{
			Conditions: []v1.CustomResourceDefinitionCondition{
				{
					Type:   v1.Established,
					Status: v1.ConditionTrue,
				},
			},
		},
	}

	// Create a fake API extensions client
	apiextensionsClient := apiextensionsfake.NewSimpleClientset(customResourceDefinition)

	// Create fake API extensions informer factory
	apiextensionsInformerFactory := apiextensionsinformers.NewSharedInformerFactory(apiextensionsClient, 300*time.Second)

	// Create OrchestratorInformerFactory with API extensions informers
	orchestratorInformerFactory := &collectors.OrchestratorInformerFactory{
		CRDInformerFactory: apiextensionsInformerFactory,
	}

	apiClient := &apiserver.APIClient{}

	// Orchestrator configuration
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

	// Create CRD collector
	collector := NewCRDCollector()

	// Initialize the collector
	collector.Init(runCfg)

	// Start the informer factory
	stopCh := make(chan struct{})
	defer close(stopCh)
	apiextensionsInformerFactory.Start(stopCh)

	// Wait for the informer to sync
	cache.WaitForCacheSync(stopCh, collector.Informer().HasSynced)

	// Run the collector
	result, err := collector.Run(runCfg)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.ResourcesListed)
	assert.Equal(t, 1, result.ResourcesProcessed)
	// CRDs produce manifest messages but metadata messages are nil
	assert.Len(t, result.Result.MetadataMessages, 1)
	assert.Nil(t, result.Result.MetadataMessages[0])
	assert.Len(t, result.Result.ManifestMessages, 1)
	assert.IsType(t, &model.CollectorManifestCRD{}, result.Result.ManifestMessages[0])
}
