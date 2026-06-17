// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package policies

import "sort"

// Label-selector operators, matching metav1.LabelSelectorOperator string values.
const (
	OpIn           = "In"
	OpNotIn        = "NotIn"
	OpExists       = "Exists"
	OpDoesNotExist = "DoesNotExist"
)

// LabelSelectorRequirement mirrors the SSI target match-expression and the
// Kubernetes metav1.LabelSelectorRequirement.
type LabelSelectorRequirement struct {
	Key      string
	Operator string
	Values   []string
}

// PodSelector mirrors the SSI target pod selector.
type PodSelector struct {
	MatchLabels      map[string]string
	MatchExpressions []LabelSelectorRequirement
}

// NamespaceSelector mirrors the SSI target namespace selector.
type NamespaceSelector struct {
	MatchNames       []string
	MatchLabels      map[string]string
	MatchExpressions []LabelSelectorRequirement
}

// Target mirrors the SSI target configuration. It is a local, dependency-free
// copy of the autoinstrumentation.Target shape so this package stays portable;
// a thin adapter maps the agent type onto it.
type Target struct {
	Name              string
	NamespaceSelector *NamespaceSelector
	PodSelector       *PodSelector
	TracerVersions    map[string]string
	TracerConfigs     []EnvVar
}

// FromTargets lowers an ordered list of SSI targets into an equivalent ordered
// list of policies. Evaluating the result with Decide reproduces the native
// target matcher: namespace and pod selectors are ANDed, match-expressions and
// match-labels are ANDed within a selector, and a target with no selectors
// matches every workload.
func FromTargets(ts []Target) []Policy {
	out := make([]Policy, 0, len(ts))
	for _, t := range ts {
		var conds []*Node
		if t.NamespaceSelector != nil {
			conds = append(conds, namespaceConds(t.NamespaceSelector)...)
		}
		if t.PodSelector != nil {
			conds = append(conds, podConds(t.PodSelector)...)
		}
		out = append(out, Policy{
			Name:  t.Name,
			Rules: and(conds),
			Outcome: Outcome{
				Inject:         true,
				TracerVersions: t.TracerVersions,
				TracerConfigs:  t.TracerConfigs,
			},
		})
	}
	return out
}

func namespaceConds(ns *NamespaceSelector) []*Node {
	var conds []*Node
	if len(ns.MatchNames) > 0 {
		names := make([]*Node, 0, len(ns.MatchNames))
		for _, n := range ns.MatchNames {
			names = append(names, leaf(SourceNamespaceName, "", CmpExact, n))
		}
		conds = append(conds, or(names))
	}
	conds = append(conds, matchLabelConds(SourceNamespaceLabel, ns.MatchLabels)...)
	for _, req := range ns.MatchExpressions {
		conds = append(conds, exprCond(SourceNamespaceLabel, req))
	}
	return conds
}

func podConds(ps *PodSelector) []*Node {
	conds := matchLabelConds(SourcePodLabel, ps.MatchLabels)
	for _, req := range ps.MatchExpressions {
		conds = append(conds, exprCond(SourcePodLabel, req))
	}
	return conds
}

func matchLabelConds(src Source, m map[string]string) []*Node {
	if len(m) == 0 {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	conds := make([]*Node, 0, len(keys))
	for _, k := range keys {
		conds = append(conds, leaf(src, k, CmpExact, m[k]))
	}
	return conds
}

func exprCond(src Source, req LabelSelectorRequirement) *Node {
	switch req.Operator {
	case OpIn:
		return or(eqNodes(src, req.Key, req.Values))
	case OpNotIn:
		return not(or(eqNodes(src, req.Key, req.Values)))
	case OpExists:
		return leaf(src, req.Key, CmpExists, "")
	case OpDoesNotExist:
		return not(leaf(src, req.Key, CmpExists, ""))
	default:
		return alwaysAbstain()
	}
}

func eqNodes(src Source, key string, values []string) []*Node {
	nodes := make([]*Node, 0, len(values))
	for _, v := range values {
		nodes = append(nodes, leaf(src, key, CmpExact, v))
	}
	return nodes
}
