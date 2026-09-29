// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	coreconfig "github.com/DataDog/datadog-agent/comp/core/config"
	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	workloadmetafxmock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/fx-mock"
	workloadmetamock "github.com/DataDog/datadog-agent/comp/core/workloadmeta/mock"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/imageresolver"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	gpuconfig "github.com/DataDog/datadog-agent/pkg/gpu/config"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

var testGPUJobs = gpuconfig.JobsConfig{
	Run:   gpuconfig.IdentifierConfig{Key: "example/job-id-label", Type: gpuconfig.IdentifierTypeLabel},
	Group: gpuconfig.IdentifierConfig{Key: "example/job-group-annotation", Type: gpuconfig.IdentifierTypeAnnotation},
}

func fieldRefEnvVar(name, fieldPath string) corev1.EnvVar {
	return corev1.EnvVar{
		Name:      name,
		ValueFrom: &corev1.EnvVarSource{FieldRef: &corev1.ObjectFieldSelector{FieldPath: fieldPath}},
	}
}

func TestNewGPUTarget(t *testing.T) {
	target := newGPUTarget(testGPUJobs)

	assert.Equal(t, Target{
		Name:           "gpu-monitoring",
		PodSelector:    &PodSelector{MatchLabels: map[string]string{"admission.datadoghq.com/gpu.enabled": "true"}},
		TracerVersions: map[string]string{"c": "0.24.0"},
		TracerConfigs: []TracerConfig{
			{Name: "DD_INJECT_NATIVE", Value: "always"},
			{Name: "DD_TRACE_HOOK_MODULES", Value: "gpu"},
			{Name: "DD_TRAINING_RUN_ID", ValueFrom: fieldRefEnvVar("", "metadata.labels['example/job-id-label']").ValueFrom},
			{Name: "DD_TRAINING_GROUP_ID", ValueFrom: fieldRefEnvVar("", "metadata.annotations['example/job-group-annotation']").ValueFrom},
		},
	}, target)
}

func TestNewGPUTargetJobIdentifiers(t *testing.T) {
	tests := map[string]struct {
		jobs     gpuconfig.JobsConfig
		expected []string
	}{
		"no jobs configured": {
			jobs: gpuconfig.JobsConfig{},
		},
		"only run configured": {
			jobs:     gpuconfig.JobsConfig{Run: testGPUJobs.Run},
			expected: []string{"DD_TRAINING_RUN_ID"},
		},
		"unknown type is ignored": {
			jobs: gpuconfig.JobsConfig{Run: gpuconfig.IdentifierConfig{Key: "a", Type: "other"}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var names []string
			for _, tc := range newGPUTarget(test.jobs).TracerConfigs[2:] { // skip the fixed configs
				names = append(names, tc.Name)
			}
			assert.Equal(t, test.expected, names)
		})
	}
}

func TestGPUTargetIsValid(t *testing.T) {
	// The GPU target goes through the same validation as user targets: allowed env var prefixes and selectors.
	cfg := &Config{}
	internal, err := buildInternalTargets(cfg, []Target{newGPUTarget(testGPUJobs)}, nil)
	require.NoError(t, err)
	require.Len(t, internal, 1)
	assert.Equal(t, "gpu-monitoring", internal[0].name)
	assert.Contains(t, internal[0].envVars, fieldRefEnvVar("DD_TRAINING_RUN_ID", "metadata.labels['example/job-id-label']"))
}

func TestWithGPUTarget(t *testing.T) {
	user := []Target{{Name: "user"}}

	assert.Equal(t, user, withGPUTarget(nil, user))
	assert.Equal(t, user, withGPUTarget(&GPUConfig{Enabled: false}, user))

	got := withGPUTarget(&GPUConfig{Enabled: true, Jobs: testGPUJobs}, user)
	require.Len(t, got, 2)
	assert.Equal(t, "gpu-monitoring", got[0].Name, "GPU target must be first, as the first match wins")
	assert.Equal(t, "user", got[1].Name)
	assert.Len(t, user, 1, "the input must not be modified")
}

func TestNewGPUConfig(t *testing.T) {
	cfg := configmock.NewFromYAML(t, `
gpu:
  tracing:
    enabled: true
  jobs:
    run:
      key: example/job-id-label
      type: label
    group:
      key: example/job-group-annotation
      type: annotation
`)

	assert.Equal(t, &GPUConfig{Enabled: true, Jobs: testGPUJobs}, NewGPUConfig(cfg))
}

func newGPUTestMutator(t *testing.T, yaml string) *TargetMutator {
	t.Helper()

	mockConfig := configmock.NewFromYAML(t, yaml)
	mockConfig.SetInTest("admission_controller.auto_instrumentation.container_registry", "registry")
	config, err := NewConfig(mockConfig)
	require.NoError(t, err)

	wmeta := fxutil.Test[workloadmetamock.Mock](t, fx.Options(
		fx.Supply(coreconfig.Params{}),
		fx.Provide(func() log.Component { return logmock.New(t) }),
		fx.Provide(func() coreconfig.Component { return coreconfig.NewMock(t) }),
		workloadmetafxmock.MockModule(workloadmeta.NewParams()),
	))

	mutator, err := NewTargetMutator(config, wmeta, imageresolver.NewNoOpResolver(), nil, nil, nil)
	require.NoError(t, err)
	return mutator
}

func testPod(labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "application", Labels: labels}}
}

func targetName(target *targetInternal) string {
	if target == nil {
		return ""
	}
	return target.name
}

const gpuTracingYAML = `
gpu:
  tracing:
    enabled: true
  jobs:
    run:
      key: example/job-id-label
      type: label
`

var (
	gpuPodLabels  = map[string]string{"admission.datadoghq.com/gpu.enabled": "true"}
	javaPodLabels = map[string]string{"language": "java"}
)

func TestGPUTargetWithSSIDisabled(t *testing.T) {
	mutator := newGPUTestMutator(t, gpuTracingYAML)

	target := mutator.getTarget(testPod(gpuPodLabels))
	require.NotNil(t, target)
	assert.Equal(t, "gpu-monitoring", target.name)
	assert.Contains(t, target.envVars, fieldRefEnvVar("DD_TRAINING_RUN_ID", "metadata.labels['example/job-id-label']"))
	assert.Contains(t, target.envVars, corev1.EnvVar{Name: "DD_TRACE_HOOK_MODULES", Value: "gpu"})

	assert.Nil(t, mutator.getTarget(testPod(nil)), "only GPU pods are instrumented when SSI is disabled")
	assert.True(t, mutator.ShouldMutatePod(testPod(gpuPodLabels)))
	assert.False(t, mutator.ShouldMutatePod(testPod(nil)))
}

func TestGPUTargetDisabled(t *testing.T) {
	mutator := newGPUTestMutator(t, "gpu:\n  tracing:\n    enabled: false\n")

	assert.Nil(t, mutator.getTarget(testPod(gpuPodLabels)))
}

func TestGPUTargetKeepsInjectAllWhenSSIEnabledWithoutTargets(t *testing.T) {
	mutator := newGPUTestMutator(t, gpuTracingYAML+`
apm_config:
  instrumentation:
    enabled: true
`)

	assert.Equal(t, "gpu-monitoring", targetName(mutator.getTarget(testPod(gpuPodLabels))))
	assert.Equal(t, "default", targetName(mutator.getTarget(testPod(nil))), "other pods must still be instrumented")
}

func TestGPUTargetWithUserTargets(t *testing.T) {
	mutator := newGPUTestMutator(t, gpuTracingYAML+`
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "java"
        podSelector:
          matchLabels:
            language: "java"
        ddTraceVersions:
          java: "default"
`)

	assert.Equal(t, "gpu-monitoring", targetName(mutator.getTarget(testPod(gpuPodLabels))))
	assert.Equal(t, "java", targetName(mutator.getTarget(testPod(javaPodLabels))))
	assert.Nil(t, mutator.getTarget(testPod(nil)), "the GPU target must not turn a targeted setup into inject-all")

	both := map[string]string{"admission.datadoghq.com/gpu.enabled": "true", "language": "java"}
	assert.Equal(t, "gpu-monitoring", targetName(mutator.getTarget(testPod(both))), "the GPU target takes precedence")
}

func TestGPULabelSelector(t *testing.T) {
	ls := NewLabelSelectors(&LabelSelectorsConfig{GPUTracing: true})

	_, objectSelector := ls.Get(false)

	assert.Equal(t, []metav1.LabelSelectorRequirement{{
		Key:      common.EnabledLabelKey,
		Operator: metav1.LabelSelectorOpNotIn,
		Values:   []string{"false"},
	}}, objectSelector.MatchExpressions, "GPU pods must reach the webhook even if SSI is disabled")
}
