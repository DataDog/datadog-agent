// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package eval holds eval related files
package eval

import (
	"reflect"
	"strconv"
	"strings"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/ast"
	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/pass"
)

// Compile runs the compilation passes over a node, and builds its evaluator, of a
// concrete type depending on the node.
func Compile(node ast.Node, opts *Opts, state *State) (interface{}, ast.Position, error) {
	pos := node.Position()
	node, err := pass.Run(node, opts.passes()...)
	if err != nil {
		return nil, pos, err
	}

	switch node := node.(type) {
	case *ast.Rule:
		return compileExpr(node.Root, opts, state)
	case *ast.Macro:
		return compileExpr(node.Root, opts, state)
	}

	return compileExpr(node, opts, state)
}

// compileExpr compiles the root of a rule or a macro, or an operand of `&&`/`||`.
// A lone boolean value, `open.flag`, is compared against `true`.
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
		unary, err := Unary(boolEvaluator, state) // compares against `true`
		if err != nil {
			return nil, node.Position(), err
		}
		return unary, node.Position(), nil
	}

	return evaluator, pos, nil
}

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

	case *ast.Ident, *ast.Selector:
		return nil, node.Position(), NewError(node.Position(), "unresolved name `%s`", ast.Sprint(node))

	case *ast.Index:
		return indexToEvaluator(node, opts, state)

	case *ast.Literal:
		return literalToEvaluator(node)

	case *ast.Interpolation:
		return interpolationToEvaluator(node, opts, state)

	case *ast.VariableRef:
		return evaluatorFromVariable(node.Name, node.Pos, opts, state)

	case *ast.FieldRef:
		return fieldToEvaluator(node, state)

	case *ast.Array:
		return arrayToEvaluator(node)
	}

	return nil, ast.Position{}, NewError(ast.Position{}, "unknown entity '%s'", reflect.TypeOf(node))
}

// indexToEvaluator compiles an index in an array, `process.argv[0]`: the array
// and the index are compiled, the index being a constant integer.
func indexToEvaluator(node *ast.Index, opts *Opts, state *State) (interface{}, ast.Position, error) {
	array, pos, err := compileNode(node.Base, opts, state)
	if err != nil {
		return nil, pos, err
	}

	index, pos, err := compileNode(node.Subscript, opts, state)
	if err != nil {
		return nil, pos, err
	}
	indexInt, ok := index.(*IntEvaluator)
	if !ok || indexInt.EvalFnc != nil {
		return nil, pos, NewError(pos, "an index is a constant integer")
	}

	arrayEvaluator, ok := array.(Evaluator)
	if !ok {
		return nil, node.Pos, NewError(node.Pos, "`%s` isn't an array", ast.Sprint(node.Base))
	}
	evaluator, err := WrapEvaluatorWithArrayIndex(arrayEvaluator, indexInt.Value, arrayEvaluator.GetField())
	if err != nil {
		return nil, node.Pos, NewError(node.Pos, "failed to create array index accessor: %v", err)
	}
	return evaluator, node.Pos, nil
}

// interpolationToEvaluator compiles a string with interpolations: its parts
// are compiled, each converted to a string, and concatenated at evaluation.
func interpolationToEvaluator(node *ast.Interpolation, opts *Opts, state *State) (interface{}, ast.Position, error) {
	parts, weight := make([]*StringEvaluator, 0, len(node.Parts)), 0
	for _, part := range node.Parts {
		evaluator, pos, err := compileNode(part, opts, state)
		if err != nil {
			return nil, pos, err
		}
		str, ok := toStringEvaluator(evaluator)
		if !ok {
			if variable, ok := part.(*ast.VariableRef); ok {
				return nil, pos, NewError(pos, "variable type not supported '%s'", variable.Name)
			}
			return nil, pos, NewError(pos, "field type not supported '%s'", ast.Sprint(part))
		}
		if field, ok := evaluator.(Evaluator); ok && field.GetField() != "" {
			state.fieldValues[field.GetField()] = append(state.fieldValues[field.GetField()], FieldValue{Type: VariableValueType})
		}
		parts, weight = append(parts, str), weight+str.Weight
	}

	return &StringEvaluator{
		ValueType: VariableValueType,
		Weight:    weight,
		EvalFnc: func(ctx *Context) string {
			var builder strings.Builder
			for _, part := range parts {
				if part.EvalFnc != nil {
					builder.WriteString(part.EvalFnc(ctx))
				} else {
					builder.WriteString(part.Value)
				}
			}
			return builder.String()
		},
	}, node.Pos, nil
}

// toStringEvaluator converts an evaluator to one of its value as a string, an
// array being joined by commas. It reports false for a type that has none.
func toStringEvaluator(evaluator interface{}) (*StringEvaluator, bool) {
	switch evaluator := evaluator.(type) {
	case *StringEvaluator:
		return evaluator, true
	case *IntEvaluator:
		if evaluator.EvalFnc == nil {
			return &StringEvaluator{Value: strconv.Itoa(evaluator.Value)}, true
		}
		return &StringEvaluator{Weight: evaluator.Weight, EvalFnc: func(ctx *Context) string {
			return strconv.Itoa(evaluator.EvalFnc(ctx))
		}}, true
	case *StringArrayEvaluator:
		return &StringEvaluator{Weight: evaluator.Weight, EvalFnc: func(ctx *Context) string {
			return strings.Join(evaluator.EvalFnc(ctx), ",")
		}}, true
	case *IntArrayEvaluator:
		return &StringEvaluator{Weight: evaluator.Weight, EvalFnc: func(ctx *Context) string {
			var builder strings.Builder
			for i, number := range evaluator.EvalFnc(ctx) {
				if i != 0 {
					builder.WriteString(",")
				}
				builder.WriteString(strconv.Itoa(number))
			}
			return builder.String()
		}}, true
	}
	return nil, false
}

// literalToEvaluator compiles a literal.
func literalToEvaluator(node *ast.Literal) (interface{}, ast.Position, error) {
	switch node.Value.Kind() {
	case ast.Bool:
		return &BoolEvaluator{
			Value: ast.BoolVal(node.Value),
		}, node.Pos, nil

	case ast.Int:
		return &IntEvaluator{
			Value:  ast.IntVal(node.Value),
			Offset: node.Pos.Offset,
		}, node.Pos, nil

	case ast.Duration:
		return &IntEvaluator{
			Value:      ast.IntVal(node.Value),
			Offset:     node.Pos.Offset,
			isDuration: true,
		}, node.Pos, nil

	case ast.String:
		return &StringEvaluator{
			Value:     ast.StringVal(node.Value),
			ValueType: ScalarValueType,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil

	case ast.Pattern:
		return &StringEvaluator{
			Value:     ast.StringVal(node.Value),
			ValueType: PatternValueType,
			Weight:    PatternWeight,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil

	case ast.Regexp:
		return &StringEvaluator{
			Value:     ast.StringVal(node.Value),
			ValueType: RegexpValueType,
			Weight:    RegexpWeight,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil

	case ast.IP, ast.CIDR:
		ipnet, err := ParseCIDR(ast.StringVal(node.Value))
		if err != nil {
			return nil, node.Pos, NewError(node.Pos, "invalid %s '%s'", node.Value.Kind(), ast.StringVal(node.Value))
		}

		return &CIDREvaluator{
			Value:     *ipnet,
			ValueType: IPNetValueType,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil
	}

	return nil, node.Pos, NewError(node.Pos, "unknown kind of value '%s'", node.Value.Kind())
}

// familyKinds holds the type reported for the members of an array of each family.
var familyKinds = map[ast.Family]reflect.Kind{
	ast.BoolFamily:   reflect.Bool,
	ast.IntFamily:    reflect.Int,
	ast.StringFamily: reflect.String,
	ast.IPFamily:     reflect.Struct,
}

// arrayToEvaluator compiles an array literal, its family being the one of its
// first member.
func arrayToEvaluator(node *ast.Array) (interface{}, ast.Position, error) {
	// an empty array, the values of a macro defined by an empty list, is an
	// array of strings
	if len(node.Members) == 0 {
		return &StringValuesEvaluator{}, node.Pos, nil
	}

	family := node.Members[0].Value.Kind().Family()
	for _, member := range node.Members[1:] {
		if member.Value.Kind().Family() != family {
			return nil, member.Pos, NewArrayTypeError(member.Pos, reflect.Array, familyKinds[family])
		}
	}

	switch family {
	case ast.BoolFamily:
		var evaluator BoolArrayEvaluator
		for _, member := range node.Members {
			evaluator.AppendValues(ast.BoolVal(member.Value))
		}
		return &evaluator, node.Pos, nil

	case ast.IntFamily:
		var evaluator IntArrayEvaluator
		for _, member := range node.Members {
			evaluator.AppendValues(ast.IntVal(member.Value))
		}
		return &evaluator, node.Pos, nil

	case ast.StringFamily:
		var evaluator StringValuesEvaluator
		evaluator.AppendMembers(node.Members...)
		return &evaluator, node.Pos, nil

	case ast.IPFamily:
		var values CIDRValues
		for _, member := range node.Members {
			var err error
			if member.Value.Kind() == ast.CIDR {
				err = values.AppendCIDR(ast.StringVal(member.Value))
			} else {
				err = values.AppendIP(ast.StringVal(member.Value))
			}
			if err != nil {
				return nil, node.Pos, NewError(node.Pos, "invalid %s '%s'", member.Value.Kind(), ast.StringVal(member.Value))
			}
		}

		return &CIDRValuesEvaluator{
			Value:     values,
			ValueType: IPNetValueType,
			Offset:    node.Pos.Offset,
		}, node.Pos, nil
	}

	return nil, node.Pos, NewError(node.Pos, "unknown kind of array '%s'", node.Members[0].Value.Kind())
}

// A chain of one associative operator, `a && b && c`, is parsed grouped from the
// left, `(a && b) && c`, and compiled grouped from the right, `a && (b && c)`:
//
//   - And and Or evaluate their lighter operand first, an operation weighing the
//     sum of its operands. Grouped from the left, the last operand would be the
//     lightest and run first, changing which operand short-circuits and the
//     matching values reported.
//   - Bit operations fold two constant operands. Grouped from the right,
//     `open.flags | 1 | 2` folds to `open.flags | 3`.
//
// Operands are compiled in source order. Errors keep the positions the tree
// grouped from the right would report.

// logicalChain returns the operands of the chain of node, in source order.
func logicalChain(node *ast.LogicalExpr) []ast.Node {
	left, ok := node.Left.(*ast.LogicalExpr)
	if !ok || left.Op != node.Op {
		return []ast.Node{node.Left, node.Right}
	}
	return append(logicalChain(left), node.Right)
}

// bitChain returns the operands of the chain of node, in source order.
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
func compileArrayOperand(node ast.Node, opts *Opts, state *State) (interface{}, ast.Position, error) {
	// a lone CIDR is an array of one CIDR
	if cidr, ok := node.(*ast.Literal); ok && cidr.Value.Kind() == ast.CIDR {
		node = &ast.Array{Meta: cidr.Meta, Members: []*ast.Literal{cidr}}
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
	case *BoolArrayEvaluator:
		leftPos := node.Left.Position()
		return nil, leftPos, NewTypeError(leftPos, reflect.Bool)
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
	case *StringValuesEvaluator:
		leftPos := node.Left.Position()
		return nil, leftPos, NewTypeError(leftPos, reflect.String)
	case *CIDRValuesEvaluator:
		leftPos := node.Left.Position()
		return nil, leftPos, NewTypeError(leftPos, familyKinds[ast.IPFamily])
	}

	return nil, ast.Position{}, NewError(ast.Position{}, "unknown entity '%s'", reflect.TypeOf(node))
}
