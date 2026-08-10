// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"fmt"
	"math/bits"
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
// The paths are enumerated statically when the rule is compiled, and the
// evaluator records which one was taken on every evaluation.

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

// covKind is the kind of a node of the boolean skeleton of a rule
type covKind uint8

const (
	// covLeaf is a sub-expression that coverage does not look into, typically a
	// comparison, a macro reference or a bare boolean field
	covLeaf covKind = iota
	// covConst is a sub-expression folded to a constant at compile time
	covConst
	covAnd
	covOr
	covNot
)

func (k covKind) operator() string {
	if k == covOr {
		return " || "
	}
	return " && "
}

// covNode is a node of the boolean skeleton of a rule. It is built while the
// rule is compiled and mirrors the short-circuit structure of the generated
// evaluator.
type covNode struct {
	// builder is the compilation the node was created by, used to tell the nodes
	// of the rule being compiled apart from the ones left on an evaluator shared
	// with another rule, such as a macro
	builder *coverageBuilder

	kind        covKind
	left, right *covNode // covAnd, covOr
	child       *covNode // covNot

	value bool // covConst

	// covLeaf. idx is the position of the leaf in the coverage bitmaps, and is
	// only assigned once the whole skeleton is known: it stays negative for
	// leaves that ended up unreachable from the root, and for every leaf of a
	// rule whose coverage could not be enumerated.
	idx    int
	offset int
	length int
	label  string
}

// coverageBuilder holds the compile time state needed to build the boolean
// skeleton of a rule.
type coverageBuilder struct {
	expr string
}

// covOperand returns the evaluator to use in place of an operand of a boolean
// operator. An operand that is not already a coverage sub-tree of the rule being
// compiled becomes a leaf, instrumented to record its outcome.
//
// The offset locates the sub-expression in the rule expression, and is used to
// recover its source text.
//
// The operand is never modified: evaluators can be shared between rules, macro
// values in particular, and each rule needs leaves of its own.
func (s *State) covOperand(operand *BoolEvaluator, offset int) *BoolEvaluator {
	if s.cov == nil || (operand.covNode != nil && operand.covNode.builder == s.cov) {
		return operand
	}

	instrumented := *operand

	if operand.EvalFnc == nil {
		instrumented.covNode = &covNode{builder: s.cov, kind: covConst, value: operand.Value, idx: -1}
		return &instrumented
	}

	node := &covNode{builder: s.cov, kind: covLeaf, idx: -1}
	node.offset, node.length = covSpan(s.cov.expr, offset)
	node.label = s.cov.expr[node.offset : node.offset+node.length]

	inner := operand.EvalFnc
	instrumented.EvalFnc = func(ctx *Context) bool {
		value := inner(ctx)
		ctx.coverage.record(node.idx, value)
		return value
	}
	instrumented.covNode = node

	return &instrumented
}

// covJoin records that the given evaluator combines the two operands with the
// given boolean operator. The evaluator is always freshly built by And or Or, so
// it is never shared with another rule.
func (s *State) covJoin(joined *BoolEvaluator, kind covKind, left, right *BoolEvaluator) {
	if s.cov == nil {
		return
	}
	joined.covNode = &covNode{builder: s.cov, kind: kind, left: left.covNode, right: right.covNode, idx: -1}
}

// covNegate records that the given evaluator is the negation of the operand
func (s *State) covNegate(negated *BoolEvaluator, operand *BoolEvaluator) {
	if s.cov == nil {
		return
	}
	negated.covNode = &covNode{builder: s.cov, kind: covNot, child: operand.covNode, idx: -1}
}

// covSpan returns the extent, within the rule expression, of the sub-expression
// starting at the given offset. The sub-expression ends at the first top level
// boolean operator or at the first closing parenthesis that it does not open
// itself.
func covSpan(expr string, start int) (int, int) {
	if start < 0 || start >= len(expr) {
		return 0, 0
	}

	depth, end := 0, len(expr)

scan:
	for i := start; i < len(expr); i++ {
		switch c := expr[i]; c {
		case '"':
			// strings, patterns and regexps are all double quoted, and may hold
			// any of the characters looked at below
			i = covSkipString(expr, i)
		case '(':
			depth++
		case ')':
			if depth == 0 {
				end = i
				break scan
			}
			depth--
		case '&', '|':
			if depth == 0 && i+1 < len(expr) && expr[i+1] == c {
				end = i
				break scan
			}
		case 'a', 'o':
			if depth == 0 && covIsWordStart(expr, i) &&
				(covHasWord(expr, i, "and") || covHasWord(expr, i, "or")) {
				end = i
				break scan
			}
		}
	}

	return start, len(strings.TrimRight(expr[start:end], " \t\n"))
}

// covSkipString returns the index of the closing quote of the string starting at
// the given opening quote, or the index of the end of the expression.
func covSkipString(expr string, start int) int {
	for i := start + 1; i < len(expr); i++ {
		switch expr[i] {
		case '\\':
			i++
		case '"':
			return i
		}
	}
	return len(expr)
}

func covIsIdentChar(c byte) bool {
	return c == '_' || c == '.' || c == '[' || c == ']' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// covIsWordStart reports whether the given offset starts an identifier
func covIsWordStart(expr string, i int) bool {
	return i == 0 || !covIsIdentChar(expr[i-1])
}

// covHasWord reports whether the given identifier starts at the given offset
func covHasWord(expr string, i int, word string) bool {
	if !strings.HasPrefix(expr[i:], word) {
		return false
	}
	next := i + len(word)
	return next == len(expr) || !covIsIdentChar(expr[next])
}

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
func enumerate(node *covNode) ([]covPath, error) {
	switch node.kind {
	case covConst:
		return []covPath{{result: node.value}}, nil
	case covLeaf:
		// false first, so that the paths of the rule are reported in the order
		// of a truth table, shortest first
		mask := uint64(1) << uint(node.idx)
		return []covPath{
			{key: covPathKey{seen: mask}, order: []int{node.idx}, result: false},
			{key: covPathKey{seen: mask, values: mask}, order: []int{node.idx}, result: true},
		}, nil
	case covNot:
		paths, err := enumerate(node.child)
		if err != nil {
			return nil, err
		}
		for i := range paths {
			paths[i].result = !paths[i].result
		}
		return paths, nil
	case covAnd, covOr:
		// the left operand short-circuits the right one when its result already
		// decides the outcome: true for `||`, false for `&&`
		shortCircuit := node.kind == covOr

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
		alwaysRight := node.kind == covOr && node.left.kind == covConst

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
				if node.kind == covOr && l.result {
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

	return nil, fmt.Errorf("unknown coverage node kind %d", node.kind)
}

// assignLeafIndices numbers the leaves reachable from the node in evaluation
// order, and returns how many there are.
func assignLeafIndices(node *covNode, next int) (int, error) {
	switch node.kind {
	case covConst:
	case covLeaf:
		if next >= maxCoverageLeaves {
			return next, errTooManyCoverageLeaves
		}
		node.idx = next
		next++
	case covNot:
		return assignLeafIndices(node.child, next)
	case covAnd, covOr:
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
func clearLeafIndices(node *covNode) {
	switch node.kind {
	case covLeaf:
		node.idx = -1
	case covNot:
		clearLeafIndices(node.child)
	case covAnd, covOr:
		clearLeafIndices(node.left)
		clearLeafIndices(node.right)
	}
}

// RuleCoverage accumulates how many times each evaluation path of a rule has
// been taken. It is safe for concurrent use.
type RuleCoverage struct {
	expr   string
	root   *covNode
	leaves []*covNode
	paths  []covPath
	index  map[covPathKey]int

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
func newRuleCoverage(expr string, root *covNode) *RuleCoverage {
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

	coverage.leaves = make([]*covNode, count)
	collectLeaves(root, coverage.leaves)

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

func collectLeaves(node *covNode, leaves []*covNode) {
	switch node.kind {
	case covLeaf:
		leaves[node.idx] = node
	case covNot:
		collectLeaves(node.child, leaves)
	case covAnd, covOr:
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
	// its short name, for instance `A && (B || C)`
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

	report.Skeleton = covRender(c.root, c.root.kind)

	report.Leaves = make([]LeafCoverage, len(c.leaves))
	for i, leaf := range c.leaves {
		report.Leaves[i] = LeafCoverage{
			Name:       covLeafName(i),
			Expression: leaf.label,
			Offset:     leaf.offset,
			Length:     leaf.length,
			True:       c.leafTrue[i].Load(),
			False:      c.leafFalse[i].Load(),
		}
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
				Leaf:  covLeafName(leaf),
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

// covLeafName returns the short name of the nth leaf: A, B, ... Z, AA, AB, ...
func covLeafName(index int) string {
	name := []byte{byte('A' + index%26)}
	for index /= 26; index > 0; index /= 26 {
		name = append([]byte{byte('A' + (index-1)%26)}, name...)
	}
	return string(name)
}

// covRender returns the boolean structure of the sub-tree with every leaf
// replaced by its short name. The parent kind is used to parenthesize only what
// needs it: SECL has no precedence between `&&` and `||`, so a nested operator
// of a different kind is always parenthesized.
func covRender(node *covNode, parent covKind) string {
	switch node.kind {
	case covLeaf:
		return covLeafName(node.idx)
	case covConst:
		if node.value {
			return "true"
		}
		return "false"
	case covNot:
		return "!" + covRender(node.child, covNot)
	case covAnd, covOr:
		rendered := covRender(node.left, node.kind) + node.kind.operator() + covRender(node.right, node.kind)
		if parent != node.kind {
			return "(" + rendered + ")"
		}
		return rendered
	}
	return "?"
}
