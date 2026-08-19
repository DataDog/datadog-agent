// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"fmt"
	"reflect"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
)

// Compile turns an expression of the abstract syntax tree into an evaluator. The
// returned evaluator is one of the concrete evaluators of this package, its type
// depends on the type of the expression.
func Compile(node ast.Node, opts *Opts, state *State) (interface{}, ast.Position, error) {
	switch node := node.(type) {
	case *ast.Rule:
		return compileExpr(node.Root, opts, state)
	case *ast.Macro:
		return compileExpr(node.Root, opts, state)
	}

	return compileExpr(node, opts, state)
}

// compileExpr compiles a boolean expression: the root of a rule, of a macro, or
// an operand of a logical operation.
//
// A boolean expression that is a lone value, `open.flag` for example, is
// compared against `true`. This is what Unary does, on top of flagging the
// evaluator as deterministic.
func compileExpr(node ast.Node, opts *Opts, state *State) (interface{}, ast.Position, error) {
	evaluator, pos, err := compileNode(node, opts, state)
	if err != nil {
		return nil, pos, err
	}

	// a logical operation already yields a boolean, it is not compared against `true`
	if _, ok := node.(*ast.LogicalExpr); ok {
		return evaluator, pos, nil
	}

	if boolEvaluator, ok := evaluator.(*BoolEvaluator); ok {
		unary, err := Unary(boolEvaluator, state)
		if err != nil {
			return nil, node.Position(), err
		}
		return unary, node.Position(), nil
	}

	return evaluator, pos, nil
}

// compileNode compiles any node of the abstract syntax tree.
func compileNode(node ast.Node, opts *Opts, state *State) (interface{}, ast.Position, error) {
	switch node := node.(type) {
	case *ast.LogicalExpr:
		return compileLogicalChain(node, opts, state)

	case *ast.BitExpr:
		return compileBitChain(node, opts, state)

	case *ast.ArithExpr:
		left, pos, err := compileNode(node.Left, opts, state)
		if err != nil {
			return nil, pos, err
		}

		leftInt, ok := left.(*IntEvaluator)
		if !ok {
			return nil, node.Pos, NewTypeError(node.Pos, reflect.Int)
		}

		right, pos, err := compileNode(node.Right, opts, state)
		if err != nil {
			return nil, pos, err
		}

		rightInt, ok := right.(*IntEvaluator)
		if !ok {
			return nil, pos, NewTypeError(pos, reflect.Int)
		}

		var intEvaluator *IntEvaluator
		switch node.Op {
		case ast.ArithAdd:
			intEvaluator, err = IntPlus(leftInt, rightInt, state)
		case ast.ArithSub:
			intEvaluator, err = IntMinus(leftInt, rightInt, state)
		default:
			return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
		}
		if err != nil {
			return nil, pos, err
		}

		intEvaluator.isFromArithmeticOperation = true
		return intEvaluator, node.Pos, nil

	case *ast.ScalarCompareExpr:
		return compileScalarCompare(node, opts, state)

	case *ast.ArrayCompareExpr:
		return compileArrayCompare(node, opts, state)

	case *ast.UnaryExpr:
		operand, pos, err := compileNode(node.Operand, opts, state)
		if err != nil {
			return nil, pos, err
		}

		switch node.Op {
		case ast.UnaryNot:
			operandBool, ok := operand.(*BoolEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Bool)
			}

			return Not(operandBool, state), node.Pos, nil
		case ast.UnaryNeg:
			operandInt, ok := operand.(*IntEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Int)
			}

			return Minus(operandInt, state), pos, nil
		case ast.UnaryBitNot:
			operandInt, ok := operand.(*IntEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Int)
			}

			return IntNot(operandInt, state), pos, nil
		}
		return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())

	case *ast.Ident:
		return identToEvaluator(node.Name, node.Pos, opts, state)

	case *ast.IntLiteral:
		return &IntEvaluator{
			Value:  node.Value,
			Offset: node.Pos.Offset,
		}, node.Pos, nil

	case *ast.DurationLiteral:
		return &IntEvaluator{
			Value:      node.Value,
			Offset:     node.Pos.Offset,
			isDuration: true,
		}, node.Pos, nil

	case *ast.StringLiteral:
		str := node.Value

		// contains variables or field references
		hasVariables := len(variableRegex.FindAllIndex([]byte(str), -1)) > 0
		hasFieldReferences := len(fieldReferenceRegex.FindAllIndex([]byte(str), -1)) > 0
		if hasVariables || hasFieldReferences {
			return stringEvaluatorFromVariable(str, node.Pos, opts, state)
		}

		return &StringEvaluator{
			Value:     str,
			ValueType: ScalarValueType,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil

	case *ast.PatternLiteral:
		return &StringEvaluator{
			Value:     node.Value,
			ValueType: PatternValueType,
			Weight:    PatternWeight,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil

	case *ast.RegexpLiteral:
		return &StringEvaluator{
			Value:     node.Value,
			ValueType: RegexpValueType,
			Weight:    RegexpWeight,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil

	case *ast.IPLiteral:
		ipnet, err := ParseCIDR(node.Value)
		if err != nil {
			return nil, node.Pos, NewError(node.Pos, "invalid IP '%s'", node.Value)
		}

		return &CIDREvaluator{
			Value:     *ipnet,
			ValueType: IPNetValueType,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil

	case *ast.CIDRLiteral:
		ipnet, err := ParseCIDR(node.Value)
		if err != nil {
			return nil, node.Pos, NewError(node.Pos, "invalid CIDR '%s'", node.Value)
		}

		return &CIDREvaluator{
			Value:     *ipnet,
			ValueType: IPNetValueType,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil

	case *ast.VariableRef:
		return evaluatorFromVariable(node.Name, node.Pos, opts)

	case *ast.FieldRef:
		return evaluatorFromFieldReference(node.Name, node.Pos, state)

	case *ast.IntArray:
		var evaluator IntArrayEvaluator
		evaluator.AppendValues(node.Values...)
		return &evaluator, node.Pos, nil

	case *ast.StringArray:
		var evaluator StringValuesEvaluator
		evaluator.AppendMembers(node.Members...)
		return &evaluator, node.Pos, nil

	case *ast.CIDRArray:
		var values CIDRValues
		for _, member := range node.Members {
			switch member := member.(type) {
			case *ast.CIDRLiteral:
				if err := values.AppendCIDR(member.Value); err != nil {
					return nil, node.Pos, NewError(node.Pos, "invalid CIDR '%s'", member.Value)
				}
			case *ast.IPLiteral:
				if err := values.AppendIP(member.Value); err != nil {
					return nil, node.Pos, NewError(node.Pos, "invalid IP '%s'", member.Value)
				}
			}
		}

		return &CIDRValuesEvaluator{
			Value:     values,
			ValueType: IPNetValueType,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil

	case *ast.IdentArray:
		// Only "Constants" idents are supported, and only string, int and boolean constants are expected.
		for _, name := range node.Names {
			if _, ok := opts.Constants[name]; !ok {
				return nil, node.Pos, fmt.Errorf("`%s` is not a constant, the members of an array of identifiers have to be constants", name)
			}
		}

		// Determine the type with the first ident
		switch reflect.TypeOf(opts.Constants[node.Names[0]]) {
		case reflect.TypeOf(&IntEvaluator{}):
			var evaluator IntArrayEvaluator
			for _, name := range node.Names {
				constant, ok := opts.Constants[name].(*IntEvaluator)
				if !ok {
					return nil, node.Pos, fmt.Errorf("can't mix constants types in arrays: `%s` is not of type int", name)
				}
				evaluator.AppendValues(constant.Value)
			}
			return &evaluator, node.Pos, nil
		case reflect.TypeOf(&StringEvaluator{}):
			var evaluator StringValuesEvaluator
			for _, name := range node.Names {
				constant, ok := opts.Constants[name].(*StringEvaluator)
				if !ok {
					return nil, node.Pos, fmt.Errorf("can't mix constants types in arrays: `%s` is not of type string", name)
				}
				evaluator.AppendMembers(&ast.StringLiteral{Value: constant.Value})
			}
			return &evaluator, node.Pos, nil
		case reflect.TypeOf(&BoolEvaluator{}):
			var evaluator BoolArrayEvaluator
			for _, name := range node.Names {
				constant, ok := opts.Constants[name].(*BoolEvaluator)
				if !ok {
					return nil, node.Pos, fmt.Errorf("can't mix constants types in arrays: `%s` is not of type bool", name)
				}
				evaluator.AppendValues(constant.Value)
			}
			return &evaluator, node.Pos, nil
		}
		return nil, node.Pos, fmt.Errorf("constant `%s` can't be a member of an array, only int, string and boolean constants can", node.Names[0])
	}

	return nil, ast.Position{}, NewError(ast.Position{}, "unknown entity '%s'", reflect.TypeOf(node))
}

// A chain of a same associative operation, `a && b && c`, is grouped from the
// left by the parser, `(a && b) && c`, and compiled grouped from the right,
// `a && (b && c)`:
//
//   - And and Or evaluate the lighter of their operands first, the weight of an
//     operation being the sum of the weights of its operands. Grouped from the
//     left, the last operand of a chain would be the lightest and evaluated
//     first, ahead of the ones the rule writer put in front, changing which
//     operand short-circuits the chain and the matching values it reports.
//   - A bit operation folds its operands when both are constant. Grouped from
//     the right, the constants at the end of a chain are folded together,
//     `open.flags | 1 | 2` is `open.flags | 3`.
//
// The operands are compiled in source order, and errors are reported at the
// positions the operations grouped from the right report them.

// logicalChain returns the operands of the chain of logical operations of node,
// in source order.
func logicalChain(node *ast.LogicalExpr) []ast.Node {
	left, ok := node.Left.(*ast.LogicalExpr)
	if !ok || left.Op != node.Op {
		return []ast.Node{node.Left, node.Right}
	}
	return append(logicalChain(left), node.Right)
}

// bitChain returns the operands of the chain of bit operations of node, in
// source order.
func bitChain(node *ast.BitExpr) []ast.Node {
	left, ok := node.Left.(*ast.BitExpr)
	if !ok || left.Op != node.Op {
		return []ast.Node{node.Left, node.Right}
	}
	return append(bitChain(left), node.Right)
}

// leftOperandPos returns the position of the operation grouped from the right
// whose left operand is the operand i of a chain: the one of the chain for the
// first operand, the one of the operand otherwise.
func leftOperandPos(node ast.Node, operands []ast.Node, i int) ast.Position {
	if i == 0 {
		return node.Position()
	}
	return operands[i].Position()
}

func compileLogicalChain(node *ast.LogicalExpr, opts *Opts, state *State) (interface{}, ast.Position, error) {
	operands := logicalChain(node)
	last := len(operands) - 1

	evaluators := make([]*BoolEvaluator, len(operands))
	for i, operand := range operands {
		evaluator, pos, err := compileExpr(operand, opts, state)
		if err != nil {
			return nil, pos, err
		}

		boolEvaluator, ok := evaluator.(*BoolEvaluator)
		if !ok {
			if i != last {
				pos = leftOperandPos(node, operands, i)
			}
			return nil, pos, NewTypeError(pos, reflect.Bool)
		}
		evaluators[i] = boolEvaluator
	}

	result := evaluators[last]
	for i := last - 1; i >= 0; i-- {
		var err error
		switch node.Op {
		case ast.LogicalOr:
			result, err = Or(evaluators[i], result, state)
		case ast.LogicalAnd:
			result, err = And(evaluators[i], result, state)
		default:
			return nil, node.Pos, NewOpUnknownError(node.Pos, node.Op.String())
		}
		if err != nil {
			return nil, leftOperandPos(node, operands, i), err
		}
	}

	return result, node.Pos, nil
}

func compileBitChain(node *ast.BitExpr, opts *Opts, state *State) (interface{}, ast.Position, error) {
	operands := bitChain(node)
	last := len(operands) - 1

	evaluators := make([]*IntEvaluator, len(operands))
	// the position of the last operand, as its compilation reports it
	var lastPos ast.Position
	for i, operand := range operands {
		evaluator, pos, err := compileNode(operand, opts, state)
		if err != nil {
			return nil, pos, err
		}

		intEvaluator, ok := evaluator.(*IntEvaluator)
		if !ok {
			if i != last {
				pos = leftOperandPos(node, operands, i)
			}
			return nil, pos, NewTypeError(pos, reflect.Int)
		}
		evaluators[i] = intEvaluator
		lastPos = pos
	}

	result := evaluators[last]
	for i := last - 1; i >= 0; i-- {
		var err error
		switch node.Op {
		case ast.BitAnd:
			result, err = IntAnd(evaluators[i], result, state)
		case ast.BitOr:
			result, err = IntOr(evaluators[i], result, state)
		case ast.BitXor:
			result, err = IntXor(evaluators[i], result, state)
		default:
			return nil, node.Pos, NewOpUnknownError(node.Pos, node.Op.String())
		}
		if err != nil {
			// reported at the position of the right operand
			pos := lastPos
			if i+1 != last {
				pos = operands[i+1].Position()
			}
			return nil, pos, err
		}
	}

	return result, node.Pos, nil
}

// compileArrayOperand compiles the right hand side of a membership test.
//
// An identifier is resolved as a macro first there, as an array is most often a
// reference to a macro.
func compileArrayOperand(node ast.Node, opts *Opts, state *State) (interface{}, ast.Position, error) {
	// a lone CIDR is an array of one CIDR
	if cidr, ok := node.(*ast.CIDRLiteral); ok {
		node = &ast.CIDRArray{Meta: cidr.Meta, Members: []ast.CIDRArrayMember{cidr}}
	}

	if ident, ok := node.(*ast.Ident); ok {
		if state.macros != nil {
			if macro, ok := state.macros.GetMacroEvaluator(ident.Name); ok {
				return macro.Value, ident.Pos, nil
			}
		}

		// could be an iterator
		return identToEvaluator(ident.Name, ident.Pos, opts, state)
	}

	return compileNode(node, opts, state)
}

func compileArrayCompare(node *ast.ArrayCompareExpr, opts *Opts, state *State) (interface{}, ast.Position, error) {
	var boolEvaluator *BoolEvaluator

	left, pos, err := compileNode(node.Left, opts, state)
	if err != nil {
		return nil, pos, err
	}

	right, pos, err := compileArrayOperand(node.Right, opts, state)
	if err != nil {
		return nil, pos, err
	}

	switch left := left.(type) {
	case *BoolEvaluator:
		switch rightBool := right.(type) {
		case *BoolArrayEvaluator:
			boolEvaluator, err = ArrayBoolContains(left, rightBool, state)
			if err != nil {
				return nil, pos, err
			}
			if node.Op == ast.ArrayNotIn {
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return boolEvaluator, node.Pos, nil
		default:
			return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.Bool)
		}
	case *StringEvaluator:
		switch rightString := right.(type) {
		case *StringArrayEvaluator:
			boolEvaluator, err = StringArrayContainsWrapper(left, rightString, state)
			if err != nil {
				return nil, pos, err
			}
		case *StringValuesEvaluator:
			boolEvaluator, err = stringValuesContainsWrapper(left, rightString, state)
			if err != nil {
				return nil, pos, err
			}
		default:
			return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.String)
		}
		if node.Op == ast.ArrayNotIn {
			return Not(boolEvaluator, state), node.Pos, nil
		}
		return boolEvaluator, node.Pos, nil
	case *StringValuesEvaluator:
		switch rightStringArray := right.(type) {
		case *StringArrayEvaluator:
			boolEvaluator, err = stringArrayMatchesWrapper(rightStringArray, left, state)
			if err != nil {
				return nil, pos, err
			}
			if node.Op == ast.ArrayNotIn {
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return boolEvaluator, node.Pos, nil
		default:
			return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.String)
		}
	case *StringArrayEvaluator:
		switch rightStringArray := right.(type) {
		case *StringValuesEvaluator:
			boolEvaluator, err = stringArrayMatchesWrapper(left, rightStringArray, state)
			if err != nil {
				return nil, pos, err
			}
		case *StringArrayEvaluator:
			boolEvaluator, err = StringArrayMatchesStringArray(left, rightStringArray, state)
			if err != nil {
				return nil, pos, err
			}
		default:
			return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.String)
		}
		if node.Op == ast.ArrayNotIn {
			return Not(boolEvaluator, state), node.Pos, nil
		}
		return boolEvaluator, node.Pos, nil
	case *IntEvaluator:
		switch rightInt := right.(type) {
		case *IntArrayEvaluator:
			boolEvaluator, err = IntArrayEquals(left, rightInt, state)
			if err != nil {
				return nil, pos, err
			}
			if node.Op == ast.ArrayNotIn {
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return boolEvaluator, node.Pos, nil
		default:
			return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.Int)
		}
	case *IntArrayEvaluator:
		switch rightIntArray := right.(type) {
		case *IntArrayEvaluator:
			boolEvaluator, err = IntArrayMatches(left, rightIntArray, state)
			if err != nil {
				return nil, pos, err
			}
			if node.Op == ast.ArrayNotIn {
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return boolEvaluator, node.Pos, nil
		default:
			return nil, pos, NewArrayTypeError(pos, reflect.Array, reflect.Int)
		}
	case *CIDREvaluator:
		switch rightCIDR := right.(type) {
		case *CIDREvaluator:
			boolEvaluator, err = CIDREquals(left, rightCIDR, state)
			if err != nil {
				return nil, node.Pos, err
			}
			switch node.Op {
			case ast.ArrayIn, ast.ArrayAllIn:
				return boolEvaluator, node.Pos, nil
			case ast.ArrayNotIn:
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
		case *CIDRValuesEvaluator:
			switch node.Op {
			case ast.ArrayIn, ast.ArrayAllIn:
				boolEvaluator, err = CIDRValuesContains(left, rightCIDR, state)
				if err != nil {
					return nil, pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ArrayNotIn:
				boolEvaluator, err = CIDRValuesContains(left, rightCIDR, state)
				if err != nil {
					return nil, pos, err
				}
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
		case *CIDRArrayEvaluator:
			switch node.Op {
			case ast.ArrayIn, ast.ArrayAllIn:
				boolEvaluator, err = CIDRArrayContains(left, rightCIDR, state)
				if err != nil {
					return nil, pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ArrayNotIn:
				boolEvaluator, err = CIDRArrayContains(left, rightCIDR, state)
				if err != nil {
					return nil, pos, err
				}
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
		default:
			return nil, pos, NewCIDRTypeError(pos, reflect.Array, right)
		}
	case *CIDRArrayEvaluator:
		switch rightCIDR := right.(type) {
		case *CIDRValuesEvaluator:
			switch node.Op {
			case ast.ArrayIn:
				boolEvaluator, err = CIDRArrayMatches(left, rightCIDR, state)
				if err != nil {
					return nil, pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ArrayAllIn:
				boolEvaluator, err = CIDRArrayMatchesAll(left, rightCIDR, state)
				if err != nil {
					return nil, pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ArrayNotIn:
				boolEvaluator, err = CIDRArrayMatches(left, rightCIDR, state)
				if err != nil {
					return nil, pos, err
				}
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
		default:
			return nil, pos, NewCIDRTypeError(pos, reflect.Array, right)
		}
	case *CIDRValuesEvaluator:
		switch rightCIDR := right.(type) {
		case *CIDREvaluator:
			switch node.Op {
			case ast.ArrayIn, ast.ArrayAllIn:
				boolEvaluator, err = CIDRValuesContains(rightCIDR, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ArrayNotIn:
				boolEvaluator, err = CIDRValuesContains(rightCIDR, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
		case *CIDRArrayEvaluator:
			switch node.Op {
			case ast.ArrayAllIn:
				boolEvaluator, err = CIDRArrayMatchesAll(rightCIDR, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ArrayIn:
				boolEvaluator, err = CIDRArrayMatches(rightCIDR, left, state)
				if err != nil {
					return nil, pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ArrayNotIn:
				boolEvaluator, err = CIDRArrayMatches(rightCIDR, left, state)
				if err != nil {
					return nil, pos, err
				}
				return Not(boolEvaluator, state), node.Pos, nil
			}
			return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
		default:
			return nil, pos, NewCIDRTypeError(pos, reflect.Array, right)
		}
	default:
		return nil, pos, NewTypeError(pos, reflect.Array)
	}
}

func compileScalarCompare(node *ast.ScalarCompareExpr, opts *Opts, state *State) (interface{}, ast.Position, error) {
	var boolEvaluator *BoolEvaluator

	left, pos, err := compileNode(node.Left, opts, state)
	if err != nil {
		return nil, pos, err
	}

	right, pos, err := compileNode(node.Right, opts, state)
	if err != nil {
		return nil, pos, err
	}

	switch left := left.(type) {
	case *BoolEvaluator:
		rightBool, ok := right.(*BoolEvaluator)
		if !ok {
			return nil, pos, NewTypeError(pos, reflect.Bool)
		}

		switch node.Op {
		case ast.ScalarNotEqual:
			boolEvaluator, err = BoolEquals(left, rightBool, state)
			if err != nil {
				return nil, pos, err
			}
			return Not(boolEvaluator, state), node.Pos, nil
		case ast.ScalarEqual:
			boolEvaluator, err = BoolEquals(left, rightBool, state)
			if err != nil {
				return nil, pos, err
			}
			return boolEvaluator, node.Pos, nil
		}
		return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
	case *BoolArrayEvaluator:
		rightBool, ok := right.(*BoolEvaluator)
		if !ok {
			return nil, pos, NewTypeError(pos, reflect.Bool)
		}

		switch node.Op {
		case ast.ScalarNotEqual:
			boolEvaluator, err = BoolArrayEquals(rightBool, left, state)
			if err != nil {
				return nil, pos, err
			}
			return Not(boolEvaluator, state), node.Pos, nil
		case ast.ScalarEqual:
			boolEvaluator, err = BoolArrayEquals(rightBool, left, state)
			if err != nil {
				return nil, pos, err
			}
			return boolEvaluator, node.Pos, nil
		}
		return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
	case *StringEvaluator:
		rightString, ok := right.(*StringEvaluator)
		if !ok {
			return nil, pos, NewTypeError(pos, reflect.String)
		}

		switch node.Op {
		case ast.ScalarNotEqual:
			boolEvaluator, err = StringEqualsWrapper(left, rightString, state)
			if err != nil {
				return nil, node.Pos, err
			}
			return Not(boolEvaluator, state), node.Pos, nil
		case ast.ScalarNotMatch:
			if rightString.EvalFnc != nil {
				return nil, node.Pos, &ErrNonStaticPattern{Field: rightString.Field}
			}

			// force pattern if needed
			if rightString.ValueType == ScalarValueType {
				rightString.ValueType = PatternValueType
			}

			boolEvaluator, err = StringEqualsWrapper(left, rightString, state)
			if err != nil {
				return nil, node.Pos, err
			}
			return Not(boolEvaluator, state), node.Pos, nil
		case ast.ScalarEqual:
			boolEvaluator, err = StringEqualsWrapper(left, rightString, state)
			if err != nil {
				return nil, node.Pos, err
			}
			return boolEvaluator, node.Pos, nil
		case ast.ScalarMatch:
			if rightString.EvalFnc != nil {
				return nil, node.Pos, &ErrNonStaticPattern{Field: rightString.Field}
			}

			// force pattern if needed
			if rightString.ValueType == ScalarValueType {
				rightString.ValueType = PatternValueType
			}

			boolEvaluator, err = StringEqualsWrapper(left, rightString, state)
			if err != nil {
				return nil, node.Pos, err
			}
			return boolEvaluator, node.Pos, nil
		}
		return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
	case *CIDREvaluator:
		switch rightIP := right.(type) {
		case *CIDREvaluator:
			switch node.Op {
			case ast.ScalarNotEqual:
				boolEvaluator, err = CIDREquals(left, rightIP, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return Not(boolEvaluator, state), node.Pos, nil
			case ast.ScalarEqual:
				boolEvaluator, err = CIDREquals(left, rightIP, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			}
			return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
		}

	case *CIDRArrayEvaluator:
		cidrEvaluator, ok := right.(*CIDREvaluator)
		if !ok {
			return nil, pos, NewTypeError(pos, reflect.String)
		}

		switch node.Op {
		case ast.ScalarNotEqual:
			boolEvaluator, err = CIDRArrayMatchesCIDREvaluator(left, cidrEvaluator, state)
			if err != nil {
				return nil, node.Pos, err
			}
			return Not(boolEvaluator, state), node.Pos, nil
		case ast.ScalarEqual:
			boolEvaluator, err = CIDRArrayMatchesCIDREvaluator(left, cidrEvaluator, state)
			if err != nil {
				return nil, pos, err
			}
			return boolEvaluator, node.Pos, nil
		}

		return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())

	case *StringArrayEvaluator:
		rightString, ok := right.(*StringEvaluator)
		if !ok {
			return nil, pos, NewTypeError(pos, reflect.String)
		}

		switch node.Op {
		case ast.ScalarNotEqual:
			boolEvaluator, err = StringArrayContainsWrapper(rightString, left, state)
			if err != nil {
				return nil, node.Pos, err
			}
			return Not(boolEvaluator, state), node.Pos, nil
		case ast.ScalarEqual:
			boolEvaluator, err = StringArrayContainsWrapper(rightString, left, state)
			if err != nil {
				return nil, node.Pos, err
			}
			return boolEvaluator, node.Pos, nil
		case ast.ScalarNotMatch:
			if rightString.EvalFnc != nil {
				return nil, node.Pos, &ErrNonStaticPattern{Field: rightString.Field}
			}

			// force pattern if needed
			if rightString.ValueType == ScalarValueType {
				rightString.ValueType = PatternValueType
			}

			boolEvaluator, err = StringArrayContainsWrapper(rightString, left, state)
			if err != nil {
				return nil, node.Pos, err
			}
			return Not(boolEvaluator, state), node.Pos, nil
		case ast.ScalarMatch:
			if rightString.EvalFnc != nil {
				return nil, node.Pos, &ErrNonStaticPattern{Field: rightString.Field}
			}

			// force pattern if needed
			if rightString.ValueType == ScalarValueType {
				rightString.ValueType = PatternValueType
			}

			boolEvaluator, err = StringArrayContainsWrapper(rightString, left, state)
			if err != nil {
				return nil, node.Pos, err
			}
			return boolEvaluator, node.Pos, nil
		}
	case *IntEvaluator:
		switch rightInt := right.(type) {
		case *IntEvaluator:
			if rightInt.isDuration {
				if left.isFromArithmeticOperation {
					switch node.Op {
					case ast.ScalarLesser:
						boolEvaluator, err = DurationLesserThanArithmeticOperation(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					case ast.ScalarLesserOrEqual:
						boolEvaluator, err = DurationLesserOrEqualThanArithmeticOperation(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					case ast.ScalarGreater:
						boolEvaluator, err = DurationGreaterThanArithmeticOperation(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					case ast.ScalarGreaterOrEqual:
						boolEvaluator, err = DurationGreaterOrEqualThanArithmeticOperation(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					case ast.ScalarEqual:
						boolEvaluator, err = DurationEqualArithmeticOperation(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					}
				} else {
					switch node.Op {
					case ast.ScalarLesser:
						boolEvaluator, err = DurationLesserThan(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					case ast.ScalarLesserOrEqual:
						boolEvaluator, err = DurationLesserOrEqualThan(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					case ast.ScalarGreater:
						boolEvaluator, err = DurationGreaterThan(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					case ast.ScalarGreaterOrEqual:
						boolEvaluator, err = DurationGreaterOrEqualThan(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					case ast.ScalarEqual:
						boolEvaluator, err = DurationEqual(left, rightInt, state)
						if err != nil {
							return nil, node.Pos, err
						}
						return boolEvaluator, node.Pos, nil
					}
				}
			} else {
				switch node.Op {
				case ast.ScalarLesser:
					boolEvaluator, err = LesserThan(left, rightInt, state)
					if err != nil {
						return nil, node.Pos, err
					}
					return boolEvaluator, node.Pos, nil
				case ast.ScalarLesserOrEqual:
					boolEvaluator, err = LesserOrEqualThan(left, rightInt, state)
					if err != nil {
						return nil, node.Pos, err
					}
					return boolEvaluator, node.Pos, nil
				case ast.ScalarGreater:
					boolEvaluator, err = GreaterThan(left, rightInt, state)
					if err != nil {
						return nil, node.Pos, err
					}
					return boolEvaluator, node.Pos, nil
				case ast.ScalarGreaterOrEqual:
					boolEvaluator, err = GreaterOrEqualThan(left, rightInt, state)
					if err != nil {
						return nil, node.Pos, err
					}
					return boolEvaluator, node.Pos, nil
				case ast.ScalarNotEqual:
					boolEvaluator, err = IntEquals(left, rightInt, state)
					if err != nil {
						return nil, node.Pos, err
					}

					return Not(boolEvaluator, state), node.Pos, nil
				case ast.ScalarEqual:
					boolEvaluator, err = IntEquals(left, rightInt, state)
					if err != nil {
						return nil, node.Pos, err
					}
					return boolEvaluator, node.Pos, nil
				default:
					return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
				}
			}
		case *IntArrayEvaluator:
			switch node.Op {
			case ast.ScalarLesser:
				boolEvaluator, err = IntArrayLesserThan(left, rightInt, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarLesserOrEqual:
				boolEvaluator, err = IntArrayLesserOrEqualThan(left, rightInt, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarGreater:
				boolEvaluator, err = IntArrayGreaterThan(left, rightInt, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarGreaterOrEqual:
				boolEvaluator, err = IntArrayGreaterOrEqualThan(left, rightInt, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarNotEqual:
				boolEvaluator, err = IntArrayEquals(left, rightInt, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return Not(boolEvaluator, state), node.Pos, nil
			case ast.ScalarEqual:
				boolEvaluator, err = IntArrayEquals(left, rightInt, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			default:
				return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
			}
		}
		return nil, pos, NewTypeError(pos, reflect.Int)
	case *IntArrayEvaluator:
		rightInt, ok := right.(*IntEvaluator)
		if !ok {
			return nil, pos, NewTypeError(pos, reflect.Int)
		}

		if rightInt.isDuration {
			switch node.Op {
			case ast.ScalarLesser:
				boolEvaluator, err = DurationArrayLesserThan(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarLesserOrEqual:
				boolEvaluator, err = DurationArrayLesserOrEqualThan(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarGreater:
				boolEvaluator, err = DurationArrayGreaterThan(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarGreaterOrEqual:
				boolEvaluator, err = DurationArrayGreaterOrEqualThan(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			}
		} else {
			switch node.Op {
			case ast.ScalarLesser:
				boolEvaluator, err = IntArrayGreaterThan(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarLesserOrEqual:
				boolEvaluator, err = IntArrayGreaterOrEqualThan(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarGreater:
				boolEvaluator, err = IntArrayLesserThan(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarGreaterOrEqual:
				boolEvaluator, err = IntArrayLesserOrEqualThan(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			case ast.ScalarNotEqual:
				boolEvaluator, err = IntArrayEquals(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return Not(boolEvaluator, state), node.Pos, nil
			case ast.ScalarEqual:
				boolEvaluator, err = IntArrayEquals(rightInt, left, state)
				if err != nil {
					return nil, node.Pos, err
				}
				return boolEvaluator, node.Pos, nil
			}
			return nil, pos, NewOpUnknownError(node.Pos, node.Op.String())
		}
	}

	return nil, ast.Position{}, NewError(ast.Position{}, "unknown entity '%s'", reflect.TypeOf(node))
}
