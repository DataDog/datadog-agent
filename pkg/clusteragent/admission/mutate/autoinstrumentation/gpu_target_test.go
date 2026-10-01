// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
)

var gpuPodLabels = map[string]string{gpuEnabledLabelKey: "true"}

func fieldRefTracerConfig(name, fieldPath string) TracerConfig {
	return TracerConfig{
		Name: name,
		ValueFrom: &corev1.EnvVarSource{
			FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath},
		},
	}
}

func fieldRefEnvVar(name, fieldPath string) corev1.EnvVar {
	tc := fieldRefTracerConfig(name, fieldPath)
	return tc.AsEnvVar()
}

func TestNewGPUTarget(t *testing.T) {
	enabled := gpuconfig.TracingConfig{Enabled: true, TracerVersions: map[string]string{"c": "0.24.0"}}
	baseConfigs := []TracerConfig{
		{Name: "DD_INJECT_NATIVE", Value: "always"},
		{Name: "DD_TRACE_HOOK_MODULES", Value: "gpu"},
	}

	tests := map[string]struct {
		tracing gpuconfig.TracingConfig
		jobs    gpuconfig.JobsConfig
		want    []TracerConfig
	}{
		"no jobs": {
			tracing: enabled,
			want:    baseConfigs,
		},
		"run from a label and group from an annotation": {
			tracing: enabled,
			jobs: gpuconfig.JobsConfig{
				Run:   gpuconfig.IdentifierConfig{Key: "example/job-id", Type: gpuconfig.IdentifierTypeLabel},
				Group: gpuconfig.IdentifierConfig{Key: "example/task-name", Type: gpuconfig.IdentifierTypeAnnotation},
			},
			want: append(baseConfigs,
				fieldRefTracerConfig(trainingRunIDEnvVar, "metadata.labels['example/job-id']"),
				fieldRefTracerConfig(trainingGroupIDEnvVar, "metadata.annotations['example/task-name']"),
			),
		},
		"env identifiers are ignored": {
			tracing: enabled,
			jobs: gpuconfig.JobsConfig{
				Run:   gpuconfig.IdentifierConfig{Key: "_RAY_SUBMISSION_ID", Type: gpuconfig.IdentifierTypeEnv},
				Group: gpuconfig.IdentifierConfig{Key: "example/task-name", Type: gpuconfig.IdentifierTypeAnnotation},
			},
			want: append(baseConfigs,
				fieldRefTracerConfig(trainingGroupIDEnvVar, "metadata.annotations['example/task-name']"),
			),
		},
		"identifiers without a key are ignored": {
			tracing: enabled,
			jobs: gpuconfig.JobsConfig{
				Run: gpuconfig.IdentifierConfig{Type: gpuconfig.IdentifierTypeLabel},
			},
			want: baseConfigs,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got := newGPUTarget(test.tracing, test.jobs)
			require.NotNil(t, got)
			assert.Equal(t, gpuTargetName, got.Name)
			assert.Equal(t, &PodSelector{MatchLabels: gpuPodLabels}, got.PodSelector)
			assert.Equal(t, test.tracing.TracerVersions, got.TracerVersions)
			assert.Equal(t, test.want, got.TracerConfigs)
		})
	}
}

func TestNewGPUTargetDisabled(t *testing.T) {
	assert.Nil(t, newGPUTarget(gpuconfig.TracingConfig{}, gpuconfig.JobsConfig{
		Run: gpuconfig.IdentifierConfig{Key: "example/job-id", Type: gpuconfig.IdentifierTypeLabel},
	}))
}

func TestMatching_GPUTarget(t *testing.T) {
	tests := map[string]struct {
		cfg   string
		cases []matchCase
	}{
		"gpu tracing disabled": {
			cfg: `
gpu:
  tracing:
    enabled: false
`,
			cases: []matchCase{
				{name: "gpu pod is not matched", podLabels: gpuPodLabels, want: ""},
			},
		},
		"gpu tracing without SSI": {
			cfg: `
gpu:
  tracing:
    enabled: true
`,
			cases: []matchCase{
				{name: "gpu pod hits the gpu target", podLabels: gpuPodLabels, want: gpuTargetName},
				{name: "other pod is not matched", podLabels: map[string]string{"app": "web"}, want: ""},
			},
		},
		"gpu tracing keeps SSI inject-all": {
			cfg: `
apm_config:
  instrumentation:
    enabled: true
gpu:
  tracing:
    enabled: true
`,
			cases: []matchCase{
				{name: "gpu pod hits the gpu target", podLabels: gpuPodLabels, want: gpuTargetName},
				{name: "other pod falls through to inject-all", podLabels: map[string]string{"app": "web"}, want: "default"},
			},
		},
		"gpu tracing keeps SSI enabled namespaces": {
			cfg: `
apm_config:
  instrumentation:
    enabled: true
    enabled_namespaces: ["ml"]
gpu:
  tracing:
    enabled: true
`,
			cases: []matchCase{
				{name: "gpu pod hits the gpu target", ns: "other", podLabels: gpuPodLabels, want: gpuTargetName},
				{name: "pod in enabled namespace hits the default target", ns: "ml", podLabels: map[string]string{"app": "web"}, want: "default"},
				{name: "pod outside enabled namespaces is not matched", ns: "other", podLabels: map[string]string{"app": "web"}, want: ""},
			},
		},
		"gpu target comes before user targets": {
			cfg: `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "catch-all"
        ddTraceVersions:
          python: "default"
gpu:
  tracing:
    enabled: true
`,
			cases: []matchCase{
				{name: "gpu pod hits the gpu target", podLabels: gpuPodLabels, want: gpuTargetName},
				{name: "other pod hits the user target", podLabels: map[string]string{"app": "web"}, want: "catch-all"},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runMatchCases(t, test.cfg, test.cases)
		})
	}
}

func TestGPUTargetInjection(t *testing.T) {
	const cfg = `
gpu:
  tracing:
    enabled: true
  jobs:
    run:
      key: example/job-id
      type: annotation
    group:
      key: example/task-name
      type: annotation
`
	m := newMatchMutator(t, cfg, newMatchTestWmeta(t))
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "ml", Labels: gpuPodLabels}}

	target := m.getSSIPlan(pod)

	require.NotNil(t, target)
	require.Len(t, target.libraries, 1)
	assert.Equal(t, c, target.libraries[0].lang)
	assert.Equal(t, "0", target.libraries[0].tag)
	assert.Equal(t, []corev1.EnvVar{
		{Name: "DD_INJECT_NATIVE", Value: "always"},
		{Name: "DD_TRACE_HOOK_MODULES", Value: "gpu"},
		fieldRefEnvVar(trainingRunIDEnvVar, "metadata.annotations['example/job-id']"),
		fieldRefEnvVar(trainingGroupIDEnvVar, "metadata.annotations['example/task-name']"),
	}, target.tracerEnvVars)
}

func TestLabelSelectorsGPUTracing(t *testing.T) {
	mockConfig := configmock.NewFromYAML(t, `
apm_config:
  instrumentation:
    enabled: false
    on_demand: false
gpu:
  tracing:
    enabled: true
`)
	cfg := NewLabelSelectorsConfig(mockConfig)
	require.True(t, cfg.GPUTracing)

	_, objectSelector := NewLabelSelectors(cfg).Get(false)

	assert.Equal(t, &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{Key: common.EnabledLabelKey, Operator: metav1.LabelSelectorOpNotIn, Values: []string{"false"}},
		},
	}, objectSelector)
}
