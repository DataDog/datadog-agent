// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

// This file pins down namespace eligibility for policy-driven matching. Pod
// matching and namespace eligibility are two answers from the same policy set,
// and a policy delivered by remote config must scope both: a policy targeting
// one namespace must not turn the whole cluster into an SSI-eligible cluster,
// because namespace eligibility gates the default library configuration, the
// UST env vars and the language detection.

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"

	mutatecommon "github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/common"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

// nsNamePolicy builds a policy scoped to a single namespace name.
func nsNamePolicy(name, namespace string, inject bool) policies.Policy {
	return policies.Policy{
		Name:    name,
		Rules:   policies.StringLeaf(policies.IDNamespaceName, policies.CmpExact, namespace),
		Outcome: policies.Outcome{Inject: inject, InjectSet: true},
	}
}

// nsLabelPolicy builds a policy scoped to a namespace label.
func nsLabelPolicy(name, key, value string) policies.Policy {
	return policies.Policy{
		Name:    name,
		Rules:   policies.LabelLeaf(policies.IDNamespaceLabel, key, policies.CmpExact, value),
		Outcome: policies.Outcome{Inject: true, InjectSet: true},
	}
}

// TestRemotePolicies_NamespaceEligibilityIsScopedToPolicy verifies that the
// namespace scope of a remote policy is honored by IsNamespaceEligible, and not
// only by pod matching.
func TestRemotePolicies_NamespaceEligibilityIsScopedToPolicy(t *testing.T) {
	tests := map[string]struct {
		policies []policies.Policy
		want     map[string]bool
	}{
		"a namespace-scoped policy only makes its namespace eligible": {
			policies: []policies.Policy{nsNamePolicy("remote-prod", "prod", true)},
			want:     map[string]bool{"prod": true, "unrelated": false},
		},
		"a pod-scoped policy leaves every namespace eligible": {
			// Nothing in the namespace facts can rule the policy out, so the
			// namespace stays a candidate: eligibility is an over-approximation
			// by design.
			policies: []policies.Policy{podLabelPolicy("remote-db", "app", "db", true, nil)},
			want:     map[string]bool{"prod": true, "unrelated": true},
		},
		"a deny policy makes no namespace eligible": {
			policies: []policies.Policy{nsNamePolicy("remote-deny", "prod", false)},
			want:     map[string]bool{"prod": false, "unrelated": false},
		},
		"a deny policy does not shadow the namespace scope of an allow policy": {
			policies: []policies.Policy{
				nsNamePolicy("remote-deny", "staging", false),
				nsNamePolicy("remote-allow", "prod", true),
			},
			want: map[string]bool{"prod": true, "staging": false, "unrelated": false},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			wmeta := newMatchTestWmeta(t)
			m := newMatchMutator(t, rcDisabledCfg, wmeta)

			// Baseline: instrumentation is disabled, nothing is eligible.
			for namespace := range test.want {
				require.False(t, m.IsNamespaceEligible(namespace))
			}

			require.NoError(t, m.SetRemotePolicies(test.policies))
			for namespace, want := range test.want {
				require.Equal(t, want, m.IsNamespaceEligible(namespace), "namespace %q", namespace)
			}

			// Clearing the remote policies reverts to the disabled baseline.
			m.ClearRemotePolicies()
			for namespace := range test.want {
				require.False(t, m.IsNamespaceEligible(namespace))
			}
		})
	}
}

// TestRemotePolicies_NamespaceLabelEligibility verifies that a remote policy
// scoped to namespace labels resolves them from workloadmeta, and that a
// namespace whose metadata is unavailable is not eligible through that policy.
func TestRemotePolicies_NamespaceLabelEligibility(t *testing.T) {
	wmeta := newMatchTestWmeta(t,
		newTestNamespace("instrumented", map[string]string{"instrument": "true"}),
		newTestNamespace("not-instrumented", map[string]string{"instrument": "false"}),
	)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		nsLabelPolicy("remote-labelled", "instrument", "true"),
	}))

	require.True(t, m.IsNamespaceEligible("instrumented"))
	require.False(t, m.IsNamespaceEligible("not-instrumented"))
	// Namespace metadata is unavailable: the policy cannot be evaluated and is
	// skipped, exactly as it is skipped for pod matching.
	require.False(t, m.IsNamespaceEligible("unknown-to-workloadmeta"))
}

// TestRemotePolicies_ConfigTargetsKeepTheirNamespaceScope verifies that layering
// remote policies does not widen the namespace scope of the configuration
// baseline, and that the config baseline does not widen the remote scope either.
func TestRemotePolicies_ConfigTargetsKeepTheirNamespaceScope(t *testing.T) {
	const cfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "config-billing"
        namespaceSelector:
          matchNames:
            - "billing"
        ddTraceVersions:
          java: "default"
`

	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, cfg, wmeta)

	require.True(t, m.IsNamespaceEligible("billing"))
	require.False(t, m.IsNamespaceEligible("prod"))
	require.False(t, m.IsNamespaceEligible("unrelated"))

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		nsNamePolicy("remote-prod", "prod", true),
	}))

	require.True(t, m.IsNamespaceEligible("billing"))
	require.True(t, m.IsNamespaceEligible("prod"))
	require.False(t, m.IsNamespaceEligible("unrelated"))
}

// TestRemotePolicies_NamespaceScopeGatesSSIDefaults is the user-visible side of
// the same property: the exact same annotation-instrumented pod must only be
// treated as an SSI pod (default library configuration, single-step install
// type) inside the namespace the remote policy targets.
func TestRemotePolicies_NamespaceScopeGatesSSIDefaults(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)
	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		nsNamePolicy("remote-prod", "prod", true),
	}))

	annotatedPod := func(namespace string) *corev1.Pod {
		return mutatecommon.FakePodSpec{
			NS:          namespace,
			Labels:      map[string]string{"admission.datadoghq.com/enabled": "true"},
			Annotations: map[string]string{"admission.datadoghq.com/java-lib.version": "v1"},
		}.Create()
	}

	tests := map[string]struct {
		namespace       string
		wantInstallType string
		wantSSIDefaults bool
	}{
		"inside the policy scope the pod is an SSI pod": {
			namespace:       "prod",
			wantInstallType: "k8s_single_step",
			wantSSIDefaults: true,
		},
		"outside the policy scope the pod is a local lib-injection pod": {
			namespace:       "unrelated",
			wantInstallType: "k8s_lib_injection",
			wantSSIDefaults: false,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			pod := annotatedPod(test.namespace)
			mutated, err := m.MutatePod(pod, test.namespace, nil)
			require.NoError(t, err)
			require.True(t, mutated, "the annotation must still drive the injection")

			env := podEnvVars(t, pod)
			require.Equal(t, test.wantInstallType, env["DD_INSTRUMENTATION_INSTALL_TYPE"])

			// The default library configuration is injected for SSI-eligible
			// namespaces only.
			for _, name := range []string{
				"DD_TRACE_ENABLED",
				"DD_LOGS_INJECTION",
				"DD_RUNTIME_METRICS_ENABLED",
				"DD_TRACE_HEALTH_METRICS_ENABLED",
			} {
				_, found := env[name]
				require.Equal(t, test.wantSSIDefaults, found, "env var %q", name)
			}
		})
	}
}

func podEnvVars(t *testing.T, pod *corev1.Pod) map[string]string {
	t.Helper()
	require.Len(t, pod.Spec.Containers, 1)
	env := make(map[string]string, len(pod.Spec.Containers[0].Env))
	for _, e := range pod.Spec.Containers[0].Env {
		env[e.Name] = e.Value
	}
	return env
}
