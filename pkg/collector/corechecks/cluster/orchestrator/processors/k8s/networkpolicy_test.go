// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

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
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestNetworkPolicyHandlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.NetworkPolicy{}
	resource := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-networkpolicy",
			Namespace: "test-ns",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("networking.k8s.io", "networkpolicies")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), resource.Namespace, resource.Name)),
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewNetworkPolicyHandlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
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
	processor := processors.NewProcessor(NewNetworkPolicyHandlers(processorstest.NewEmptyFakeTagger()))
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
