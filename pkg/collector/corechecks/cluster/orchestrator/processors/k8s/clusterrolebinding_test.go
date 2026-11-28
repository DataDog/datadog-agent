// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

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
	taggertypes "github.com/DataDog/datadog-agent/comp/core/tagger/types"
	wmutil "github.com/DataDog/datadog-agent/comp/core/workloadmeta/collectors/util"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processorstest"
	k8sTransformers "github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/transformers/k8s"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func TestClusterRoleBindingHandlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.ClusterRoleBinding{}
	resource := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-clusterrolebinding",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("rbac.authorization.k8s.io", "clusterrolebindings")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), resource.Namespace, resource.Name)),
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewClusterRoleBindingHandlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
}

func TestClusterRoleBindingHandlers_GetMetadataTags_InvalidType(t *testing.T) {
	handlers := NewClusterRoleBindingHandlers(nil)

	// Pass invalid type
	tags := handlers.GetMetadataTags(nil, "invalid-type")

	// Should return nil
	assert.Nil(t, tags)
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
	processor := processors.NewProcessor(NewClusterRoleBindingHandlers(processorstest.NewEmptyFakeTagger()))
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
