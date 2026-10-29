// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds the abstract syntax tree of the SECL language.
package ast

import "slices"

// EditChildren replaces each child x of n by edit(x). It is the building block of
// the compilation passes.
//
// Only the immediate children of n are edited: edit(x) calls EditChildren(x, edit)
// itself if the children of x have to be edited as well.
//
// The tree is edited in place, edit a DeepCopy to leave it unchanged.
func EditChildren(n Node, edit func(Node) Node) {
	if n == nil {
		return
	}
	n.editChildren(edit)
}

// Copy returns a shallow copy of n: the copy shares the children of n.
func Copy[N Node](n N) N {
	return n.copy().(N)
}

// DeepCopy returns a deep copy of the tree rooted at n.
func DeepCopy[N Node](n N) N {
	var edit func(Node) Node
	edit = func(x Node) Node {
		x = x.copy()
		EditChildren(x, edit)
		return x
	}
	return edit(n).(N)
}

func (n *Rule) copy() Node { c := *n; return &c }
func (n *Rule) editChildren(edit func(Node) Node) {
	n.Root = edit(n.Root)
}

func (n *Macro) copy() Node { c := *n; return &c }
func (n *Macro) editChildren(edit func(Node) Node) {
	n.Root = edit(n.Root)
}

func (n *LogicalExpr) copy() Node { c := *n; return &c }
func (n *LogicalExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}

func (n *ScalarCompareExpr) copy() Node { c := *n; return &c }
func (n *ScalarCompareExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}

func (n *ArrayCompareExpr) copy() Node { c := *n; return &c }
func (n *ArrayCompareExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}

func (n *BitExpr) copy() Node { c := *n; return &c }
func (n *BitExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}

func (n *ArithExpr) copy() Node { c := *n; return &c }
func (n *ArithExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}

func (n *UnaryExpr) copy() Node { c := *n; return &c }
func (n *UnaryExpr) editChildren(edit func(Node) Node) {
	n.Operand = edit(n.Operand)
}

func (n *Ident) copy() Node                   { c := *n; return &c }
func (n *Ident) editChildren(func(Node) Node) {}

func (n *BoolLiteral) copy() Node                   { c := *n; return &c }
func (n *BoolLiteral) editChildren(func(Node) Node) {}

func (n *IntLiteral) copy() Node                   { c := *n; return &c }
func (n *IntLiteral) editChildren(func(Node) Node) {}

func (n *DurationLiteral) copy() Node                   { c := *n; return &c }
func (n *DurationLiteral) editChildren(func(Node) Node) {}

func (n *StringLiteral) copy() Node                   { c := *n; return &c }
func (n *StringLiteral) editChildren(func(Node) Node) {}

func (n *PatternLiteral) copy() Node                   { c := *n; return &c }
func (n *PatternLiteral) editChildren(func(Node) Node) {}

func (n *RegexpLiteral) copy() Node                   { c := *n; return &c }
func (n *RegexpLiteral) editChildren(func(Node) Node) {}

func (n *IPLiteral) copy() Node                   { c := *n; return &c }
func (n *IPLiteral) editChildren(func(Node) Node) {}

func (n *CIDRLiteral) copy() Node                   { c := *n; return &c }
func (n *CIDRLiteral) editChildren(func(Node) Node) {}

func (n *VariableRef) copy() Node                   { c := *n; return &c }
func (n *VariableRef) editChildren(func(Node) Node) {}

func (n *FieldRef) copy() Node                   { c := *n; return &c }
func (n *FieldRef) editChildren(func(Node) Node) {}

// The copy of an array owns its slice of members, editing them leaves the
// original unchanged.

func (n *StringArray) copy() Node { c := *n; c.Members = slices.Clone(n.Members); return &c }
func (n *StringArray) editChildren(edit func(Node) Node) {
	for i, member := range n.Members {
		n.Members[i] = edit(member).(StringArrayMember)
	}
}

func (n *CIDRArray) copy() Node { c := *n; c.Members = slices.Clone(n.Members); return &c }
func (n *CIDRArray) editChildren(edit func(Node) Node) {
	for i, member := range n.Members {
		n.Members[i] = edit(member).(CIDRArrayMember)
	}
}

func (n *IntArray) copy() Node                   { c := *n; c.Values = slices.Clone(n.Values); return &c }
func (n *IntArray) editChildren(func(Node) Node) {}

func (n *IdentArray) copy() Node                   { c := *n; c.Names = slices.Clone(n.Names); return &c }
func (n *IdentArray) editChildren(func(Node) Node) {}
