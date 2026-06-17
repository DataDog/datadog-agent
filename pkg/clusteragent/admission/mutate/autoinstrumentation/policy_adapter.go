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

// toPolicyTargets adapts the agent Target configuration onto the dependency-free
// policies.Target shape so it can be lowered into the policy model.
func toPolicyTargets(targets []Target) []policies.Target {
	out := make([]policies.Target, 0, len(targets))
	for _, t := range targets {
		out = append(out, policies.Target{
			Name:              t.Name,
			NamespaceSelector: toPolicyNamespaceSelector(t.NamespaceSelector),
			PodSelector:       toPolicyPodSelector(t.PodSelector),
			TracerVersions:    t.TracerVersions,
			TracerConfigs:     toPolicyEnvVars(t.TracerConfigs),
		})
	}
	return out
}

func toPolicyNamespaceSelector(ns *NamespaceSelector) *policies.NamespaceSelector {
	if ns == nil {
		return nil
	}
	return &policies.NamespaceSelector{
		MatchNames:       ns.MatchNames,
		MatchLabels:      ns.MatchLabels,
		MatchExpressions: toPolicyExpressions(ns.MatchExpressions),
	}
}

func toPolicyPodSelector(ps *PodSelector) *policies.PodSelector {
	if ps == nil {
		return nil
	}
	return &policies.PodSelector{
		MatchLabels:      ps.MatchLabels,
		MatchExpressions: toPolicyExpressions(ps.MatchExpressions),
	}
}

func toPolicyExpressions(exprs []SelectorMatchExpression) []policies.LabelSelectorRequirement {
	if len(exprs) == 0 {
		return nil
	}
	out := make([]policies.LabelSelectorRequirement, len(exprs))
	for i, e := range exprs {
		out[i] = policies.LabelSelectorRequirement{
			Key:      e.Key,
			Operator: string(e.Operator),
			Values:   e.Values,
		}
	}
	return out
}

func toPolicyEnvVars(configs []TracerConfig) []policies.EnvVar {
	if len(configs) == 0 {
		return nil
	}
	out := make([]policies.EnvVar, len(configs))
	for i, c := range configs {
		out[i] = policies.EnvVar{Name: c.Name, Value: c.Value}
	}
	return out
}

// policyMatcher evaluates SSI policies against pods using the pure Go policy
// engine. It is the native (CGO-free) counterpart of the targetInternal match
// path and is fed by the same targets, compiled once per remote-config update.
type policyMatcher struct {
	policies             []policies.Policy
	wmeta                workloadmeta.Component
	needsNamespaceLabels bool
}

// newPolicyMatcher compiles the targets into policies and records whether any
// rule reads namespace labels, so we only pay the workloadmeta lookup when a
// policy actually needs it.
func newPolicyMatcher(targets []Target, wmeta workloadmeta.Component) *policyMatcher {
	compiled := policies.FromTargets(toPolicyTargets(targets))
	return &policyMatcher{
		policies:             compiled,
		wmeta:                wmeta,
		needsNamespaceLabels: usesNamespaceLabels(compiled),
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
