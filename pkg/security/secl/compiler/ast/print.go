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
func Sprint(node Node) string {
	var sb strings.Builder
	sprint(&sb, node)
	return sb.String()
}

func sprint(sb *strings.Builder, node Node) {
	switch node := node.(type) {
	case *Rule:
		sprint(sb, node.Root)
	case *Macro:
		sprint(sb, node.Root)
	case *LogicalExpr:
		sprintOp(sb, node.Op.String(), node.Left, node.Right)
	case *ScalarCompareExpr:
		sprintOp(sb, node.Op.String(), node.Left, node.Right)
	case *ArrayCompareExpr:
		sprintOp(sb, node.Op.String(), node.Left, node.Right)
	case *BitExpr:
		sprintOp(sb, node.Op.String(), node.Left, node.Right)
	case *ArithExpr:
		sprintOp(sb, node.Op.String(), node.Left, node.Right)
	case *UnaryExpr:
		sprintOp(sb, node.Op.String(), node.Operand)
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
		sprintArray(sb, members)
	case *CIDRArray:
		members := make([]Node, 0, len(node.Members))
		for _, member := range node.Members {
			members = append(members, member)
		}
		sprintArray(sb, members)
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

func sprintOp(sb *strings.Builder, op string, operands ...Node) {
	sb.WriteString("(")
	sb.WriteString(op)
	for _, operand := range operands {
		sb.WriteString(" ")
		sprint(sb, operand)
	}
	sb.WriteString(")")
}

func sprintArray(sb *strings.Builder, members []Node) {
	sb.WriteString("[")
	for i, member := range members {
		if i != 0 {
			sb.WriteString(" ")
		}
		sprint(sb, member)
	}
	sb.WriteString("]")
}
