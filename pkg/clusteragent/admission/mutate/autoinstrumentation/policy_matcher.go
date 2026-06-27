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
	policies             []policies.Policy
	wmeta                workloadmeta.Component
	needsNamespaceLabels bool
}

// newPolicyMatcher builds a matcher over the given policies and records whether
// any rule reads namespace labels, so we only pay the workloadmeta lookup when a
// policy actually needs it.
func newPolicyMatcher(ps []policies.Policy, wmeta workloadmeta.Component) *policyMatcher {
	return &policyMatcher{
		policies:             ps,
		wmeta:                wmeta,
		needsNamespaceLabels: usesNamespaceLabels(ps),
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
// A policy that reads namespace labels which could not be resolved (e.g. the
// pod's namespace is absent from the workloadmeta store) is skipped rather than
// fatal: it cannot be evaluated, so it is ignored, and the remaining policies
// are still evaluated in order. This guarantees that an unrelated unresolvable
// namespace rule never prevents an otherwise-matching rule from injecting.
func (m *policyMatcher) matchIndex(pod *corev1.Pod) int {
	if m == nil || pod == nil {
		return -1
	}
	facts, namespaceLabelsResolved := m.factsForPod(pod)
	for i := range m.policies {
		if !namespaceLabelsResolved && nodeUsesNamespaceLabels(m.policies[i].Rules) {
			// Cannot evaluate this rule without namespace labels; ignore it.
			continue
		}
		if policies.Evaluate(m.policies[i].Rules, facts) == policies.ResultTrue {
			return i
		}
	}
	return -1
}

// factsForPod builds the evaluation facts for a pod. The boolean reports whether
// namespace labels were resolved: when a policy needs them but they cannot be
// fetched, it is false and namespace-label rules are skipped by matchIndex.
func (m *policyMatcher) factsForPod(pod *corev1.Pod) (policies.Facts, bool) {
	facts := policies.Facts{
		NamespaceName: pod.Namespace,
		PodLabels:     pod.Labels,
	}
	if m.needsNamespaceLabels && m.wmeta != nil {
		nsLabels, err := getNamespaceLabels(m.wmeta, pod.Namespace)
		if err != nil {
			log.Debugf("policy matcher: namespace labels unavailable for namespace %q, namespace-label rules will be skipped: %v", pod.Namespace, err)
			return facts, false
		}
		facts.NamespaceLabels = nsLabels
	}
	return facts, true
}

// usesNamespaceLabels reports whether any policy rule reads a namespace label,
// so the matcher can skip the workloadmeta lookup otherwise.
func usesNamespaceLabels(ps []policies.Policy) bool {
	for i := range ps {
		if nodeUsesNamespaceLabels(ps[i].Rules) {
			return true
		}
	}
	return false
}

func nodeUsesNamespaceLabels(n *policies.Node) bool {
	if n == nil {
		return false
	}
	if n.Eval != nil {
		return n.Eval.Source == policies.SourceNamespaceLabel
	}
	for _, c := range n.Children {
		if nodeUsesNamespaceLabels(c) {
			return true
		}
	}
	return false
}
