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
	rbacv1 "k8s.io/api/rbac/v1"
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

func TestClusterRoleHandlers_ExtractResource(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	// Create test cluster role
	clusterRole := createTestClusterRole("test-clusterrole", "test-namespace")

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
	resourceModel := handlers.ExtractResource(ctx, clusterRole)

	// Validate extraction
	clusterRoleModel, ok := resourceModel.(*model.ClusterRole)
	assert.True(t, ok)
	assert.NotNil(t, clusterRoleModel)
	assert.Equal(t, "test-clusterrole", clusterRoleModel.Metadata.Name)
	assert.Equal(t, "test-namespace", clusterRoleModel.Metadata.Namespace)
	assert.Len(t, clusterRoleModel.Rules, 2)
	assert.Len(t, clusterRoleModel.AggregationRules, 1)
}

func TestClusterRoleHandlers_ResourceList(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	// Create test cluster roles
	clusterRole1 := createTestClusterRole("clusterrole-1", "namespace-1")
	clusterRole2 := createTestClusterRole("clusterrole-2", "namespace-2")

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
	resourceList := []*rbacv1.ClusterRole{clusterRole1, clusterRole2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*rbacv1.ClusterRole)
	assert.True(t, ok)
	assert.Equal(t, "clusterrole-1", resource1.Name)
	assert.NotSame(t, clusterRole1, resource1) // Should be a copy

	resource2, ok := resources[1].(*rbacv1.ClusterRole)
	assert.True(t, ok)
	assert.Equal(t, "clusterrole-2", resource2.Name)
	assert.NotSame(t, clusterRole2, resource2) // Should be a copy
}

func TestClusterRoleHandlers_ResourceUID(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	clusterRole := createTestClusterRole("test-clusterrole", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	clusterRole.UID = expectedUID

	uid := handlers.ResourceUID(nil, clusterRole)
	assert.Equal(t, expectedUID, uid)
}

func TestClusterRoleHandlers_ResourceVersion(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	clusterRole := createTestClusterRole("test-clusterrole", "test-namespace")
	expectedVersion := "v123"
	clusterRole.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.ClusterRole{}

	version := handlers.ResourceVersion(nil, clusterRole, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestClusterRoleHandlers_BuildMessageBody(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	clusterRole1 := createTestClusterRole("clusterrole-1", "namespace-1")
	clusterRole2 := createTestClusterRole("clusterrole-2", "namespace-2")

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

	clusterRole1Model := k8sTransformers.ExtractClusterRole(ctx, clusterRole1)
	clusterRole2Model := k8sTransformers.ExtractClusterRole(ctx, clusterRole2)

	// Build message body
	resourceModels := []interface{}{clusterRole1Model, clusterRole2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorClusterRole)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.ClusterRoles, 2)
	assert.Equal(t, "clusterrole-1", collectorMsg.ClusterRoles[0].Metadata.Name)
	assert.Equal(t, "clusterrole-2", collectorMsg.ClusterRoles[1].Metadata.Name)
}

func TestClusterRoleHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	clusterRole := createTestClusterRole("test-clusterrole", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "ClusterRole",
			APIVersion:       "rbac.authorization.k8s.io/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.ClusterRole{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, clusterRole, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "ClusterRole", clusterRole.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", clusterRole.APIVersion)
}

func TestClusterRoleHandlers_AfterMarshalling(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	clusterRole := createTestClusterRole("test-clusterrole", "test-namespace")
	resourceModel := &model.ClusterRole{}

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
	testYAML := []byte(`{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"ClusterRole","metadata":{"name":"test"}}`)

	// Call AfterMarshalling
	skip := handlers.AfterMarshalling(ctx, clusterRole, resourceModel, testYAML)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestClusterRoleHandlers_GetMetadataTags(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	// Create cluster role model with tags
	clusterRoleModel := &model.ClusterRole{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, clusterRoleModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestClusterRoleHandlers_GetMetadataTags_InvalidType(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	// Pass invalid type
	tags := handlers.GetMetadataTags(nil, "invalid-type")

	// Should return nil
	assert.Nil(t, tags)
}

func TestClusterRoleHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &ClusterRoleHandlers{}

	// Create cluster role with sensitive annotations and labels
	clusterRole := createTestClusterRole("test-clusterrole", "test-namespace")
	clusterRole.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	clusterRole.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, clusterRole)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", clusterRole.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", clusterRole.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestClusterRoleProcessor_Process(t *testing.T) {
	// Create test cluster roles with unique UIDs
	clusterRole1 := createTestClusterRole("clusterrole-1", "namespace-1")
	clusterRole1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	clusterRole1.ResourceVersion = "1200"

	clusterRole2 := createTestClusterRole("clusterrole-2", "namespace-2")
	clusterRole2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	clusterRole2.ResourceVersion = "1300"

	// Create fake client
	client := fake.NewClientset(clusterRole1, clusterRole2)
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
			NodeType:         orchestrator.K8sClusterRole,
			Kind:             "ClusterRole",
			APIVersion:       "rbac.authorization.k8s.io/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process cluster roles
	processor := processors.NewProcessor(&ClusterRoleHandlers{})
	result, listed, processed := processor.Process(ctx, []*rbacv1.ClusterRole{clusterRole1, clusterRole2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorClusterRole)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.ClusterRoles, 2)

	expectedClusterRole1 := k8sTransformers.ExtractClusterRole(ctx, clusterRole1)

	assert.Equal(t, expectedClusterRole1.Metadata, metaMsg.ClusterRoles[0].Metadata)
	assert.Equal(t, expectedClusterRole1.Rules, metaMsg.ClusterRoles[0].Rules)
	assert.Equal(t, expectedClusterRole1.AggregationRules, metaMsg.ClusterRoles[0].AggregationRules)
	assert.Equal(t, expectedClusterRole1.Tags, metaMsg.ClusterRoles[0].Tags)

	// Validate manifest message
	manifestMsg, ok := result.ManifestMessages[0].(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", manifestMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", manifestMsg.ClusterId)
	assert.Equal(t, int32(1), manifestMsg.GroupId)
	assert.Equal(t, "test-host", manifestMsg.HostName)
	assert.Len(t, manifestMsg.Manifests, 2)
	assert.Equal(t, manifestMsg.OriginCollector, model.OriginCollector_datadogAgent)
	assert.Equal(t, manifestMsg.OriginCollector, model.OriginCollector_datadogAgent)

	// Validate manifest details
	manifest1 := manifestMsg.Manifests[0]
	assert.Equal(t, clusterRole1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, clusterRole1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(14), manifest1.Type) // K8sClusterRole
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestClusterRole rbacv1.ClusterRole
	err := json.Unmarshal(manifest1.Content, &actualManifestClusterRole)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestClusterRole.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestClusterRole.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, clusterRole1.ObjectMeta, actualManifestClusterRole.ObjectMeta)
	assert.Equal(t, clusterRole1.Rules, actualManifestClusterRole.Rules)
	assert.Equal(t, clusterRole1.AggregationRule, actualManifestClusterRole.AggregationRule)
}

func createTestClusterRole(name, namespace string) *rbacv1.ClusterRole {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &rbacv1.ClusterRole{
		AggregationRule: &rbacv1.AggregationRule{
			ClusterRoleSelectors: []metav1.LabelSelector{
				{
					MatchLabels: map[string]string{"rbac.example.com/aggregate-to-edit": "true"},
				},
			},
		},
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
			ResourceVersion: "1200",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"nodes", "pods", "services"},
				Verbs:     []string{"get", "patch", "list"},
			},
			{
				APIGroups: []string{"batch"},
				Resources: []string{"cronjobs", "jobs"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}
}
