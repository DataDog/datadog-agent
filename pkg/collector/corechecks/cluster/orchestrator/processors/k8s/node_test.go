// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

// Package k8s defines handlers for processing kubernetes resources
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
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestNodeHandlers_ExtractResource(t *testing.T) {
	handlers := &NodeHandlers{}

	// Create test node
	node := createTestNode("test-node", "test-namespace")

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
	resourceModel := handlers.ExtractResource(ctx, node)

	// Validate extraction
	nodeModel, ok := resourceModel.(*model.Node)
	assert.True(t, ok)
	assert.NotNil(t, nodeModel)
	assert.Equal(t, "test-node", nodeModel.Metadata.Name)
	assert.Equal(t, "test-namespace", nodeModel.Metadata.Namespace)
	assert.NotNil(t, nodeModel.Status)
	assert.Equal(t, "Ready", nodeModel.Status.Status)
}

func TestNodeHandlers_ResourceList(t *testing.T) {
	handlers := &NodeHandlers{}

	// Create test nodes
	node1 := createTestNode("node-1", "namespace-1")
	node2 := createTestNode("node-2", "namespace-2")

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
	resourceList := []*corev1.Node{node1, node2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*corev1.Node)
	assert.True(t, ok)
	assert.Equal(t, "node-1", resource1.Name)
	assert.NotSame(t, node1, resource1) // Should be a copy

	resource2, ok := resources[1].(*corev1.Node)
	assert.True(t, ok)
	assert.Equal(t, "node-2", resource2.Name)
	assert.NotSame(t, node2, resource2) // Should be a copy
}

func TestNodeHandlers_ResourceUID(t *testing.T) {
	handlers := &NodeHandlers{}

	node := createTestNode("test-node", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	node.UID = expectedUID

	uid := handlers.ResourceUID(nil, node)
	assert.Equal(t, expectedUID, uid)
}

func TestNodeHandlers_ResourceVersion(t *testing.T) {
	handlers := &NodeHandlers{}

	node := createTestNode("test-node", "test-namespace")
	expectedVersion := "v123"
	node.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.Node{}

	version := handlers.ResourceVersion(nil, node, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestNodeHandlers_BuildMessageBody(t *testing.T) {
	handlers := &NodeHandlers{}

	node1 := createTestNode("node-1", "namespace-1")
	node2 := createTestNode("node-2", "namespace-2")

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

	node1Model := k8sTransformers.ExtractNode(ctx, node1)
	node2Model := k8sTransformers.ExtractNode(ctx, node2)

	// Build message body
	resourceModels := []interface{}{node1Model, node2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorNode)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.Nodes, 2)
	assert.Equal(t, "node-1", collectorMsg.Nodes[0].Metadata.Name)
	assert.Equal(t, "node-2", collectorMsg.Nodes[1].Metadata.Name)
}

func TestNodeHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &NodeHandlers{}

	node := createTestNode("test-node", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "Node",
			APIVersion:       "v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.Node{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, node, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "Node", node.Kind)
	assert.Equal(t, "v1", node.APIVersion)
}

func TestNodeHandlers_AfterMarshalling(t *testing.T) {
	handlers := &NodeHandlers{}

	node := createTestNode("test-node", "test-namespace")

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
	resourceModel := &model.Node{
		Metadata: &model.Metadata{
			Name: "test-node",
		},
	}

	// Test YAML
	testYAML := []byte(`{"apiVersion":"v1","kind":"Node","metadata":{"name":"test-node"}}`)

	skip := handlers.AfterMarshalling(ctx, node, resourceModel, testYAML)
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestNodeHandlers_GetMetadataTags(t *testing.T) {
	handlers := &NodeHandlers{}

	// Create node model with tags
	nodeModel := &model.Node{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, nodeModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestNodeHandlers_GetNodeName(t *testing.T) {
	handlers := &NodeHandlers{}

	node := createTestNode("test-node", "test-namespace")

	// Get node name
	nodeName := handlers.GetNodeName(nil, node)

	// Validate
	assert.Equal(t, "test-node", nodeName)
}

func TestNodeHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &NodeHandlers{}

	// Create node with sensitive annotations and labels
	node := createTestNode("test-node", "test-namespace")
	node.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	node.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, node)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", node.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", node.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestNodeProcessor_Process(t *testing.T) {
	// Create test nodes with unique UIDs
	node1 := createTestNode("node-1", "namespace-1")
	node1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	node1.ResourceVersion = "1214"

	node2 := createTestNode("node-2", "namespace-2")
	node2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	node2.ResourceVersion = "1314"

	// Create fake client
	client := fake.NewClientset(node1, node2)
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
			NodeType:         orchestrator.K8sNode,
			Kind:             "Node",
			APIVersion:       "v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process nodes
	processor := processors.NewProcessor(&NodeHandlers{})
	result, listed, processed := processor.Process(ctx, []*corev1.Node{node1, node2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorNode)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.Nodes, 2)

	expectedNode1 := k8sTransformers.ExtractNode(ctx, node1)

	assert.Equal(t, expectedNode1.Metadata, metaMsg.Nodes[0].Metadata)
	assert.Equal(t, expectedNode1.Status, metaMsg.Nodes[0].Status)
	assert.Equal(t, expectedNode1.Tags, metaMsg.Nodes[0].Tags)
	assert.Equal(t, expectedNode1.Roles, metaMsg.Nodes[0].Roles)

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
	assert.Equal(t, node1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, node1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(4), manifest1.Type) // K8sNode
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestNode corev1.Node
	err := json.Unmarshal(manifest1.Content, &actualManifestNode)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestNode.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestNode.ObjectMeta.CreationTimestamp.Time.UTC()}
	actualManifestNode.Status.Conditions[0].LastHeartbeatTime = metav1.Time{Time: actualManifestNode.Status.Conditions[0].LastHeartbeatTime.Time.UTC()}
	actualManifestNode.Status.Conditions[0].LastTransitionTime = metav1.Time{Time: actualManifestNode.Status.Conditions[0].LastTransitionTime.Time.UTC()}
	assert.Equal(t, node1.ObjectMeta, actualManifestNode.ObjectMeta)
	assert.Equal(t, node1.Spec, actualManifestNode.Spec)
	assert.Equal(t, node1.Status, actualManifestNode.Status)
}

func createTestNode(name, namespace string) *corev1.Node {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &corev1.Node{
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
			ResourceVersion: "1214",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Spec: corev1.NodeSpec{
			PodCIDR:       "10.244.0.0/24",
			PodCIDRs:      []string{"10.244.0.0/24"},
			ProviderID:    "aws:///us-west-2a/i-1234567890abcdef0",
			Unschedulable: false,
			Taints: []corev1.Taint{
				{
					Key:    "node-role.kubernetes.io/master",
					Value:  "",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
		Status: corev1.NodeStatus{
			NodeInfo: corev1.NodeSystemInfo{
				KernelVersion:           "5.4.0-1045-aws",
				OSImage:                 "Ubuntu 20.04.3 LTS",
				ContainerRuntimeVersion: "docker://20.10.8",
				KubeletVersion:          "v1.21.0",
				OperatingSystem:         "linux",
				Architecture:            "amd64",
			},
			Addresses: []corev1.NodeAddress{
				{
					Type:    corev1.NodeInternalIP,
					Address: "10.0.0.1",
				},
				{
					Type:    corev1.NodeHostName,
					Address: name,
				},
			},
			Images: []corev1.ContainerImage{
				{
					Names:     []string{"nginx:1.21", "nginx:latest"},
					SizeBytes: 133000000,
				},
			},
			DaemonEndpoints: corev1.NodeDaemonEndpoints{
				KubeletEndpoint: corev1.DaemonEndpoint{
					Port: 10250,
				},
			},
			Capacity: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourcePods:   resource.MustParse("110"),
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
			Allocatable: map[corev1.ResourceName]resource.Quantity{
				corev1.ResourcePods:   resource.MustParse("110"),
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("8Gi"),
			},
			Conditions: []corev1.NodeCondition{
				{
					Type:               corev1.NodeReady,
					Status:             corev1.ConditionTrue,
					LastHeartbeatTime:  creationTime,
					LastTransitionTime: creationTime,
					Reason:             "KubeletReady",
					Message:            "kubelet is posting ready status",
				},
			},
		},
	}
}
