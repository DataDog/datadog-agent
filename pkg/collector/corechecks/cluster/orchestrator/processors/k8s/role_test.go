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

func TestRoleHandlers_ExtractResource(t *testing.T) {
	handlers := &RoleHandlers{}

	// Create test role
	role := createTestRole()

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
	resourceModel := handlers.ExtractResource(ctx, role)

	// Validate extraction
	roleModel, ok := resourceModel.(*model.Role)
	assert.True(t, ok)
	assert.NotNil(t, roleModel)
	assert.Equal(t, "test-role", roleModel.Metadata.Name)
	assert.Equal(t, "default", roleModel.Metadata.Namespace)
	assert.Len(t, roleModel.Rules, 3)
	assert.Equal(t, "nodes", roleModel.Rules[0].Resources[0])
}

func TestRoleHandlers_ResourceList(t *testing.T) {
	handlers := &RoleHandlers{}

	// Create test roles
	role1 := createTestRole()
	role2 := createTestRole()
	role2.Name = "role2"
	role2.UID = "uid2"

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
	resourceList := []*rbacv1.Role{role1, role2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*rbacv1.Role)
	assert.True(t, ok)
	assert.Equal(t, "test-role", resource1.Name)
	assert.NotSame(t, role1, resource1) // Should be a copy

	resource2, ok := resources[1].(*rbacv1.Role)
	assert.True(t, ok)
	assert.Equal(t, "role2", resource2.Name)
	assert.NotSame(t, role2, resource2) // Should be a copy
}

func TestRoleHandlers_ResourceUID(t *testing.T) {
	handlers := &RoleHandlers{}

	role := createTestRole()
	expectedUID := types.UID("test-role-uid")
	role.UID = expectedUID

	uid := handlers.ResourceUID(nil, role)
	assert.Equal(t, expectedUID, uid)
}

func TestRoleHandlers_ResourceVersion(t *testing.T) {
	handlers := &RoleHandlers{}

	role := createTestRole()
	expectedVersion := "123"
	role.ResourceVersion = expectedVersion

	version := handlers.ResourceVersion(nil, role, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestRoleHandlers_BuildMessageBody(t *testing.T) {
	handlers := &RoleHandlers{}

	role1 := createTestRole()
	role2 := createTestRole()
	role2.Name = "role2"

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

	role1Model := k8sTransformers.ExtractRole(ctx, role1)
	role2Model := k8sTransformers.ExtractRole(ctx, role2)

	// Build message body
	resourceModels := []interface{}{role1Model, role2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorRole)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.Roles, 2)
	assert.Equal(t, "test-role", collectorMsg.Roles[0].Metadata.Name)
	assert.Equal(t, "role2", collectorMsg.Roles[1].Metadata.Name)
}

func TestRoleHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &RoleHandlers{}

	role := createTestRole()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "Role",
			APIVersion:       "rbac.authorization.k8s.io/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	resourceModel := &model.Role{}
	skip := handlers.BeforeMarshalling(ctx, role, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "Role", role.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", role.APIVersion)
}

func TestRoleHandlers_AfterMarshalling(t *testing.T) {
	handlers := &RoleHandlers{}

	role := createTestRole()

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
	resourceModel := &model.Role{}

	// Create test YAML
	testYAML := []byte("apiVersion: rbac.authorization.k8s.io/v1\nkind: Role\nmetadata:\n  name: test-role")

	// Call AfterMarshalling
	skip := handlers.AfterMarshalling(ctx, role, resourceModel, testYAML)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestRoleHandlers_GetMetadataTags(t *testing.T) {
	handlers := &RoleHandlers{}

	// Create a role model with tags
	roleModel := &model.Role{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	tags := handlers.GetMetadataTags(nil, roleModel)
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestRoleHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &RoleHandlers{}

	// Create role with sensitive annotations and labels
	role := createTestRole()
	role.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	role.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, role)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", role.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", role.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestRoleProcessor_Process(t *testing.T) {
	// Create test roles with unique UIDs
	role1 := createTestRole()
	role1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	role1.ResourceVersion = "1220"

	role2 := createTestRole()
	role2.Name = "role2"
	role2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	role2.ResourceVersion = "1320"

	// Create fake client
	client := fake.NewClientset(role1, role2)
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
			NodeType:         orchestrator.K8sRole,
			Kind:             "Role",
			APIVersion:       "rbac.authorization.k8s.io/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process roles
	processor := processors.NewProcessor(&RoleHandlers{})
	result, listed, processed := processor.Process(ctx, []*rbacv1.Role{role1, role2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorRole)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.Roles, 2)

	expectedRole1 := k8sTransformers.ExtractRole(ctx, role1)

	assert.Equal(t, expectedRole1.Metadata, metaMsg.Roles[0].Metadata)
	assert.Equal(t, expectedRole1.Rules, metaMsg.Roles[0].Rules)
	assert.Equal(t, expectedRole1.Tags, metaMsg.Roles[0].Tags)

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
	assert.Equal(t, role1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, role1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(12), manifest1.Type) // K8sRole
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestRole rbacv1.Role
	err := json.Unmarshal(manifest1.Content, &actualManifestRole)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestRole.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestRole.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, role1.ObjectMeta, actualManifestRole.ObjectMeta)
	assert.Equal(t, role1.Rules, actualManifestRole.Rules)
}

// Helper function to create a test role
func createTestRole() *rbacv1.Role {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-role",
			Namespace:       "default",
			UID:             "test-role-uid",
			ResourceVersion: "1220",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
			CreationTimestamp: creationTime,
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
			{
				APIGroups: []string{"rbac.authorization.k8s.io"},
				Resources: []string{"rolebindings"},
				Verbs:     []string{"create"},
			},
		},
	}
}
