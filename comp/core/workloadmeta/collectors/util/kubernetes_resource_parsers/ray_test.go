// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package kubernetesresourceparsers

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
)

func TestRayServiceParser(t *testing.T) {
	for _, tt := range []struct {
		name, config, image string
		want                bool
	}{
		{name: "custom image with LLM config", config: `applications: [{import_path: 'llm_service.custom_vllm_app:build_custom_vllm_app', args: {llm_configs: [model.yaml]}}]`, image: "private/ray-llm-service:vllm-stable", want: true},
		{name: "generic service", config: `applications: [{import_path: 'image_classifier:app'}]`, image: "rayproject/ray:2.59.0"},
		{name: "image fallback", image: "rayproject/ray-llm:2.59.0", want: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			obj := &unstructured.Unstructured{Object: map[string]interface{}{
				"metadata": map[string]interface{}{"name": "nlq-metrics-qwen3", "namespace": "ray-llm"},
				"spec":     map[string]interface{}{"serveConfigV2": tt.config, "rayClusterConfig": map[string]interface{}{"headGroupSpec": map[string]interface{}{"template": map[string]interface{}{"spec": map[string]interface{}{"containers": []interface{}{map[string]interface{}{"name": "ray-head", "image": tt.image}}}}}}},
				"status":   map[string]interface{}{"activeServiceStatus": map[string]interface{}{"rayClusterName": "nlq-metrics-qwen3-ldcdx"}, "pendingServiceStatus": map[string]interface{}{"rayClusterName": "nlq-metrics-qwen3-4spzc"}},
			}}
			parser, err := NewRayServiceParser(schema.GroupVersionResource{Group: "ray.io", Version: "v1", Resource: "rayservices"})
			require.NoError(t, err)
			entity := parser.Parse(obj).(*workloadmeta.KubernetesMetadata)
			require.NotNil(t, entity.RayService)
			assert.Equal(t, tt.want, entity.RayService.IsLLM)
			assert.Equal(t, []string{"nlq-metrics-qwen3-ldcdx", "nlq-metrics-qwen3-4spzc"}, entity.RayService.ClusterNames)
			assert.Empty(t, entity.Annotations)
		})
	}
}
