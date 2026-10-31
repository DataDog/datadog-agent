// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds the abstract syntax tree of the SECL language.
//
// The types of this package form the tree the SECL compiler works on. A tree is
// built by the parser (see the `parser` package), and is the input of the
// compilation to evaluators (see the `eval` package):
//
//	source ──parse──▶ ast ──compile──▶ evaluators
//
// The tree has one type per construct of the language. The `Node` interface is
// a closed union: a type switch over a `Node` enumerates every construct of the
// language, and only this package can add a new one.
package ast

import "fmt"

// Position is a position in the source expression.
type Position struct {
	// Offset is the offset in bytes, from 0.
	Offset int
	// Line is the line number, from 1.
	Line int
	// Column is the column in runes, from 1.
	Column int
}

// String renders the position as `line:column`.
func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Node is a node of the abstract syntax tree.
//
// The union is closed, the only implementations are the ones of this package.
type Node interface {
	// Position returns the position, in the source expression, of the first
	// token of the node.
	Position() Position

	// node closes the union.
	node()
}

// Meta is embedded in every node of the tree and holds what is common to all of
// them.
type Meta struct {
	// Pos is the position, in the source expression, of the first token of the
	// node.
	Pos Position
}

// At returns the Meta of a node starting at the given position.
func At(pos Position) Meta {
	return Meta{Pos: pos}
}

// Position returns the position, in the source expression, of the first token of
// the node.
func (m Meta) Position() Position {
	return m.Pos
}

func (Meta) node() {}

// Rule is the root of a rule. Its expression evaluates to a boolean.
type Rule struct {
	Meta

	// Expr is the source expression the rule has been parsed from.
	Expr string
	// Root is the expression of the rule.
	Root Node
}

// Macro is the root of a macro. Its expression evaluates either to a value or to
// an array of values.
type Macro struct {
	Meta

	// Root is the expression of the macro.
	Root Node
}
