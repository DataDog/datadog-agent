// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"fmt"
	"math/bits"
	"slices"
	"strings"
	"sync/atomic"
)

// Rule coverage tracks which evaluation paths through the boolean expression of
// a rule have been taken at least once. A path is one distinct short-circuit
// walk through the expression: for `A && (B || C)` the four paths are
//
//	A=false                 => false
//	A=true B=true           => true
//	A=true B=false C=true   => true
//	A=true B=false C=false  => false
//
// The paths are enumerated statically from the boolean skeleton built while the
// rule is compiled (see skeleton.go), and the evaluator records which one was
// taken on every evaluation.

const (
	// maxCoverageLeaves is the highest number of boolean leaves a rule can hold
	// for its coverage to be tracked. A path is recorded as a pair of bitmaps
	// over the leaves, so this is bounded by the width of the bitmap.
	maxCoverageLeaves = 64

	// maxCoveragePaths caps the number of paths enumerated for a single rule.
	// The path count is multiplicative in the number of nested alternatives, so
	// pathological expressions are reported as unsupported rather than
	// enumerated, which also bounds the size of the coverage report.
	maxCoveragePaths = 1024
)

var (
	errTooManyCoverageLeaves = fmt.Errorf("more than %d boolean sub-expressions", maxCoverageLeaves)
	errTooManyCoveragePaths  = fmt.Errorf("more than %d evaluation paths", maxCoveragePaths)
)

// covPathKey identifies an evaluation path by the set of leaves it evaluates and
// the value each of them took.
type covPathKey struct {
	seen   uint64
	values uint64
}

// covPath is one statically enumerated evaluation path
type covPath struct {
	key    covPathKey
	order  []int // leaves, in evaluation order
	result bool
}

// enumerate returns every evaluation path of the sub-tree, in the order in
// which the evaluator walks them.
func enumerate(node *skelNode) ([]covPath, error) {
	switch node.kind {
	case skelConst:
		return []covPath{{result: node.value}}, nil
	case skelLeaf:
		// false first, so that the paths of the rule are reported in the order
		// of a truth table, shortest first
		mask := uint64(1) << uint(node.covIdx)
		return []covPath{
			{key: covPathKey{seen: mask}, order: []int{node.covIdx}, result: false},
			{key: covPathKey{seen: mask, values: mask}, order: []int{node.covIdx}, result: true},
		}, nil
	case skelNot:
		paths, err := enumerate(node.child)
		if err != nil {
			return nil, err
		}
		for i := range paths {
			paths[i].result = !paths[i].result
		}
		return paths, nil
	case skelAnd, skelOr:
		// the left operand short-circuits the right one when its result already
		// decides the outcome: true for `||`, false for `&&`
		shortCircuit := node.kind == skelOr

		left, err := enumerate(node.left)
		if err != nil {
			return nil, err
		}
		right, err := enumerate(node.right)
		if err != nil {
			return nil, err
		}

		// a left operand folded to a constant does not short-circuit an `||`: Or
		// combines both sides instead of testing them in turn, so the right
		// operand is evaluated whatever the constant is
		alwaysRight := node.kind == skelOr && node.left.kind == skelConst

		var paths []covPath
		for _, l := range left {
			if l.result == shortCircuit && !alwaysRight {
				paths = append(paths, l)
				continue
			}
			for _, r := range right {
				if len(paths) >= maxCoveragePaths {
					return nil, errTooManyCoveragePaths
				}
				result := r.result
				if node.kind == skelOr && l.result {
					result = true
				}
				paths = append(paths, covPath{
					key:    covPathKey{seen: l.key.seen | r.key.seen, values: l.key.values | r.key.values},
					order:  append(append(make([]int, 0, len(l.order)+len(r.order)), l.order...), r.order...),
					result: result,
				})
			}
		}
		return paths, nil
	}

	return nil, fmt.Errorf("unknown skeleton node kind %d", node.kind)
}

// assignLeafIndices numbers the leaves reachable from the node in evaluation
// order, and returns how many there are.
func assignLeafIndices(node *skelNode, next int) (int, error) {
	switch node.kind {
	case skelConst:
	case skelLeaf:
		if next >= maxCoverageLeaves {
			return next, errTooManyCoverageLeaves
		}
		node.covIdx = next
		next++
	case skelNot:
		return assignLeafIndices(node.child, next)
	case skelAnd, skelOr:
		var err error
		if next, err = assignLeafIndices(node.left, next); err != nil {
			return next, err
		}
		return assignLeafIndices(node.right, next)
	}
	return next, nil
}

// clearLeafIndices detaches every leaf of the sub-tree from the coverage
// bitmaps, turning the instrumentation into a no-op.
func clearLeafIndices(node *skelNode) {
	switch node.kind {
	case skelLeaf:
		node.covIdx = -1
	case skelNot:
		clearLeafIndices(node.child)
	case skelAnd, skelOr:
		clearLeafIndices(node.left)
		clearLeafIndices(node.right)
	}
}

// RuleCoverage accumulates how many times each evaluation path of a rule has
// been taken. It is safe for concurrent use.
type RuleCoverage struct {
	expr   string
	root   *skelNode
	leaves []*skelNode
	paths  []covPath
	index  map[covPathKey]int

	// inRuleOrder lists the leaves, by index, in the order they appear in the
	// rule. The indexes themselves follow the evaluation order, which the
	// operators are free to reorder.
	inRuleOrder []int

	// reason is set when the paths of the rule could not be enumerated, in
	// which case nothing is recorded
	reason string

	// hasEmptyPath reports whether one of the paths evaluates no leaf at all,
	// which happens for rules folded to a constant
	hasEmptyPath bool

	hits      []atomic.Uint64 // parallel to paths
	leafTrue  []atomic.Uint64 // parallel to leaves
	leafFalse []atomic.Uint64
	unmatched atomic.Uint64
}

// newRuleCoverage enumerates the evaluation paths of the boolean skeleton and
// returns the accumulator for them. It always returns a usable value: a rule
// whose paths cannot be enumerated is reported as unsupported.
func newRuleCoverage(expr string, root *skelNode) *RuleCoverage {
	coverage := &RuleCoverage{expr: expr, root: root}
	if root == nil {
		coverage.reason = "the rule holds no boolean expression"
		return coverage
	}

	count, err := assignLeafIndices(root, 0)
	if err == nil {
		coverage.paths, err = enumerate(root)
	}
	if err != nil {
		clearLeafIndices(root)
		coverage.reason = err.Error()
		return coverage
	}

	coverage.leaves = make([]*skelNode, count)
	collectLeaves(root, coverage.leaves)

	// name the leaves after their position in the rule rather than after their
	// evaluation order, so that the legend of the report reads in the same order
	// as the rule itself
	coverage.inRuleOrder = make([]int, count)
	for i := range coverage.inRuleOrder {
		coverage.inRuleOrder[i] = i
	}
	slices.SortStableFunc(coverage.inRuleOrder, func(a, b int) int {
		return coverage.leaves[a].offset - coverage.leaves[b].offset
	})
	for position, leaf := range coverage.inRuleOrder {
		coverage.leaves[leaf].name = skelLeafName(position)
	}

	coverage.index = make(map[covPathKey]int, len(coverage.paths))
	for i, path := range coverage.paths {
		if path.key.seen == 0 {
			coverage.hasEmptyPath = true
		}
		if _, dup := coverage.index[path.key]; !dup {
			coverage.index[path.key] = i
		}
	}

	coverage.hits = make([]atomic.Uint64, len(coverage.paths))
	coverage.leafTrue = make([]atomic.Uint64, count)
	coverage.leafFalse = make([]atomic.Uint64, count)

	return coverage
}

func collectLeaves(node *skelNode, leaves []*skelNode) {
	switch node.kind {
	case skelLeaf:
		leaves[node.covIdx] = node
	case skelNot:
		collectLeaves(node.child, leaves)
	case skelAnd, skelOr:
		collectLeaves(node.left, leaves)
		collectLeaves(node.right, leaves)
	}
}

// record accounts for one evaluation that walked the given path
func (c *RuleCoverage) record(seen, values uint64) {
	if index, ok := c.index[covPathKey{seen: seen, values: values}]; ok {
		c.hits[index].Add(1)
	} else {
		c.unmatched.Add(1)
	}

	for seen != 0 {
		leaf := bits.TrailingZeros64(seen)
		if values&(uint64(1)<<uint(leaf)) != 0 {
			c.leafTrue[leaf].Add(1)
		} else {
			c.leafFalse[leaf].Add(1)
		}
		seen &= seen - 1
	}
}

// coverageRecorder holds the path being walked by the evaluation in progress. It
// belongs to the evaluation context, so it is never shared between goroutines.
type coverageRecorder struct {
	target *RuleCoverage
	seen   uint64
	values uint64
}

// record accounts for one leaf of the rule being evaluated
func (r *coverageRecorder) record(leaf int, value bool) {
	if r.target == nil || leaf < 0 {
		return
	}

	mask := uint64(1) << uint(leaf)
	if r.seen&mask != 0 {
		// the same leaf is being evaluated a second time: the rule iterates
		// over a register, so close the path of the previous iteration
		r.finish()
	}

	r.seen |= mask
	if value {
		r.values |= mask
	}
}

// finish accounts for the path walked so far and starts a new one
func (r *coverageRecorder) finish() {
	if r.target == nil {
		return
	}
	if r.seen != 0 || r.target.hasEmptyPath {
		r.target.record(r.seen, r.values)
	}
	r.seen, r.values = 0, 0
}

// LeafCoverage reports how often one boolean sub-expression of a rule evaluated
// to true and to false.
type LeafCoverage struct {
	// Name is the short name given to the sub-expression in Skeleton
	Name string `json:"name"`
	// Expression is the source text of the sub-expression
	Expression string `json:"expression"`
	// Offset and Length locate the sub-expression in the rule expression
	Offset int    `json:"offset"`
	Length int    `json:"length"`
	True   uint64 `json:"true"`
	False  uint64 `json:"false"`
}

// PathCondition is one leaf outcome required to walk an evaluation path
type PathCondition struct {
	Leaf  string `json:"leaf"`
	Value bool   `json:"value"`
}

// PathCoverage reports how often one evaluation path of a rule was taken
type PathCoverage struct {
	// Conditions lists the leaf outcomes of the path, in evaluation order
	Conditions []PathCondition `json:"conditions"`
	// Result is the outcome of the rule when this path is taken
	Result bool   `json:"result"`
	Hits   uint64 `json:"hits"`
}

// String returns the path as a `A=true B=false` condition list
func (p PathCoverage) String() string {
	var builder strings.Builder
	for i, condition := range p.Conditions {
		if i > 0 {
			builder.WriteByte(' ')
		}
		builder.WriteString(condition.Leaf)
		if condition.Value {
			builder.WriteString("=true")
		} else {
			builder.WriteString("=false")
		}
	}
	if builder.Len() == 0 {
		return "<constant>"
	}
	return builder.String()
}

// Coverage reports the evaluation coverage of a single rule
type Coverage struct {
	// Expression is the rule expression
	Expression string `json:"expression"`
	// Skeleton is the boolean structure of the rule, with every leaf replaced by
	// its short name, for instance `A && (B || C)`. The leaves are named after
	// their position in the rule but the structure follows the evaluation order,
	// so a skeleton such as `B && A` means the operands were reordered to test
	// the cheapest one first.
	Skeleton string `json:"skeleton"`
	// Unsupported explains why the coverage of the rule is not tracked. The
	// remaining fields are empty when it is set.
	Unsupported string `json:"unsupported,omitempty"`

	TotalPaths   int `json:"total_paths"`
	CoveredPaths int `json:"covered_paths"`
	// Evaluations is the number of recorded evaluations of the rule
	Evaluations uint64 `json:"evaluations"`
	// Unmatched counts the evaluations that walked a path that was not
	// enumerated, which would be an instrumentation bug
	Unmatched uint64 `json:"unmatched,omitempty"`

	Leaves []LeafCoverage `json:"leaves,omitempty"`
	Paths  []PathCoverage `json:"paths,omitempty"`
}

// Report returns a snapshot of the coverage accumulated so far
func (c *RuleCoverage) Report() *Coverage {
	report := &Coverage{
		Expression:  c.expr,
		Unsupported: c.reason,
	}
	if c.reason != "" {
		return report
	}

	report.Skeleton = skelRender(c.root, c.root.kind)

	report.Leaves = make([]LeafCoverage, 0, len(c.leaves))
	for _, i := range c.inRuleOrder {
		leaf := c.leaves[i]
		report.Leaves = append(report.Leaves, LeafCoverage{
			Name:       leaf.name,
			Expression: leaf.label,
			Offset:     leaf.offset,
			Length:     leaf.length,
			True:       c.leafTrue[i].Load(),
			False:      c.leafFalse[i].Load(),
		})
	}

	report.TotalPaths = len(c.paths)
	report.Unmatched = c.unmatched.Load()
	report.Evaluations = report.Unmatched
	report.Paths = make([]PathCoverage, len(c.paths))
	for i, path := range c.paths {
		hits := c.hits[i].Load()
		if hits > 0 {
			report.CoveredPaths++
		}
		report.Evaluations += hits

		conditions := make([]PathCondition, len(path.order))
		for j, leaf := range path.order {
			conditions[j] = PathCondition{
				Leaf:  c.leaves[leaf].name,
				Value: path.key.values&(uint64(1)<<uint(leaf)) != 0,
			}
		}

		report.Paths[i] = PathCoverage{
			Conditions: conditions,
			Result:     path.result,
			Hits:       hits,
		}
	}

	return report
}

// Reset drops the coverage accumulated so far
func (c *RuleCoverage) Reset() {
	for i := range c.hits {
		c.hits[i].Store(0)
	}
	for i := range c.leafTrue {
		c.leafTrue[i].Store(0)
		c.leafFalse[i].Store(0)
	}
	c.unmatched.Store(0)
}
