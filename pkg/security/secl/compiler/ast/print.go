// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package ast holds the abstract syntax tree of the SECL language.
package ast

import (
	"fmt"
	"strconv"
	"strings"
)

// Sprint returns the tree as an s-expression. It is meant for debugging and
// testing, it is not a SECL expression and can't be parsed back.
//
// A subtree coming from an expanded macro is enclosed in braces, along with the
// ID of the macro: `{my_macro (== process.uid 0)}`. The root of an expanded
// macro being positioned at its reference, a macro that is a lone value isn't.
func Sprint(node Node) string {
	var sb strings.Builder
	p := printer{sb: &sb, source: node.Position().Source}
	p.sprint(node)
	return sb.String()
}

type printer struct {
	sb *strings.Builder
	// source is the source of the node being printed
	source string
}

func (p printer) sprint(node Node) {
	sb := p.sb
	switch node := node.(type) {
	case *Rule:
		p.sprint(node.Root)
	case *Macro:
		p.sprint(node.Root)
	case *LogicalExpr:
		p.sprintOp(node.Op.String(), node.Left, node.Right)
	case *ScalarCompareExpr:
		p.sprintOp(node.Op.String(), node.Left, node.Right)
	case *ArrayCompareExpr:
		p.sprintOp(node.Op.String(), node.Left, node.Right)
	case *BitExpr:
		p.sprintOp(node.Op.String(), node.Left, node.Right)
	case *ArithExpr:
		p.sprintOp(node.Op.String(), node.Left, node.Right)
	case *UnaryExpr:
		p.sprintOp(node.Op.String(), node.Operand)
	case *Ident:
		sb.WriteString(node.Name)
	case *IntLiteral:
		sb.WriteString(strconv.Itoa(node.Value))
	case *DurationLiteral:
		sb.WriteString(strconv.Itoa(node.Value))
		sb.WriteString("ns")
	case *StringLiteral:
		sb.WriteString(strconv.Quote(node.Value))
	case *PatternLiteral:
		sb.WriteString("~")
		sb.WriteString(strconv.Quote(node.Value))
	case *RegexpLiteral:
		sb.WriteString("r")
		sb.WriteString(strconv.Quote(node.Value))
	case *IPLiteral:
		sb.WriteString(node.Value)
	case *CIDRLiteral:
		sb.WriteString(node.Value)
	case *VariableRef:
		sb.WriteString("${")
		sb.WriteString(node.Name)
		sb.WriteString("}")
	case *FieldRef:
		sb.WriteString("%{")
		sb.WriteString(node.Name)
		sb.WriteString("}")
	case *StringArray:
		members := make([]Node, 0, len(node.Members))
		for _, member := range node.Members {
			members = append(members, member)
		}
		p.sprintArray(members)
	case *CIDRArray:
		members := make([]Node, 0, len(node.Members))
		for _, member := range node.Members {
			members = append(members, member)
		}
		p.sprintArray(members)
	case *IntArray:
		sb.WriteString("[")
		for i, value := range node.Values {
			if i != 0 {
				sb.WriteString(" ")
			}
			sb.WriteString(strconv.Itoa(value))
		}
		sb.WriteString("]")
	case *IdentArray:
		sb.WriteString("[")
		sb.WriteString(strings.Join(node.Names, " "))
		sb.WriteString("]")
	default:
		fmt.Fprintf(sb, "<%T>", node)
	}
}

// enter prints a node whose children are in another source, the root of an
// expanded macro, in braces. It reports whether it did.
func (p printer) enter(children []Node, print func(printer)) bool {
	if len(children) == 0 || children[0].Position().Source == p.source {
		return false
	}
	source := children[0].Position().Source
	p.sb.WriteString("{" + source + " ")
	print(printer{sb: p.sb, source: source})
	p.sb.WriteString("}")
	return true
}

func (p printer) sprintOp(op string, operands ...Node) {
	if p.enter(operands, func(p printer) { p.sprintOp(op, operands...) }) {
		return
	}
	p.sb.WriteString("(")
	p.sb.WriteString(op)
	for _, operand := range operands {
		p.sb.WriteString(" ")
		p.sprint(operand)
	}
	p.sb.WriteString(")")
}

func (p printer) sprintArray(members []Node) {
	if p.enter(members, func(p printer) { p.sprintArray(members) }) {
		return
	}
	p.sb.WriteString("[")
	for i, member := range members {
		if i != 0 {
			p.sb.WriteString(" ")
		}
		p.sprint(member)
	}
	p.sb.WriteString("]")
}
