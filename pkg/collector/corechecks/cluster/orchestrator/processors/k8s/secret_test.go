// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver && test

package k8s

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func createTestSecret(name string) *metav1.PartialObjectMetadata {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	secret := &metav1.PartialObjectMetadata{
		// The metadata API reports objects as PartialObjectMetadata.
		TypeMeta: metav1.TypeMeta{
			Kind:       "PartialObjectMetadata",
			APIVersion: "meta.k8s.io/v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "test-namespace",
			UID:               "test-secret-uid-123",
			ResourceVersion:   "1234",
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
		},
	}
	secret.ManagedFields = []metav1.ManagedFieldsEntry{
		{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply},
	}
	return secret
}

func newSecretProcessorContext(cfg *orchestratorconfig.OrchestratorConfig) *processors.K8sProcessorContext {
	return &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "Secret",
			APIVersion:       "v1",
			NodeType:         orchestrator.K8sSecret,
		},
		APIClient: &apiserver.APIClient{Cl: fake.NewClientset()},
		HostName:  "test-host",
	}
}

func TestSecretHandlers_ResourceList(t *testing.T) {
	handlers := NewSecretHandlers()

	secret1 := createTestSecret("secret-1")
	secret2 := createTestSecret("secret-2")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newSecretProcessorContext(cfg)

	resources := handlers.ResourceList(ctx, []*metav1.PartialObjectMetadata{secret1, secret2})

	assert.Len(t, resources, 2)

	r1, ok := resources[0].(*metav1.PartialObjectMetadata)
	assert.True(t, ok)
	assert.Equal(t, "secret-1", r1.Name)
	assert.Same(t, secret1, r1)

	r2, ok := resources[1].(*metav1.PartialObjectMetadata)
	assert.True(t, ok)
	assert.Equal(t, "secret-2", r2.Name)
	assert.Same(t, secret2, r2)
}

func TestSecretHandlers_ResourceUID(t *testing.T) {
	handlers := NewSecretHandlers()

	secret := createTestSecret("test-secret")
	expectedUID := types.UID("test-uid-456")
	secret.UID = expectedUID

	uid := handlers.ResourceUID(nil, secret)
	assert.Equal(t, expectedUID, uid)
}

func TestSecretHandlers_ResourceVersion(t *testing.T) {
	handlers := NewSecretHandlers()

	secret := createTestSecret("test-secret")
	secret.ResourceVersion = "v999"

	version := handlers.ResourceVersion(nil, secret, nil)
	assert.Equal(t, "v999", version)
}

func TestSecretHandlers_ResourceVersionFromRaw(t *testing.T) {
	handlers := NewSecretHandlers()

	secret := createTestSecret("test-secret")
	secret.ResourceVersion = "v42"

	version := handlers.ResourceVersionFromRaw(nil, secret)
	assert.Equal(t, "v42", version)
}

func TestSecretHandlers_BeforeMarshalling(t *testing.T) {
	handlers := NewSecretHandlers()

	secret := createTestSecret("test-secret")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newSecretProcessorContext(cfg)

	skip := handlers.BeforeMarshalling(ctx, secret, nil)

	assert.False(t, skip)
	assert.Equal(t, "Secret", secret.Kind)
	assert.Equal(t, "v1", secret.APIVersion)
}

func TestSecretHandlers_AfterMarshalling(t *testing.T) {
	handlers := NewSecretHandlers()

	secret := createTestSecret("test-secret")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newSecretProcessorContext(cfg)

	testYAML := []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"test"}}`)
	skip := handlers.AfterMarshalling(ctx, secret, nil, testYAML)
	assert.False(t, skip)
}

func TestSecretHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := NewSecretHandlers()

	secret := createTestSecret("test-secret")
	secret.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	secret.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newSecretProcessorContext(cfg)

	handlers.ScrubBeforeExtraction(ctx, secret)

	assert.Equal(t, "-", secret.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", secret.Labels["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "my-annotation", secret.Annotations["annotation"])
	assert.Equal(t, "my-app", secret.Labels["app"])
}

func TestSecretHandlers_ScrubBeforeMarshalling(t *testing.T) {
	handlers := NewSecretHandlers()

	secret := createTestSecret("test-secret")
	assert.NotEmpty(t, secret.ManagedFields)

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newSecretProcessorContext(cfg)

	handlers.ScrubBeforeMarshalling(ctx, secret)

	assert.Nil(t, secret.ManagedFields)
}

func TestSecretHandlers_CloneResource(t *testing.T) {
	handlers := NewSecretHandlers()

	original := createTestSecret("test-secret")
	cloned := handlers.CloneResource(original)

	clonedTyped, ok := cloned.(*metav1.PartialObjectMetadata)
	assert.True(t, ok)
	assert.NotSame(t, original, clonedTyped)
	assert.Equal(t, original, clonedTyped)

	// Mutating the clone must not affect the original (informer cache protection).
	clonedTyped.ManagedFields = nil
	clonedTyped.Annotations["annotation"] = "changed"
	assert.NotEmpty(t, original.ManagedFields)
	assert.Equal(t, "my-annotation", original.Annotations["annotation"])
}

func TestSecretHandlers_BuildManifestMessageBody(t *testing.T) {
	handlers := NewSecretHandlers()

	manifest1 := &model.Manifest{
		Uid:             "test-uid-1",
		ResourceVersion: "1203",
		Type:            int32(orchestrator.K8sSecret),
		Version:         "v1",
		ContentType:     "json",
		Content:         []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"secret-1"}}`),
	}
	manifest2 := &model.Manifest{
		Uid:             "test-uid-2",
		ResourceVersion: "5678",
		Type:            int32(orchestrator.K8sSecret),
		Version:         "v1",
		ContentType:     "json",
		Content:         []byte(`{"apiVersion":"v1","kind":"Secret","metadata":{"name":"secret-2"}}`),
	}

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"
	ctx := newSecretProcessorContext(cfg)

	messageBody := handlers.BuildManifestMessageBody(ctx, []interface{}{manifest1, manifest2}, 2)

	collectorMsg, ok := messageBody.(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.Manifests, 2)
}

func TestSecretProcessor_Process(t *testing.T) {
	secret1 := createTestSecret("secret-1")
	secret1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	secret1.ResourceVersion = "1203"

	secret2 := createTestSecret("secret-2")
	secret2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	secret2.ResourceVersion = "1303"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"
	ctx := newSecretProcessorContext(cfg)

	processor := processors.NewProcessor(NewSecretHandlers())
	result, listed, processed := processor.Process(ctx, []*metav1.PartialObjectMetadata{secret1, secret2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)

	// Secret is manifest-only: metadata messages are built but nil.
	assert.Len(t, result.MetadataMessages, 1)
	assert.Nil(t, result.MetadataMessages[0])
	assert.Len(t, result.ManifestMessages, 1)

	collectorMsg, ok := result.ManifestMessages[0].(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, "test-host", collectorMsg.HostName)
	assert.Len(t, collectorMsg.Manifests, 2)

	manifest1 := collectorMsg.Manifests[0]
	assert.Equal(t, string(secret1.UID), manifest1.Uid)
	assert.Equal(t, secret1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(orchestrator.K8sSecret), manifest1.Type)
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// The manifest is emitted as a Secret, not as PartialObjectMetadata.
	var parsed map[string]interface{}
	err := yaml.Unmarshal(manifest1.Content, &parsed)
	assert.NoError(t, err)
	assert.Equal(t, "Secret", parsed["kind"])
	assert.Equal(t, "v1", parsed["apiVersion"])
	assert.NotContains(t, parsed, "data")
	assert.NotContains(t, parsed, "stringData")

	metadata, ok := parsed["metadata"].(map[string]interface{})
	assert.True(t, ok)
	assert.NotContains(t, metadata, "managedFields", "managedFields must be stripped before marshalling")
	assert.Equal(t, "secret-1", metadata["name"])
	assert.Equal(t, string(secret1.UID), metadata["uid"])

	// Verify the informer cache is unaffected.
	assert.Equal(t, "PartialObjectMetadata", secret1.Kind)
	assert.NotEmpty(t, secret1.ManagedFields)
}
