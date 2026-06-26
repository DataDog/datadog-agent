// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package policies

import "strings"

// Facts carries the workload attributes a policy is evaluated against.
type Facts struct {
	NamespaceName   string
	NamespaceLabels map[string]string
	PodLabels       map[string]string
}

// Decide evaluates the policies in order and returns the outcome of the first
// one whose rule evaluates to ResultTrue. This reproduces the "first match
// wins" semantics of the native target matcher. The boolean is false when no
// policy matched.
func Decide(ps []Policy, f Facts) (Outcome, bool) {
	for i := range ps {
		if Evaluate(ps[i].Rules, f) == ResultTrue {
			return ps[i].Outcome, true
		}
	}
	return Outcome{}, false
}

// Evaluate walks the rule tree and returns its tri-state result.
func Evaluate(n *Node, f Facts) Result {
	if n == nil {
		return ResultAbstain
	}
	if n.Eval != nil {
		return n.Eval.eval(f)
	}
	switch n.Op {
	case OpNot:
		if len(n.Children) != 1 {
			return ResultAbstain
		}
		return doNot(Evaluate(n.Children[0], f))
	case OpOr:
		res := ResultFalse
		for _, c := range n.Children {
			res = doOr(res, Evaluate(c, f))
			if res == ResultTrue {
				return res
			}
		}
		return res
	case OpAnd:
		res := ResultTrue
		for _, c := range n.Children {
			res = doAnd(res, Evaluate(c, f))
			if res == ResultFalse {
				return res
			}
		}
		return res
	default:
		return ResultAbstain
	}
}

// doAnd implements tri-state AND: false dominates, abstain is contagious among
// non-false operands.
func doAnd(a, b Result) Result {
	if a == ResultFalse || b == ResultFalse {
		return ResultFalse
	}
	if a != ResultAbstain && b != ResultAbstain {
		return ResultTrue
	}
	return ResultAbstain
}

// doOr implements tri-state OR: true dominates, abstain is contagious among
// non-true operands.
func doOr(a, b Result) Result {
	if a == ResultTrue || b == ResultTrue {
		return ResultTrue
	}
	if a != ResultAbstain && b != ResultAbstain {
		return ResultFalse
	}
	return ResultAbstain
}

// doNot flips true/false and leaves abstain unchanged.
func doNot(a Result) Result {
	switch a {
	case ResultTrue:
		return ResultFalse
	case ResultFalse:
		return ResultTrue
	default:
		return ResultAbstain
	}
}

func (e *Evaluator) eval(f Facts) Result {
	switch e.Source {
	case SourceAlwaysTrue:
		return ResultTrue
	case SourceAlwaysFalse:
		return ResultFalse
	case SourceAlwaysAbstain:
		return ResultAbstain
	case SourceNamespaceName:
		return boolToResult(compare(e.Cmp, e.Value, f.NamespaceName))
	case SourceNamespaceLabel:
		v, ok := f.NamespaceLabels[e.Key]
		return labelResult(e.Cmp, e.Value, v, ok)
	case SourcePodLabel:
		v, ok := f.PodLabels[e.Key]
		return labelResult(e.Cmp, e.Value, v, ok)
	default:
		return ResultAbstain
	}
}

// labelResult resolves a label-keyed evaluator. A missing label is false for
// every comparison except CmpExists (which reports presence). This matches
// Kubernetes label-selector semantics once composed with the tree operators:
// In/matchLabels require presence, while NotIn/DoesNotExist match absent keys
// via the surrounding NOT.
func labelResult(cmp Cmp, want, got string, present bool) Result {
	if cmp == CmpExists {
		return boolToResult(present)
	}
	if !present {
		return ResultFalse
	}
	return boolToResult(compare(cmp, want, got))
}

func compare(cmp Cmp, pattern, value string) bool {
	switch cmp {
	case CmpExact:
		return pattern == value
	case CmpPrefix:
		return strings.HasPrefix(value, pattern)
	case CmpSuffix:
		return strings.HasSuffix(value, pattern)
	case CmpContains:
		return strings.Contains(value, pattern)
	case CmpWildcard:
		return wildcardMatch(pattern, value)
	case CmpExists:
		return true
	default:
		return false
	}
}

func boolToResult(b bool) Result {
	if b {
		return ResultTrue
	}
	return ResultFalse
}

// wildcardMatch reports whether s matches the glob pattern, where '*' matches
// any run of characters and '?' matches a single character. It is a linear
// two-pointer matcher operating on bytes, sufficient for label/namespace values.
func wildcardMatch(pattern, s string) bool {
	starPat, starStr := -1, -1
	p, i := 0, 0
	for i < len(s) {
		if p < len(pattern) && (pattern[p] == s[i] || pattern[p] == '?') {
			p++
			i++
			continue
		}
		if p < len(pattern) && pattern[p] == '*' {
			for p < len(pattern) && pattern[p] == '*' {
				p++
			}
			if p == len(pattern) {
				return true
			}
			starPat = p
			starStr = i
			continue
		}
		if starPat != -1 {
			p = starPat
			starStr++
			i = starStr
			continue
		}
		return false
	}
	for p < len(pattern) && pattern[p] == '*' {
		p++
	}
	return p == len(pattern)
}
