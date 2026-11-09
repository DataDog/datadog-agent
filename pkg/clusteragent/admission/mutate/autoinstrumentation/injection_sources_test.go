// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

const otelInjectJava = "instrumentation.opentelemetry.io/inject-java"

func findInjectionSource(t *testing.T, mutator *TargetMutator, name injectionSourceName) injectionSource {
	t.Helper()
	for _, entry := range mutator.sources {
		if entry.name == name {
			return entry.source
		}
	}
	require.FailNow(t, "injection source not found", "name: %s", name)
	return nil
}

func TestInjectAllSourceActivation(t *testing.T) {
	const configYAML = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: static
`
	mutator := newMatchMutator(t, configYAML, newMatchTestWmeta(t))

	require.Equal(t, sourcePass, findInjectionSource(t, mutator, injectionSourceInjectAll).resolve(&corev1.Pod{}).action)

	const injectAllConfig = `
apm_config:
  instrumentation:
    enabled: true
`
	injectAllMutator := newMatchMutator(t, injectAllConfig, newMatchTestWmeta(t))
	require.Equal(t, sourceInject, findInjectionSource(t, injectAllMutator, injectionSourceInjectAll).resolve(&corev1.Pod{}).action)

	require.NoError(t, injectAllMutator.SetRemotePolicies([]policies.Policy{{
		Name:    "remote",
		Rules:   policies.AlwaysTrue(),
		Outcome: policies.Outcome{Inject: true, InjectSet: true},
	}}))
	require.Equal(t, sourcePass, findInjectionSource(t, injectAllMutator, injectionSourceInjectAll).resolve(&corev1.Pod{}).action)
	injectAllMutator.ClearRemotePolicies()
	require.Equal(t, sourceInject, findInjectionSource(t, injectAllMutator, injectionSourceInjectAll).resolve(&corev1.Pod{}).action)
}

func TestOTelOperatorSource(t *testing.T) {
	tests := map[string]struct {
		annotations map[string]string
		// wantReason is empty when the source passes.
		wantReason string
	}{
		"no annotation": {},
		"inject annotation": {
			annotations: map[string]string{otelInjectJava: "true"},
			wantReason:  "pod requests instrumentation from the OpenTelemetry Operator (instrumentation.opentelemetry.io/inject-java=true)",
		},
		"annotations are sorted": {
			annotations: map[string]string{"instrumentation.opentelemetry.io/inject-python": "my-instrumentation", otelInjectJava: "true"},
			wantReason:  "pod requests instrumentation from the OpenTelemetry Operator (instrumentation.opentelemetry.io/inject-java=true, instrumentation.opentelemetry.io/inject-python=my-instrumentation)",
		},
		"false in any case": {
			annotations: map[string]string{otelInjectJava: "FALSE"},
		},
		"empty value": {
			annotations: map[string]string{otelInjectJava: ""},
		},
		"container selector": {
			annotations: map[string]string{"instrumentation.opentelemetry.io/inject-nginx-container-names": "nginx"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result := otelOperatorSource{}.resolve(&corev1.Pod{ObjectMeta: metav1.ObjectMeta{Annotations: test.annotations}})
			if test.wantReason == "" {
				require.Equal(t, sourcePass, result.action)
				return
			}
			require.Equal(t, sourceVeto, result.action)
			require.Equal(t, &injectionPlan{blocked: true, skipReason: test.wantReason}, result.plan)
		})
	}
}

// TestOTelOperatorVeto verifies that remote-config policies, static targets and
// inject-all skip pods that request instrumentation from the OpenTelemetry
// Operator, while denials and explicit Datadog opt-ins are unaffected.
func TestOTelOperatorVeto(t *testing.T) {
	otel := map[string]string{otelInjectJava: "true"}
	tests := map[string]struct {
		cfg         string
		policies    []policies.Policy
		labels      map[string]string
		annotations map[string]string
		// want is empty when no source resolves.
		want injectionSourceName
	}{
		"inject-all is skipped": {
			cfg: rcSSIOnNoTargets, annotations: otel, want: injectionSourceOTelOperator,
		},
		"static target is skipped": {
			cfg: rcCatchAllCfg, annotations: otel, want: injectionSourceOTelOperator,
		},
		"remote-config policy is skipped": {
			cfg: rcDisabledCfg, policies: []policies.Policy{podLabelPolicy("allow", "app", "web", true, nil)}, annotations: otel, want: injectionSourceOTelOperator,
		},
		"remote-config denial is still recorded": {
			cfg: rcCatchAllCfg, policies: []policies.Policy{podLabelPolicy("deny", "app", "web", false, nil)}, annotations: otel, want: injectionSourceRemoteConfig,
		},
		"pod that no source targets is left alone": {
			cfg: rcDisabledCfg, annotations: otel,
		},
		"false is injected": {
			cfg: rcSSIOnNoTargets, annotations: map[string]string{otelInjectJava: "false"}, want: injectionSourceInjectAll,
		},
		"datadog annotation wins": {
			cfg:         rcSSIOnNoTargets,
			labels:      map[string]string{"admission.datadoghq.com/enabled": "true"},
			annotations: map[string]string{otelInjectJava: "true", "admission.datadoghq.com/java-lib.version": "v1"},
			want:        injectionSourceAnnotation,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := newMatchMutator(t, test.cfg, newMatchTestWmeta(t))
			if len(test.policies) > 0 {
				require.NoError(t, m.SetRemotePolicies(test.policies))
			}
			labels := map[string]string{"app": "web"}
			for k, v := range test.labels {
				labels[k] = v
			}
			pod := mutatecommon.FakePodSpec{NS: "ns", Labels: labels, Annotations: test.annotations}.Create()

			resolved := m.resolveTarget(pod)
			if test.want == "" {
				require.Nil(t, resolved)
				return
			}
			require.Equal(t, test.want, resolved.selectedBy)
			require.Equal(t, test.want == injectionSourceOTelOperator, resolved.plan.skipReason != "")
			require.Equal(t, !resolved.plan.blocked, m.ShouldMutatePod(pod))
			if !resolved.plan.blocked {
				require.True(t, resolved.isSSI)
			}
		})
	}
}

func TestOTelOperatorVeto_MutatePod(t *testing.T) {
	m := newMatchMutator(t, rcSSIOnNoTargets, newMatchTestWmeta(t))
	pod := mutatecommon.FakePodSpec{NS: "ns", Annotations: map[string]string{otelInjectJava: "true"}}.Create()

	// The webhook can be reinvoked: the result must be the same.
	for range 2 {
		mutated, err := m.MutatePod(pod, "ns", nil)
		require.NoError(t, err)
		require.False(t, mutated)
	}

	require.Equal(t, map[string]string{
		otelInjectJava:             "true",
		annotation.InjectionSource: "opentelemetry-operator",
		annotation.InjectionStatus: annotation.InjectionStatusSkipped,
		annotation.InjectionError:  "pod requests instrumentation from the OpenTelemetry Operator (instrumentation.opentelemetry.io/inject-java=true)",
	}, pod.Annotations)
	require.Empty(t, pod.Spec.InitContainers)
	require.Empty(t, pod.Spec.Volumes)
}
