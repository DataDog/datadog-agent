// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"bytes"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

const (
	otelInjectJava = "instrumentation.opentelemetry.io/inject-java"
	otelJavaReason = "pod requests instrumentation from the OpenTelemetry Operator (instrumentation.opentelemetry.io/inject-java=true)"
)

func otelPod(labels, annotations map[string]string) *corev1.Pod {
	return mutatecommon.FakePodSpec{NS: "apps", Labels: labels, Annotations: annotations}.Create()
}

func newOTelTestMutator(t *testing.T, yamlCfg string, ddi DDITargetProvider) *TargetMutator {
	t.Helper()
	mockConfig := configmock.NewFromYAML(t, yamlCfg)
	mockConfig.SetInTest("admission_controller.auto_instrumentation.container_registry", "registry")
	config, err := NewConfig(mockConfig)
	require.NoError(t, err)
	m, err := NewTargetMutator(config, newMatchTestWmeta(t), imageResolver, nil, nil, ddi)
	require.NoError(t, err)
	return m
}

func TestOTelOperatorSource(t *testing.T) {
	const reasonPrefix = "pod requests instrumentation from the OpenTelemetry Operator "

	tests := map[string]struct {
		disabled    bool
		annotations map[string]string
		// wantReason is empty when the source passes.
		wantReason string
	}{
		"no annotation": {},
		"inject annotation": {
			annotations: map[string]string{otelInjectJava: "true"},
			wantReason:  otelJavaReason,
		},
		"value naming an instrumentation": {
			annotations: map[string]string{otelInjectJava: "otel/java"},
			wantReason:  reasonPrefix + "(instrumentation.opentelemetry.io/inject-java=otel/java)",
		},
		"sdk only": {
			annotations: map[string]string{"instrumentation.opentelemetry.io/inject-sdk": "true"},
			wantReason:  reasonPrefix + "(instrumentation.opentelemetry.io/inject-sdk=true)",
		},
		"language unknown to the Cluster Agent": {
			annotations: map[string]string{"instrumentation.opentelemetry.io/inject-rust": "true"},
			wantReason:  reasonPrefix + "(instrumentation.opentelemetry.io/inject-rust=true)",
		},
		"false": {
			annotations: map[string]string{otelInjectJava: "false"},
		},
		"false in upper case": {
			annotations: map[string]string{otelInjectJava: "FALSE"},
		},
		"empty value": {
			annotations: map[string]string{otelInjectJava: ""},
		},
		"value is not trimmed, like the operator": {
			annotations: map[string]string{otelInjectJava: " false"},
			wantReason:  reasonPrefix + "(instrumentation.opentelemetry.io/inject-java= false)",
		},
		"requests are sorted": {
			annotations: map[string]string{
				"instrumentation.opentelemetry.io/inject-python": "true",
				"instrumentation.opentelemetry.io/inject-nginx":  "false",
				otelInjectJava: "true",
			},
			wantReason: reasonPrefix + "(instrumentation.opentelemetry.io/inject-java=true, instrumentation.opentelemetry.io/inject-python=true)",
		},
		"container selectors": {
			annotations: map[string]string{
				"instrumentation.opentelemetry.io/inject-nginx-container-names": "nginx",
				"instrumentation.opentelemetry.io/java-container-names":         "app",
				"instrumentation.opentelemetry.io/container-names":              "app",
			},
		},
		"other OpenTelemetry annotations": {
			annotations: map[string]string{
				"instrumentation.opentelemetry.io/inject-":                 "true",
				"instrumentation.opentelemetry.io/otel-go-auto-target-exe": "/app",
				"sidecar.opentelemetry.io/inject":                          "true",
			},
		},
		"filter disabled": {
			disabled:    true,
			annotations: map[string]string{otelInjectJava: "true"},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			source := &otelOperatorSource{enabled: !test.disabled}
			// Resolve twice: the reason must not depend on map iteration order.
			for range 2 {
				result := source.resolve(otelPod(nil, test.annotations))
				if test.wantReason == "" {
					require.Equal(t, sourcePass, result.action)
					require.Nil(t, result.plan)
					continue
				}
				require.Equal(t, sourceVeto, result.action)
				require.Equal(t, &injectionPlan{
					blocked:    true,
					skipReason: test.wantReason,
				}, result.plan)
			}
		})
	}
}

// TestOTelOperatorFilter_Resolution pins how the filter interacts with the other
// sources: explicit Datadog opt-ins win, remote-config policies, static targets
// and inject-all are vetoed, remote-config denials are still recorded, and a
// pod that no source would inject is left alone.
func TestOTelOperatorFilter_Resolution(t *testing.T) {
	const filterOffCfg = `
apm_config:
  instrumentation:
    enabled: true
    skip_otel_operator_pods: false
`
	const mutateUnlabelledCfg = `
admission_controller:
  mutate_unlabelled: true
apm_config:
  instrumentation:
    enabled: true
`
	const gpuCfg = `
gpu:
  tracing:
    enabled: true
`
	const apmBasicCfg = `
apm_config:
  instrumentation:
    enabled: true
    apm_basic:
      enabled: true
`
	const enabledLabel = "admission.datadoghq.com/enabled"
	const javaLibVersion = "admission.datadoghq.com/java-lib.version"

	allowWeb := podLabelPolicy("allow-web", "app", "web", true, nil)
	denyWeb := podLabelPolicy("deny-web", "app", "web", false, nil)
	ddiWeb := mockDDIProvider{
		{Kind: "Deployment", Namespace: "apps", Name: "web"}: ddiTarget(types.NamespacedName{Namespace: "apps", Name: "ddi-web"}, true, map[string]string{"java": "v1"}, nil),
	}

	tests := map[string]struct {
		cfg         string
		policies    []policies.Policy
		ddi         DDITargetProvider
		labels      map[string]string
		annotations map[string]string
		// wantSource is empty when nothing resolves. The opentelemetry-operator
		// source means the pod is skipped.
		wantSource injectionSourceName
		// wantBlocked means a remote-config policy denied injection.
		wantBlocked bool
	}{
		"inject-all is vetoed": {
			cfg:         rcSSIOnNoTargets,
			annotations: map[string]string{otelInjectJava: "true"},
			wantSource:  injectionSourceOTelOperator,
		},
		"static target is vetoed": {
			cfg:         rcCatchAllCfg,
			annotations: map[string]string{otelInjectJava: "true"},
			wantSource:  injectionSourceOTelOperator,
		},
		"remote-config policy is vetoed": {
			cfg:         rcDisabledCfg,
			policies:    []policies.Policy{allowWeb},
			annotations: map[string]string{otelInjectJava: "true"},
			wantSource:  injectionSourceOTelOperator,
		},
		"remote-config denial is still recorded": {
			cfg:         rcCatchAllCfg,
			policies:    []policies.Policy{denyWeb},
			annotations: map[string]string{otelInjectJava: "true"},
			wantSource:  injectionSourceRemoteConfig,
			wantBlocked: true,
		},
		"pod that no source targets is left alone": {
			cfg:         rcDisabledCfg,
			annotations: map[string]string{otelInjectJava: "true"},
		},
		"opt-out label leaves the pod alone": {
			cfg:         rcSSIOnNoTargets,
			labels:      map[string]string{enabledLabel: "false"},
			annotations: map[string]string{otelInjectJava: "true"},
		},
		"false value is injected": {
			cfg:         rcSSIOnNoTargets,
			annotations: map[string]string{otelInjectJava: "false"},
			wantSource:  injectionSourceInjectAll,
		},
		"filter disabled": {
			cfg:         filterOffCfg,
			annotations: map[string]string{otelInjectJava: "true"},
			wantSource:  injectionSourceInjectAll,
		},
		"datadog annotation wins": {
			cfg:         rcSSIOnNoTargets,
			labels:      map[string]string{enabledLabel: "true"},
			annotations: map[string]string{otelInjectJava: "true", javaLibVersion: "v1"},
			wantSource:  injectionSourceAnnotation,
		},
		"datadog annotation on an unlabelled pod wins with mutate_unlabelled": {
			cfg:         mutateUnlabelledCfg,
			annotations: map[string]string{otelInjectJava: "true", javaLibVersion: "v1"},
			wantSource:  injectionSourceAnnotation,
		},
		"ignored datadog annotation is not an opt-in": {
			cfg:         rcSSIOnNoTargets,
			annotations: map[string]string{otelInjectJava: "true", javaLibVersion: "v1"},
			wantSource:  injectionSourceOTelOperator,
		},
		"enabled label alone is not an opt-in": {
			cfg:         rcSSIOnNoTargets,
			labels:      map[string]string{enabledLabel: "true"},
			annotations: map[string]string{otelInjectJava: "true"},
			wantSource:  injectionSourceOTelOperator,
		},
		"datadog instrumentation wins": {
			cfg:         rcSSIOnNoTargets,
			ddi:         ddiWeb,
			annotations: map[string]string{otelInjectJava: "true"},
			wantSource:  injectionSourceDatadogInstrumentation,
		},
		"gpu label wins over a vetoed remote-config policy": {
			cfg:         gpuCfg,
			policies:    []policies.Policy{allowWeb},
			labels:      gpuPodLabels,
			annotations: map[string]string{otelInjectJava: "true"},
			wantSource:  injectionSourceGPU,
		},
		"apm basic label wins": {
			cfg:         apmBasicCfg,
			labels:      apmBasicPodLabels,
			annotations: map[string]string{otelInjectJava: "true"},
			wantSource:  injectionSourceAPMBasic,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			m := newOTelTestMutator(t, test.cfg, test.ddi)
			if len(test.policies) > 0 {
				require.NoError(t, m.SetRemotePolicies(test.policies))
			}
			labels := map[string]string{"app": "web"}
			for k, v := range test.labels {
				labels[k] = v
			}
			// The pod belongs to the web deployment that ddiWeb targets.
			pod := mutatecommon.FakePodSpec{
				NS:          "apps",
				Labels:      labels,
				Annotations: test.annotations,
				ParentKind:  "replicaset",
				ParentName:  "web-bcdfg",
			}.Create()

			resolved := m.resolveTarget(pod)
			if test.wantSource == "" {
				require.Nil(t, resolved)
				require.False(t, m.ShouldMutatePod(pod))
				return
			}

			skipped := test.wantSource == injectionSourceOTelOperator
			require.NotNil(t, resolved)
			require.Equal(t, test.wantSource, resolved.selectedBy)
			require.Equal(t, skipped, resolved.plan.skipReason != "")
			require.Equal(t, skipped || test.wantBlocked, resolved.plan.blocked)
			require.Equal(t, !skipped && !test.wantBlocked, m.ShouldMutatePod(pod))
			if !skipped && !test.wantBlocked {
				require.True(t, resolved.isSSI, "explicit opt-ins and unfiltered pods keep the SSI mode")
			}
		})
	}
}

// TestOTelOperatorFilter_MutatePod verifies that a skipped pod is not mutated
// but records why, identically when the webhook is reinvoked, and that an
// explicit Datadog opt-in is injected as before.
func TestOTelOperatorFilter_MutatePod(t *testing.T) {
	t.Run("skipped pod is annotated without being injected", func(t *testing.T) {
		m := newOTelTestMutator(t, rcSSIOnNoTargets, nil)
		pod := otelPod(nil, map[string]string{otelInjectJava: "true"})
		require.False(t, m.ShouldMutatePod(pod))

		for range 2 {
			mutated, err := m.MutatePod(pod, "apps", nil)
			require.NoError(t, err)
			require.False(t, mutated)
		}

		require.Equal(t, map[string]string{
			otelInjectJava:             "true",
			annotation.InjectionSource: "opentelemetry-operator",
			annotation.InjectionStatus: annotation.InjectionStatusSkipped,
			annotation.InjectionError:  otelJavaReason,
		}, pod.Annotations)
		require.Empty(t, pod.Spec.InitContainers)
		require.Empty(t, pod.Spec.Volumes)
		require.Empty(t, pod.Spec.Containers[0].Env)
	})

	t.Run("datadog annotation is injected in SSI mode", func(t *testing.T) {
		m := newOTelTestMutator(t, rcSSIOnNoTargets, nil)
		pod := annotatedEnabledPod("apps", nil)
		pod.Annotations[otelInjectJava] = "true"

		mutated, err := m.MutatePod(pod, "apps", nil)
		require.NoError(t, err)
		require.True(t, mutated)
		require.Equal(t, string(injectionSourceAnnotation), pod.Annotations[annotation.InjectionSource])
		require.Equal(t, annotation.InjectionStatusInjected, pod.Annotations[annotation.InjectionStatus])
		require.NotContains(t, pod.Annotations, annotation.InjectionError)
		env := podEnv(t, pod)
		require.Equal(t, singleStepInstrumentationInstallType, env[instrumentationInstallTypeEnvVarName])
		requireSSIDefaults(t, env, true)
	})
}

// syncBuffer is a bytes.Buffer safe for the logger's concurrent writes.
type syncBuffer struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

func TestOTelOperatorFilter_LogsSkipOnce(t *testing.T) {
	// Build the mutator first: the workloadmeta mock replaces the global logger.
	m := newOTelTestMutator(t, rcSSIOnNoTargets, nil)

	var logs syncBuffer
	logger, err := log.LoggerFromWriterWithMinLevelAndMsgFormat(&logs, log.InfoLvl)
	require.NoError(t, err)
	log.SetupLogger(logger, "info")
	t.Cleanup(func() { log.SetupLogger(log.Default(), "off") })

	pod := otelPod(nil, map[string]string{otelInjectJava: "true"})
	pod.Name, pod.GenerateName = "", "web-6d4cf56db6-"
	// The admission, then two reinvocations.
	for range 3 {
		_, err := m.MutatePod(pod, "apps", nil)
		require.NoError(t, err)
	}
	log.Flush()

	output := logs.String()
	require.Equal(t, 1, strings.Count(output, otelJavaReason), output)
	require.Contains(t, output, `Skipping APM library injection for pod with generate name web-6d4cf56db6- in namespace "apps": `+otelJavaReason)
}

func TestSkipOTelOperatorPodsEnvVar(t *testing.T) {
	t.Setenv("DD_APM_INSTRUMENTATION_SKIP_OTEL_OPERATOR_PODS", "false")
	cfg, err := NewInstrumentationConfig(configmock.New(t))
	require.NoError(t, err)
	require.False(t, cfg.SkipOTelOperatorPods)
}
