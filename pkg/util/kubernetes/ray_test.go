// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package kubernetes

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRayLLMSignals(t *testing.T) {
	for _, tt := range []struct {
		image         string
		command, args []string
		want          bool
	}{
		{image: "rayproject/ray-llm:2.59.0", want: true},
		{image: "registry.example.com:5000/ray-llm@sha256:123", want: true},
		{image: "vllm/vllm-openai:latest", want: true},
		{image: "custom:latest", command: []string{"vllm", "serve", "Qwen/Qwen3-8B"}, want: true},
		{image: "custom:latest", command: []string{"python", "-m"}, args: []string{"vllm.entrypoints.openai.api_server"}, want: true},
		{command: []string{"python", "-m", "ray.serve.llm"}, want: true},
		{image: "rayproject/ray:2.59.0"}, {image: "rayproject/ray-ml:2.59.0"},
		{image: "vllm-registry.example.com/ray:2.59.0"},
		{image: "registry.example.com/vllm/ray:2.59.0"},
		{image: "rayproject/ray:vllm-tools"},
		// This is the private image from the supplied pod. Its tag is not an LLM signal.
		{image: "registry.example.com/apps/ray-llm-service:vllm-stable-nydus", command: []string{"ray", "start"}},
	} {
		assert.Equal(t, tt.want, IsRayLLMContainer(tt.image, tt.command, tt.args), "%+v", tt)
	}
}
func TestRayServiceLLMConfig(t *testing.T) {
	for _, tt := range []struct {
		config string
		want   bool
	}{
		{`applications: [{import_path: 'ray.serve.llm:build_openai_app'}]`, true},
		{`applications: [{import_path: 'custom:app', args: {llm_configs: [config.yaml]}}]`, true},
		{`applications: [{import_path: 'custom:app', runtime_env: {env_vars: {MODEL_ID: Qwen/Qwen3-8B}}}]`, true},
		{`applications: [{import_path: 'ray.serve.llm:build_pd_openai_app', args: {prefill_config: /mnt/config.yaml}}]`, true},
		{`applications: [{import_path: 'image_classifier:app'}]`, false},
		{`[invalid`, false},
	} {
		assert.Equal(t, tt.want, RayServiceHasLLMConfig(tt.config), tt.config)
	}
}
func TestRayWorkloadNameMatchesBackend(t *testing.T) {
	for _, tt := range []struct{ cluster, want string }{
		{"ray-aggregated-4spzc", "ray-aggregated"},
		{"nlq-metrics-qwen3-ldcdx", "nlq-metrics-qwen3"},
		{"ray-disaggregated-decode-2pnmw", "ray-disaggregated-decode"},
		{"ray-aggregated", "ray-aggregated"}, {"ray-serve", "ray-serve"}, {"ray", "ray"}, {"", ""},
	} {
		assert.Equal(t, tt.want, RayClusterWorkloadName(tt.cluster))
	}
}
