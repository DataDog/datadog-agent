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
	v1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
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

func TestDeploymentCollector(t *testing.T) {
	testIntOrStrPercent := intstr.FromString("1%")
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC)) // 1389744000
	testInt32 := int32(2)

	deployment := &v1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			UID:               types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
			Name:              "deploy",
			Namespace:         "namespace",
			CreationTimestamp: timestamp,
			Labels: map[string]string{
				"label": "foo",
			},
			Annotations: map[string]string{
				"annotation": "bar",
			},
			ResourceVersion: "1209",
		},
		Spec: v1.DeploymentSpec{
			MinReadySeconds:         600,
			ProgressDeadlineSeconds: &testInt32,
			Replicas:                &testInt32,
			RevisionHistoryLimit:    &testInt32,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-deploy",
				},
			},
			Strategy: v1.DeploymentStrategy{
				Type: v1.DeploymentStrategyType("RollingUpdate"),
				RollingUpdate: &v1.RollingUpdateDeployment{
					MaxSurge:       &testIntOrStrPercent,
					MaxUnavailable: &testIntOrStrPercent,
				},
			},
		},
		Status: v1.DeploymentStatus{
			AvailableReplicas:  2,
			ObservedGeneration: 3,
			ReadyReplicas:      2,
			Replicas:           2,
			UpdatedReplicas:    2,
			Conditions: []v1.DeploymentCondition{
				{
					Type:    v1.DeploymentAvailable,
					Status:  corev1.ConditionFalse,
					Reason:  "MinimumReplicasAvailable",
					Message: "Deployment has minimum availability.",
				},
				{
					Type:    v1.DeploymentProgressing,
					Status:  corev1.ConditionFalse,
					Reason:  "NewReplicaSetAvailable",
					Message: `ReplicaSet "orchestrator-intake-6d65b45d4d" has timed out progressing.`,
				},
			},
		},
	}
	client := fake.NewClientset(deployment)

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
	collector := NewDeploymentCollector(metadataAsTags)

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
	assert.IsType(t, &model.CollectorDeployment{}, result.Result.MetadataMessages[0])
	assert.IsType(t, &model.CollectorManifest{}, result.Result.ManifestMessages[0])
}
