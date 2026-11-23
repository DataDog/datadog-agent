// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver && test

package common

import (
	"encoding/json"
	"testing"

	"github.com/benbjohnson/clock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/cluster/orchestrator/processors"
	"github.com/DataDog/datadog-agent/pkg/orchestrator"
	orchestratorconfig "github.com/DataDog/datadog-agent/pkg/orchestrator/config"
)

func TestManifestHandlers(t *testing.T) {
	objectMeta := metav1.ObjectMeta{
		Name:            "name",
		Namespace:       "default",
		UID:             "uid",
		ResourceVersion: "1",
		ManagedFields:   []metav1.ManagedFieldsEntry{{Manager: "kubectl", Operation: metav1.ManagedFieldsOperationApply}},
	}

	t.Run("metadata-only resource", func(t *testing.T) {
		// The metadata API reports objects as PartialObjectMetadata, without their data.
		configMap := &metav1.PartialObjectMetadata{TypeMeta: metav1.TypeMeta{APIVersion: "meta.k8s.io/v1", Kind: "PartialObjectMetadata"}, ObjectMeta: objectMeta}
		content := testManifestHandlers(t, configMap)
		assert.NotContains(t, content, "data")
		assert.Equal(t, "PartialObjectMetadata", configMap.Kind, "the informer cache is unchanged")
		assert.NotEmpty(t, configMap.ManagedFields, "the informer cache is unchanged")
	})
	t.Run("typed resource", func(t *testing.T) {
		helmRelease := &corev1.ConfigMap{ObjectMeta: objectMeta, Data: map[string]string{"release": "encoded"}}
		content := testManifestHandlers(t, helmRelease)
		assert.Equal(t, map[string]any{"release": "encoded"}, content["data"])
	})
}

// testManifestHandlers processes a ConfigMap and returns the content of its manifest.
func testManifestHandlers[P TypedObject[P]](t *testing.T, resource P) map[string]any {
	ctx := &processors.K8sProcessorContext{BaseProcessorContext: processors.BaseProcessorContext{
		Cfg:              orchestratorconfig.NewDefaultOrchestratorConfig(nil),
		Clock:            clock.New(),
		ManifestProducer: true,
		Kind:             "ConfigMap",
		APIVersion:       "v1",
		NodeType:         orchestrator.K8sConfigMap,
	}}
	ctx.Cfg.IsManifestCollectionEnabled = true
	orchestrator.KubernetesResourceCache = orchestrator.NewKubernetesResourceCache()

	result, listed, processed := processors.NewProcessor(new(ManifestHandlers[P])).Process(ctx, []P{resource})
	require.Equal(t, 1, listed)
	require.Equal(t, 1, processed)

	// No structured metadata payload is built.
	assert.Equal(t, []model.MessageBody{nil}, result.MetadataMessages)
	require.Len(t, result.ManifestMessages, 1)
	manifests := result.ManifestMessages[0].(*model.CollectorManifest).Manifests
	require.Len(t, manifests, 1)

	var content map[string]any
	require.NoError(t, json.Unmarshal(manifests[0].Content, &content))
	assert.Equal(t, "ConfigMap", content["kind"])
	assert.Equal(t, "v1", content["apiVersion"])
	assert.NotContains(t, content["metadata"], "managedFields")
	return content
}
