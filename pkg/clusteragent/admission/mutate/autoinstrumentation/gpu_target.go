// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/comp/core/config"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	// gpuTargetName is the name of the target used to instrument GPU workloads.
	gpuTargetName = "gpu-monitoring"
	// gpuPodLabel is the label that opts a pod in to GPU instrumentation.
	gpuPodLabel = "admission.datadoghq.com/gpu.enabled"
	// gpuTracerVersion is the version of the C tracer injected in GPU workloads.
	gpuTracerVersion = "0"

	trainingRunIDEnvVar   = "DD_TRAINING_RUN_ID"
	trainingGroupIDEnvVar = "DD_TRAINING_GROUP_ID"
)

// GPUConfig is the configuration for the instrumentation of GPU workloads. Full config key: gpu.tracing and gpu.jobs
type GPUConfig struct {
	// Enabled adds a target that instruments GPU workloads. Full config key: gpu.tracing.enabled
	Enabled bool
	// Jobs defines where the training run and group IDs of a workload are read from. Full config key: gpu.jobs
	Jobs gpuconfig.JobsConfig
}

// NewGPUConfig creates a new GPUConfig from the datadog config.
func NewGPUConfig(datadogConfig config.Component) *GPUConfig {
	return &GPUConfig{
		Enabled: datadogConfig.GetBool("gpu.tracing.enabled"),
		Jobs:    gpuconfig.NewJobsConfig(datadogConfig),
	}
}

// newGPUTarget builds the target that instruments the pods opted in to GPU instrumentation.
func newGPUTarget(jobs gpuconfig.JobsConfig) Target {
	tracerConfigs := []TracerConfig{
		{Name: "DD_INJECT_NATIVE", Value: "always"},
		{Name: "DD_TRACE_HOOK_MODULES", Value: "gpu"},
	}
	tracerConfigs = appendIdentifierConfig(tracerConfigs, trainingRunIDEnvVar, jobs.Run)
	tracerConfigs = appendIdentifierConfig(tracerConfigs, trainingGroupIDEnvVar, jobs.Group)

	return Target{
		Name: gpuTargetName,
		PodSelector: &PodSelector{
			MatchLabels: map[string]string{gpuPodLabel: "true"},
		},
		TracerVersions: map[string]string{string(c): gpuTracerVersion},
		TracerConfigs:  tracerConfigs,
	}
}

// appendIdentifierConfig adds an env var populated from the pod label or annotation referenced by the identifier.
// Nothing is added if the identifier is not configured or is of a type that cannot be read from the pod metadata.
func appendIdentifierConfig(configs []TracerConfig, envVarName string, id gpuconfig.IdentifierConfig) []TracerConfig {
	if !id.Configured() {
		return configs
	}

	fieldPath, ok := podFieldPath(id)
	if !ok {
		log.Warnf("gpu.jobs identifier %q of type %q cannot be injected in %s: only labels and annotations are supported", id.Key, id.Type, envVarName)
		return configs
	}

	return append(configs, TracerConfig{
		Name:      envVarName,
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath}},
	})
}

// podFieldPath returns the downward API field path of the label or annotation referenced by the identifier.
func podFieldPath(id gpuconfig.IdentifierConfig) (string, bool) {
	switch id.Type {
	case gpuconfig.IdentifierTypeLabel:
		return fmt.Sprintf("metadata.labels['%s']", id.Key), true
	case gpuconfig.IdentifierTypeAnnotation:
		return fmt.Sprintf("metadata.annotations['%s']", id.Key), true
	default:
		return "", false
	}
}

// withGPUTarget returns the targets with the GPU target added, if enabled. The GPU target goes first, as the first
// matching target wins, so GPU pods get the GPU configuration even if another target also matches them.
func withGPUTarget(gpu *GPUConfig, targets []Target) []Target {
	if gpu == nil || !gpu.Enabled {
		return targets
	}
	return append([]Target{newGPUTarget(gpu.Jobs)}, targets...)
}
