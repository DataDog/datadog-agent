// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import "strings"

// The boolean skeleton of a rule mirrors the short-circuit structure of the
// generated evaluator: the `&&`, `||` and `!` nodes, and the sub-expressions
// they combine, which the skeleton keeps as opaque leaves. It is built while the
// rule is compiled, and the operands are instrumented as they are turned into
// leaves so that an evaluation can report which of them it reached.
//
// The skeleton is what rule coverage enumerates the evaluation paths from. It
// carries no accounting of its own: what is recorded, and how, belongs to the
// consumers in coverage.go.
//
// Which leaves an evaluation reaches depends on the order the operators test
// their operands in, and And and Or test the cheapest one first. The skeleton
// therefore follows that same order, through swapsOperands, rather than the
// order the operands appear in the rule.

// skelKind is the kind of a node of the boolean skeleton of a rule
type skelKind uint8

const (
	// skelLeaf is a sub-expression the skeleton does not look into, typically a
	// comparison, a macro reference or a bare boolean field
	skelLeaf skelKind = iota
	// skelConst is a sub-expression folded to a constant at compile time
	skelConst
	skelAnd
	skelOr
	skelNot
)

func (k skelKind) operator() string {
	if k == skelOr {
		return " || "
	}
	return " && "
}

// skelNode is a node of the boolean skeleton of a rule
type skelNode struct {
	// builder is the compilation the node was created by, used to tell the nodes
	// of the rule being compiled apart from the ones left on an evaluator shared
	// with another rule, such as a macro
	builder *skeletonBuilder

	kind        skelKind
	left, right *skelNode // skelAnd, skelOr
	child       *skelNode // skelNot

	value bool // skelConst

	// skelLeaf. covIdx is the position of the leaf in the coverage bitmaps, and
	// is only assigned once the whole skeleton is known: it stays negative for
	// leaves that ended up unreachable from the root, and for every leaf of a
	// rule whose coverage could not be enumerated. It follows the evaluation
	// order, whereas name follows the order the leaves appear in the rule.
	covIdx int
	name   string
	offset int
	length int
	label  string
}

// skeletonBuilder holds the compile time state needed to build the boolean
// skeleton of a rule.
type skeletonBuilder struct {
	expr string
}

// skelOperand returns the evaluator to use in place of an operand of a boolean
// operator. An operand that is not already a skeleton sub-tree of the rule being
// compiled becomes a leaf, instrumented to record its outcome.
//
// The offset locates the sub-expression in the rule expression, and is used to
// recover its source text.
//
// The operand is never modified: evaluators can be shared between rules, macro
// values in particular, and each rule needs leaves of its own.
func (s *State) skelOperand(operand *BoolEvaluator, offset int) *BoolEvaluator {
	if s.skel == nil || (operand.skelNode != nil && operand.skelNode.builder == s.skel) {
		return operand
	}

	instrumented := *operand

	if operand.EvalFnc == nil {
		instrumented.skelNode = &skelNode{builder: s.skel, kind: skelConst, value: operand.Value, covIdx: -1}
		return &instrumented
	}

	node := &skelNode{builder: s.skel, kind: skelLeaf, covIdx: -1}
	node.offset, node.length = skelSpan(s.skel.expr, offset)
	node.label = s.skel.expr[node.offset : node.offset+node.length]

	inner := operand.EvalFnc
	instrumented.EvalFnc = func(ctx *Context) bool {
		value := inner(ctx)
		ctx.coverage.record(node.covIdx, value)
		return value
	}
	instrumented.skelNode = node

	return &instrumented
}

// skelJoin records that the given evaluator combines the two operands with the
// given boolean operator. The evaluator is always freshly built by And or Or, so
// it is never shared with another rule.
//
// The children are stored in the order the operator evaluates them, not in the
// order they appear in the rule: the paths have to be enumerated the way they are
// walked, otherwise the recorded ones would match none of them.
func (s *State) skelJoin(joined *BoolEvaluator, kind skelKind, left, right *BoolEvaluator) {
	if s.skel == nil {
		return
	}

	first, second := left.skelNode, right.skelNode
	if swapsOperands(left, right) {
		first, second = second, first
	}

	joined.skelNode = &skelNode{builder: s.skel, kind: kind, left: first, right: second, covIdx: -1}
}

// skelNegate records that the given evaluator is the negation of the operand
func (s *State) skelNegate(negated *BoolEvaluator, operand *BoolEvaluator) {
	if s.skel == nil {
		return
	}
	negated.skelNode = &skelNode{builder: s.skel, kind: skelNot, child: operand.skelNode, covIdx: -1}
}

// skelSpan returns the extent, within the rule expression, of the sub-expression
// starting at the given offset. The sub-expression ends at the first top level
// boolean operator or at the first closing parenthesis that it does not open
// itself.
func skelSpan(expr string, start int) (int, int) {
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
			i = skelSkipString(expr, i)
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
			if depth == 0 && skelIsWordStart(expr, i) &&
				(skelHasWord(expr, i, "and") || skelHasWord(expr, i, "or")) {
				end = i
				break scan
			}
		}
	}

	return start, len(strings.TrimRight(expr[start:end], " \t\n"))
}

// skelSkipString returns the index of the closing quote of the string starting at
// the given opening quote, or the index of the end of the expression.
func skelSkipString(expr string, start int) int {
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

func skelIsIdentChar(c byte) bool {
	return c == '_' || c == '.' || c == '[' || c == ']' ||
		(c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

// skelIsWordStart reports whether the given offset starts an identifier
func skelIsWordStart(expr string, i int) bool {
	return i == 0 || !skelIsIdentChar(expr[i-1])
}

// skelHasWord reports whether the given identifier starts at the given offset
func skelHasWord(expr string, i int, word string) bool {
	if !strings.HasPrefix(expr[i:], word) {
		return false
	}
	next := i + len(word)
	return next == len(expr) || !skelIsIdentChar(expr[next])
}

// skelLeafName returns the short name of the nth leaf: A, B, ... Z, AA, AB, ...
func skelLeafName(index int) string {
	name := []byte{byte('A' + index%26)}
	for index /= 26; index > 0; index /= 26 {
		name = append([]byte{byte('A' + (index-1)%26)}, name...)
	}
	return string(name)
}

// skelRender returns the boolean structure of the sub-tree with every leaf
// replaced by its short name. The parent kind is used to parenthesize only what
// needs it: SECL has no precedence between `&&` and `||`, so a nested operator
// of a different kind is always parenthesized.
func skelRender(node *skelNode, parent skelKind) string {
	switch node.kind {
	case skelLeaf:
		return node.name
	case skelConst:
		if node.value {
			return "true"
		}
		return "false"
	case skelNot:
		return "!" + skelRender(node.child, skelNot)
	case skelAnd, skelOr:
		rendered := skelRender(node.left, node.kind) + node.kind.operator() + skelRender(node.right, node.kind)
		if parent != node.kind {
			return "(" + rendered + ")"
		}
		return rendered
	}
	return "?"
}
