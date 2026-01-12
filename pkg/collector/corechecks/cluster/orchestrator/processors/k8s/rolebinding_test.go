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

func TestRoleBindingHandlers_ExtractResource(t *testing.T) {
	handlers := &RoleBindingHandlers{}

	// Create test rolebinding
	roleBinding := createTestRoleBinding()

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
	resourceModel := handlers.ExtractResource(ctx, roleBinding)

	// Validate extraction
	roleBindingModel, ok := resourceModel.(*model.RoleBinding)
	assert.True(t, ok)
	assert.NotNil(t, roleBindingModel)
	assert.Equal(t, "test-rolebinding", roleBindingModel.Metadata.Name)
	assert.Equal(t, "default", roleBindingModel.Metadata.Namespace)
	assert.Equal(t, "Role", roleBindingModel.RoleRef.Kind)
	assert.Equal(t, "my-role", roleBindingModel.RoleRef.Name)
	assert.Len(t, roleBindingModel.Subjects, 1)
	assert.Equal(t, "User", roleBindingModel.Subjects[0].Kind)
}

func TestRoleBindingHandlers_ResourceList(t *testing.T) {
	handlers := &RoleBindingHandlers{}

	// Create test rolebindings
	roleBinding1 := createTestRoleBinding()
	roleBinding2 := createTestRoleBinding()
	roleBinding2.Name = "rolebinding2"
	roleBinding2.UID = "uid2"

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
	resourceList := []*rbacv1.RoleBinding{roleBinding1, roleBinding2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*rbacv1.RoleBinding)
	assert.True(t, ok)
	assert.Equal(t, "test-rolebinding", resource1.Name)
	assert.NotSame(t, roleBinding1, resource1) // Should be a copy

	resource2, ok := resources[1].(*rbacv1.RoleBinding)
	assert.True(t, ok)
	assert.Equal(t, "rolebinding2", resource2.Name)
	assert.NotSame(t, roleBinding2, resource2) // Should be a copy
}

func TestRoleBindingHandlers_ResourceUID(t *testing.T) {
	handlers := &RoleBindingHandlers{}

	roleBinding := createTestRoleBinding()
	expectedUID := types.UID("test-rolebinding-uid")
	roleBinding.UID = expectedUID

	uid := handlers.ResourceUID(nil, roleBinding)
	assert.Equal(t, expectedUID, uid)
}

func TestRoleBindingHandlers_ResourceVersion(t *testing.T) {
	handlers := &RoleBindingHandlers{}

	roleBinding := createTestRoleBinding()
	expectedVersion := "123"
	roleBinding.ResourceVersion = expectedVersion

	version := handlers.ResourceVersion(nil, roleBinding, nil)
	assert.Equal(t, expectedVersion, version)
}

func TestRoleBindingHandlers_BuildMessageBody(t *testing.T) {
	handlers := &RoleBindingHandlers{}

	roleBinding1 := createTestRoleBinding()
	roleBinding2 := createTestRoleBinding()
	roleBinding2.Name = "rolebinding2"

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

	roleBinding1Model := k8sTransformers.ExtractRoleBinding(ctx, roleBinding1)
	roleBinding2Model := k8sTransformers.ExtractRoleBinding(ctx, roleBinding2)

	// Build message body
	resourceModels := []interface{}{roleBinding1Model, roleBinding2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorRoleBinding)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.RoleBindings, 2)
	assert.Equal(t, "test-rolebinding", collectorMsg.RoleBindings[0].Metadata.Name)
	assert.Equal(t, "rolebinding2", collectorMsg.RoleBindings[1].Metadata.Name)
}

func TestRoleBindingHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &RoleBindingHandlers{}

	roleBinding := createTestRoleBinding()

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "RoleBinding",
			APIVersion:       "rbac.authorization.k8s.io/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	resourceModel := &model.RoleBinding{}
	skip := handlers.BeforeMarshalling(ctx, roleBinding, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "RoleBinding", roleBinding.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", roleBinding.APIVersion)
}

func TestRoleBindingHandlers_AfterMarshalling(t *testing.T) {
	handlers := &RoleBindingHandlers{}

	roleBinding := createTestRoleBinding()

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
	resourceModel := &model.RoleBinding{}

	// Create test YAML
	testYAML := []byte("apiVersion: rbac.authorization.k8s.io/v1\nkind: RoleBinding\nmetadata:\n  name: test-rolebinding")

	// Call AfterMarshalling
	skip := handlers.AfterMarshalling(ctx, roleBinding, resourceModel, testYAML)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestRoleBindingHandlers_GetMetadataTags(t *testing.T) {
	handlers := &RoleBindingHandlers{}

	// Create a rolebinding model with tags
	roleBindingModel := &model.RoleBinding{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	tags := handlers.GetMetadataTags(nil, roleBindingModel)
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestRoleBindingHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &RoleBindingHandlers{}

	// Create rolebinding with sensitive annotations and labels
	roleBinding := createTestRoleBinding()
	roleBinding.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	roleBinding.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, roleBinding)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", roleBinding.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", roleBinding.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestRoleBindingProcessor_Process(t *testing.T) {
	// Create test rolebindings with unique UIDs
	roleBinding1 := createTestRoleBinding()
	roleBinding1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	roleBinding1.ResourceVersion = "1221"

	roleBinding2 := createTestRoleBinding()
	roleBinding2.Name = "rolebinding2"
	roleBinding2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	roleBinding2.ResourceVersion = "1321"

	// Create fake client
	client := fake.NewClientset(roleBinding1, roleBinding2)
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
			NodeType:         orchestrator.K8sRoleBinding,
			Kind:             "RoleBinding",
			APIVersion:       "rbac.authorization.k8s.io/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process rolebindings
	processor := processors.NewProcessor(&RoleBindingHandlers{})
	result, listed, processed := processor.Process(ctx, []*rbacv1.RoleBinding{roleBinding1, roleBinding2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorRoleBinding)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.RoleBindings, 2)

	expectedRoleBinding1 := k8sTransformers.ExtractRoleBinding(ctx, roleBinding1)

	assert.Equal(t, expectedRoleBinding1.Metadata, metaMsg.RoleBindings[0].Metadata)
	assert.Equal(t, expectedRoleBinding1.RoleRef, metaMsg.RoleBindings[0].RoleRef)
	assert.Equal(t, expectedRoleBinding1.Subjects, metaMsg.RoleBindings[0].Subjects)
	assert.Equal(t, expectedRoleBinding1.Tags, metaMsg.RoleBindings[0].Tags)

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
	assert.Equal(t, roleBinding1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, roleBinding1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(13), manifest1.Type) // K8sRoleBinding
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestRoleBinding rbacv1.RoleBinding
	err := json.Unmarshal(manifest1.Content, &actualManifestRoleBinding)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestRoleBinding.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestRoleBinding.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, roleBinding1.ObjectMeta, actualManifestRoleBinding.ObjectMeta)
	assert.Equal(t, roleBinding1.RoleRef, actualManifestRoleBinding.RoleRef)
	assert.Equal(t, roleBinding1.Subjects, actualManifestRoleBinding.Subjects)
}

// Helper function to create a test rolebinding
func createTestRoleBinding() *rbacv1.RoleBinding {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-rolebinding",
			Namespace:       "default",
			UID:             "test-rolebinding-uid",
			ResourceVersion: "1221",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
			CreationTimestamp: creationTime,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "Role",
			Name:     "my-role",
		},
		Subjects: []rbacv1.Subject{
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "User",
				Name:     "firstname.lastname@company.com",
			},
		},
	}
}
