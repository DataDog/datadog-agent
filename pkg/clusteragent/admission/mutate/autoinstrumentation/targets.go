// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package autoinstrumentation

import (
	"sort"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/DataDog/datadog-agent/pkg/clusteragent/admission/mutate/autoinstrumentation/policies"
)

// policiesFromTargets lowers an ordered list of SSI configuration targets into
// an equivalent ordered list of policies. Evaluating the result reproduces the
// native target matcher: namespace and pod selectors are ANDed, match-expressions
// and match-labels are ANDed within a selector, and a target with no selectors
// matches every workload.
//
// This is the static-configuration counterpart of the remote-config dd-wls
// parser: both produce policies.Policy, which is the single shape the matcher
// understands. The targets configuration is an autoinstrumentation concept, so
// the lowering lives here rather than in the policies engine package.
func policiesFromTargets(ts []Target) []policies.Policy {
	out := make([]policies.Policy, 0, len(ts))
	for _, t := range ts {
		var conds []*policies.Node
		if t.NamespaceSelector != nil {
			conds = append(conds, namespacePolicyConds(t.NamespaceSelector)...)
		}
		if t.PodSelector != nil {
			conds = append(conds, podPolicyConds(t.PodSelector)...)
		}
		out = append(out, policies.Policy{
			Name:  t.Name,
			Rules: policies.And(conds),
			Outcome: policies.Outcome{
				Inject:         true,
				TracerVersions: t.TracerVersions,
				TracerConfigs:  tracerConfigsToEnvVars(t.TracerConfigs),
			},
		})
	}
	return out
}

func namespacePolicyConds(ns *NamespaceSelector) []*policies.Node {
	var conds []*policies.Node
	if len(ns.MatchNames) > 0 {
		names := make([]*policies.Node, 0, len(ns.MatchNames))
		for _, n := range ns.MatchNames {
			names = append(names, policies.Leaf(policies.SourceNamespaceName, "", policies.CmpExact, n))
		}
		conds = append(conds, policies.Or(names))
	}
	conds = append(conds, matchLabelPolicyConds(policies.SourceNamespaceLabel, ns.MatchLabels)...)
	for _, req := range ns.MatchExpressions {
		conds = append(conds, exprPolicyCond(policies.SourceNamespaceLabel, req))
	}
	return conds
}

func podPolicyConds(ps *PodSelector) []*policies.Node {
	conds := matchLabelPolicyConds(policies.SourcePodLabel, ps.MatchLabels)
	for _, req := range ps.MatchExpressions {
		conds = append(conds, exprPolicyCond(policies.SourcePodLabel, req))
	}
	return conds
}

func matchLabelPolicyConds(src policies.Source, m map[string]string) []*policies.Node {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	conds := make([]*policies.Node, 0, len(keys))
	for _, k := range keys {
		conds = append(conds, policies.Leaf(src, k, policies.CmpExact, m[k]))
	}
	return conds
}

func exprPolicyCond(src policies.Source, req SelectorMatchExpression) *policies.Node {
	switch req.Operator {
	case metav1.LabelSelectorOpIn:
		return policies.Or(eqPolicyNodes(src, req.Key, req.Values))
	case metav1.LabelSelectorOpNotIn:
		return policies.Not(policies.Or(eqPolicyNodes(src, req.Key, req.Values)))
	case metav1.LabelSelectorOpExists:
		return policies.Leaf(src, req.Key, policies.CmpExists, "")
	case metav1.LabelSelectorOpDoesNotExist:
		return policies.Not(policies.Leaf(src, req.Key, policies.CmpExists, ""))
	default:
		return policies.AlwaysAbstain()
	}
}

func eqPolicyNodes(src policies.Source, key string, values []string) []*policies.Node {
	nodes := make([]*policies.Node, 0, len(values))
	for _, v := range values {
		nodes = append(nodes, policies.Leaf(src, key, policies.CmpExact, v))
	}
	return nodes
}

func tracerConfigsToEnvVars(configs []TracerConfig) []policies.EnvVar {
	if len(configs) == 0 {
		return nil
	}
	out := make([]policies.EnvVar, len(configs))
	for i, c := range configs {
		out[i] = policies.EnvVar{Name: c.Name, Value: c.Value}
	}
	return out
}
