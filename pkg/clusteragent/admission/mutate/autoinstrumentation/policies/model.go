// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package policies

// Result is the tri-state outcome of evaluating a rule node, mirroring the
// dd-policy-engine C engine.
type Result uint8

const (
	// ResultFalse means the node evaluated to false.
	ResultFalse Result = iota
	// ResultTrue means the node evaluated to true.
	ResultTrue
	// ResultAbstain means the node could not produce a decision (e.g. an
	// unknown evaluator, or a fact source unavailable in this environment).
	ResultAbstain
)

// BoolOp is the boolean operator of a composite node.
type BoolOp uint8

const (
	// OpAnd is logical AND over the children.
	OpAnd BoolOp = iota
	// OpOr is logical OR over the children.
	OpOr
	// OpNot is logical NOT over a single child.
	OpNot
)

// Source identifies where a leaf evaluator reads its fact from.
type Source uint8

const (
	// SourceAlwaysTrue always evaluates to ResultTrue.
	SourceAlwaysTrue Source = iota
	// SourceAlwaysFalse always evaluates to ResultFalse.
	SourceAlwaysFalse
	// SourceAlwaysAbstain always evaluates to ResultAbstain.
	SourceAlwaysAbstain
	// SourceNamespaceName matches against the workload namespace name.
	SourceNamespaceName
	// SourceNamespaceLabel matches against a namespace label identified by Key.
	SourceNamespaceLabel
	// SourcePodLabel matches against a pod label identified by Key.
	SourcePodLabel
)

// Cmp is the comparison applied by a leaf evaluator between its Value and the
// fact read from the Source.
type Cmp uint8

const (
	// CmpExact is string equality.
	CmpExact Cmp = iota
	// CmpPrefix is true when the fact starts with Value.
	CmpPrefix
	// CmpSuffix is true when the fact ends with Value.
	CmpSuffix
	// CmpContains is true when the fact contains Value.
	CmpContains
	// CmpWildcard is glob matching (* and ?) of the fact against Value.
	CmpWildcard
	// CmpExists is true when the keyed fact is present, ignoring Value.
	CmpExists
)

// Node is a node in the rule tree. It is either a composite node (Eval == nil,
// using Op and Children) or a leaf (Eval != nil).
type Node struct {
	Op       BoolOp
	Children []*Node
	Eval     *Evaluator
}

// Evaluator is a leaf condition: it reads a fact from Source (keyed by Key for
// label sources) and compares it to Value using Cmp.
type Evaluator struct {
	Source Source
	Key    string
	Cmp    Cmp
	Value  string
}

// EnvVar is a tracer configuration environment variable returned by a matched
// policy.
type EnvVar struct {
	Name  string
	Value string
}

// Outcome is the configuration applied when a policy matches.
type Outcome struct {
	// Inject reports whether a matched workload should be instrumented. It is
	// true for an allow decision and false for a deny (first match wins, so a
	// matched deny stops evaluation without injecting).
	Inject bool
	// TracerVersions maps a tracer name to the version to inject.
	TracerVersions map[string]string
	// TracerConfigs are extra environment variables added alongside the tracer.
	TracerConfigs []EnvVar
}

// Policy pairs a rule tree with the outcome applied when the rule is true.
type Policy struct {
	Name string
	// ID is the canonical UUID of the policy, when the document carries one.
	// It is empty otherwise.
	ID      string
	Version int64
	Rules   *Node
	Outcome Outcome
}

func leaf(src Source, key string, cmp Cmp, val string) *Node {
	return &Node{Eval: &Evaluator{Source: src, Key: key, Cmp: cmp, Value: val}}
}

func alwaysTrue() *Node    { return &Node{Eval: &Evaluator{Source: SourceAlwaysTrue}} }
func alwaysFalse() *Node   { return &Node{Eval: &Evaluator{Source: SourceAlwaysFalse}} }
func alwaysAbstain() *Node { return &Node{Eval: &Evaluator{Source: SourceAlwaysAbstain}} }

func and(conds []*Node) *Node {
	switch len(conds) {
	case 0:
		return alwaysTrue()
	case 1:
		return conds[0]
	default:
		return &Node{Op: OpAnd, Children: conds}
	}
}

func or(conds []*Node) *Node {
	switch len(conds) {
	case 0:
		return alwaysFalse()
	case 1:
		return conds[0]
	default:
		return &Node{Op: OpOr, Children: conds}
	}
}

func not(n *Node) *Node { return &Node{Op: OpNot, Children: []*Node{n}} }
