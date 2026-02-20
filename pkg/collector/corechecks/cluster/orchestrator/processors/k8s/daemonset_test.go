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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestDaemonSetHandlers_ExtractResource(t *testing.T) {
	handlers := &DaemonSetHandlers{}

	// Create test daemon set
	daemonSet := createTestDaemonSet("test-daemonset", "test-namespace")

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
	resourceModel := handlers.ExtractResource(ctx, daemonSet)

	// Validate extraction
	daemonSetModel, ok := resourceModel.(*model.DaemonSet)
	assert.True(t, ok)
	assert.NotNil(t, daemonSetModel)
	assert.Equal(t, "test-daemonset", daemonSetModel.Metadata.Name)
	assert.Equal(t, "test-namespace", daemonSetModel.Metadata.Namespace)
	assert.NotNil(t, daemonSetModel.Spec)
	assert.NotNil(t, daemonSetModel.Status)
}

func TestDaemonSetHandlers_ResourceList(t *testing.T) {
	handlers := &DaemonSetHandlers{}

	// Create test daemon sets
	daemonSet1 := createTestDaemonSet("daemonset-1", "namespace-1")
	daemonSet2 := createTestDaemonSet("daemonset-2", "namespace-2")

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
	resourceList := []*appsv1.DaemonSet{daemonSet1, daemonSet2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*appsv1.DaemonSet)
	assert.True(t, ok)
	assert.Equal(t, "daemonset-1", resource1.Name)
	assert.NotSame(t, daemonSet1, resource1) // Should be a copy

	resource2, ok := resources[1].(*appsv1.DaemonSet)
	assert.True(t, ok)
	assert.Equal(t, "daemonset-2", resource2.Name)
	assert.NotSame(t, daemonSet2, resource2) // Should be a copy
}

func TestDaemonSetHandlers_ResourceUID(t *testing.T) {
	handlers := &DaemonSetHandlers{}

	daemonSet := createTestDaemonSet("test-daemonset", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	daemonSet.UID = expectedUID

	uid := handlers.ResourceUID(nil, daemonSet)
	assert.Equal(t, expectedUID, uid)
}

func TestDaemonSetHandlers_ResourceVersion(t *testing.T) {
	handlers := &DaemonSetHandlers{}

	daemonSet := createTestDaemonSet("test-daemonset", "test-namespace")
	expectedVersion := "v123"
	daemonSet.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.DaemonSet{}

	version := handlers.ResourceVersion(nil, daemonSet, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestDaemonSetHandlers_BuildMessageBody(t *testing.T) {
	handlers := &DaemonSetHandlers{}

	daemonSet1 := createTestDaemonSet("daemonset-1", "namespace-1")
	daemonSet2 := createTestDaemonSet("daemonset-2", "namespace-2")

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

	daemonSet1Model := k8sTransformers.ExtractDaemonSet(ctx, daemonSet1)
	daemonSet2Model := k8sTransformers.ExtractDaemonSet(ctx, daemonSet2)

	// Build message body
	resourceModels := []interface{}{daemonSet1Model, daemonSet2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorDaemonSet)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.DaemonSets, 2)
	assert.Equal(t, "daemonset-1", collectorMsg.DaemonSets[0].Metadata.Name)
	assert.Equal(t, "daemonset-2", collectorMsg.DaemonSets[1].Metadata.Name)
}

func TestDaemonSetHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &DaemonSetHandlers{}

	daemonSet := createTestDaemonSet("test-daemonset", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "DaemonSet",
			APIVersion:       "apps/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.DaemonSet{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, daemonSet, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "DaemonSet", daemonSet.Kind)
	assert.Equal(t, "apps/v1", daemonSet.APIVersion)
}

func TestDaemonSetHandlers_AfterMarshalling(t *testing.T) {
	handlers := &DaemonSetHandlers{}

	daemonSet := createTestDaemonSet("test-daemonset", "test-namespace")
	resourceModel := &model.DaemonSet{}

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
	testYAML := []byte(`{"apiVersion":"apps/v1","kind":"DaemonSet","metadata":{"name":"test"}}`)

	skip := handlers.AfterMarshalling(ctx, daemonSet, resourceModel, testYAML)
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestDaemonSetHandlers_GetMetadataTags(t *testing.T) {
	handlers := &DaemonSetHandlers{}

	// Create daemon set model with tags
	daemonSetModel := &model.DaemonSet{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, daemonSetModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestDaemonSetHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &DaemonSetHandlers{}

	// Create daemon set with sensitive annotations and labels
	daemonSet := createTestDaemonSet("test-daemonset", "test-namespace")
	daemonSet.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	daemonSet.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, daemonSet)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", daemonSet.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", daemonSet.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestDaemonSetProcessor_Process(t *testing.T) {
	// Create test daemon sets with unique UIDs
	daemonSet1 := createTestDaemonSet("daemonset-1", "namespace-1")
	daemonSet1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	daemonSet1.ResourceVersion = "1206"

	daemonSet2 := createTestDaemonSet("daemonset-2", "namespace-2")
	daemonSet2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	daemonSet2.ResourceVersion = "1306"

	// Create fake client
	client := fake.NewClientset(daemonSet1, daemonSet2)
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
			NodeType:         orchestrator.K8sDaemonSet,
			Kind:             "DaemonSet",
			APIVersion:       "apps/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process daemon sets
	processor := processors.NewProcessor(&DaemonSetHandlers{})
	result, listed, processed := processor.Process(ctx, []*appsv1.DaemonSet{daemonSet1, daemonSet2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorDaemonSet)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.DaemonSets, 2)

	expectedDaemonSet1 := k8sTransformers.ExtractDaemonSet(ctx, daemonSet1)

	assert.Equal(t, expectedDaemonSet1.Metadata, metaMsg.DaemonSets[0].Metadata)
	assert.Equal(t, expectedDaemonSet1.Spec, metaMsg.DaemonSets[0].Spec)
	assert.Equal(t, expectedDaemonSet1.Status, metaMsg.DaemonSets[0].Status)
	assert.Equal(t, expectedDaemonSet1.Tags, metaMsg.DaemonSets[0].Tags)

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
	assert.Equal(t, daemonSet1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, daemonSet1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(8), manifest1.Type) // K8sDaemonSet
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestDaemonSet appsv1.DaemonSet
	err := json.Unmarshal(manifest1.Content, &actualManifestDaemonSet)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestDaemonSet.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestDaemonSet.ObjectMeta.CreationTimestamp.Time.UTC()}
	actualManifestDaemonSet.Status.Conditions[0].LastTransitionTime = metav1.NewTime(actualManifestDaemonSet.Status.Conditions[0].LastTransitionTime.UTC())
	assert.Equal(t, daemonSet1.ObjectMeta, actualManifestDaemonSet.ObjectMeta)
	assert.Equal(t, daemonSet1.Spec, actualManifestDaemonSet.Spec)
	assert.Equal(t, daemonSet1.Status, actualManifestDaemonSet.Status)
}

func createTestDaemonSet(name, namespace string) *appsv1.DaemonSet {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	testIntOrStrPercent := intstr.FromString("1%")

	return &appsv1.DaemonSet{
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
			ResourceVersion: "1206",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Spec: appsv1.DaemonSetSpec{
			MinReadySeconds: 5,
			UpdateStrategy: appsv1.DaemonSetUpdateStrategy{
				Type: appsv1.DaemonSetUpdateStrategyType("RollingUpdate"),
				RollingUpdate: &appsv1.RollingUpdateDaemonSet{
					MaxUnavailable: &testIntOrStrPercent,
				},
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-app",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app": "my-app",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "nginx",
							Image: "nginx:latest",
						},
					},
				},
			},
		},
		Status: appsv1.DaemonSetStatus{
			CurrentNumberScheduled: 3,
			NumberMisscheduled:     0,
			DesiredNumberScheduled: 3,
			NumberReady:            3,
			UpdatedNumberScheduled: 3,
			NumberAvailable:        3,
			NumberUnavailable:      0,
			Conditions: []appsv1.DaemonSetCondition{
				{
					Type:               "Test",
					Status:             corev1.ConditionFalse,
					LastTransitionTime: creationTime,
					Reason:             "test reason",
					Message:            "test message",
				},
			},
		},
	}
}
