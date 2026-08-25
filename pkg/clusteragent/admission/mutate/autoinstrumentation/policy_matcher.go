// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/dd-policy-engine/go/policies"
)

// policyMatcher evaluates SSI policies against pods using the pure Go policy
// engine. It holds the effective ordered policy set (configuration policies,
// optionally augmented with remote-config ones) and resolves the first match.
type policyMatcher struct {
	policies []policies.Policy
	wmeta    workloadmeta.Component
}

// newPolicyMatcher builds a matcher over the given policies.
func newPolicyMatcher(ps []policies.Policy, wmeta workloadmeta.Component) *policyMatcher {
	return &policyMatcher{
		policies: ps,
		wmeta:    wmeta,
	}
}

// Match returns the outcome of the first policy that matches the pod, mirroring
// the "first match wins" semantics of the target mutator.
func (m *policyMatcher) Match(pod *corev1.Pod) (policies.Outcome, bool) {
	idx := m.matchIndex(pod)
	if idx < 0 {
		return policies.Outcome{}, false
	}
	return m.policies[idx].Outcome, true
}

// matchIndex returns the index of the first policy that matches the pod, or -1
// if none match. Policies are evaluated in order (first match wins).
//
// Namespace labels are fetched lazily when the first policy that needs them is
// reached. If they cannot be resolved, policies that need namespace labels are
// skipped while policies using the available pod and namespace-name facts keep
// their relative first-match ordering.
func (m *policyMatcher) matchIndex(pod *corev1.Pod) int {
	if m == nil || pod == nil {
		return -1
	}

	ctx := policies.Context{
		Strings: map[string]string{policies.IDNamespaceName: pod.Namespace},
		Labels:  map[string]map[string]string{policies.IDPodLabel: pod.Labels},
	}
	namespaceLabelsLoaded := false
	namespaceLabelsUnavailable := false

	for i := range m.policies {
		if nodeUsesNamespaceLabels(m.policies[i].Rules) && !namespaceLabelsLoaded {
			if namespaceLabelsUnavailable || m.wmeta == nil {
				namespaceLabelsUnavailable = true
				continue
			}

			nsLabels, err := getNamespaceLabels(m.wmeta, pod.Namespace)
			if err != nil {
				log.Debugf("policy matcher: namespace labels unavailable for namespace %q, namespace-label rules will be skipped: %v", pod.Namespace, err)
				namespaceLabelsUnavailable = true
				continue
			}
			ctx.Labels[policies.IDNamespaceLabel] = nsLabels
			namespaceLabelsLoaded = true
		}

		if policies.Evaluate(m.policies[i].Rules, ctx) == policies.ResultTrue {
			return i
		}
	}
	return -1
}

func nodeUsesNamespaceLabels(n *policies.Node) bool {
	if n == nil {
		return false
	}
	if n.Eval != nil {
		return n.Eval.ID == policies.IDNamespaceLabel
	}
	for _, c := range n.Children {
		if nodeUsesNamespaceLabels(c) {
			return true
		}
	}
	return false
}
