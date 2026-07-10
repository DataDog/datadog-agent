// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	corev1 "k8s.io/api/core/v1"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/policies"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
	if m == nil || pod == nil {
		return policies.Outcome{}, false
	}
	facts, err := m.factsForPod(pod)
	if err != nil {
		log.Debugf("policy matcher: could not build facts for pod in %q: %v", pod.Namespace, err)
		return policies.Outcome{}, false
	}
	return policies.Decide(m.policies, facts)
}

// matchIndex returns the index of the first policy that matches the pod, or -1
// if none match. It returns an error when a required fact (e.g. namespace
// labels) could not be resolved, so the caller can abort matching rather than
// risk an inaccurate decision.
func (m *policyMatcher) matchIndex(pod *corev1.Pod) (int, error) {
	if m == nil || pod == nil {
		return -1, nil
	}
	facts, err := m.factsForPod(pod)
	if err != nil {
		return -1, err
	}
	for i := range m.policies {
		if policies.Evaluate(m.policies[i].Rules, facts) == policies.ResultTrue {
			return i, nil
		}
	}
	return -1, nil
}

func (m *policyMatcher) factsForPod(pod *corev1.Pod) (policies.Facts, error) {
	facts := policies.Facts{
		NamespaceName: pod.Namespace,
		PodLabels:     pod.Labels,
	}
	if m.needsNamespaceLabels && m.wmeta != nil {
		nsLabels, err := getNamespaceLabels(m.wmeta, pod.Namespace)
		if err != nil {
			return facts, err
		}
		facts.NamespaceLabels = nsLabels
	}
	return facts, nil
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
