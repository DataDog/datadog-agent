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

func TestClusterRoleBindingHandlers_ExtractResource(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	// Create test cluster role binding
	clusterRoleBinding := createTestClusterRoleBinding("test-clusterrolebinding", "test-namespace")

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
	resourceModel := handlers.ExtractResource(ctx, clusterRoleBinding)

	// Validate extraction
	clusterRoleBindingModel, ok := resourceModel.(*model.ClusterRoleBinding)
	assert.True(t, ok)
	assert.NotNil(t, clusterRoleBindingModel)
	assert.Equal(t, "test-clusterrolebinding", clusterRoleBindingModel.Metadata.Name)
	assert.Equal(t, "test-namespace", clusterRoleBindingModel.Metadata.Namespace)
	assert.NotNil(t, clusterRoleBindingModel.RoleRef)
	assert.Len(t, clusterRoleBindingModel.Subjects, 2)
}

func TestClusterRoleBindingHandlers_ResourceList(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	// Create test cluster role bindings
	clusterRoleBinding1 := createTestClusterRoleBinding("clusterrolebinding-1", "namespace-1")
	clusterRoleBinding2 := createTestClusterRoleBinding("clusterrolebinding-2", "namespace-2")

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
	resourceList := []*rbacv1.ClusterRoleBinding{clusterRoleBinding1, clusterRoleBinding2}
	resources := handlers.ResourceList(ctx, resourceList)

	// Validate conversion
	assert.Len(t, resources, 2)

	// Verify deep copy was made
	resource1, ok := resources[0].(*rbacv1.ClusterRoleBinding)
	assert.True(t, ok)
	assert.Equal(t, "clusterrolebinding-1", resource1.Name)
	assert.NotSame(t, clusterRoleBinding1, resource1) // Should be a copy

	resource2, ok := resources[1].(*rbacv1.ClusterRoleBinding)
	assert.True(t, ok)
	assert.Equal(t, "clusterrolebinding-2", resource2.Name)
	assert.NotSame(t, clusterRoleBinding2, resource2) // Should be a copy
}

func TestClusterRoleBindingHandlers_ResourceUID(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	clusterRoleBinding := createTestClusterRoleBinding("test-clusterrolebinding", "test-namespace")
	expectedUID := types.UID("test-uid-123")
	clusterRoleBinding.UID = expectedUID

	uid := handlers.ResourceUID(nil, clusterRoleBinding)
	assert.Equal(t, expectedUID, uid)
}

func TestClusterRoleBindingHandlers_ResourceVersion(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	clusterRoleBinding := createTestClusterRoleBinding("test-clusterrolebinding", "test-namespace")
	expectedVersion := "v123"
	clusterRoleBinding.ResourceVersion = expectedVersion

	// Create a mock resource model
	resourceModel := &model.ClusterRoleBinding{}

	version := handlers.ResourceVersion(nil, clusterRoleBinding, resourceModel)
	assert.Equal(t, expectedVersion, version)
}

func TestClusterRoleBindingHandlers_BuildMessageBody(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	clusterRoleBinding1 := createTestClusterRoleBinding("clusterrolebinding-1", "namespace-1")
	clusterRoleBinding2 := createTestClusterRoleBinding("clusterrolebinding-2", "namespace-2")

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

	clusterRoleBinding1Model := k8sTransformers.ExtractClusterRoleBinding(ctx, clusterRoleBinding1)
	clusterRoleBinding2Model := k8sTransformers.ExtractClusterRoleBinding(ctx, clusterRoleBinding2)

	// Build message body
	resourceModels := []interface{}{clusterRoleBinding1Model, clusterRoleBinding2Model}
	messageBody := handlers.BuildMessageBody(ctx, resourceModels, 2)

	// Validate message body
	collectorMsg, ok := messageBody.(*model.CollectorClusterRoleBinding)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.ClusterRoleBindings, 2)
	assert.Equal(t, "clusterrolebinding-1", collectorMsg.ClusterRoleBindings[0].Metadata.Name)
	assert.Equal(t, "clusterrolebinding-2", collectorMsg.ClusterRoleBindings[1].Metadata.Name)
}

func TestClusterRoleBindingHandlers_BeforeMarshalling(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	clusterRoleBinding := createTestClusterRoleBinding("test-clusterrolebinding", "test-namespace")

	// Create processor context
	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "ClusterRoleBinding",
			APIVersion:       "rbac.authorization.k8s.io/v1",
		},
		APIClient: &apiserver.APIClient{},
		HostName:  "test-host",
	}

	// Create resource model
	resourceModel := &model.ClusterRoleBinding{}

	// Call BeforeMarshalling
	skip := handlers.BeforeMarshalling(ctx, clusterRoleBinding, resourceModel)

	// Validate
	assert.False(t, skip)
	assert.Equal(t, "ClusterRoleBinding", clusterRoleBinding.Kind)
	assert.Equal(t, "rbac.authorization.k8s.io/v1", clusterRoleBinding.APIVersion)
}

func TestClusterRoleBindingHandlers_AfterMarshalling(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	clusterRoleBinding := createTestClusterRoleBinding("test-clusterrolebinding", "test-namespace")
	resourceModel := &model.ClusterRoleBinding{}

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
	testYAML := []byte(`{"apiVersion":"rbac.authorization.k8s.io/v1","kind":"ClusterRoleBinding","metadata":{"name":"test"}}`)

	skip := handlers.AfterMarshalling(ctx, clusterRoleBinding, resourceModel, testYAML)
	assert.False(t, skip)
	assert.Equal(t, testYAML, resourceModel.Yaml)
}

func TestClusterRoleBindingHandlers_GetMetadataTags(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	// Create cluster role binding model with tags
	clusterRoleBindingModel := &model.ClusterRoleBinding{
		Tags: []string{"tag1", "tag2", "tag3"},
	}

	// Get metadata tags
	tags := handlers.GetMetadataTags(nil, clusterRoleBindingModel)

	// Validate
	assert.Equal(t, []string{"tag1", "tag2", "tag3"}, tags)
}

func TestClusterRoleBindingHandlers_GetMetadataTags_InvalidType(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	// Pass invalid type
	tags := handlers.GetMetadataTags(nil, "invalid-type")

	// Should return nil
	assert.Nil(t, tags)
}

func TestClusterRoleBindingHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := &ClusterRoleBindingHandlers{}

	// Create cluster role binding with sensitive annotations and labels
	clusterRoleBinding := createTestClusterRoleBinding("test-clusterrolebinding", "test-namespace")
	clusterRoleBinding.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	clusterRoleBinding.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	// Call ScrubBeforeExtraction
	handlers.ScrubBeforeExtraction(nil, clusterRoleBinding)

	// Validate that sensitive data was removed
	assert.Equal(t, "-", clusterRoleBinding.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", clusterRoleBinding.Labels["kubectl.kubernetes.io/last-applied-configuration"])
}

func TestClusterRoleBindingProcessor_Process(t *testing.T) {
	// Create test cluster role bindings with unique UIDs
	clusterRoleBinding1 := createTestClusterRoleBinding("clusterrolebinding-1", "namespace-1")
	clusterRoleBinding1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	clusterRoleBinding1.ResourceVersion = "1201"

	clusterRoleBinding2 := createTestClusterRoleBinding("clusterrolebinding-2", "namespace-2")
	clusterRoleBinding2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	clusterRoleBinding2.ResourceVersion = "1301"

	// Create fake client
	client := fake.NewClientset(clusterRoleBinding1, clusterRoleBinding2)
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
			NodeType:         orchestrator.K8sClusterRoleBinding,
			Kind:             "ClusterRoleBinding",
			APIVersion:       "rbac.authorization.k8s.io/v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process cluster role bindings
	processor := processors.NewProcessor(&ClusterRoleBindingHandlers{})
	result, listed, processed := processor.Process(ctx, []*rbacv1.ClusterRoleBinding{clusterRoleBinding1, clusterRoleBinding2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorClusterRoleBinding)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.ClusterRoleBindings, 2)

	expectedClusterRoleBinding1 := k8sTransformers.ExtractClusterRoleBinding(ctx, clusterRoleBinding1)

	assert.Equal(t, expectedClusterRoleBinding1.Metadata, metaMsg.ClusterRoleBindings[0].Metadata)
	assert.Equal(t, expectedClusterRoleBinding1.RoleRef, metaMsg.ClusterRoleBindings[0].RoleRef)
	assert.Equal(t, expectedClusterRoleBinding1.Subjects, metaMsg.ClusterRoleBindings[0].Subjects)
	assert.Equal(t, expectedClusterRoleBinding1.Tags, metaMsg.ClusterRoleBindings[0].Tags)

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
	assert.Equal(t, clusterRoleBinding1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, clusterRoleBinding1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(15), manifest1.Type) // K8sClusterRoleBinding
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestClusterRoleBinding rbacv1.ClusterRoleBinding
	err := json.Unmarshal(manifest1.Content, &actualManifestClusterRoleBinding)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestClusterRoleBinding.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestClusterRoleBinding.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, clusterRoleBinding1.ObjectMeta, actualManifestClusterRoleBinding.ObjectMeta)
	assert.Equal(t, clusterRoleBinding1.RoleRef, actualManifestClusterRoleBinding.RoleRef)
	assert.Equal(t, clusterRoleBinding1.Subjects, actualManifestClusterRoleBinding.Subjects)
}

func createTestClusterRoleBinding(name, namespace string) *rbacv1.ClusterRoleBinding {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &rbacv1.ClusterRoleBinding{
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
			ResourceVersion: "1201",
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     "test-cluster-role",
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "test-service-account",
				Namespace: "default",
			},
			{
				APIGroup: "rbac.authorization.k8s.io",
				Kind:     "User",
				Name:     "test-user",
			},
		},
	}
}
