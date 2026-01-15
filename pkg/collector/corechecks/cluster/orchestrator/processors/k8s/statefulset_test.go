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
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
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

func TestStatefulSetHandlers_ExtractResource(t *testing.T) {
	handlers := &StatefulSetHandlers{}

	// Create test statefulset
	statefulSet := createTestStatefulSet()

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
	resourceModel := handlers.ExtractResource(ctx, statefulSet)

	// Validate extraction
	statefulSetModel, ok := resourceModel.(*model.StatefulSet)
	assert.True(t, ok)
	assert.NotNil(t, statefulSetModel)
	assert.Equal(t, "test-statefulset", statefulSetModel.Metadata.Name)
	assert.Equal(t, "default", statefulSetModel.Metadata.Namespace)
	assert.Equal(t, "test-service", statefulSetModel.Spec.ServiceName)
	assert.Equal(t, int32(3), statefulSetModel.Spec.DesiredReplicas)
	assert.Equal(t, "RollingUpdate", statefulSetModel.Spec.UpdateStrategy)
	assert.Equal(t, int32(2), statefulSetModel.Spec.Partition)
	assert.Equal(t, int32(3), statefulSetModel.Status.Replicas)
	assert.Equal(t, int32(2), statefulSetModel.Status.ReadyReplicas)
}

func TestStatefulSetHandlers_ResourceList(t *testing.T) {
	handlers := &StatefulSetHandlers{}

	// Create test statefulsets
	statefulSet1 := createTestStatefulSet()
	statefulSet2 := createTestStatefulSet()
	statefulSet2.Name = "statefulset2"
	statefulSet2.UID = "uid2"

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
	resourceList := []*appsv1.StatefulSet{statefulSet1, statefulSet2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*appsv1.StatefulSet)
	assert.True(t, ok)
	assert.Equal(t, "test-statefulset", resource1.Name)
	assert.NotSame(t, statefulSet1, resource1) // Should be a copy

	resource2, ok := resources[1].(*appsv1.StatefulSet)
	assert.True(t, ok)
	assert.Equal(t, "statefulset2", resource2.Name)
	assert.NotSame(t, statefulSet2, resource2) // Should be a copy
}

func TestStatefulSetHandlers_ResourceUID(t *testing.T) {
	handlers := &StatefulSetHandlers{}

	statefulSet := createTestStatefulSet()
	expectedUID := types.UID("test-statefulset-uid")
	statefulSet.UID = expectedUID

	uid := handlers.ResourceUID(nil, statefulSet)
	assert.Equal(t, expectedUID, uid)
}

func TestStatefulSetHandlers_ResourceVersion(t *testing.T) {
	handlers := &StatefulSetHandlers{}

	statefulSet := createTestStatefulSet()
	expectedVersion := "123"
	statefulSet.ResourceVersion = expectedVersion

	version := handlers.ResourceVersion(nil, statefulSet, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestStatefulSetHandlers_BuildMessageBody(t *testing.T) {
	handlers := &StatefulSetHandlers{}

	statefulSet1 := createTestStatefulSet()
	statefulSet2 := createTestStatefulSet()
	statefulSet2.Name = "statefulset2"

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

	statefulSet1Model := k8sTransformers.ExtractStatefulSet(ctx, statefulSet1)
	statefulSet2Model := k8sTransformers.ExtractStatefulSet(ctx, statefulSet2)

	// Build message body
	resourceModels := []interface{}{statefulSet1Model, statefulSet2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorStatefulSet)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.StatefulSets, 2)
	assert.Equal(t, "test-statefulset", collectorMsg.StatefulSets[0].Metadata.Name)
	assert.Equal(t, "statefulset2", collectorMsg.StatefulSets[1].Metadata.Name)
}

func TestStatefulSetHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &StatefulSetHandlers{}

	statefulSet := createTestStatefulSet()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "StatefulSet",
			APIVersion:       "apps/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	resourceModel := &model.StatefulSet{}
	skip := handlers.BeforeMarshalling(ctx, statefulSet, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "StatefulSet", statefulSet.Kind)
	assert.Equal(t, "apps/v1", statefulSet.APIVersion)
}

func TestStatefulSetHandlers_AfterMarshalling(t *testing.T) {
	handlers := &StatefulSetHandlers{}

	statefulSet := createTestStatefulSet()

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
	resourceModel := &model.StatefulSet{}

	// Create test YAML
	testYAML := []byte("apiVersion: apps/v1\nkind: StatefulSet\nmetadata:\n  name: test-statefulset")

	// Call AfterMarshalling
	skip := handlers.AfterMarshalling(ctx, statefulSet, resourceModel, testYAML)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestStatefulSetHandlers_GetMetadataTags(t *testing.T) {
	handlers := &StatefulSetHandlers{}

	// Create a statefulset model with tags
	statefulSetModel := &model.StatefulSet{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	tags := handlers.GetMetadataTags(nil, statefulSetModel)
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestStatefulSetHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &StatefulSetHandlers{}

	// Create statefulset with sensitive annotations and labels
	statefulSet := createTestStatefulSet()
	statefulSet.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	statefulSet.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, statefulSet)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", statefulSet.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", statefulSet.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestStatefulSetProcessor_Process(t *testing.T) {
	// Create test statefulsets with unique UIDs
	statefulSet1 := createTestStatefulSet()
	statefulSet1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	statefulSet1.ResourceVersion = "1224"

	statefulSet2 := createTestStatefulSet()
	statefulSet2.Name = "statefulset2"
	statefulSet2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	statefulSet2.ResourceVersion = "1324"

	// Create fake client
	client := fake.NewClientset(statefulSet1, statefulSet2)
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
			NodeType:         orchestrator.K8sStatefulSet,
			Kind:             "StatefulSet",
			APIVersion:       "apps/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process statefulsets
	processor := processors.NewProcessor(&StatefulSetHandlers{})
	result, listed, processed := processor.Process(ctx, []*appsv1.StatefulSet{statefulSet1, statefulSet2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorStatefulSet)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.StatefulSets, 2)

	expectedStatefulSet1 := k8sTransformers.ExtractStatefulSet(ctx, statefulSet1)

	assert.Equal(t, expectedStatefulSet1.Metadata, metaMsg.StatefulSets[0].Metadata)
	assert.Equal(t, expectedStatefulSet1.Spec, metaMsg.StatefulSets[0].Spec)
	assert.Equal(t, expectedStatefulSet1.Status, metaMsg.StatefulSets[0].Status)
	assert.Equal(t, expectedStatefulSet1.Conditions, metaMsg.StatefulSets[0].Conditions)
	assert.Equal(t, expectedStatefulSet1.Tags, metaMsg.StatefulSets[0].Tags)

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
	assert.Equal(t, statefulSet1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, statefulSet1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(9), manifest1.Type) // K8sStatefulSet
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestStatefulSet appsv1.StatefulSet
	err := json.Unmarshal(manifest1.Content, &actualManifestStatefulSet)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestStatefulSet.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestStatefulSet.ObjectMeta.CreationTimestamp.Time.UTC()}
	if len(actualManifestStatefulSet.Status.Conditions) > 0 {
		actualManifestStatefulSet.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: actualManifestStatefulSet.Status.Conditions[0].LastTransitionTime.Time.UTC()}
	}
	assert.Equal(t, statefulSet1.ObjectMeta, actualManifestStatefulSet.ObjectMeta)
	assert.Equal(t, statefulSet1.Spec, actualManifestStatefulSet.Spec)
	assert.Equal(t, statefulSet1.Status, actualManifestStatefulSet.Status)
}

// Helper function to create a test statefulset
func createTestStatefulSet() *appsv1.StatefulSet {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC))
	testInt32 := int32(3)
	partitionInt32 := int32(2)

	return &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-statefulset",
			Namespace:       "default",
			UID:             "test-statefulset-uid",
			ResourceVersion: "1224",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
			CreationTimestamp: timestamp,
		},
		Spec: appsv1.StatefulSetSpec{
			Replicas:             &testInt32,
			ServiceName:          "test-service",
			PodManagementPolicy:  appsv1.OrderedReadyPodManagement,
			RevisionHistoryLimit: &testInt32,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
			},
			UpdateStrategy: appsv1.StatefulSetUpdateStrategy{
				Type: appsv1.RollingUpdateStatefulSetStrategyType,
				RollingUpdate: &appsv1.RollingUpdateStatefulSetStrategy{
					Partition: &partitionInt32,
				},
			},
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "test-app",
					},
				},
				Spec: v1.PodSpec{
					Containers: []v1.Container{
						{
							Name:  "test-container",
							Image: "test-image:latest",
						},
					},
				},
			},
		},
		Status: appsv1.StatefulSetStatus{
			ObservedGeneration: 1,
			Replicas:           3,
			ReadyReplicas:      2,
			CurrentReplicas:    3,
			UpdatedReplicas:    2,
			Conditions: []appsv1.StatefulSetCondition{
				{
					Type:               "Test",
					Status:             v1.ConditionFalse,
					LastTransitionTime: timestamp,
					Reason:             "testing",
					Message:            "test message",
				},
			},
		},
	}
}
