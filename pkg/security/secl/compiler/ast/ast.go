// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds the abstract syntax tree of the SECL language.
package ast

import "fmt"

// Position is a position in the source expression.
type Position struct {
	Offset int
	Line   int
	Column int // in runes
	// Source is the ID of the macro the position is in, empty for the
	// expression being compiled. An expanded macro keeps its own positions.
	Source string
}

// String renders the position as `line:column`.
func (p Position) String() string {
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Node is a node of the abstract syntax tree.
type Node interface {
	// Position returns the position of the first token of the node.
	Position() Position
	// Metadata returns the Meta of the node, for the passes to update it.
	Metadata() *Meta

	// copy returns a shallow copy of the node, see Copy.
	copy() Node
	// editChildren replaces each child of the node, see EditChildren.
	editChildren(edit func(Node) Node)
	// doChildren calls do on each child of the node, see DoChildren.
	doChildren(do func(Node) bool) bool
}

// Meta is embedded in every node of the tree.
type Meta struct {
	Pos Position
}

// At returns the Meta of a node starting at the given position.
func At(pos Position) Meta {
	return Meta{Pos: pos}
}

// Position returns the position of the first token of the node.
func (m Meta) Position() Position {
	return m.Pos
}

// Metadata returns the Meta of the node.
func (m *Meta) Metadata() *Meta {
	return m
}

// Rule is the root of a rule. Its expression evaluates to a boolean.
type Rule struct {
	Meta

	Expr string
	Root Node
}

// Macro is the root of a macro. Its expression evaluates either to a value or to
// an array of values.
type Macro struct {
	Meta

	Root Node
}
