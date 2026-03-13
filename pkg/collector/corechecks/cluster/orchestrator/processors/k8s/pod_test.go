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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

// mockPodTagProvider is a simple mock implementation for testing
type mockPodTagProvider struct{}

func (m *mockPodTagProvider) GetTags(pod *corev1.Pod, _ taggertypes.TagCardinality) ([]string, error) {
	// Return some basic tags for testing
	return []string{"kube_namespace:default", "kube_pod_name:" + pod.Name}, nil
}

func TestPodHandlers_ExtractResource(t *testing.T) {
	handlers := &PodHandlers{}

	// Create test pod
	pod := createTestPod("test-pod", "test-namespace")

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
	resourceModel := handlers.ExtractResource(ctx, pod)

	// Validate extraction
	podModel, ok := resourceModel.(*model.Pod)
	assert.True(t, ok)
	assert.NotNil(t, podModel)
	assert.Equal(t, "test-pod", podModel.Metadata.Name)
	assert.Equal(t, "test-namespace", podModel.Metadata.Namespace)
	assert.NotNil(t, podModel.Status)
	assert.Equal(t, "Running", podModel.Status)
}

func TestPodHandlers_ResourceList(t *testing.T) {
	handlers := &PodHandlers{}

	// Create test pods
	pod1 := createTestPod("pod-1", "namespace-1")
	pod2 := createTestPod("pod-2", "namespace-2")

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
	resourceList := []*corev1.Pod{pod1, pod2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*corev1.Pod)
	assert.True(t, ok)
	assert.Equal(t, "pod-1", resource1.Name)
	assert.NotSame(t, pod1, resource1) // Should be a copy

	resource2, ok := resources[1].(*corev1.Pod)
	assert.True(t, ok)
	assert.Equal(t, "pod-2", resource2.Name)
	assert.NotSame(t, pod2, resource2) // Should be a copy
}

func TestPodHandlers_ResourceUID(t *testing.T) {
	handlers := &PodHandlers{}

	pod := createTestPod("test-pod", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	pod.UID = expectedUID

	uid := handlers.ResourceUID(nil, pod)
	assert.Equal(t, expectedUID, uid)
}

func TestPodHandlers_ResourceVersion(t *testing.T) {
	handlers := &PodHandlers{}

	pod := createTestPod("test-pod", "test-namespace")
	expectedVersion := "v123"

	// Create a mock resource model
	resourceModel := &model.Pod{
		Metadata: &model.Metadata{
			ResourceVersion: expectedVersion,
		},
	}

	version := handlers.ResourceVersion(nil, pod, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestPodHandlers_BuildMessageBody(t *testing.T) {
	handlers := &PodHandlers{}

	pod1 := createTestPod("pod-1", "namespace-1")
	pod2 := createTestPod("pod-2", "namespace-2")

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

	pod1Model := k8sTransformers.ExtractPod(ctx, pod1)
	pod2Model := k8sTransformers.ExtractPod(ctx, pod2)

	// Build message body
	resourceModels := []interface{}{pod1Model, pod2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorPod)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Equal(t, "test-host", collectorMsg.HostName)
	assert.Len(t, collectorMsg.Pods, 2)
	assert.Equal(t, "pod-1", collectorMsg.Pods[0].Metadata.Name)
	assert.Equal(t, "pod-2", collectorMsg.Pods[1].Metadata.Name)
}

func TestPodHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &PodHandlers{}

	pod := createTestPod("test-pod", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "Pod",
			APIVersion:       "v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.Pod{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, pod, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "Pod", pod.Kind)
	assert.Equal(t, "v1", pod.APIVersion)
}

func TestPodHandlers_AfterMarshalling(t *testing.T) {
	handlers := &PodHandlers{}

	pod := createTestPod("test-pod", "test-namespace")

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
	resourceModel := &model.Pod{}

	// Call AfterMarshalling
	yaml := []byte("test-yaml")
	skip := handlers.AfterMarshalling(ctx, pod, resourceModel, yaml)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, yaml, resourceModel.Yaml)
}

func TestPodHandlers_GetMetadataTags(t *testing.T) {
	handlers := &PodHandlers{}

	pod := &model.Pod{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, pod)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestPodHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &PodHandlers{}

	// Create pod with sensitive annotations and labels
	pod := createTestPod("test-pod", "test-namespace")
	pod.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	pod.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, pod)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", pod.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", pod.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestPodHandlers_ScrubBeforeMarshalling(t *testing.T) {
	handlers := &PodHandlers{}

	pod := createTestPod("test-pod", "test-namespace")

	// Create processor context with scrubbing enabled
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.IsScrubbingEnabled = true

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

	// Call ScrubBeforeMarshalling
	handlers.ScrubBeforeMarshalling(ctx, pod)

	// Validate that scrubbing was applied (no error should occur)
	assert.NotNil(t, pod)
}

func TestPodProcessor_Process(t *testing.T) {
	// Create test pods with unique UIDs
	pod1 := createTestPod("pod-1", "namespace-1")
	pod1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	pod1.ResourceVersion = "1217"

	pod2 := createTestPod("pod-2", "namespace-2")
	pod2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	pod2.ResourceVersion = "1317"

	// Create fake client
	client := fake.NewClientset(pod1, pod2)
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
			NodeType:         orchestrator.K8sPod,
			Kind:             "Pod",
			APIVersion:       "v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process pods
	handlers := &PodHandlers{
		tagProvider: &mockPodTagProvider{},
	}
	processor := processors.NewProcessor(handlers)
	result, listed, processed := processor.Process(ctx, []*corev1.Pod{pod1, pod2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorPod)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Equal(t, "test-host", metaMsg.HostName)
	assert.Len(t, metaMsg.Pods, 2)

	expectedPod1 := k8sTransformers.ExtractPod(ctx, pod1)

	// Resource version is computed for pods
	expectedPod1.Metadata.ResourceVersion = metaMsg.Pods[0].Metadata.ResourceVersion
	assert.Equal(t, expectedPod1.Metadata, metaMsg.Pods[0].Metadata)
	assert.Equal(t, expectedPod1.Status, metaMsg.Pods[0].Status)
	// Add the tags that are computed for the pod in BeforeCacheCheck
	expectedPod1.Tags = append(expectedPod1.Tags, "kube_namespace:default", "kube_pod_name:pod-1", "pod_status:running")
	assert.ElementsMatch(t, expectedPod1.Tags, metaMsg.Pods[0].Tags)

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
	assert.Equal(t, pod1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, int32(1), manifest1.Type) // K8sPod
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)
	assert.Equal(t, "test-node", manifest1.NodeName)

	// Parse the actual manifest content
	var actualManifestPod corev1.Pod
	err := json.Unmarshal(manifest1.Content, &actualManifestPod)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestPod.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestPod.ObjectMeta.CreationTimestamp.Time.UTC()}
	actualManifestPod.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: actualManifestPod.Status.Conditions[0].LastTransitionTime.Time.UTC()}
	actualManifestPod.Status.Conditions[1].LastTransitionTime = metav1.Time{Time: actualManifestPod.Status.Conditions[1].LastTransitionTime.Time.UTC()}
	actualManifestPod.Status.StartTime = &metav1.Time{Time: actualManifestPod.Status.StartTime.Time.UTC()}
	actualManifestPod.Status.ContainerStatuses[0].State.Running.StartedAt = metav1.Time{Time: actualManifestPod.Status.ContainerStatuses[0].State.Running.StartedAt.Time.UTC()}
	assert.Equal(t, pod1.ObjectMeta, actualManifestPod.ObjectMeta)
	assert.Equal(t, pod1.Spec, actualManifestPod.Spec)
	assert.Equal(t, pod1.Status, actualManifestPod.Status)
}

func createTestPod(name, namespace string) *corev1.Pod {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	startTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 31, 0, 0, time.UTC))

	return &corev1.Pod{
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
			ResourceVersion: "1217",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Spec: corev1.PodSpec{
			NodeName: "test-node",
			Containers: []corev1.Container{
				{
					Name:  "nginx",
					Image: "nginx:1.21",
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("100m"),
							corev1.ResourceMemory: resource.MustParse("128Mi"),
						},
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("200m"),
							corev1.ResourceMemory: resource.MustParse("256Mi"),
						},
					},
				},
			},
			PriorityClassName: "high-priority",
		},
		Status: corev1.PodStatus{
			Phase:     corev1.PodRunning,
			StartTime: &startTime,
			PodIP:     "10.244.0.1",
			QOSClass:  corev1.PodQOSGuaranteed,
			ContainerStatuses: []corev1.ContainerStatus{
				{
					Name:         "nginx",
					Ready:        true,
					RestartCount: 0,
					State: corev1.ContainerState{
						Running: &corev1.ContainerStateRunning{
							StartedAt: startTime,
						},
					},
				},
			},
			Conditions: []corev1.PodCondition{
				{
					Type:               corev1.PodReady,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: startTime,
					Reason:             "PodCompleted",
					Message:            "Pod is ready",
				},
				{
					Type:               corev1.PodScheduled,
					Status:             corev1.ConditionTrue,
					LastTransitionTime: startTime,
					Reason:             "Scheduled",
					Message:            "Pod is scheduled",
				},
			},
		},
	}
}
