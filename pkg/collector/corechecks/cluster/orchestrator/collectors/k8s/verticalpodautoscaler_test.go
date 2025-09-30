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
	autoscaling "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	vpafake "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/clientset/versioned/fake"
	vpai "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/client/informers/externalversions"
	"k8s.io/client-go/tools/cache"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/collectors"
	mockconfig "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/config/utils"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestVerticalPodAutoscalerCollector(t *testing.T) {
	exampleTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	mode := v1.ContainerScalingModeAuto
	updateMode := v1.UpdateModeOff
	controlledValues := v1.ContainerControlledValuesRequestsAndLimits

	verticalPodAutoscaler := &v1.VerticalPodAutoscaler{
		TypeMeta: metav1.TypeMeta{},
		ObjectMeta: metav1.ObjectMeta{
			Name:              "VPATest",
			Namespace:         "Namespace",
			UID:               "326331f4-77e2-11ed-a1eb-0242ac120002",
			ResourceVersion:   "1227",
			CreationTimestamp: exampleTime,
			DeletionTimestamp: &exampleTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			Finalizers: []string{"final", "izers"},
		},
		Spec: v1.VerticalPodAutoscalerSpec{
			TargetRef: &autoscaling.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: "My Service",
			},
			UpdatePolicy: &v1.PodUpdatePolicy{
				UpdateMode: &updateMode,
			},
			ResourcePolicy: &v1.PodResourcePolicy{
				ContainerPolicies: []v1.ContainerResourcePolicy{
					{
						ContainerName: "TestContainer",
						Mode:          &mode,
						MinAllowed: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("12345"),
						},
						MaxAllowed: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceMemory: resource.MustParse("6789"),
						},
						ControlledResources: &[]corev1.ResourceName{
							corev1.ResourceCPU,
						},
						ControlledValues: &controlledValues,
					},
				},
			},
			Recommenders: []*v1.VerticalPodAutoscalerRecommenderSelector{
				{
					Name: "Test",
				},
			},
		},
		Status: v1.VerticalPodAutoscalerStatus{
			Recommendation: &v1.RecommendedPodResources{
				ContainerRecommendations: []v1.RecommendedContainerResources{
					{
						ContainerName: "TestContainer",
						Target: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("2"),
						},
						LowerBound: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("1"),
						},
						UpperBound: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("3"),
						},
						UncappedTarget: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("4"),
						},
					},
				},
			},
			Conditions: []v1.VerticalPodAutoscalerCondition{
				{
					Type:               v1.RecommendationProvided,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: exampleTime,
				},
				{
					Type:               v1.NoPodsMatched,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: exampleTime,
					Reason:             "NoPodsMatched",
					Message:            "No pods match this VPA object",
				},
			},
		},
	}

	client := vpafake.NewSimpleClientset(verticalPodAutoscaler)

	// Create fake VPA informer factory
	vpaInformerFactory := vpai.NewSharedInformerFactory(client, 300*time.Second)

	orchestratorInformerFactory := &collectors.OrchestratorInformerFactory{
		VPAInformerFactory: vpaInformerFactory,
	}

	apiClient := &apiserver.APIClient{VPAInformerClient: client}

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
	collector := NewVerticalPodAutoscalerCollector(metadataAsTags)

	collector.Init(runCfg)

	// Start the informer factories
	stopCh := make(chan struct{})
	defer close(stopCh)
	// informerFactory.Start(stopCh)
	vpaInformerFactory.Start(stopCh)

	// Wait for the informers to sync
	cache.WaitForCacheSync(stopCh, collector.Informer().HasSynced)

	// Run the collector
	result, err := collector.Run(runCfg)
	assert.NoError(t, err)
	assert.NotNil(t, result)

	assert.Equal(t, 1, result.ResourcesListed)
	assert.Equal(t, 1, result.ResourcesProcessed)

	assert.Len(t, result.Result.MetadataMessages, 1)
	assert.Len(t, result.Result.ManifestMessages, 1)
	assert.IsType(t, &model.CollectorVerticalPodAutoscaler{}, result.Result.MetadataMessages[0])
	assert.IsType(t, &model.CollectorManifest{}, result.Result.ManifestMessages[0])
}
