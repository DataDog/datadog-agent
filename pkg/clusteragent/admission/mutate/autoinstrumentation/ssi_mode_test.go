// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

// SSI mode is decided by whether a target/policy matched the pod, not by a
// namespace-level eligibility approximation. Annotations may override library
// versions but do not, by themselves, make a pod SSI.

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/common"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/annotation"
	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

func annotatedEnabledPod(namespace string, labels map[string]string) *corev1.Pod {
	if labels == nil {
		labels = map[string]string{}
	}
	labels["admission.datadoghq.com/enabled"] = "true"
	return mutatecommon.FakePodSpec{
		NS:          namespace,
		Labels:      labels,
		Annotations: map[string]string{"admission.datadoghq.com/java-lib.version": "v1"},
	}.Create()
}

func podEnv(t *testing.T, pod *corev1.Pod) map[string]string {
	t.Helper()
	require.NotEmpty(t, pod.Spec.Containers)
	env := make(map[string]string, len(pod.Spec.Containers[0].Env))
	for _, e := range pod.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	return env
}

func requireSSIDefaults(t *testing.T, env map[string]string, want bool) {
	t.Helper()
	for _, name := range []string{
		"DD_TRACE_ENABLED",
		"DD_LOGS_INJECTION",
		"DD_RUNTIME_METRICS_ENABLED",
		"DD_TRACE_HEALTH_METRICS_ENABLED",
	} {
		_, found := env[name]
		require.Equal(t, want, found, "env var %q", name)
	}
}

func nsNamePolicy(name, namespace string, inject bool) policies.Policy {
	return policies.Policy{
		Name:    name,
		Rules:   policies.StringLeaf(policies.IDNamespaceName, policies.CmpExact, namespace),
		Outcome: policies.Outcome{Inject: inject, InjectSet: true},
	}
}

// TestSSIMode_FollowsPolicyMatchNotNamespaceApprox pins the abstraction:
// install_type and SSI defaults follow an exact policy/target match on the pod.
func TestSSIMode_FollowsPolicyMatchNotNamespaceApprox(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)
	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		nsNamePolicy("remote-prod", "prod", true),
	}))

	t.Run("annotation-only outside policy scope is lib-injection", func(t *testing.T) {
		pod := annotatedEnabledPod("unrelated", nil)
		mutated, err := m.MutatePod(pod, "unrelated", nil)
		require.NoError(t, err)
		require.True(t, mutated)

		env := podEnv(t, pod)
		require.Equal(t, "k8s_lib_injection", env["DD_INSTRUMENTATION_INSTALL_TYPE"])
		requireSSIDefaults(t, env, false)
	})

	t.Run("annotation plus matching policy is SSI with annotation libs", func(t *testing.T) {
		pod := annotatedEnabledPod("prod", nil)
		mutated, err := m.MutatePod(pod, "prod", nil)
		require.NoError(t, err)
		require.True(t, mutated)

		env := podEnv(t, pod)
		require.Equal(t, "k8s_single_step", env["DD_INSTRUMENTATION_INSTALL_TYPE"])
		requireSSIDefaults(t, env, true)
		require.NotEmpty(t, env[AppliedPolicyEnvVar], "matched policy should still be recorded")
	})
}

// TestSSIMode_PodFactsRequiredForMixedPolicy ensures a policy that ANDs
// namespace and pod labels does not put unmatched pods into SSI mode just
// because they share the namespace.
func TestSSIMode_PodFactsRequiredForMixedPolicy(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)
	require.NoError(t, m.SetRemotePolicies([]policies.Policy{{
		Name: "remote-prod-db",
		Rules: policies.And([]*policies.Node{
			policies.StringLeaf(policies.IDNamespaceName, policies.CmpExact, "prod"),
			policies.LabelLeaf(policies.IDPodLabel, "app", policies.CmpExact, "db"),
		}),
		Outcome: policies.Outcome{Inject: true, InjectSet: true},
	}}))

	t.Run("annotated pod in scope namespace but wrong labels is lib-injection", func(t *testing.T) {
		pod := annotatedEnabledPod("prod", map[string]string{"app": "web"})
		mutated, err := m.MutatePod(pod, "prod", nil)
		require.NoError(t, err)
		require.True(t, mutated)

		env := podEnv(t, pod)
		require.Equal(t, "k8s_lib_injection", env["DD_INSTRUMENTATION_INSTALL_TYPE"])
		requireSSIDefaults(t, env, false)
	})

	t.Run("annotated pod matching the full policy is SSI", func(t *testing.T) {
		pod := annotatedEnabledPod("prod", map[string]string{"app": "db"})
		mutated, err := m.MutatePod(pod, "prod", nil)
		require.NoError(t, err)
		require.True(t, mutated)

		env := podEnv(t, pod)
		require.Equal(t, "k8s_single_step", env["DD_INSTRUMENTATION_INSTALL_TYPE"])
		requireSSIDefaults(t, env, true)
	})
}

// TestSSIMode_ConfigTargetMatchIsSSI verifies enabled_namespaces/targets still
// produce SSI when they match, including when annotations override lib versions.
func TestSSIMode_ConfigTargetMatchIsSSI(t *testing.T) {
	cfg := `
apm_config:
  instrumentation:
    enabled: true
    enabled_namespaces:
      - application
`
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, cfg, wmeta)

	pod := annotatedEnabledPod("application", nil)
	mutated, err := m.MutatePod(pod, "application", nil)
	require.NoError(t, err)
	require.True(t, mutated)

	env := podEnv(t, pod)
	require.Equal(t, "k8s_single_step", env["DD_INSTRUMENTATION_INSTALL_TYPE"])
	requireSSIDefaults(t, env, true)
}

// TestSSIMode_AnnotationOverridesLibsKeepsTargetConfigs pins the GA-visible
// precedence change versus main: a library annotation no longer short-circuits
// target matching. The matched target's ddTraceConfigs and applied-target
// metadata are kept; annotation only overrides library versions. Tracer configs
// from annotations merge by env var name (annotation wins on conflict).
func TestSSIMode_AnnotationOverridesLibsKeepsTargetConfigs(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "Python Apps"
        podSelector:
          matchLabels:
            language: "python"
        ddTraceVersions:
          python: "v3"
        ddTraceConfigs:
          - name: "DD_PROFILING_ENABLED"
            value: "true"
          - name: "DD_DATA_JOBS_ENABLED"
            value: "true"
`
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, cfg, wmeta)

	pod := mutatecommon.FakePodSpec{
		NS: "application",
		Labels: map[string]string{
			"language":             "python",
			common.EnabledLabelKey: "true",
		},
		Annotations: map[string]string{
			// Different language/version than the matching target: annotation wins for libs.
			"admission.datadoghq.com/java-lib.version": "v1",
		},
	}.Create()

	mutated, err := m.MutatePod(pod, "application", nil)
	require.NoError(t, err)
	require.True(t, mutated)

	env := podEnv(t, pod)
	require.Equal(t, "k8s_single_step", env["DD_INSTRUMENTATION_INSTALL_TYPE"])
	requireSSIDefaults(t, env, true)
	require.Equal(t, "true", env["DD_PROFILING_ENABLED"], "target ddTraceConfigs must survive lib annotations")
	require.Equal(t, "true", env["DD_DATA_JOBS_ENABLED"])
	require.NotEmpty(t, env[AppliedTargetEnvVar])
	require.NotEmpty(t, pod.Annotations[annotation.AppliedTarget])

	// Library comes from the annotation, not the python target version.
	require.Contains(t, pod.Annotations[annotation.InjectedLibraries], "java")
	require.Contains(t, pod.Annotations[annotation.InjectedLibraries], "v1")
}

// TestSSIMode_DenyBlocksAnnotationInjection ensures an explicit deny match is
// not the same as "no match": annotations must not reinstrument the pod.
func TestSSIMode_DenyBlocksAnnotationInjection(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)
	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		nsNamePolicy("remote-deny-prod", "prod", false),
	}))

	pod := annotatedEnabledPod("prod", nil)
	mutated, err := m.MutatePod(pod, "prod", nil)
	require.NoError(t, err)
	require.False(t, mutated, "deny must win over annotation-driven lib-injection")
	require.Empty(t, pod.Spec.InitContainers)
}

func TestMergeEnvVarsByName(t *testing.T) {
	base := []corev1.EnvVar{
		{Name: "DD_PROFILING_ENABLED", Value: "true"},
		{Name: "DD_DATA_JOBS_ENABLED", Value: "true"},
	}
	override := []corev1.EnvVar{
		{Name: "DD_PROFILING_ENABLED", Value: "false"},
		{Name: "DD_RUNTIME_METRICS_ENABLED", Value: "true"},
	}
	require.Equal(t, []corev1.EnvVar{
		{Name: "DD_PROFILING_ENABLED", Value: "false"},
		{Name: "DD_DATA_JOBS_ENABLED", Value: "true"},
		{Name: "DD_RUNTIME_METRICS_ENABLED", Value: "true"},
	}, mergeEnvVarsByName(base, override))
	require.Equal(t, base, mergeEnvVarsByName(base, nil))
	require.Equal(t, override, mergeEnvVarsByName(nil, override))
}
