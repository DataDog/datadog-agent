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
	v2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestHorizontalPodAutoscalerHandlers_ExtractResource(t *testing.T) {
	handlers := &HorizontalPodAutoscalerHandlers{}

	// Create test HPA
	hpa := createTestHorizontalPodAutoscaler("test-hpa", "test-namespace")

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
	resourceModel := handlers.ExtractResource(ctx, hpa)

	// Validate extraction
	hpaModel, ok := resourceModel.(*model.HorizontalPodAutoscaler)
	assert.True(t, ok)
	assert.NotNil(t, hpaModel)
	assert.Equal(t, "test-hpa", hpaModel.Metadata.Name)
	assert.Equal(t, "test-namespace", hpaModel.Metadata.Namespace)
	assert.NotNil(t, hpaModel.Spec)
	assert.NotNil(t, hpaModel.Status)
}

func TestHorizontalPodAutoscalerHandlers_ResourceList(t *testing.T) {
	handlers := &HorizontalPodAutoscalerHandlers{}

	// Create test HPAs
	hpa1 := createTestHorizontalPodAutoscaler("hpa-1", "namespace-1")
	hpa2 := createTestHorizontalPodAutoscaler("hpa-2", "namespace-2")

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
	resourceList := []*v2.HorizontalPodAutoscaler{hpa1, hpa2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*v2.HorizontalPodAutoscaler)
	assert.True(t, ok)
	assert.Equal(t, "hpa-1", resource1.Name)
	assert.NotSame(t, hpa1, resource1) // Should be a copy

	resource2, ok := resources[1].(*v2.HorizontalPodAutoscaler)
	assert.True(t, ok)
	assert.Equal(t, "hpa-2", resource2.Name)
	assert.NotSame(t, hpa2, resource2) // Should be a copy
}

func TestHorizontalPodAutoscalerHandlers_ResourceUID(t *testing.T) {
	handlers := &HorizontalPodAutoscalerHandlers{}

	hpa := createTestHorizontalPodAutoscaler("test-hpa", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	hpa.UID = expectedUID

	uid := handlers.ResourceUID(nil, hpa)
	assert.Equal(t, expectedUID, uid)
}

func TestHorizontalPodAutoscalerHandlers_ResourceVersion(t *testing.T) {
	handlers := &HorizontalPodAutoscalerHandlers{}

	hpa := createTestHorizontalPodAutoscaler("test-hpa", "test-namespace")
	expectedVersion := "v123"
	hpa.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.HorizontalPodAutoscaler{}

	version := handlers.ResourceVersion(nil, hpa, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestHorizontalPodAutoscalerHandlers_BuildMessageBody(t *testing.T) {
	handlers := &HorizontalPodAutoscalerHandlers{}

	hpa1 := createTestHorizontalPodAutoscaler("hpa-1", "namespace-1")
	hpa2 := createTestHorizontalPodAutoscaler("hpa-2", "namespace-2")

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

	hpa1Model := k8sTransformers.ExtractHorizontalPodAutoscaler(ctx, hpa1)
	hpa2Model := k8sTransformers.ExtractHorizontalPodAutoscaler(ctx, hpa2)

	// Build message body
	resourceModels := []interface{}{hpa1Model, hpa2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorHorizontalPodAutoscaler)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.HorizontalPodAutoscalers, 2)
	assert.Equal(t, "hpa-1", collectorMsg.HorizontalPodAutoscalers[0].Metadata.Name)
	assert.Equal(t, "hpa-2", collectorMsg.HorizontalPodAutoscalers[1].Metadata.Name)
}

func TestHorizontalPodAutoscalerHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &HorizontalPodAutoscalerHandlers{}

	hpa := createTestHorizontalPodAutoscaler("test-hpa", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "HorizontalPodAutoscaler",
			APIVersion:       "autoscaling/v2",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.HorizontalPodAutoscaler{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, hpa, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "HorizontalPodAutoscaler", hpa.Kind)
	assert.Equal(t, "autoscaling/v2", hpa.APIVersion)
}

func TestHorizontalPodAutoscalerHandlers_AfterMarshalling(t *testing.T) {
	handlers := &HorizontalPodAutoscalerHandlers{}

	hpa := createTestHorizontalPodAutoscaler("test-hpa", "test-namespace")
	resourceModel := &model.HorizontalPodAutoscaler{}

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

	// Test YAML
	testYAML := []byte(`{"apiVersion":"autoscaling/v2","kind":"HorizontalPodAutoscaler","metadata":{"name":"test"}}`)

	skip := handlers.AfterMarshalling(ctx, hpa, resourceModel, testYAML)
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestHorizontalPodAutoscalerHandlers_GetMetadataTags(t *testing.T) {
	handlers := &HorizontalPodAutoscalerHandlers{}

	// Create HPA model with tags
	hpaModel := &model.HorizontalPodAutoscaler{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, hpaModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestHorizontalPodAutoscalerHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &HorizontalPodAutoscalerHandlers{}

	// Create HPA with sensitive annotations and labels
	hpa := createTestHorizontalPodAutoscaler("test-hpa", "test-namespace")
	hpa.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	hpa.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, hpa)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", hpa.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", hpa.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestHorizontalPodAutoscalerProcessor_Process(t *testing.T) {
	// Create test HPAs with unique UIDs
	hpa1 := createTestHorizontalPodAutoscaler("hpa-1", "namespace-1")
	hpa1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	hpa1.ResourceVersion = "1208"

	hpa2 := createTestHorizontalPodAutoscaler("hpa-2", "namespace-2")
	hpa2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	hpa2.ResourceVersion = "1308"

	// Create fake client
	client := fake.NewClientset(hpa1, hpa2)
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
			NodeType:         orchestrator.K8sHorizontalPodAutoscaler,
			Kind:             "HorizontalPodAutoscaler",
			APIVersion:       "autoscaling/v2",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process HPAs
	processor := processors.NewProcessor(&HorizontalPodAutoscalerHandlers{})
	result, listed, processed := processor.Process(ctx, []*v2.HorizontalPodAutoscaler{hpa1, hpa2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorHorizontalPodAutoscaler)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.HorizontalPodAutoscalers, 2)

	expectedHpa1 := k8sTransformers.ExtractHorizontalPodAutoscaler(ctx, hpa1)

	assert.Equal(t, expectedHpa1.Metadata, metaMsg.HorizontalPodAutoscalers[0].Metadata)
	assert.Equal(t, expectedHpa1.Spec, metaMsg.HorizontalPodAutoscalers[0].Spec)
	assert.Equal(t, expectedHpa1.Status, metaMsg.HorizontalPodAutoscalers[0].Status)
	assert.Equal(t, expectedHpa1.Conditions, metaMsg.HorizontalPodAutoscalers[0].Conditions)
	assert.Equal(t, expectedHpa1.Tags, metaMsg.HorizontalPodAutoscalers[0].Tags)

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
	assert.Equal(t, hpa1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, hpa1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(23), manifest1.Type) // K8sHorizontalPodAutoscaler
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestHpa v2.HorizontalPodAutoscaler
	err := json.Unmarshal(manifest1.Content, &actualManifestHpa)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestHpa.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestHpa.ObjectMeta.CreationTimestamp.Time.UTC()}
	actualManifestHpa.Status.LastScaleTime = &metav1.Time{Time: actualManifestHpa.Status.LastScaleTime.Time.UTC()}
	actualManifestHpa.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: actualManifestHpa.Status.Conditions[0].LastTransitionTime.Time.UTC()}
	actualManifestHpa.Status.Conditions[1].LastTransitionTime = metav1.Time{Time: actualManifestHpa.Status.Conditions[1].LastTransitionTime.Time.UTC()}
	actualManifestHpa.Status.Conditions[2].LastTransitionTime = metav1.Time{Time: actualManifestHpa.Status.Conditions[2].LastTransitionTime.Time.UTC()}
	assert.Equal(t, hpa1.ObjectMeta, actualManifestHpa.ObjectMeta)
	assert.Equal(t, hpa1.Spec.String(), actualManifestHpa.Spec.String())
	assert.Equal(t, hpa1.Status.String(), actualManifestHpa.Status.String())
}

func createTestHorizontalPodAutoscaler(name, namespace string) *v2.HorizontalPodAutoscaler {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	minReplicas := int32(1)
	resourceQuantity := resource.MustParse("5332m")
	window := int32(10)
	selectPolicy := v2.MaxChangePolicySelect
	observedGeneration := int64(1)
	averageUtilization := int32(60)

	return &v2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Name:            name,
			Namespace:       namespace,
			ResourceVersion: "1208",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Spec: v2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: v2.CrossVersionObjectReference{
				Kind: "Deployment",
				Name: "agent",
			},
			MinReplicas: &minReplicas,
			MaxReplicas: 3,
			Metrics: []v2.MetricSpec{
				{
					Type: "Object",
					Object: &v2.ObjectMetricSource{
						DescribedObject: v2.CrossVersionObjectReference{
							Kind:       "Pod",
							Name:       "agent",
							APIVersion: "v1",
						},
						Target: v2.MetricTarget{
							Type:  v2.ValueMetricType,
							Value: &resourceQuantity,
						},
						Metric: v2.MetricIdentifier{
							Name: "CPU",
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"service": "datadog",
								},
							},
						},
					},
				},
				{
					Type: "Pods",
					Pods: &v2.PodsMetricSource{
						Metric: v2.MetricIdentifier{
							Name: "CPU",
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"service": "datadog",
								},
							},
						},
						Target: v2.MetricTarget{
							Type:               v2.UtilizationMetricType,
							AverageUtilization: &averageUtilization,
						},
					},
				},
				{
					Type: "Resource",
					Resource: &v2.ResourceMetricSource{
						Name: "CPU",
						Target: v2.MetricTarget{
							Type:               v2.UtilizationMetricType,
							AverageUtilization: &averageUtilization,
						},
					},
				},
				{
					Type: "ContainerResource",
					ContainerResource: &v2.ContainerResourceMetricSource{
						Name: "CPU",
						Target: v2.MetricTarget{
							Type:               v2.UtilizationMetricType,
							AverageUtilization: &averageUtilization,
						},
						Container: "agent",
					},
				},
				{
					Type: "External",
					External: &v2.ExternalMetricSource{
						Metric: v2.MetricIdentifier{
							Name: "CPU",
							Selector: &metav1.LabelSelector{
								MatchLabels: map[string]string{
									"service": "datadog",
								},
							},
						},
						Target: v2.MetricTarget{
							Type:               v2.UtilizationMetricType,
							AverageUtilization: &averageUtilization,
						},
					},
				},
			},
			Behavior: &v2.HorizontalPodAutoscalerBehavior{
				ScaleUp: &v2.HPAScalingRules{
					StabilizationWindowSeconds: &window,
					SelectPolicy:               &selectPolicy,
					Policies: []v2.HPAScalingPolicy{
						{
							Type:          v2.PodsScalingPolicy,
							Value:         4,
							PeriodSeconds: 60,
						},
					},
				},
			},
		},
		Status: v2.HorizontalPodAutoscalerStatus{
			ObservedGeneration: &observedGeneration,
			LastScaleTime:      &creationTime,
			CurrentReplicas:    2,
			DesiredReplicas:    2,
			CurrentMetrics: []v2.MetricStatus{
				{
					Type: "External",
					External: &v2.ExternalMetricStatus{
						Current: v2.MetricValueStatus{
							AverageValue: &resourceQuantity,
						},
						Metric: v2.MetricIdentifier{
							Name: "CPU",
						},
					},
				},
			},
			Conditions: []v2.HorizontalPodAutoscalerCondition{
				{
					Type:               v2.AbleToScale,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: creationTime,
					Reason:             "ReadyForNewScale",
					Message:            "recommended size matches current size",
				},
				{
					Type:               v2.ScalingActive,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: creationTime,
					Reason:             "ValidMetricFound",
					Message:            "the HPA was able to successfully calculate a replica count from external metric",
				},
				{
					Type:               v2.ScalingLimited,
					Status:             corev1.ConditionFalse,
					LastTransitionTime: creationTime,
					Reason:             "DesiredWithinRange",
					Message:            "the desired count is within the acceptable range",
				},
			},
		},
	}
}
