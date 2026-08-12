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
	loader := namespaceLabelLoader{wmeta: m.wmeta, namespace: pod.Namespace}

	for i := range m.policies {
		if nodeUsesNamespaceLabels(m.policies[i].Rules) && !loader.ensure(&ctx) {
			continue
		}

		if policies.Evaluate(m.policies[i].Rules, ctx) == policies.ResultTrue {
			return i
		}
	}
	return -1
}

// namespaceCouldMatch reports whether any policy could match a workload in the
// given namespace. It answers a strictly weaker question than matchIndex: only
// the namespace facts are known here, so a rule reading pod labels evaluates to
// ResultAbstain and keeps its policy a candidate. A namespace is ruled out only
// when every policy is contradicted by the namespace facts (ResultFalse).
//
// This is what keeps namespace eligibility aligned with the policies actually in
// effect, whether they come from the configuration targets or from remote config:
// a namespace-scoped policy no longer makes unrelated namespaces eligible.
//
// Policies that deny injection are skipped: they never instrument anything, so
// they must not make a namespace eligible on their own.
func (m *policyMatcher) namespaceCouldMatch(namespace string) bool {
	if m == nil {
		return false
	}

	ctx := policies.Context{
		Strings: map[string]string{policies.IDNamespaceName: namespace},
		// Pod labels are deliberately absent rather than empty: an unavailable
		// label source abstains, while an empty one would evaluate to false and
		// wrongly rule out every pod-scoped policy.
		Labels: map[string]map[string]string{},
	}
	loader := namespaceLabelLoader{wmeta: m.wmeta, namespace: namespace}

	for i := range m.policies {
		if !m.policies[i].Outcome.Inject {
			continue
		}

		if nodeUsesNamespaceLabels(m.policies[i].Rules) && !loader.ensure(&ctx) {
			continue
		}

		if policies.Evaluate(m.policies[i].Rules, ctx) != policies.ResultFalse {
			return true
		}
	}
	return false
}

// namespaceLabelLoader resolves a namespace's labels into an evaluation context
// at most once. When they cannot be resolved, the policies that read them are
// skipped rather than aborting the whole evaluation, so a namespace-name,
// pod-only or catch-all policy keeps its chance to match.
type namespaceLabelLoader struct {
	wmeta       workloadmeta.Component
	namespace   string
	loaded      bool
	unavailable bool
}

// ensure loads the namespace labels into ctx if they are not there yet, and
// reports whether policies reading namespace labels can be evaluated.
func (l *namespaceLabelLoader) ensure(ctx *policies.Context) bool {
	if l.loaded {
		return true
	}
	if l.unavailable || l.wmeta == nil {
		l.unavailable = true
		return false
	}

	nsLabels, err := getNamespaceLabels(l.wmeta, l.namespace)
	if err != nil {
		log.Debugf("policy matcher: namespace labels unavailable for namespace %q, namespace-label rules will be skipped: %v", l.namespace, err)
		l.unavailable = true
		return false
	}

	ctx.Labels[policies.IDNamespaceLabel] = nsLabels
	l.loaded = true
	return true
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
