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

	"github.com/DataDog/datadog-agent/pkg/remoteconfig/state"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

const rcDisabledCfg = `
apm_config:
  instrumentation:
    enabled: false
`

const rcCatchAllCfg = `
apm_config:
  instrumentation:
    enabled: true
    targets:
      - name: "config-default"
        ddTraceVersions:
          java: "default"
`

func rcPod(ns string, labels map[string]string) *corev1.Pod {
	return &corev1.Pod{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Labels: labels}}
}

// podLabelPolicy builds a policy matching a single pod label with exact equality.
func podLabelPolicy(name, key, val string, inject bool, versions map[string]string) policies.Policy {
	return policies.Policy{
		Name:    name,
		Rules:   policies.Leaf(policies.SourcePodLabel, key, policies.CmpExact, val),
		Outcome: policies.Outcome{Inject: inject, TracerVersions: versions},
	}
}

// matchedTarget returns the matched target name and whether it came from a
// remote-config policy.
func matchedTarget(t *testing.T, m *TargetMutator, pod *corev1.Pod) (string, bool) {
	t.Helper()
	target := m.getMatchingTarget(pod)
	if target == nil {
		return "", false
	}
	return target.name, target.fromPolicy
}

// TestRemotePolicies_AppliedOnEmptyBaseline verifies that remote policies match
// even when instrumentation is disabled in the configuration (empty baseline).
func TestRemotePolicies_AppliedOnEmptyBaseline(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)

	// No remote policies yet: nothing matches.
	require.Nil(t, m.getMatchingTarget(rcPod("ns", map[string]string{"app": "db"})))

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		podLabelPolicy("remote-java", "app", "db", true, map[string]string{"java": "default"}),
	}))

	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "remote-java", name)
	require.True(t, fromPolicy)

	require.Nil(t, m.getMatchingTarget(rcPod("ns", map[string]string{"app": "other"})))
}

// TestRemotePolicies_PrecedenceOverConfig verifies that remote policies are
// evaluated before the configuration baseline (first match wins), while
// non-matching pods still fall through to the configuration.
func TestRemotePolicies_PrecedenceOverConfig(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcCatchAllCfg, wmeta)

	// Baseline: the catch-all config target matches everything.
	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "config-default", name)
	require.False(t, fromPolicy)

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		podLabelPolicy("remote", "app", "db", true, map[string]string{"python": "default"}),
	}))

	// Remote wins for the matching pod...
	name, fromPolicy = matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "remote", name)
	require.True(t, fromPolicy)

	// ...but unrelated pods still fall through to the config baseline.
	name, fromPolicy = matchedTarget(t, m, rcPod("ns", map[string]string{"app": "other"}))
	require.Equal(t, "config-default", name)
	require.False(t, fromPolicy)
}

// TestRemotePolicies_DenyStopsInjection verifies that a matched deny policy
// prevents injection even when a later policy (or the config baseline) would
// otherwise match.
func TestRemotePolicies_DenyStopsInjection(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcCatchAllCfg, wmeta)

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		podLabelPolicy("remote-deny", "app", "legacy", false, nil),
	}))

	// The deny policy matches first, so no target is returned even though the
	// config catch-all would otherwise apply.
	require.Nil(t, m.getMatchingTarget(rcPod("ns", map[string]string{"app": "legacy"})))

	// A non-matching pod still hits the config baseline.
	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "ok"}))
	require.Equal(t, "config-default", name)
	require.False(t, fromPolicy)
}

// TestRemotePolicies_ClearRevertsToBaseline verifies that clearing the remote
// policies reverts the mutator to its configuration baseline.
func TestRemotePolicies_ClearRevertsToBaseline(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcCatchAllCfg, wmeta)

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		podLabelPolicy("remote", "app", "db", true, map[string]string{"python": "default"}),
	}))
	name, _ := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "remote", name)

	m.ClearRemotePolicies()
	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "config-default", name)
	require.False(t, fromPolicy)
}

// TestRemotePolicies_FirstMatchWins verifies the ordering among remote policies.
func TestRemotePolicies_FirstMatchWins(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		podLabelPolicy("first", "app", "db", true, map[string]string{"java": "default"}),
		podLabelPolicy("second", "app", "db", true, map[string]string{"python": "default"}),
	}))

	name, _ := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "first", name)
}

// TestOnRemoteConfigUpdate_ParsesAndApplies exercises the remote-config callback
// end to end with a dd-wls policies document, then verifies that an empty update
// reverts the mutator to the configuration baseline.
func TestOnRemoteConfigUpdate_ParsesAndApplies(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcCatchAllCfg, wmeta)

	const raw = `{
      "policies": [{
        "description": "java for db-user",
        "rules": {
          "node_type": "EvaluatorNode",
          "node": {
            "eval_type": "StrEvaluator",
            "eval": {"id": "POD_LABEL", "cmp": "CMP_EXACT", "value": "app=db-user"}
          }
        },
        "actions": [
          {"action": "INJECT_ALLOW"},
          {"action": "ENABLE_SDK", "values": ["java=latest"]}
        ]
      }]
    }`

	var applied []state.ApplyStatus
	apply := func(_ string, s state.ApplyStatus) { applied = append(applied, s) }

	m.onRemoteConfigUpdate(map[string]state.RawConfig{
		"datadog/2/APM_POLICIES/policy-1/config": {Config: []byte(raw)},
	}, apply)

	require.Len(t, applied, 1)
	require.Equal(t, state.ApplyStateAcknowledged, applied[0].State)

	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db-user"}))
	require.Equal(t, "java for db-user", name)
	require.True(t, fromPolicy)

	// An empty update reverts to the config baseline.
	m.onRemoteConfigUpdate(map[string]state.RawConfig{}, apply)
	name, fromPolicy = matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db-user"}))
	require.Equal(t, "config-default", name)
	require.False(t, fromPolicy)
}

// TestOnRemoteConfigUpdate_InvalidPayloadKeepsBaseline verifies that a malformed
// policies document is reported as an error and does not disturb the baseline.
func TestOnRemoteConfigUpdate_InvalidPayloadKeepsBaseline(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcCatchAllCfg, wmeta)

	var applied []state.ApplyStatus
	apply := func(_ string, s state.ApplyStatus) { applied = append(applied, s) }

	m.onRemoteConfigUpdate(map[string]state.RawConfig{
		"datadog/2/APM_POLICIES/bad/config": {Config: []byte("{")},
	}, apply)

	require.Len(t, applied, 1)
	require.Equal(t, state.ApplyStateError, applied[0].State)

	// Baseline is untouched.
	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "config-default", name)
	require.False(t, fromPolicy)
}
