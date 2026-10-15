// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package kubernetes

import (
	"path"
	"strings"

	"go.yaml.in/yaml/v3"
)

// IsRayLLMContainer follows the backend's Ray pod image and command signals.
// Registry names, image tags, GPU requests and Serve labels are not LLM signals.
func IsRayLLMContainer(image string, command, args []string) bool {
	name := image[strings.LastIndex(image, "/")+1:]
	name, _, _ = strings.Cut(name, "@")
	name, _, _ = strings.Cut(name, ":")
	name = strings.ToLower(name)
	if name == "ray-llm" || strings.Contains(name, "vllm") {
		return true
	}
	arguments := append(append([]string{}, command...), args...)
	joined := strings.Join(arguments, " ")
	if strings.Contains(joined, "ray.serve.llm") {
		return true
	}
	var tokens []string
	for _, argument := range arguments {
		tokens = append(tokens, strings.Fields(strings.ToLower(argument))...)
	}
	for i, token := range tokens {
		token = strings.Trim(token, "\"';")
		if token == "dynamo.vllm" || strings.HasPrefix(token, "dynamo.vllm.") || strings.HasPrefix(token, "vllm.entrypoints.") {
			return true
		}
		if path.Base(token) == "vllm" && i+1 < len(tokens) && tokens[i+1] == "serve" {
			return true
		}
	}
	return false
}

// RayServiceHasLLMConfig identifies LLM applications in serveConfigV2.
func RayServiceHasLLMConfig(config string) bool {
	var parsed struct {
		Applications []struct {
			ImportPath string `yaml:"import_path"`
			Args       struct {
				LLMConfigs    []any `yaml:"llm_configs"`
				PrefillConfig any   `yaml:"prefill_config"`
				DecodeConfig  any   `yaml:"decode_config"`
			} `yaml:"args"`
			RuntimeEnv struct {
				EnvVars map[string]string `yaml:"env_vars"`
			} `yaml:"runtime_env"`
		} `yaml:"applications"`
	}
	if yaml.Unmarshal([]byte(config), &parsed) != nil {
		return false
	}
	for _, app := range parsed.Applications {
		module, _, _ := strings.Cut(app.ImportPath, ":")
		if module == "ray.serve.llm" || strings.HasPrefix(module, "ray.serve.llm.") || len(app.Args.LLMConfigs) > 0 {
			return true
		}
		for _, key := range []string{"SERVED_MODEL_NAME", "MODEL_NAME", "MODEL_ID", "HF_MODEL_ID", "HUGGING_FACE_MODEL_ID"} {
			if strings.TrimSpace(app.RuntimeEnv.EnvVars[key]) != "" {
				return true
			}
		}
		for _, raw := range []any{app.Args.PrefillConfig, app.Args.DecodeConfig} {
			cfg, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			if loading, ok := cfg["model_loading_config"].(map[string]any); ok {
				for _, key := range []string{"model_id", "model_source"} {
					if value, ok := loading[key].(string); ok && strings.TrimSpace(value) != "" {
						return true
					}
				}
			}
		}
	}
	return false
}

// RayClusterWorkloadName removes the generated suffix, as in the backend.
func RayClusterWorkloadName(name string) string {
	if suffix := strings.LastIndex(name, "-"); suffix > 0 {
		value := name[suffix+1:]
		if len(value) == 5 && strings.Trim(value, "bcdfghjklmnpqrstvwxz2456789") == "" {
			return name[:suffix]
		}
	}
	return name
}
