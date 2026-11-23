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

	"github.com/DataDog/dd-policy-engine/go/policies"

	admissioncommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
)

var apmBasicPodLabels = map[string]string{apmBasicEnabledLabelKey: "true"}

func TestAPMBasicConfig(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		cfg, err := NewInstrumentationConfig(mutatecommon.FakeConfigWithValues(t, nil))
		require.NoError(t, err)
		assert.False(t, cfg.APMBasic.Enabled)
		assert.Equal(t, map[string]string{"c": "0"}, cfg.APMBasic.TracerVersions)
	})

	t.Run("overridden", func(t *testing.T) {
		cfg, err := NewInstrumentationConfig(mutatecommon.FakeConfigWithValues(t, map[string]any{
			"apm_config.instrumentation.apm_basic.enabled":         true,
			"apm_config.instrumentation.apm_basic.tracer_versions": map[string]string{"c": "0.24.0"},
		}))
		require.NoError(t, err)
		assert.True(t, cfg.APMBasic.Enabled)
		assert.Equal(t, map[string]string{"c": "0.24.0"}, cfg.APMBasic.TracerVersions)
	})
}

func TestNewAPMBasicTarget(t *testing.T) {
	versions := map[string]string{"c": "0.24.0"}
	got := newAPMBasicTarget(APMBasicConfig{Enabled: true, TracerVersions: versions})

	require.NotNil(t, got)
	assert.Equal(t, apmBasicTargetName, got.Name)
	assert.Equal(t, &PodSelector{MatchLabels: apmBasicPodLabels}, got.PodSelector)
	assert.Equal(t, versions, got.TracerVersions)
	assert.Equal(t, []TracerConfig{
		{Name: "DD_INJECT_NATIVE", Value: "always"},
		{Name: "DD_APM_BASIC_ENABLED", Value: "true"},
	}, got.TracerConfigs)
}

func TestNewAPMBasicTargetDisabled(t *testing.T) {
	assert.Nil(t, newAPMBasicTarget(APMBasicConfig{TracerVersions: map[string]string{"c": "0"}}))
}

func TestMatching_APMBasicTarget(t *testing.T) {
	tests := map[string]struct {
		cfg   string
		cases []matchCase
	}{
		"apm basic disabled by default": {
			cfg: `{}`,
			cases: []matchCase{
				{name: "apm basic pod is not matched", podLabels: apmBasicPodLabels, want: ""},
			},
		},
		"apm basic without SSI": {
			cfg: `
apm_config:
  instrumentation:
    apm_basic:
      enabled: true
`,
			cases: []matchCase{
				{name: "apm basic pod hits the apm basic target", podLabels: apmBasicPodLabels, want: apmBasicTargetName},
				{name: "other pod is not matched", podLabels: map[string]string{"app": "web"}, want: ""},
			},
		},
		"apm basic is layered on SSI inject-all": {
			cfg: `
apm_config:
  instrumentation:
    enabled: true
    apm_basic:
      enabled: true
`,
			cases: []matchCase{
				{name: "apm basic pod keeps inject-all", podLabels: apmBasicPodLabels, want: "default"},
				{name: "other pod falls through to inject-all", podLabels: map[string]string{"app": "web"}, want: "default"},
			},
		},
		"apm basic is layered on user targets": {
			cfg: `
apm_config:
  instrumentation:
    enabled: true
    apm_basic:
      enabled: true
    targets:
      - name: "catch-all"
        ddTraceVersions:
          python: "default"
`,
			cases: []matchCase{
				{name: "apm basic pod keeps the user target", podLabels: apmBasicPodLabels, want: "catch-all"},
				{name: "other pod hits the user target", podLabels: map[string]string{"app": "web"}, want: "catch-all"},
			},
		},
		"gpu target comes before apm basic": {
			cfg: `
apm_config:
  instrumentation:
    apm_basic:
      enabled: true
gpu:
  tracing:
    enabled: true
`,
			cases: []matchCase{
				{
					name:      "pod opted into both hits the gpu target",
					podLabels: map[string]string{apmBasicEnabledLabelKey: "true", gpuEnabledLabelKey: "true"},
					want:      gpuTargetName,
				},
				{name: "apm basic pod hits the apm basic target", podLabels: apmBasicPodLabels, want: apmBasicTargetName},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			runMatchCases(t, test.cfg, test.cases)
		})
	}
}

func TestAPMBasicTargetInjection(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    apm_basic:
      enabled: true
`
	m := newMatchMutator(t, cfg, newMatchTestWmeta(t))
	pod := &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: "web", Labels: apmBasicPodLabels}}

	resolved := m.resolveTarget(pod)

	require.NotNil(t, resolved)
	require.Equal(t, injectionSourceAPMBasic, resolved.selectedBy)
	require.True(t, resolved.isSSI)
	target := resolved.plan
	require.NotNil(t, target)
	require.False(t, target.blocked)
	require.Len(t, target.libraries, 1)
	assert.Equal(t, c, target.libraries[0].lang)
	assert.Equal(t, "0", target.libraries[0].tag)
	assert.Equal(t, []corev1.EnvVar{
		{Name: "DD_INJECT_NATIVE", Value: "always"},
		{Name: "DD_APM_BASIC_ENABLED", Value: "true"},
	}, target.tracerEnvVars)
}

func TestAPMBasicLayering(t *testing.T) {
	const pythonTarget = `
apm_config:
  instrumentation:
    enabled: true
    apm_basic:
      enabled: true
    targets:
      - name: "python"
        ddTraceVersions:
          python: "v4"
`
	const apmBasicOnly = `
apm_config:
  instrumentation:
    apm_basic:
      enabled: true
`
	const gpu = `
apm_config:
  instrumentation:
    apm_basic:
      enabled: true
gpu:
  tracing:
    enabled: true
`
	optedIn := func(extra map[string]string) map[string]string {
		labels := map[string]string{apmBasicEnabledLabelKey: "true"}
		for k, v := range extra {
			labels[k] = v
		}
		return labels
	}
	enabled := map[string]string{admissioncommon.EnabledLabelKey: "true"}

	tests := map[string]struct {
		cfg         string
		labels      map[string]string
		annotations map[string]string
		wantSource  injectionSourceName
		wantLayered bool
	}{
		"user target keeps its SDK": {
			cfg: pythonTarget, labels: optedIn(nil), wantSource: injectionSourceStatic, wantLayered: true,
		},
		"annotation keeps its SDK": {
			cfg: apmBasicOnly, labels: optedIn(enabled),
			annotations: map[string]string{annotation.LibraryVersion.Format("python"): "v4"},
			wantSource:  injectionSourceAnnotation, wantLayered: true,
		},
		"c-lib annotation owns dd-trace-c": {
			cfg: apmBasicOnly, labels: optedIn(enabled),
			annotations: map[string]string{annotation.LibraryVersion.Format("c"): "0"},
			wantSource:  injectionSourceAnnotation,
		},
		"gpu target owns dd-trace-c": {
			cfg: gpu, labels: optedIn(map[string]string{gpuEnabledLabelKey: "true"}), wantSource: injectionSourceGPU,
		},
		"apm basic alone": {
			cfg: apmBasicOnly, labels: optedIn(nil), wantSource: injectionSourceAPMBasic,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := newMatchMutator(t, test.cfg, newMatchTestWmeta(t))
			pod := mutatecommon.FakePodSpec{NS: "web", Labels: test.labels, Annotations: test.annotations}.Create()

			resolved := m.resolveTarget(pod)

			require.NotNil(t, resolved)
			assert.Equal(t, test.wantSource, resolved.selectedBy)
			assert.True(t, resolved.isSSI)
			if !test.wantLayered {
				assert.Empty(t, resolved.additions)
				return
			}
			require.Len(t, resolved.additions, 1)
			assert.Equal(t, c, resolved.additions[0].libraries[0].lang)
		})
	}

	t.Run("ssi denial keeps the annotation but blocks apm basic", func(t *testing.T) {
		m := newMatchMutator(t, apmBasicOnly, newMatchTestWmeta(t))
		require.NoError(t, m.SetRemotePolicies([]policies.Policy{podLabelPolicy("deny", "app", "web", false, nil)}))
		pod := mutatecommon.FakePodSpec{
			NS:          "web",
			Labels:      optedIn(map[string]string{admissioncommon.EnabledLabelKey: "true", "app": "web"}),
			Annotations: map[string]string{annotation.LibraryVersion.Format("python"): "v4"},
		}.Create()

		resolved := m.resolveTarget(pod)

		require.NotNil(t, resolved)
		assert.Equal(t, injectionSourceAnnotation, resolved.selectedBy)
		assert.False(t, resolved.isSSI)
		assert.Empty(t, resolved.additions)
	})

	t.Run("opt-out blocks apm basic", func(t *testing.T) {
		m := newMatchMutator(t, apmBasicOnly, newMatchTestWmeta(t))
		pod := mutatecommon.FakePodSpec{NS: "web", Labels: optedIn(map[string]string{admissioncommon.EnabledLabelKey: "false"})}.Create()

		assert.Nil(t, m.resolveTarget(pod))
	})
}

func TestAPMBasicLayeredInjection(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    apm_basic:
      enabled: true
    targets:
      - name: "python"
        ddTraceVersions:
          python: "v4"
        ddTraceConfigs:
          - name: "DD_INJECT_NATIVE"
            value: "never"
`
	m := newMatchMutator(t, cfg, newMatchTestWmeta(t))
	pod := mutatecommon.FakePodSpec{NS: "web", Labels: apmBasicPodLabels}.Create()

	mutated, err := m.MutatePod(pod, pod.Namespace, nil)

	require.NoError(t, err)
	require.True(t, mutated)
	images := make([]string, 0, len(pod.Spec.InitContainers))
	for _, ctr := range pod.Spec.InitContainers {
		images = append(images, ctr.Image)
	}
	assert.Contains(t, images, "registry/dd-lib-python-init:v4")
	assert.Contains(t, images, "registry/dd-lib-c-init:0")
	env := map[string]string{}
	for _, e := range pod.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	assert.Equal(t, "true", env["DD_APM_BASIC_ENABLED"])
	assert.Equal(t, "never", env["DD_INJECT_NATIVE"], "the selected target wins on a shared name")
	assert.Equal(t, string(injectionSourceStatic), pod.Annotations[annotation.InjectionSource])

	resolved := m.resolveTarget(mutatecommon.FakePodSpec{NS: "web", Labels: apmBasicPodLabels}.Create())
	require.NotNil(t, resolved)
	assert.Len(t, resolved.plan.libraries, 1, "the shared target plan is not modified")
}
