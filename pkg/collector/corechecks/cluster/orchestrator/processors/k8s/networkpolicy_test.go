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
	v1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
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

func TestNetworkPolicyHandlers_ExtractResource(t *testing.T) {
	handlers := &NetworkPolicyHandlers{}

	// Create test network policy
	networkPolicy := createTestNetworkPolicy("test-networkpolicy", "test-namespace")

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
	resourceModel := handlers.ExtractResource(ctx, networkPolicy)

	// Validate extraction
	networkPolicyModel, ok := resourceModel.(*model.NetworkPolicy)
	assert.True(t, ok)
	assert.NotNil(t, networkPolicyModel)
	assert.Equal(t, "test-networkpolicy", networkPolicyModel.Metadata.Name)
	assert.Equal(t, "test-namespace", networkPolicyModel.Metadata.Namespace)
	assert.NotNil(t, networkPolicyModel.Spec)
	assert.NotNil(t, networkPolicyModel.Spec.Ingress)
}

func TestNetworkPolicyHandlers_ResourceList(t *testing.T) {
	handlers := &NetworkPolicyHandlers{}

	// Create test network policies
	networkPolicy1 := createTestNetworkPolicy("networkpolicy-1", "namespace-1")
	networkPolicy2 := createTestNetworkPolicy("networkpolicy-2", "namespace-2")

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
	resourceList := []*networkingv1.NetworkPolicy{networkPolicy1, networkPolicy2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*networkingv1.NetworkPolicy)
	assert.True(t, ok)
	assert.Equal(t, "networkpolicy-1", resource1.Name)
	assert.NotSame(t, networkPolicy1, resource1) // Should be a copy

	resource2, ok := resources[1].(*networkingv1.NetworkPolicy)
	assert.True(t, ok)
	assert.Equal(t, "networkpolicy-2", resource2.Name)
	assert.NotSame(t, networkPolicy2, resource2) // Should be a copy
}

func TestNetworkPolicyHandlers_ResourceUID(t *testing.T) {
	handlers := &NetworkPolicyHandlers{}

	networkPolicy := createTestNetworkPolicy("test-networkpolicy", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	networkPolicy.UID = expectedUID

	uid := handlers.ResourceUID(nil, networkPolicy)
	assert.Equal(t, expectedUID, uid)
}

func TestNetworkPolicyHandlers_ResourceVersion(t *testing.T) {
	handlers := &NetworkPolicyHandlers{}

	networkPolicy := createTestNetworkPolicy("test-networkpolicy", "test-namespace")
	expectedVersion := "v123"
	networkPolicy.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.NetworkPolicy{}

	version := handlers.ResourceVersion(nil, networkPolicy, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestNetworkPolicyHandlers_BuildMessageBody(t *testing.T) {
	handlers := &NetworkPolicyHandlers{}

	networkPolicy1 := createTestNetworkPolicy("networkpolicy-1", "namespace-1")
	networkPolicy2 := createTestNetworkPolicy("networkpolicy-2", "namespace-2")

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

	networkPolicy1Model := k8sTransformers.ExtractNetworkPolicy(ctx, networkPolicy1)
	networkPolicy2Model := k8sTransformers.ExtractNetworkPolicy(ctx, networkPolicy2)

	// Build message body
	resourceModels := []interface{}{networkPolicy1Model, networkPolicy2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorNetworkPolicy)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.NetworkPolicies, 2)
	assert.Equal(t, "networkpolicy-1", collectorMsg.NetworkPolicies[0].Metadata.Name)
	assert.Equal(t, "networkpolicy-2", collectorMsg.NetworkPolicies[1].Metadata.Name)
}

func TestNetworkPolicyHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &NetworkPolicyHandlers{}

	networkPolicy := createTestNetworkPolicy("test-networkpolicy", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "NetworkPolicy",
			APIVersion:       "networking.k8s.io/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.NetworkPolicy{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, networkPolicy, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "NetworkPolicy", networkPolicy.Kind)
	assert.Equal(t, "networking.k8s.io/v1", networkPolicy.APIVersion)
}

func TestNetworkPolicyHandlers_AfterMarshalling(t *testing.T) {
	handlers := &NetworkPolicyHandlers{}

	networkPolicy := createTestNetworkPolicy("test-networkpolicy", "test-namespace")

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
	resourceModel := &model.NetworkPolicy{
		Metadata: &model.Metadata{
			Name: "test-networkpolicy",
		},
	}

	// Test YAML
	testYAML := []byte(`{"apiVersion":"networking.k8s.io/v1","kind":"NetworkPolicy","metadata":{"name":"test-networkpolicy"}}`)

	skip := handlers.AfterMarshalling(ctx, networkPolicy, resourceModel, testYAML)
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestNetworkPolicyHandlers_GetMetadataTags(t *testing.T) {
	handlers := &NetworkPolicyHandlers{}

	// Create network policy model with tags
	networkPolicyModel := &model.NetworkPolicy{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, networkPolicyModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestNetworkPolicyHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &NetworkPolicyHandlers{}

	// Create network policy with sensitive annotations and labels
	networkPolicy := createTestNetworkPolicy("test-networkpolicy", "test-namespace")
	networkPolicy.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	networkPolicy.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, networkPolicy)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", networkPolicy.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", networkPolicy.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestNetworkPolicyProcessor_Process(t *testing.T) {
	// Create test network policies with unique UIDs
	networkPolicy1 := createTestNetworkPolicy("networkpolicy-1", "namespace-1")
	networkPolicy1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	networkPolicy1.ResourceVersion = "1213"

	networkPolicy2 := createTestNetworkPolicy("networkpolicy-2", "namespace-2")
	networkPolicy2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	networkPolicy2.ResourceVersion = "1313"

	// Create fake client
	client := fake.NewClientset(networkPolicy1, networkPolicy2)
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
			NodeType:         orchestrator.K8sNetworkPolicy,
			Kind:             "NetworkPolicy",
			APIVersion:       "networking.k8s.io/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process network policies
	processor := processors.NewProcessor(&NetworkPolicyHandlers{})
	result, listed, processed := processor.Process(ctx, []*networkingv1.NetworkPolicy{networkPolicy1, networkPolicy2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorNetworkPolicy)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.NetworkPolicies, 2)

	expectedNetworkPolicy1 := k8sTransformers.ExtractNetworkPolicy(ctx, networkPolicy1)

	assert.Equal(t, expectedNetworkPolicy1.Metadata, metaMsg.NetworkPolicies[0].Metadata)
	assert.Equal(t, expectedNetworkPolicy1.Spec, metaMsg.NetworkPolicies[0].Spec)
	assert.Equal(t, expectedNetworkPolicy1.Tags, metaMsg.NetworkPolicies[0].Tags)

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
	assert.Equal(t, networkPolicy1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, networkPolicy1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(24), manifest1.Type) // K8sNetworkPolicy
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestNetworkPolicy networkingv1.NetworkPolicy
	err := json.Unmarshal(manifest1.Content, &actualManifestNetworkPolicy)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestNetworkPolicy.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestNetworkPolicy.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, networkPolicy1.ObjectMeta, actualManifestNetworkPolicy.ObjectMeta)
	assert.Equal(t, networkPolicy1.Spec, actualManifestNetworkPolicy.Spec)
}

func createTestNetworkPolicy(name, namespace string) *networkingv1.NetworkPolicy {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))
	protocol := v1.Protocol("TCP")

	return &networkingv1.NetworkPolicy{
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
			ResourceVersion: "1213",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": "my-app",
				},
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					From: []networkingv1.NetworkPolicyPeer{
						{
							IPBlock: &networkingv1.IPBlock{
								CIDR: "10.0.0.0/24",
							},
						},
					},
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 80},
							Protocol: &protocol,
						},
					},
				},
			},
		},
	}
}
