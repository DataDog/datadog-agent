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
		{Name: "DD_INJECT_NATIVE", Value: "fallback"},
		{Name: "DD_TRACE_HOOK_MODULES", Value: "socket,openssl,postgres,mysql,sqlite,mongodb,oracle"},
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
		"apm basic keeps SSI inject-all": {
			cfg: `
apm_config:
  instrumentation:
    enabled: true
    apm_basic:
      enabled: true
`,
			cases: []matchCase{
				{name: "apm basic pod hits the apm basic target", podLabels: apmBasicPodLabels, want: apmBasicTargetName},
				{name: "other pod falls through to inject-all", podLabels: map[string]string{"app": "web"}, want: "default"},
			},
		},
		"apm basic comes before user targets": {
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
				{name: "apm basic pod hits the apm basic target", podLabels: apmBasicPodLabels, want: apmBasicTargetName},
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
		{Name: "DD_INJECT_NATIVE", Value: "fallback"},
		{Name: "DD_TRACE_HOOK_MODULES", Value: "socket,openssl,postgres,mysql,sqlite,mongodb,oracle"},
		{Name: "DD_APM_BASIC_ENABLED", Value: "true"},
	}, target.tracerEnvVars)
}
