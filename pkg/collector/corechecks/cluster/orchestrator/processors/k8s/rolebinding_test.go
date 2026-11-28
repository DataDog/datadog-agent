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

func TestRoleBindingHandlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.RoleBinding{}
	resource := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-rolebinding",
			Namespace: "test-ns",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("rbac.authorization.k8s.io", "rolebindings")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), resource.Namespace, resource.Name)),
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewRoleBindingHandlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
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
	processor := processors.NewProcessor(NewRoleBindingHandlers(processorstest.NewEmptyFakeTagger()))
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
