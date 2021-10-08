// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"github.com/alecthomas/participle/lexer"
)

// Expression represents basic expression syntax that can be evaluated for an Instance
type Expression struct {
	Pos lexer.Position

	Comparison *Comparison `parser:"@@"`
	Op         *string     `parser:"[ @( \"|\" \"|\" | \"&\" \"&\" )"`
	Next       *Expression `parser:"  @@ ]"`
}

// IterableExpression represents an iterable expration that can be evaluated for an Iterator
type IterableExpression struct {
	Pos lexer.Position

	IterableComparison *IterableComparison `parser:"@@"`
	Expression         *Expression         `parser:"| @@"`
}

// IterableComparison allows evaluating a builtin pseudo-funciion for an iterable expression
type IterableComparison struct {
	Pos lexer.Position

	Fn               *string           `parser:"@( \"count\" | \"all\" | \"none\" )"`
	Expression       *Expression       `parser:"\"(\" @@ \")\""`
	ScalarComparison *ScalarComparison `parser:"[ @@ ]"`
}

// PathExpression represents an expression evaluating to a file path or file glob
type PathExpression struct {
	Pos lexer.Position

	Path       *string     `parser:"@UnixSystemPath"`
	Expression *Expression `parser:"| @@"`
}

// Comparison represents syntax for comparison operations
type Comparison struct {
	Pos lexer.Position

	Term             *Term             `parser:"@@"`
	ScalarComparison *ScalarComparison `parser:"[ @@"`
	ArrayComparison  *ArrayComparison  `parser:"| @@ ]"`
}

// ScalarComparison represents syntax for scalar comparison
type ScalarComparison struct {
	Pos lexer.Position

	Op   *string     `parser:"@( \">\" \"=\" | \"<\" \"=\" | \">\" | \"<\" | \"!\" \"=\" | \"=\" \"=\" | \"=\" \"~\" | \"!\" \"~\" )"`
	Next *Comparison `parser:"  @@"`
}

// ArrayComparison represents syntax for array comparison
type ArrayComparison struct {
	Pos lexer.Position

	Op *string `parser:"( @( \"in\" | \"not\" \"in\" )"`
	// TODO: likely doesn't work with rhs expression
	Array *Array `parser:"@@ )"`
}

// Term is an abstract term allowing optional binary bit operation syntax
type Term struct {
	Pos lexer.Position

	Unary *Unary  `parser:"@@"`
	Op    *string `parser:"[ @( \"&\" | \"|\" | \"^\" | \"+\" )"`
	Next  *Term   `parser:"  @@ ]"`
}

// Unary is a unary bit operation syntax
type Unary struct {
	Pos lexer.Position

	Op    *string `parser:"( @( \"!\" | \"-\" | \"^\" )"`
	Unary *Unary  `parser:"  @@ )"`
	Value *Value  `parser:"| @@"`
}

// Array provides support for array syntax and may contain any valid Values (mixed allowed)
type Array struct {
	Pos lexer.Position

	Values []Value `parser:"\"[\" @@ { \",\" @@ } \"]\""`
	Ident  *string `parser:"| @Ident"`
}

// Value provides support for various value types in expression including
// integers in various form, strings, function calls, variables and
// subexpressions
type Value struct {
	Pos lexer.Position

	Hex           *string     `parser:"  @Hex"`
	Octal         *string     `parser:"| @Octal"`
	Decimal       *int64      `parser:"| @Decimal"`
	String        *string     `parser:"| @String"`
	Call          *Call       `parser:"| @@"`
	Variable      *string     `parser:"| @Ident"`
	Subexpression *Expression `parser:"| \"(\" @@ \")\""`
}

// Call implements function call syntax
type Call struct {
	Pos lexer.Position

	Name string        `parser:"@Ident"`
	Args []*Expression `parser:"\"(\" [ @@ { \",\" @@ } ] \")\""`
}
