// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

package k8s

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"

	model "github.com/DataDog/agent-payload/v5/process"

	autoscaling "k8s.io/api/autoscaling/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	v1 "k8s.io/autoscaler/vertical-pod-autoscaler/pkg/apis/autoscaling.k8s.io/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestVerticalPodAutoscalerHandlers_ExtractResource(t *testing.T) {
	handlers := &VerticalPodAutoscalerHandlers{}

	// Create test verticalpodautoscaler
	vpa := createTestVerticalPodAutoscaler()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Extract resource
	resourceModel := handlers.ExtractResource(ctx, vpa)

	// Validate extraction
	vpaModel, ok := resourceModel.(*model.VerticalPodAutoscaler)
	assert.True(t, ok)
	assert.NotNil(t, vpaModel)
	assert.Equal(t, "test-vpa", vpaModel.Metadata.Name)
	assert.Equal(t, "default", vpaModel.Metadata.Namespace)
	assert.Equal(t, "Deployment", vpaModel.Spec.Target.Kind)
	assert.Equal(t, "test-deployment", vpaModel.Spec.Target.Name)
	assert.Equal(t, "Off", vpaModel.Spec.UpdateMode)
	assert.Len(t, vpaModel.Spec.ResourcePolicies, 1)
	assert.Equal(t, "test-container", vpaModel.Spec.ResourcePolicies[0].ContainerName)
	assert.Equal(t, "Auto", vpaModel.Spec.ResourcePolicies[0].Mode)
}

func TestVerticalPodAutoscalerHandlers_ResourceList(t *testing.T) {
	handlers := &VerticalPodAutoscalerHandlers{}

	// Create test verticalpodautoscalers
	vpa1 := createTestVerticalPodAutoscaler()
	vpa2 := createTestVerticalPodAutoscaler()
	vpa2.Name = "vpa2"
	vpa2.UID = "uid2"

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Convert list
	resourceList := []*v1.VerticalPodAutoscaler{vpa1, vpa2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*v1.VerticalPodAutoscaler)
	assert.True(t, ok)
	assert.Equal(t, "test-vpa", resource1.Name)
	assert.NotSame(t, vpa1, resource1) // Should be a copy

	resource2, ok := resources[1].(*v1.VerticalPodAutoscaler)
	assert.True(t, ok)
	assert.Equal(t, "vpa2", resource2.Name)
	assert.NotSame(t, vpa2, resource2) // Should be a copy
}

func TestVerticalPodAutoscalerHandlers_ResourceUID(t *testing.T) {
	handlers := &VerticalPodAutoscalerHandlers{}

	vpa := createTestVerticalPodAutoscaler()
	expectedUID := types.UID("test-vpa-uid")
	vpa.UID = expectedUID

	uid := handlers.ResourceUID(nil, vpa)
	assert.Equal(t, expectedUID, uid)
}

func TestVerticalPodAutoscalerHandlers_ResourceVersion(t *testing.T) {
	handlers := &VerticalPodAutoscalerHandlers{}

	vpa := createTestVerticalPodAutoscaler()
	expectedVersion := "123"
	vpa.ResourceVersion = expectedVersion

	version := handlers.ResourceVersion(nil, vpa, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestVerticalPodAutoscalerHandlers_BuildMessageBody(t *testing.T) {
	handlers := &VerticalPodAutoscalerHandlers{}

	vpa1 := createTestVerticalPodAutoscaler()
	vpa2 := createTestVerticalPodAutoscaler()
	vpa2.Name = "vpa2"

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	vpa1Model := k8sTransformers.ExtractVerticalPodAutoscaler(ctx, vpa1)
	vpa2Model := k8sTransformers.ExtractVerticalPodAutoscaler(ctx, vpa2)

	// Build message body
	resourceModels := []interface{}{vpa1Model, vpa2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorVerticalPodAutoscaler)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.VerticalPodAutoscalers, 2)
	assert.Equal(t, "test-vpa", collectorMsg.VerticalPodAutoscalers[0].Metadata.Name)
	assert.Equal(t, "vpa2", collectorMsg.VerticalPodAutoscalers[1].Metadata.Name)
}

func TestVerticalPodAutoscalerHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &VerticalPodAutoscalerHandlers{}

	vpa := createTestVerticalPodAutoscaler()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "VerticalPodAutoscaler",
			APIVersion:       "autoscaling.k8s.io/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	resourceModel := &model.VerticalPodAutoscaler{}
	skip := handlers.BeforeMarshalling(ctx, vpa, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "VerticalPodAutoscaler", vpa.Kind)
	assert.Equal(t, "autoscaling.k8s.io/v1", vpa.APIVersion)
}

func TestVerticalPodAutoscalerHandlers_AfterMarshalling(t *testing.T) {
	handlers := &VerticalPodAutoscalerHandlers{}

	vpa := createTestVerticalPodAutoscaler()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.VerticalPodAutoscaler{}

	// Create test YAML
	testYAML := []byte("apiVersion: autoscaling.k8s.io/v1\nkind: VerticalPodAutoscaler\nmetadata:\n  name: test-vpa")

	// Call AfterMarshalling
	skip := handlers.AfterMarshalling(ctx, vpa, resourceModel, testYAML)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestVerticalPodAutoscalerHandlers_GetMetadataTags(t *testing.T) {
	handlers := &VerticalPodAutoscalerHandlers{}

	// Create a vpa model with tags
	vpaModel := &model.VerticalPodAutoscaler{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	tags := handlers.GetMetadataTags(nil, vpaModel)
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestVerticalPodAutoscalerHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &VerticalPodAutoscalerHandlers{}

	// Create vpa with sensitive annotations and labels
	vpa := createTestVerticalPodAutoscaler()
	vpa.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	vpa.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, vpa)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", vpa.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", vpa.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestVerticalPodAutoscalerProcessor_Process(t *testing.T) {
	// Create test vpas with unique UIDs
	vpa1 := createTestVerticalPodAutoscaler()
	vpa1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	vpa1.ResourceVersion = "1226"

	vpa2 := createTestVerticalPodAutoscaler()
	vpa2.Name = "vpa2"
	vpa2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	vpa2.ResourceVersion = "1326"

	// Create fake client
	client := fake.NewClientset()
	apiClient := &apiserver.APIClient{Cl: client}

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"

	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			NodeType:         orchestrator.K8sVerticalPodAutoscaler,
			Kind:             "VerticalPodAutoscaler",
			APIVersion:       "autoscaling.k8s.io/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process vpas
	processor := processors.NewProcessor(&VerticalPodAutoscalerHandlers{})
	result, listed, processed := processor.Process(ctx, []*v1.VerticalPodAutoscaler{vpa1, vpa2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorVerticalPodAutoscaler)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.VerticalPodAutoscalers, 2)

	expectedVPA1 := k8sTransformers.ExtractVerticalPodAutoscaler(ctx, vpa1)

	assert.Equal(t, expectedVPA1.Metadata, metaMsg.VerticalPodAutoscalers[0].Metadata)
	assert.Equal(t, expectedVPA1.Spec, metaMsg.VerticalPodAutoscalers[0].Spec)
	assert.Equal(t, expectedVPA1.Status, metaMsg.VerticalPodAutoscalers[0].Status)
	assert.Equal(t, expectedVPA1.Conditions, metaMsg.VerticalPodAutoscalers[0].Conditions)
	assert.Equal(t, expectedVPA1.Tags, metaMsg.VerticalPodAutoscalers[0].Tags)

	// Validate manifest message
	manifestMsg, ok := result.ManifestMessages[0].(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", manifestMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", manifestMsg.ClusterId)
	assert.Equal(t, int32(1), manifestMsg.GroupId)
	assert.Equal(t, "test-host", manifestMsg.HostName)
	assert.Len(t, manifestMsg.Manifests, 2)
	assert.Equal(t, manifestMsg.OriginCollector, model.OriginCollector_datadogAgent)

	// Validate manifest details
	manifest1 := manifestMsg.Manifests[0]
	assert.Equal(t, vpa1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, vpa1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(22), manifest1.Type) // K8sVerticalPodAutoscaler
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestVPA v1.VerticalPodAutoscaler
	err := json.Unmarshal(manifest1.Content, &actualManifestVPA)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestVPA.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestVPA.ObjectMeta.CreationTimestamp.Time.UTC()}
	if actualManifestVPA.ObjectMeta.DeletionTimestamp != nil {
		actualManifestVPA.ObjectMeta.DeletionTimestamp = &metav1.Time{Time: actualManifestVPA.ObjectMeta.DeletionTimestamp.Time.UTC()}
	}
	if len(actualManifestVPA.Status.Conditions) > 0 {
		actualManifestVPA.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: actualManifestVPA.Status.Conditions[0].LastTransitionTime.Time.UTC()}
	}
	assert.Equal(t, vpa1.ObjectMeta, actualManifestVPA.ObjectMeta)
	assert.Equal(t, vpa1.Spec, actualManifestVPA.Spec)
	assert.Equal(t, vpa1.Status, actualManifestVPA.Status)
}

// Helper function to create a test verticalpodautoscaler
func createTestVerticalPodAutoscaler() *v1.VerticalPodAutoscaler {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	mode := v1.ContainerScalingModeAuto
	updateMode := v1.UpdateModeOff
	controlledValues := v1.ContainerControlledValuesRequestsAndLimits

	return &v1.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-vpa",
			Namespace:       "default",
			UID:             "test-vpa-uid",
			ResourceVersion: "1226",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
			CreationTimestamp: creationTime,
		},
		Spec: v1.VerticalPodAutoscalerSpec{
			TargetRef: &autoscaling.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: "test-deployment",
			},
			UpdatePolicy: &v1.PodUpdatePolicy{
				UpdateMode: &updateMode,
			},
			ResourcePolicy: &v1.PodResourcePolicy{
				ContainerPolicies: []v1.ContainerResourcePolicy{
					{
						ContainerName: "test-container",
						Mode:          &mode,
						MinAllowed: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("100m"),
						},
						MaxAllowed: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
						ControlledResources: &[]corev1.ResourceName{
							corev1.ResourceCPU,
						},
						ControlledValues: &controlledValues,
					},
				},
			},
		},
		Status: v1.VerticalPodAutoscalerStatus{
			Recommendation: &v1.RecommendedPodResources{
				ContainerRecommendations: []v1.RecommendedContainerResources{
					{
						ContainerName: "test-container",
						Target: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("200m"),
						},
						LowerBound: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("150m"),
						},
						UpperBound: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("250m"),
						},
						UncappedTarget: map[corev1.ResourceName]resource.Quantity{
							corev1.ResourceCPU: resource.MustParse("300m"),
						},
					},
				},
			},
			Conditions: []v1.VerticalPodAutoscalerCondition{
				{
					Type:               v1.RecommendationProvided,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: creationTime,
				},
			},
		},
	}
}
