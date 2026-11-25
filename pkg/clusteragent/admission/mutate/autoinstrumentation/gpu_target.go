// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
)

const (
	// gpuTargetName is the name of the target injecting tracers into GPU workloads.
	gpuTargetName = "gpu-monitoring"
	// gpuEnabledLabelKey is the pod label that opts a GPU workload into tracer injection.
	gpuEnabledLabelKey = "admission.datadoghq.com/gpu.enabled"
	// trainingRunIDEnvVar holds the training run ID read from gpu.jobs.run.
	trainingRunIDEnvVar = "DD_TRAINING_RUN_ID"
	// trainingGroupIDEnvVar holds the training group ID read from gpu.jobs.group.
	trainingGroupIDEnvVar = "DD_TRAINING_GROUP_ID"
	// trainingIDEnvRefPrefix marks a training ID value as the name of another environment variable, which the
	// tracer dereferences at runtime. It is used for identifiers of type env, which may only be set at runtime.
	trainingIDEnvRefPrefix = "env:"
)

// newGPUTarget builds the target injecting tracers into GPU workloads, or nil when gpu.tracing is disabled.
func newGPUTarget(tracing gpuconfig.TracingConfig, jobs gpuconfig.JobsConfig) *Target {
	if !tracing.Enabled {
		return nil
	}

	tracerConfigs := []TracerConfig{
		{Name: "DD_INJECT_NATIVE", Value: "always"},
		{Name: "DD_TRACE_HOOK_MODULES", Value: "gpu"},
	}
	tracerConfigs = appendJobTracerConfig(tracerConfigs, trainingRunIDEnvVar, jobs.Run)
	tracerConfigs = appendJobTracerConfig(tracerConfigs, trainingGroupIDEnvVar, jobs.Group)

	return &Target{
		Name: gpuTargetName,
		PodSelector: &PodSelector{
			MatchLabels: map[string]string{gpuEnabledLabelKey: "true"},
		},
		TracerVersions: tracing.TracerVersions,
		TracerConfigs:  tracerConfigs,
	}
}

// appendJobTracerConfig adds an env var named name holding the job identifier, if the identifier is configured.
func appendJobTracerConfig(tracerConfigs []TracerConfig, name string, id gpuconfig.IdentifierConfig) []TracerConfig {
	tracerConfig, ok := jobTracerConfig(name, id)
	if !ok {
		return tracerConfigs
	}
	return append(tracerConfigs, tracerConfig)
}

// jobTracerConfig returns an env var named name holding the job identifier. Label and annotation identifiers are
// read from the pod metadata through the downward API. Env identifiers are passed to the tracer as an env:
// reference, since the variable may only be set at runtime.
func jobTracerConfig(name string, id gpuconfig.IdentifierConfig) (TracerConfig, bool) {
	if !id.Configured() {
		return TracerConfig{}, false
	}
	switch id.Type {
	case gpuconfig.IdentifierTypeLabel:
		return podMetadataTracerConfig(name, fmt.Sprintf("metadata.labels['%s']", id.Key)), true
	case gpuconfig.IdentifierTypeAnnotation:
		return podMetadataTracerConfig(name, fmt.Sprintf("metadata.annotations['%s']", id.Key)), true
	case gpuconfig.IdentifierTypeEnv:
		return TracerConfig{Name: name, Value: trainingIDEnvRefPrefix + id.Key}, true
	default:
		return TracerConfig{}, false
	}
}

// podMetadataTracerConfig returns an env var named name reading fieldPath through the downward API.
func podMetadataTracerConfig(name, fieldPath string) TracerConfig {
	return TracerConfig{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath},
		},
	}
}
