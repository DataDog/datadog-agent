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

func TestReplicaSetHandlers_ExtractResource(t *testing.T) {
	handlers := &ReplicaSetHandlers{}

	// Create test replica set
	rs := createTestReplicaSet()

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
	resourceModel := handlers.ExtractResource(ctx, rs)

	// Validate extraction
	rsModel, ok := resourceModel.(*model.ReplicaSet)
	assert.True(t, ok)
	assert.NotNil(t, rsModel)
	assert.Equal(t, "test-rs", rsModel.Metadata.Name)
	assert.Equal(t, "default", rsModel.Metadata.Namespace)
	assert.Equal(t, int32(3), rsModel.ReplicasDesired)
	assert.Equal(t, int32(2), rsModel.Replicas)
}

func TestReplicaSetHandlers_ResourceList(t *testing.T) {
	handlers := &ReplicaSetHandlers{}

	// Create test replica sets
	rs1 := createTestReplicaSet()
	rs2 := createTestReplicaSet()
	rs2.Name = "rs2"
	rs2.UID = "uid2"

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
	resourceList := []*appsv1.ReplicaSet{rs1, rs2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*appsv1.ReplicaSet)
	assert.True(t, ok)
	assert.Equal(t, "test-rs", resource1.Name)
	assert.NotSame(t, rs1, resource1) // Should be a copy

	resource2, ok := resources[1].(*appsv1.ReplicaSet)
	assert.True(t, ok)
	assert.Equal(t, "rs2", resource2.Name)
	assert.NotSame(t, rs2, resource2) // Should be a copy
}

func TestReplicaSetHandlers_ResourceUID(t *testing.T) {
	handlers := &ReplicaSetHandlers{}

	rs := createTestReplicaSet()
	expectedUID := types.UID("test-rs-uid")
	rs.UID = expectedUID

	uid := handlers.ResourceUID(nil, rs)
	assert.Equal(t, expectedUID, uid)
}

func TestReplicaSetHandlers_ResourceVersion(t *testing.T) {
	handlers := &ReplicaSetHandlers{}

	rs := createTestReplicaSet()
	expectedVersion := "123"
	rs.ResourceVersion = expectedVersion

	version := handlers.ResourceVersion(nil, rs, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestReplicaSetHandlers_BuildMessageBody(t *testing.T) {
	handlers := &ReplicaSetHandlers{}

	rs1 := createTestReplicaSet()
	rs2 := createTestReplicaSet()
	rs2.Name = "rs2"

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

	rs1Model := k8sTransformers.ExtractReplicaSet(ctx, rs1)
	rs2Model := k8sTransformers.ExtractReplicaSet(ctx, rs2)

	// Build message body
	resourceModels := []interface{}{rs1Model, rs2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorReplicaSet)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.ReplicaSets, 2)
	assert.Equal(t, "test-rs", collectorMsg.ReplicaSets[0].Metadata.Name)
	assert.Equal(t, "rs2", collectorMsg.ReplicaSets[1].Metadata.Name)
}

func TestReplicaSetHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &ReplicaSetHandlers{}

	rs := createTestReplicaSet()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "ReplicaSet",
			APIVersion:       "apps/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	resourceModel := &model.ReplicaSet{}
	skip := handlers.BeforeMarshalling(ctx, rs, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "ReplicaSet", rs.Kind)
	assert.Equal(t, "apps/v1", rs.APIVersion)
}

func TestReplicaSetHandlers_AfterMarshalling(t *testing.T) {
	handlers := &ReplicaSetHandlers{}

	rs := createTestReplicaSet()

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
	resourceModel := &model.ReplicaSet{}

	// Create test YAML
	testYAML := []byte("apiVersion: apps/v1\nkind: ReplicaSet\nmetadata:\n  name: test-rs")

	// Call AfterMarshalling
	skip := handlers.AfterMarshalling(ctx, rs, resourceModel, testYAML)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestReplicaSetHandlers_GetMetadataTags(t *testing.T) {
	handlers := &ReplicaSetHandlers{}

	// Create a replica set model with tags
	rsModel := &model.ReplicaSet{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	tags := handlers.GetMetadataTags(nil, rsModel)
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestReplicaSetHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &ReplicaSetHandlers{}

	// Create replica set with sensitive annotations and labels
	rs := createTestReplicaSet()
	rs.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	rs.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, rs)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", rs.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", rs.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestReplicaSetProcessor_Process(t *testing.T) {
	// Create test replica sets with unique UIDs
	rs1 := createTestReplicaSet()
	rs1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	rs1.ResourceVersion = "1219"

	rs2 := createTestReplicaSet()
	rs2.Name = "rs2"
	rs2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	rs2.ResourceVersion = "1319"

	// Create fake client
	client := fake.NewClientset(rs1, rs2)
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
			NodeType:         orchestrator.K8sReplicaSet,
			Kind:             "ReplicaSet",
			APIVersion:       "apps/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process replica sets
	processor := processors.NewProcessor(&ReplicaSetHandlers{})
	result, listed, processed := processor.Process(ctx, []*appsv1.ReplicaSet{rs1, rs2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorReplicaSet)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.ReplicaSets, 2)

	expectedRs1 := k8sTransformers.ExtractReplicaSet(ctx, rs1)

	assert.Equal(t, expectedRs1.Metadata, metaMsg.ReplicaSets[0].Metadata)
	assert.Equal(t, expectedRs1.ReplicasDesired, metaMsg.ReplicaSets[0].ReplicasDesired)
	assert.Equal(t, expectedRs1.Selectors, metaMsg.ReplicaSets[0].Selectors)
	assert.Equal(t, expectedRs1.Replicas, metaMsg.ReplicaSets[0].Replicas)
	assert.Equal(t, expectedRs1.FullyLabeledReplicas, metaMsg.ReplicaSets[0].FullyLabeledReplicas)
	assert.Equal(t, expectedRs1.ReadyReplicas, metaMsg.ReplicaSets[0].ReadyReplicas)
	assert.Equal(t, expectedRs1.AvailableReplicas, metaMsg.ReplicaSets[0].AvailableReplicas)
	assert.Equal(t, expectedRs1.ResourceRequirements, metaMsg.ReplicaSets[0].ResourceRequirements)
	assert.Equal(t, expectedRs1.Conditions, metaMsg.ReplicaSets[0].Conditions)
	assert.Equal(t, expectedRs1.Tags, metaMsg.ReplicaSets[0].Tags)

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
	assert.Equal(t, rs1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, rs1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(2), manifest1.Type) // K8sReplicaSet
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestRs appsv1.ReplicaSet
	err := json.Unmarshal(manifest1.Content, &actualManifestRs)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestRs.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestRs.ObjectMeta.CreationTimestamp.Time.UTC()}
	actualManifestRs.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: actualManifestRs.Status.Conditions[0].LastTransitionTime.Time.UTC()}
	assert.Equal(t, rs1.ObjectMeta, actualManifestRs.ObjectMeta)
	assert.Equal(t, rs1.Spec, actualManifestRs.Spec)
	assert.Equal(t, rs1.Status, actualManifestRs.Status)
}

// Helper function to create a test replica set
func createTestReplicaSet() *appsv1.ReplicaSet {
	timestamp := metav1.NewTime(time.Date(2014, time.January, 15, 0, 0, 0, 0, time.UTC))
	testInt32 := int32(3)

	return &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-rs",
			Namespace:       "default",
			UID:             "test-rs-uid",
			ResourceVersion: "1219",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
			CreationTimestamp: timestamp,
		},
		Spec: appsv1.ReplicaSetSpec{
			Replicas: &testInt32,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "test-app",
				},
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "environment",
						Operator: "NotIn",
						Values:   []string{"staging", "prod"},
					},
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
							Name:  "nginx",
							Image: "nginx:1.21",
							Resources: v1.ResourceRequirements{
								Requests: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("100m"),
									v1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: v1.ResourceList{
									v1.ResourceCPU:    resource.MustParse("200m"),
									v1.ResourceMemory: resource.MustParse("256Mi"),
								},
							},
						},
					},
				},
			},
		},
		Status: appsv1.ReplicaSetStatus{
			Replicas:             2,
			FullyLabeledReplicas: 2,
			ReadyReplicas:        1,
			AvailableReplicas:    1,
			Conditions: []appsv1.ReplicaSetCondition{
				{
					Type:               appsv1.ReplicaSetReplicaFailure,
					Status:             v1.ConditionFalse,
					LastTransitionTime: timestamp,
					Reason:             "test reason",
					Message:            "test message",
				},
			},
		},
	}
}
