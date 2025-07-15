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
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes/fake"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestPodDisruptionBudgetCollector(t *testing.T) {
	iVal := int32(95)
	sVal := "reshape"
	iOSI := intstr.FromInt32(iVal)
	iOSS := intstr.FromString(sVal)
	var labels = map[string]string{"reshape": "all"}
	ePolicy := policyv1.AlwaysAllow
	t0 := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	t1 := metav1.NewTime(time.Date(2021, time.April, 16, 14, 31, 0, 0, time.UTC))

	podDisruptionBudget := &policyv1.PodDisruptionBudget{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "gwern",
			Namespace:       "kog",
			UID:             "e42e5adc-0749-11e8-a2b8-000c29dea4f6",
			ResourceVersion: "1219",
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
		},
		Spec: policyv1.PodDisruptionBudgetSpec{
			MinAvailable:   &iOSI,
			MaxUnavailable: &iOSS,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			UnhealthyPodEvictionPolicy: &ePolicy,
		},
		Status: policyv1.PodDisruptionBudgetStatus{
			ObservedGeneration: 3,
			DisruptedPods:      map[string]metav1.Time{"liborio": t0},
			DisruptionsAllowed: 4,
			CurrentHealthy:     5,
			DesiredHealthy:     6,
			ExpectedPods:       7,
			Conditions: []metav1.Condition{
				{
					Type:               "regular",
					Status:             metav1.ConditionUnknown,
					ObservedGeneration: 2,
					LastTransitionTime: t1,
					Reason:             "why not",
					Message:            "instant",
				},
			},
		},
	}
	client := fake.NewClientset(podDisruptionBudget)

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
	collector := NewPodDisruptionBudgetCollectorVersion(metadataAsTags)

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
	assert.IsType(t, &model.CollectorPodDisruptionBudget{}, result.Result.MetadataMessages[0])
	assert.IsType(t, &model.CollectorManifest{}, result.Result.ManifestMessages[0])
}
