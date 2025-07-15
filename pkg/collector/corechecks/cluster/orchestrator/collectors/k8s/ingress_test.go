//go:build kubeapiserver && orchestrator

package k8s

// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/atomic"
	v1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestIngressCollector(t *testing.T) {
	pathType1 := netv1.PathTypeImplementationSpecific

	ingress := &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "ingress",
			Namespace:       "namespace",
			Annotations:     map[string]string{"annotation": "my-annotation"},
			Labels:          map[string]string{"app": "my-app"},
			UID:             "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
			ResourceVersion: "1211",
		},
		Spec: netv1.IngressSpec{
			Rules: []netv1.IngressRule{
				{
					Host: "*.host.com",
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{Paths: []netv1.HTTPIngressPath{
							{
								Path:     "/*",
								PathType: &pathType1,
								Backend: netv1.IngressBackend{
									Service: &netv1.IngressServiceBackend{
										Name: "service",
										Port: netv1.ServiceBackendPort{
											Number: 443,
											Name:   "https",
										},
									},
								},
							},
						}},
					},
				},
			},
			DefaultBackend: &netv1.IngressBackend{
				Resource: &v1.TypedLocalObjectReference{
					APIGroup: pointer.Ptr("apiGroup"),
					Kind:     "kind",
					Name:     "name",
				},
			},
			TLS: []netv1.IngressTLS{
				{
					Hosts:      []string{"*.host.com"},
					SecretName: "secret",
				},
			},
			IngressClassName: pointer.Ptr("ingressClassName"),
		},
		Status: netv1.IngressStatus{
			LoadBalancer: netv1.IngressLoadBalancerStatus{
				Ingress: []netv1.IngressLoadBalancerIngress{
					{Hostname: "foo.us-east-1.elb.amazonaws.com"},
				},
			},
		},
	}
	client := fake.NewClientset(ingress)

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

	metadataAsTags := utils.GetMetadataAsTags(mockconfig.New(t))
	collector := NewIngressCollector(metadataAsTags)

	collector.Init(runCfg)

	// Start the informer factory
	stopCh := make(chan struct{})
	defer close(stopCh)
	informerFactory.Start(stopCh)

	// Wait for the informer to sync
	cache.WaitForCacheSync(stopCh, collector.Informer().HasSynced)

	// Run the collector
	result, err := collector.Run(runCfg)
	assert.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, 1, result.ResourcesListed)
	assert.Equal(t, 1, result.ResourcesProcessed)
	assert.Len(t, result.Result.MetadataMessages, 1)
	assert.Len(t, result.Result.ManifestMessages, 1)
	assert.IsType(t, &model.CollectorIngress{}, result.Result.MetadataMessages[0])
	assert.IsType(t, &model.CollectorManifest{}, result.Result.ManifestMessages[0])
}
