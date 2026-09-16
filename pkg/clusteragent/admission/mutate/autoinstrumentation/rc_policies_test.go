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

const rcSSIOnNoTargets = `
apm_config:
  instrumentation:
    enabled: true
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
		Rules:   policies.LabelLeaf(policies.IDPodLabel, key, policies.CmpExact, val),
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

// TestRemotePolicies_OverrideStaticMatch verifies last-TRUE-wins across planes:
// a matching RC policy overrides a static target; a static match stands when
// no RC policy matches.
func TestRemotePolicies_OverrideStaticMatch(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcCatchAllCfg, wmeta)

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		podLabelPolicy("remote", "app", "db", true, map[string]string{"python": "default"}),
		podLabelPolicy("remote-deny", "app", "legacy", false, nil),
	}))

	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "remote", name)
	require.True(t, fromPolicy)

	name, fromPolicy = matchedTarget(t, m, rcPod("ns", map[string]string{"app": "legacy"}))
	require.Equal(t, "", name)
	require.False(t, fromPolicy)

	name, fromPolicy = matchedTarget(t, m, rcPod("ns", map[string]string{"app": "other"}))
	require.Equal(t, "config-default", name)
	require.False(t, fromPolicy)
}

// TestRemotePolicies_ClearRevertsToBaseline verifies that clearing remote
// policies restores the synthetic inject-all default.
func TestRemotePolicies_ClearRevertsToBaseline(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcSSIOnNoTargets, wmeta)

	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "default", name)
	require.False(t, fromPolicy)

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		podLabelPolicy("remote", "app", "db", true, map[string]string{"python": "default"}),
	}))
	name, fromPolicy = matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "remote", name)
	require.True(t, fromPolicy)

	m.ClearRemotePolicies()
	name, fromPolicy = matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "default", name)
	require.False(t, fromPolicy)
}

// TestRemotePolicies_LastMatchWins verifies last-TRUE-wins among remote policies.
func TestRemotePolicies_LastMatchWins(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)

	require.NoError(t, m.SetRemotePolicies([]policies.Policy{
		podLabelPolicy("first", "app", "db", true, map[string]string{"java": "default"}),
		podLabelPolicy("second", "app", "db", true, map[string]string{"python": "default"}),
	}))

	name, _ := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "second", name)
}

// TestOnRemoteConfigUpdate_ParsesAndApplies exercises the remote-config callback
// end to end with a dd-wls policies document, then verifies that an empty update
// clears remote policies.
func TestOnRemoteConfigUpdate_ParsesAndApplies(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)

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
		"datadog/2/APM_POLICIES/1.kubernetes/config": {Config: []byte(raw)},
	}, apply)

	require.Len(t, applied, 1)
	require.Equal(t, state.ApplyStateAcknowledged, applied[0].State)

	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db-user"}))
	require.Equal(t, "java for db-user", name)
	require.True(t, fromPolicy)

	// An empty update clears remote policies (SSI off → nothing).
	m.onRemoteConfigUpdate(map[string]state.RawConfig{}, apply)
	require.Nil(t, m.getMatchingTarget(rcPod("ns", map[string]string{"app": "db-user"})))
}

func TestOnRemoteConfigUpdate_OrdersPolicyIDsByNumericPrefix(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)

	const allow = `{
      "policies": [{
        "description": "allow",
        "rules": {
          "node_type": "EvaluatorNode",
          "node": {
            "eval_type": "StrEvaluator",
            "eval": {"id": "POD_LABEL", "cmp": "CMP_EXACT", "value": "app=db"}
          }
        },
        "actions": [
          {"action": "INJECT_ALLOW"},
          {"action": "ENABLE_SDK", "values": ["java=latest"]}
        ]
      }]
    }`
	const deny = `{
      "policies": [{
        "description": "deny",
        "rules": {
          "node_type": "EvaluatorNode",
          "node": {
            "eval_type": "StrEvaluator",
            "eval": {"id": "POD_LABEL", "cmp": "CMP_EXACT", "value": "app=db"}
          }
        },
        "actions": [{"action": "INJECT_DENY"}]
      }]
    }`

	m.onRemoteConfigUpdate(map[string]state.RawConfig{
		"datadog/2/APM_POLICIES/10.kubernetes.deny/config": {Config: []byte(deny)},
		"datadog/2/APM_POLICIES/2.kubernetes.allow/config": {Config: []byte(allow)},
	}, func(string, state.ApplyStatus) {})

	remotePolicies := m.remotePolicies.Load()
	require.NotNil(t, remotePolicies)
	require.Len(t, remotePolicies.matcher.policies, 2)
	// Numeric prefix sorts 2.kubernetes.allow before 10.kubernetes.deny.
	require.Equal(t, "allow", remotePolicies.matcher.policies[0].Name)
	require.Equal(t, "deny", remotePolicies.matcher.policies[1].Name)

	// Last-TRUE-wins: deny is after allow, both match app=db.
	require.Nil(t, m.getMatchingTarget(rcPod("ns", map[string]string{"app": "db"})))
}

func TestOnRemoteConfigUpdate_KeepsOnlyKubernetesPolicyIDs(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcDisabledCfg, wmeta)

	const k8s = `{
      "policies": [{
        "description": "k8s-allow",
        "rules": {
          "node_type": "EvaluatorNode",
          "node": {
            "eval_type": "StrEvaluator",
            "eval": {"id": "POD_LABEL", "cmp": "CMP_EXACT", "value": "app=db"}
          }
        },
        "actions": [
          {"action": "INJECT_ALLOW"},
          {"action": "ENABLE_SDK", "values": ["java=latest"]}
        ]
      }]
    }`
	const linux = `{
      "policies": [{
        "description": "linux-deny",
        "rules": {
          "node_type": "EvaluatorNode",
          "node": {
            "eval_type": "StrEvaluator",
            "eval": {"id": "POD_LABEL", "cmp": "CMP_EXACT", "value": "app=db"}
          }
        },
        "actions": [{"action": "INJECT_DENY"}]
      }]
    }`

	applied := make(map[string]state.ApplyStatus)
	m.onRemoteConfigUpdate(map[string]state.RawConfig{
		"datadog/2/APM_POLICIES/1.linux/config":         {Config: []byte(linux)},
		"datadog/2/APM_POLICIES/1.kubernetesfoo/config": {Config: []byte(linux)},
		"datadog/2/APM_POLICIES/2.kubernetes/config":    {Config: []byte(k8s)},
		"datadog/2/APM_POLICIES/3.windows.block/config": {Config: []byte(linux)},
		"datadog/2/APM_POLICIES/policy-1/config":        {Config: []byte(linux)},
	}, func(path string, status state.ApplyStatus) { applied[path] = status })

	require.Len(t, applied, 5)
	for _, path := range []string{
		"datadog/2/APM_POLICIES/1.linux/config",
		"datadog/2/APM_POLICIES/1.kubernetesfoo/config",
		"datadog/2/APM_POLICIES/2.kubernetes/config",
		"datadog/2/APM_POLICIES/3.windows.block/config",
		"datadog/2/APM_POLICIES/policy-1/config",
	} {
		require.Equal(t, state.ApplyStateAcknowledged, applied[path].State)
	}

	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "k8s-allow", name)
	require.True(t, fromPolicy)

	m.onRemoteConfigUpdate(map[string]state.RawConfig{
		"datadog/2/APM_POLICIES/1.linux/config": {Config: []byte(linux)},
	}, func(string, state.ApplyStatus) {})
	require.Nil(t, m.getMatchingTarget(rcPod("ns", map[string]string{"app": "db"})))
}

// TestOnRemoteConfigUpdate_InvalidPayloadKeepsBaseline verifies that one malformed
// policies document reports every config as failed and does not disturb the baseline.
func TestOnRemoteConfigUpdate_InvalidPayloadKeepsBaseline(t *testing.T) {
	wmeta := newMatchTestWmeta(t)
	m := newMatchMutator(t, rcCatchAllCfg, wmeta)

	const validDeny = `{
      "policies": [{
        "description": "deny db",
        "rules": {
          "node_type": "EvaluatorNode",
          "node": {
            "eval_type": "StrEvaluator",
            "eval": {"id": "POD_LABEL", "cmp": "CMP_EXACT", "value": "app=db"}
          }
        },
        "actions": [{"action": "INJECT_DENY"}]
      }]
    }`

	applied := make(map[string]state.ApplyStatus)
	apply := func(path string, status state.ApplyStatus) { applied[path] = status }

	m.onRemoteConfigUpdate(map[string]state.RawConfig{
		"datadog/2/APM_POLICIES/1.kubernetes.valid/config": {Config: []byte(validDeny)},
		"datadog/2/APM_POLICIES/1.linux/config":            {Config: []byte(validDeny)},
		"datadog/2/APM_POLICIES/2.kubernetes.bad/config":   {Config: []byte("{")},
	}, apply)

	require.Len(t, applied, 3)
	for _, path := range []string{
		"datadog/2/APM_POLICIES/1.kubernetes.valid/config",
		"datadog/2/APM_POLICIES/2.kubernetes.bad/config",
	} {
		require.Equal(t, state.ApplyStateError, applied[path].State)
		require.NotEmpty(t, applied[path].Error)
	}
	require.Equal(t, state.ApplyStateAcknowledged, applied["datadog/2/APM_POLICIES/1.linux/config"].State)

	// Baseline is untouched.
	name, fromPolicy := matchedTarget(t, m, rcPod("ns", map[string]string{"app": "db"}))
	require.Equal(t, "config-default", name)
	require.False(t, fromPolicy)
}
