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
	corev1 "k8s.io/api/core/v1"
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

func TestNamespaceHandlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.Namespace{}
	resource := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-namespace",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("", "namespaces")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), resource.Namespace, resource.Name)),
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewNamespaceHandlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
}

func TestNamespaceProcessor_Process(t *testing.T) {
	// Create test namespaces with unique UIDs
	namespace1 := createTestNamespace("namespace-1")
	namespace1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	namespace1.ResourceVersion = "1212"

	namespace2 := createTestNamespace("namespace-2")
	namespace2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	namespace2.ResourceVersion = "1312"

	// Create fake client
	client := fake.NewClientset(namespace1, namespace2)
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
			NodeType:         orchestrator.K8sNamespace,
			Kind:             "Namespace",
			APIVersion:       "v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process namespaces
	processor := processors.NewProcessor(NewNamespaceHandlers(processorstest.NewEmptyFakeTagger()))
	result, listed, processed := processor.Process(ctx, []*corev1.Namespace{namespace1, namespace2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorNamespace)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.Namespaces, 2)

	expectedNamespace1 := k8sTransformers.ExtractNamespace(ctx, namespace1)

	assert.Equal(t, expectedNamespace1.Metadata, metaMsg.Namespaces[0].Metadata)
	assert.Equal(t, expectedNamespace1.Status, metaMsg.Namespaces[0].Status)
	assert.Equal(t, expectedNamespace1.Conditions, metaMsg.Namespaces[0].Conditions)
	assert.Equal(t, expectedNamespace1.ConditionMessage, metaMsg.Namespaces[0].ConditionMessage)
	assert.Equal(t, expectedNamespace1.Tags, metaMsg.Namespaces[0].Tags)

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
	assert.Equal(t, namespace1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, namespace1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(19), manifest1.Type) // K8sNamespace
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestNamespace corev1.Namespace
	err := json.Unmarshal(manifest1.Content, &actualManifestNamespace)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestNamespace.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestNamespace.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, namespace1.ObjectMeta, actualManifestNamespace.ObjectMeta)
	assert.Equal(t, namespace1.Spec, actualManifestNamespace.Spec)
	assert.Equal(t, namespace1.Status, actualManifestNamespace.Status)
}

func createTestNamespace(name string) *corev1.Namespace {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Name:            name,
			ResourceVersion: "1212",
			Finalizers:      []string{"final", "izers"},
			UID:             types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6"),
		},
		Status: corev1.NamespaceStatus{
			Phase: "Active",
			Conditions: []corev1.NamespaceCondition{
				{
					Type:    "NamespaceFinalizersRemaining",
					Status:  "False",
					Message: "wrong msg",
				},
				{
					Type:    "NamespaceDeletionContentFailure",
					Status:  "True",
					Message: "also the wrong msg",
				},
				{
					Type:    "NamespaceDeletionDiscoveryFailure",
					Status:  "True",
					Message: "right msg",
				},
			},
		},
	}
}
