// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build orchestrator && test

package k8s

import (
	"testing"
	"time"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"go.yaml.in/yaml/v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/fake"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
	"github.com/DataDog/datadog-agent/pkg/util/kubernetes/apiserver"
)

func createTestConfigMap(name string) *corev1.ConfigMap {
	creationTime := metav1.NewTime(time.Date(2021, time.April, 16, 14, 30, 0, 0, time.UTC))

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "test-namespace",
			UID:               "test-configmap-uid-123",
			ResourceVersion:   "1234",
			CreationTimestamp: creationTime,
			Labels: map[string]string{
				"app": "my-app",
			},
			Annotations: map[string]string{
				"annotation": "my-annotation",
			},
		},
		Data: map[string]string{
			"key1": "value1",
			"key2": "value2",
		},
		BinaryData: map[string][]byte{
			"bin-key": []byte("binary-value"),
		},
	}
	cm.ManagedFields = []metav1.ManagedFieldsEntry{
		{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply},
	}
	return cm
}

func newConfigMapProcessorContext(cfg *orchestratorconfig.OrchestratorConfig) *processors.K8sProcessorContext {
	return &processors.K8sProcessorContext{
		BaseProcessorContext: processors.BaseProcessorContext{
			Cfg:              cfg,
			Clock:            clock.New(),
			ClusterID:        "test-cluster-id",
			MsgGroupID:       1,
			ManifestProducer: true,
			Kind:             "ConfigMap",
			APIVersion:       "v1",
			NodeType:         orchestrator.K8sConfigMap,
		},
		APIClient: &apiserver.APIClient{Cl: fake.NewClientset()},
		HostName:  "test-host",
	}
}

func TestConfigMapHandlers_ResourceList(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm1 := createTestConfigMap("cm-1")
	cm2 := createTestConfigMap("cm-2")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	resources := handlers.ResourceList(ctx, []*corev1.ConfigMap{cm1, cm2})

	assert.Len(t, resources, 2)

	r1, ok := resources[0].(*corev1.ConfigMap)
	assert.True(t, ok)
	assert.Equal(t, "cm-1", r1.Name)
	assert.Same(t, cm1, r1)

	r2, ok := resources[1].(*corev1.ConfigMap)
	assert.True(t, ok)
	assert.Equal(t, "cm-2", r2.Name)
	assert.Same(t, cm2, r2)
}

func TestConfigMapHandlers_ResourceUID(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	expectedUID := types.UID("test-uid-456")
	cm.UID = expectedUID

	uid := handlers.ResourceUID(nil, cm)
	assert.Equal(t, expectedUID, uid)
}

func TestConfigMapHandlers_ResourceVersion(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.ResourceVersion = "v999"

	version := handlers.ResourceVersion(nil, cm, nil)
	assert.Equal(t, "v999", version)
}

func TestConfigMapHandlers_ResourceVersionFromRaw(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.ResourceVersion = "v42"

	version := handlers.ResourceVersionFromRaw(nil, cm)
	assert.Equal(t, "v42", version)
}

func TestConfigMapHandlers_BeforeMarshalling(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	skip := handlers.BeforeMarshalling(ctx, cm, nil)

	assert.False(t, skip)
	assert.Equal(t, "ConfigMap", cm.Kind)
	assert.Equal(t, "v1", cm.APIVersion)
}

func TestConfigMapHandlers_AfterMarshalling(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	testYAML := []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"test"}}`)
	skip := handlers.AfterMarshalling(ctx, cm, nil, testYAML)
	assert.False(t, skip)
}

func TestConfigMapHandlers_ScrubBeforeExtraction(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	cm.Annotations["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"
	cm.Labels["kubectl.kubernetes.io/last-applied-configuration"] = "secret-value"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	handlers.ScrubBeforeExtraction(ctx, cm)

	assert.Equal(t, "-", cm.Annotations["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "-", cm.Labels["kubectl.kubernetes.io/last-applied-configuration"])
	assert.Equal(t, "my-annotation", cm.Annotations["annotation"])
	assert.Equal(t, "my-app", cm.Labels["app"])
}

func TestConfigMapHandlers_ScrubBeforeMarshalling(t *testing.T) {
	handlers := NewConfigMapHandlers()

	cm := createTestConfigMap("test-cm")
	assert.NotEmpty(t, cm.Data)
	assert.NotEmpty(t, cm.BinaryData)
	assert.NotEmpty(t, cm.ManagedFields)

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	ctx := newConfigMapProcessorContext(cfg)

	handlers.ScrubBeforeMarshalling(ctx, cm)

	assert.Nil(t, cm.Data)
	assert.Nil(t, cm.BinaryData)
	assert.Nil(t, cm.ManagedFields)
}

func TestConfigMapHandlers_CloneResource(t *testing.T) {
	handlers := NewConfigMapHandlers()

	original := createTestConfigMap("test-cm")
	cloned := handlers.CloneResource(original)

	clonedTyped, ok := cloned.(*corev1.ConfigMap)
	assert.True(t, ok)
	assert.NotSame(t, original, clonedTyped)
	assert.Equal(t, original, clonedTyped)

	// Mutating the clone must not affect the original (informer cache protection).
	clonedTyped.Data = nil
	clonedTyped.BinaryData = nil
	clonedTyped.ManagedFields = nil
	assert.NotEmpty(t, original.Data)
	assert.NotEmpty(t, original.BinaryData)
	assert.NotEmpty(t, original.ManagedFields)
}

func TestConfigMapHandlers_BuildManifestMessageBody(t *testing.T) {
	handlers := NewConfigMapHandlers()

	manifest1 := &model.Manifest{
		Uid:             "test-uid-1",
		ResourceVersion: "1203",
		Type:            int32(orchestrator.K8sConfigMap),
		Version:         "v1",
		ContentType:     "json",
		Content:         []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm-1"}}`),
	}
	manifest2 := &model.Manifest{
		Uid:             "test-uid-2",
		ResourceVersion: "5678",
		Type:            int32(orchestrator.K8sConfigMap),
		Version:         "v1",
		ContentType:     "json",
		Content:         []byte(`{"apiVersion":"v1","kind":"ConfigMap","metadata":{"name":"cm-2"}}`),
	}

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"
	ctx := newConfigMapProcessorContext(cfg)

	messageBody := handlers.BuildManifestMessageBody(ctx, []interface{}{manifest1, manifest2}, 2)

	collectorMsg, ok := messageBody.(*model.CollectorManifest)
	assert.True(t, ok)
	assert.Equal(t, "test-cluster", collectorMsg.ClusterName)
	assert.Equal(t, "test-cluster-id", collectorMsg.ClusterId)
	assert.Equal(t, int32(1), collectorMsg.GroupId)
	assert.Equal(t, int32(2), collectorMsg.GroupSize)
	assert.Len(t, collectorMsg.Manifests, 2)
}

func TestConfigMapProcessor_Process(t *testing.T) {
	cm1 := createTestConfigMap("cm-1")
	cm1.UID = types.UID("e42e5adc-0749-11e8-a2b8-000c29dea4f6")
	cm1.ResourceVersion = "1203"

	cm2 := createTestConfigMap("cm-2")
	cm2.UID = types.UID("f53f6bed-0749-11e8-a2b8-000c29dea4f7")
	cm2.ResourceVersion = "1303"

	cfg := orchestratorconfig.NewDefaultOrchestratorConfig(nil)
	cfg.KubeClusterName = "test-cluster"
	ctx := newConfigMapProcessorContext(cfg)

	processor := processors.NewProcessor(NewConfigMapHandlers())
	result, listed, processed := processor.Process(ctx, []*corev1.ConfigMap{cm1, cm2})

	assert.Equal(t, 2, listed)
	assert.Equal(t, 2, processed)

	// ConfigMap is manifest-only: metadata messages are built but nil.
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
	assert.Equal(t, string(cm1.UID), manifest1.Uid)
	assert.Equal(t, cm1.ResourceVersion, manifest1.ResourceVersion)
	assert.Equal(t, int32(orchestrator.K8sConfigMap), manifest1.Type)
	assert.Equal(t, "v1", manifest1.Version)
	assert.Equal(t, "json", manifest1.ContentType)

	// Data and BinaryData must be absent from the emitted manifest.
	var parsed map[string]interface{}
	err := yaml.Unmarshal(manifest1.Content, &parsed)
	assert.NoError(t, err)
	assert.NotContains(t, parsed, "data", "data must be stripped before marshalling")
	assert.NotContains(t, parsed, "binaryData", "binaryData must be stripped before marshalling")

	metadata, ok := parsed["metadata"].(map[string]interface{})
	assert.True(t, ok)
	assert.NotContains(t, metadata, "managedFields", "managedFields must be stripped before marshalling")
	assert.Equal(t, "cm-1", metadata["name"])
	assert.Equal(t, string(cm1.UID), metadata["uid"])

	// Verify the informer cache is unaffected: the originals still have their data.
	assert.NotEmpty(t, cm1.Data)
	assert.NotEmpty(t, cm1.BinaryData)
}
