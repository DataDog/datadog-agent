// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ir

// Expression is a typed and validated set of operations for compilation
// and evaluation.
type Expression struct {
	// The type of the expression.
	Type Type
	// The operations that make up the expression, in reverse-polish notation.
	Operations []ExpressionOp
}

var (
	_ ExpressionOp = (*LocationOp)(nil)
)

// ExpressionOp is an operation that can be performed on an expression.
type ExpressionOp interface {
	irOp() // marker
}

// LocationOp references data of Size bytes
// at Offset in Variable.
type LocationOp struct {
	// The variable that is referenced.
	Variable *Variable

	// The offset in bytes from the start of the variable to extract.
	Offset uint32

	// The size of the data to extract in bytes.
	ByteSize uint32
}

func (*LocationOp) irOp() {}

// DSLExpression represents a parsed DSL expression
type DSLExpression struct {
	// Type of the resulting value (boolean for conditions, varies for others)
	Type Type
	// AST is the parsed AST from remote config
	AST DSLNode
	// OriginalDSL is the original DSL text
	OriginalDSL string
}

// DSLNode represents a node in the DSL abstract syntax tree
type DSLNode interface {
	dslNode() // marker
}

// ComparisonNode represents a comparison operation in the DSL (<, >, <=, >=, ==, !=)
type ComparisonNode struct {
	Op    ComparisonOp // <, >, <=, >=, ==, !=
	Left  DSLNode      // left side of the comparison
	Right DSLNode      // right side of the comparison
}

func (ComparisonNode) dslNode() {}

// LogicalNode represents a logical operation in the DSL (&&, ||, !)
type LogicalNode struct {
	Op    LogicalOp // &&, ||, !
	Left  DSLNode   // nil in case of  op == !
	Right DSLNode
}

func (LogicalNode) dslNode() {}

// VaValueReferenceNode represents a value reference in the DSL (@duration, var.field, etc...)
type ValueReferenceNode struct {
	Path         []string // ["var", "field1", "field2"]
	IsContextual bool     // true for @duration, @return, etc
}

func (ValueReferenceNode) dslNode() {}

// LiteralNode represents a literal value in the DSL
type LiteralNode struct {
	Value interface{} // string, number, boolean, null
	Type  Type        // IR type of the literal
}

func (LiteralNode) dslNode() {}

// Function calls (isEmpty, len, any, all, etc)
type FunctionCallNode struct {
	Name string    // "isEmpty", "len", "any", etc
	Args []DSLNode // Arguments to the function
}

func (FunctionCallNode) dslNode() {}

// Collection operations with predicates
type CollectionOpNode struct {
	Op        CollectionOp // any, all, filter
	Source    DSLNode      // Collection to operate on
	Predicate DSLNode      // Predicate for each element (@it context)
}

func (CollectionOpNode) dslNode() {}

// ComparisonOp is an operation that can be performed on a comparison expression
type ComparisonOp int

const (
	// ComparisonLT is the less than operation (<)
	ComparisonLT ComparisonOp = iota // <
	// ComparisonLE is the less than or equal operation (<=)
	ComparisonLE
	// ComparisonGT is the greater than operation (>)
	ComparisonGT
	// ComparisonGE is the greater than or equal operation (>=)
	ComparisonGE
	// ComparisonEQ is the equal operation (==)
	ComparisonEQ
	// ComparisonNE is the not equal operation (!=)
	ComparisonNE
)

// LogicalOp is an operation that can be performed on a logical expression
type LogicalOp int

const (
	// LogicalAnd is the and operation (&&)
	LogicalAnd LogicalOp = iota
	// LogicalOr is the or operation (||)
	LogicalOr
	// LogicalNot is the not operation (!)
	LogicalNot
)

// CollectionOp is an operation that can be performed on a collection expression
type CollectionOp int

const (
	// CollectionAny is the any operation (any)
	CollectionAny CollectionOp = iota
	// CollectionAll is the all operation (all)
	CollectionAll
	// CollectionFilter is the filter operation (filter)
	CollectionFilter
)
