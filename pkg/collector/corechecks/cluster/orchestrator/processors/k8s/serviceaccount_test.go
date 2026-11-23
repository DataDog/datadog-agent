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
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

func TestServiceAccountHandlers_BeforeCacheCheck(t *testing.T) {
	resourceModel := &model.ServiceAccount{}
	resource := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-serviceaccount",
			Namespace: "test-ns",
		},
	}

	ctx := processorstest.NewProcessorContextBeforeCacheCheck("", "serviceaccounts")
	entityID := taggertypes.NewEntityID(
		taggertypes.KubernetesMetadata,
		string(wmutil.GenerateKubeMetadataEntityID(ctx.GetCollectorGroup(), ctx.GetCollectorName(), resource.Namespace, resource.Name)),
	)
	tagger := processorstest.NewFakeTagger(map[taggertypes.EntityID][]string{entityID: {"tagger-tag:value"}})
	handlers := NewServiceAccountHandlers(tagger)

	skip := handlers.EnrichModel(ctx, resource, resourceModel)
	assert.False(t, skip)
	assert.Equal(t, []string{"tagger-tag:value"}, resourceModel.Tags)
}

func TestServiceAccountProcessor_Process(t *testing.T) {
	// Create test serviceaccounts with unique UIDs
	serviceAccount1 := createTestServiceAccount()
	serviceAccount1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	serviceAccount1.ResourceVersion = "1223"

	serviceAccount2 := createTestServiceAccount()
	serviceAccount2.Name = "serviceaccount2"
	serviceAccount2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	serviceAccount2.ResourceVersion = "1323"

	// Create fake client
	client := fake.NewClientset(serviceAccount1, serviceAccount2)
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
			NodeType:         orchestrator.K8sServiceAccount,
			Kind:             "ServiceAccount",
			APIVersion:       "v1",
		},
		APIClient: apiClient,
		HostName:  "test-host",
	}

	// Create processor and process serviceaccounts
	processor := processors.NewProcessor(NewServiceAccountHandlers(processorstest.NewEmptyFakeTagger()))
	result, listed, processed := processor.Process(ctx, []*corev1.ServiceAccount{serviceAccount1, serviceAccount2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)
	assert.Len(t, result.MetadataMessages, 1)
	assert.Len(t, result.ManifestMessages, 1)

	// Validate metadata message
	metaMsg, ok := result.MetadataMessages[0].(*model.CollectorServiceAccount)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", metaMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", metaMsg.ClusterId)
	assert.Equal(t, int32(1), metaMsg.GroupId)
	assert.Equal(t, int32(1), metaMsg.GroupSize)
	assert.Len(t, metaMsg.ServiceAccounts, 2)

	expectedServiceAccount1 := k8sTransformers.ExtractServiceAccount(ctx, serviceAccount1)

	assert.Equal(t, expectedServiceAccount1.Metadata, metaMsg.ServiceAccounts[0].Metadata)
	assert.Equal(t, expectedServiceAccount1.Secrets, metaMsg.ServiceAccounts[0].Secrets)
	assert.Equal(t, expectedServiceAccount1.ImagePullSecrets, metaMsg.ServiceAccounts[0].ImagePullSecrets)
	assert.Equal(t, expectedServiceAccount1.AutomountServiceAccountToken, metaMsg.ServiceAccounts[0].AutomountServiceAccountToken)
	assert.Equal(t, expectedServiceAccount1.Tags, metaMsg.ServiceAccounts[0].Tags)

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
	assert.Equal(t, serviceAccount1.UID, types.UID(manifest1.Uid))
	assert.Equal(t, serviceAccount1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(16), manifest1.Type) // K8sServiceAccount
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Parse the actual manifest content
	var actualManifestServiceAccount corev1.ServiceAccount
	err := json.Unmarshal(manifest1.Content, &actualManifestServiceAccount)
	assert.NoError(t, err, "JSON should be valid")

	// converts time back to UTC for comparison
	actualManifestServiceAccount.ObjectMeta.CreationTimestamp = metav1.Time{Time: actualManifestServiceAccount.ObjectMeta.CreationTimestamp.Time.UTC()}
	assert.Equal(t, serviceAccount1.ObjectMeta, actualManifestServiceAccount.ObjectMeta)
	assert.Equal(t, serviceAccount1.Secrets, actualManifestServiceAccount.Secrets)
	assert.Equal(t, serviceAccount1.ImagePullSecrets, actualManifestServiceAccount.ImagePullSecrets)
}

// Helper function to create a test serviceaccount
func createTestServiceAccount() *corev1.ServiceAccount {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-serviceaccount",
			Namespace:       "default",
			UID:             "test-serviceaccount-uid",
			ResourceVersion: "1223",
			Labels: map[string]string{
				"app": "test-app",
			},
			Annotations: map[string]string{
				"test-annotation": "test-value",
			},
			CreationTimestamp: creationTime,
		},
		AutomountServiceAccountToken: pointer.Ptr(true),
		ImagePullSecrets: []corev1.LocalObjectReference{
			{
				Name: "registry-key",
			},
		},
		Secrets: []corev1.ObjectReference{
			{
				Name: "default-token-uudge",
			},
		},
	}
}
