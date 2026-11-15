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

// DoChildren calls do on each child x of n, in order, and stops at the first
// do(x) returning true. It returns whether one did.
//
// Only the immediate children of n are visited: do(x) calls DoChildren(x, do)
// itself if the children of x have to be visited as well. The tree is left
// unchanged.
func DoChildren(n Node, do func(Node) bool) bool {
	if n == nil {
		return false
	}
	return n.doChildren(do)
}

// Any returns whether a node of the tree rooted at n satisfies cond. The nodes are
// visited depth first, parents before children, and the walk stops at the first
// one satisfying cond.
func Any(n Node, cond func(Node) bool) bool {
	if n == nil {
		return false
	}
	var do func(Node) bool
	do = func(x Node) bool {
		return cond(x) || DoChildren(x, do)
	}
	return do(n)
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
func (n *Rule) doChildren(do func(Node) bool) bool {
	return do(n.Root)
}

func (n *Macro) copy() Node { c := *n; return &c }
func (n *Macro) editChildren(edit func(Node) Node) {
	n.Root = edit(n.Root)
}
func (n *Macro) doChildren(do func(Node) bool) bool {
	return do(n.Root)
}

func (n *LogicalExpr) copy() Node { c := *n; return &c }
func (n *LogicalExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}
func (n *LogicalExpr) doChildren(do func(Node) bool) bool {
	return do(n.Left) || do(n.Right)
}

func (n *ScalarCompareExpr) copy() Node { c := *n; return &c }
func (n *ScalarCompareExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}
func (n *ScalarCompareExpr) doChildren(do func(Node) bool) bool {
	return do(n.Left) || do(n.Right)
}

func (n *ArrayCompareExpr) copy() Node { c := *n; return &c }
func (n *ArrayCompareExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}
func (n *ArrayCompareExpr) doChildren(do func(Node) bool) bool {
	return do(n.Left) || do(n.Right)
}

func (n *BitExpr) copy() Node { c := *n; return &c }
func (n *BitExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}
func (n *BitExpr) doChildren(do func(Node) bool) bool {
	return do(n.Left) || do(n.Right)
}

func (n *ArithExpr) copy() Node { c := *n; return &c }
func (n *ArithExpr) editChildren(edit func(Node) Node) {
	n.Left, n.Right = edit(n.Left), edit(n.Right)
}
func (n *ArithExpr) doChildren(do func(Node) bool) bool {
	return do(n.Left) || do(n.Right)
}

func (n *UnaryExpr) copy() Node { c := *n; return &c }
func (n *UnaryExpr) editChildren(edit func(Node) Node) {
	n.Operand = edit(n.Operand)
}
func (n *UnaryExpr) doChildren(do func(Node) bool) bool {
	return do(n.Operand)
}

func (n *Ident) copy() Node                      { c := *n; return &c }
func (n *Ident) editChildren(func(Node) Node)    {}
func (n *Ident) doChildren(func(Node) bool) bool { return false }

func (n *Literal) copy() Node                      { c := *n; return &c }
func (n *Literal) editChildren(func(Node) Node)    {}
func (n *Literal) doChildren(func(Node) bool) bool { return false }

func (n *VariableRef) copy() Node                      { c := *n; return &c }
func (n *VariableRef) editChildren(func(Node) Node)    {}
func (n *VariableRef) doChildren(func(Node) bool) bool { return false }

func (n *FieldRef) copy() Node                      { c := *n; return &c }
func (n *FieldRef) editChildren(func(Node) Node)    {}
func (n *FieldRef) doChildren(func(Node) bool) bool { return false }

// The copy of an array owns its slice of members, editing them leaves the
// original unchanged.

func (n *Array) copy() Node { c := *n; c.Members = slices.Clone(n.Members); return &c }
func (n *Array) editChildren(edit func(Node) Node) {
	for i, member := range n.Members {
		n.Members[i] = edit(member).(*Literal)
	}
}
func (n *Array) doChildren(do func(Node) bool) bool {
	for _, member := range n.Members {
		if do(member) {
			return true
		}
	}
	return false
}

func (n *IdentArray) copy() Node                      { c := *n; c.Names = slices.Clone(n.Names); return &c }
func (n *IdentArray) editChildren(func(Node) Node)    {}
func (n *IdentArray) doChildren(func(Node) bool) bool { return false }
