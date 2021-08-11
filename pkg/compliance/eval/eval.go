// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package eval

import (
	"strconv"

	"github.com/alecthomas/participle/lexer"
	"github.com/alecthomas/repr"
)

// Evaluatable abstracts part of an expression that can be evaluated for an instance
type Evaluatable interface {
	Evaluate(instance Instance) (interface{}, error)
}

// Function describes a function callable for an instance
type Function func(instance Instance, args ...interface{}) (interface{}, error)

// FunctionMap describes a map of functions
type FunctionMap map[string]Function

// VarMap describes a map of variables
type VarMap map[string]interface{}

// Instance for evaluation
type Instance interface {
	Var(name string) (interface{}, bool)
	Vars() VarMap
	Function(name string) (Function, bool)
	Functions() FunctionMap
}

// instance for evaluation
type instance struct {
	// Instance functions
	functions FunctionMap
	// Vars defined during evaluation.
	vars VarMap
}

func (i *instance) Vars() VarMap {
	if i == nil || i.vars == nil {
		return VarMap{}
	}
	return i.vars
}

func (v VarMap) GoMap() map[string]interface{} {
	return map[string]interface{}(v)
}

func (i *instance) Functions() FunctionMap {
	if i == nil || i.functions == nil {
		return FunctionMap{}
	}
	return i.functions
}

func (i *instance) Var(name string) (interface{}, bool) {
	if i == nil || i.vars == nil {
		return nil, false
	}
	value, ok := i.vars[name]
	return value, ok
}

func (i *instance) Function(name string) (Function, bool) {
	if i == nil || i.functions == nil {
		return nil, false
	}
	function, ok := i.functions[name]
	return function, ok
}

// NewInstance instantiates a new evaluation instance
func NewInstance(vars VarMap, functions FunctionMap) Instance {
	return &instance{
		vars:      vars,
		functions: functions,
	}
}

// Iterator abstracts iteration over a set of instances for expression evaluation
type Iterator interface {
	Next() (Instance, error)
	Done() bool
}

// InstanceResult captures an Instance along with the passed or failed status for the result
type InstanceResult struct {
	Instance Instance
	Passed   bool
}

const (
	allFn   = "all"
	noneFn  = "none"
	countFn = "count"
)

var (
	builtInVars = VarMap{
		"_": true,
	}
)

// EvaluateIterator evaluates an iterable expression for an iterator
func (e *IterableExpression) EvaluateIterator(it Iterator, global Instance) ([]*InstanceResult, error) {
	if e.IterableComparison == nil {
		return e.iterate(
			it,
			e.Expression,
			nil,
		)
	}

	if e.IterableComparison.Fn == nil {
		return nil, lexer.Errorf(e.Pos, "expecting function for iterable comparison")
	}

	var (
		totalCount  int64
		passedCount int64
	)

	_, err := e.iterate(
		it,
		e.IterableComparison.Expression, func(instance Instance, passed bool) bool {
			totalCount++
			if passed {
				passedCount++
			}
			return passed
		},
	)
	if err != nil {
		return nil, err
	}

	passed, err := e.evaluatePassed(global, passedCount, totalCount)
	if err != nil {
		return nil, err
	}

	return []*InstanceResult{
		{
			Passed: passed,
		},
	}, nil
}

func (e *IterableExpression) evaluatePassed(instance Instance, passedCount, totalCount int64) (bool, error) {
	fn := *e.IterableComparison.Fn
	switch fn {
	case allFn:
		return passedCount == totalCount, nil

	case noneFn:
		return passedCount == 0, nil

	case countFn:
		comparison := e.IterableComparison.ScalarComparison
		if comparison == nil {
			return false, lexer.Errorf(e.Pos, `expecting rhs of iterable comparison using "%s()"`, fn)
		}

		if comparison.Op == nil {
			return false, lexer.Errorf(e.Pos, `expecting operator for iterable comparison using "%s()"`, fn)
		}

		rhs, err := comparison.Next.Evaluate(instance)
		if err != nil {
			return false, err
		}

		switch expectedCount := rhs.(type) {
		case int64:
			return intCompare(*comparison.Op, passedCount, expectedCount, e.Pos)
		case uint64:
			return intCompare(*comparison.Op, passedCount, int64(expectedCount), e.Pos)
		default:
			return false, lexer.Errorf(e.Pos, `expecting an integer rhs for iterable comparison using "%s()"`, fn)
		}

	default:
		return false, lexer.Errorf(e.Pos, `unexpected function "%s()" for iterable comparison`, *e.IterableComparison.Fn)
	}
}

func (e *IterableExpression) iterate(it Iterator, expression *Expression, checkResult func(instance Instance, passed bool) bool) ([]*InstanceResult, error) {
	var (
		instance Instance
		results  []*InstanceResult
		err      error
		passed   bool
	)

	if it.Done() {
		return []*InstanceResult{
			{
				Passed: false,
			},
		}, nil
	}

	for !it.Done() {
		instance, err = it.Next()
		if err != nil {
			return nil, err
		}

		passed, err = e.evaluateSubExpression(instance, expression)
		if err != nil {
			return nil, err
		}

		// iterable comparison, then returning only matching instance as the
		// real check will be done in the evaluatePassed function
		if checkResult != nil {
			passed = checkResult(instance, passed)

		}
		results = append(results, &InstanceResult{
			Instance: instance,
			Passed:   passed,
		})
	}

	return results, nil
}

func (e *IterableExpression) evaluateSubExpression(instance Instance, expression *Expression) (bool, error) {
	v, err := expression.Evaluate(instance)
	if err != nil {
		return false, err
	}

	passed, ok := v.(bool)
	if !ok {
		return false, lexer.Errorf(e.Pos, "expression in iteration must evaluate to a boolean")
	}
	return passed, nil
}

// Evaluate evaluates an iterable expression for a single instance
func (e *IterableExpression) Evaluate(instance Instance) (bool, error) {
	if e.IterableComparison == nil {
		return e.evaluateSubExpression(instance, e.Expression)
	}

	passed, err := e.evaluateSubExpression(instance, e.IterableComparison.Expression)
	if err != nil {
		return false, err
	}
	var (
		passedCount int64
		totalCount  = int64(1)
	)

	if passed {
		passedCount = 1
	}

	return e.evaluatePassed(instance, passedCount, totalCount)
}

// Evaluate evaluates a path expression for an instance
func (e *PathExpression) Evaluate(instance Instance) (interface{}, error) {
	if e.Path != nil {
		return *e.Path, nil
	}
	return e.Expression.Evaluate(instance)
}

// Evaluate evaluates an expression for an instance
func (e *Expression) Evaluate(instance Instance) (interface{}, error) {
	lhs, err := e.Comparison.Evaluate(instance)
	if err != nil {
		return nil, err
	}

	if e.Next == nil {
		return lhs, nil
	}

	left, ok := lhs.(bool)
	if !ok {
		return nil, lexer.Errorf(e.Pos, "type mismatch, expected bool in lhs of boolean expression")
	}

	rhs, err := e.Next.Evaluate(instance)
	if err != nil {
		return nil, err
	}

	right, ok := rhs.(bool)
	if !ok {
		return nil, lexer.Errorf(e.Pos, "type mismatch, expected bool in rhs of boolean expression")
	}

	switch *e.Op {
	case "&&":
		return left && right, nil
	case "||":
		return left || right, nil
	default:
		return nil, lexer.Errorf(e.Pos, "unsupported operator %q in boolean expression", *e.Op)
	}
}

// BoolEvaluate evaluates an expression for an instance as a boolean value
func (e *Expression) BoolEvaluate(instance Instance) (bool, error) {
	v, err := e.Evaluate(instance)
	if err != nil {
		return false, err
	}
	passed, ok := v.(bool)
	if !ok {
		return false, lexer.Errorf(e.Pos, "expression must evaluate to a boolean")
	}
	return passed, nil
}

// Evaluate implements Evaluatable interface
func (c *Comparison) Evaluate(instance Instance) (interface{}, error) {
	lhs, err := c.Term.Evaluate(instance)
	if err != nil {
		return nil, err
	}
	switch {
	case c.ArrayComparison != nil:
		if c.ArrayComparison.Array == nil {
			return nil, lexer.Errorf(c.Pos, "missing rhs of array operation %q", *c.ArrayComparison.Op)
		}

		rhs, err := c.ArrayComparison.Array.Evaluate(instance)
		if err != nil {
			return nil, err
		}

		array, ok := rhs.([]interface{})
		if !ok {
			return nil, lexer.Errorf(c.Pos, "rhs of %q array operation must be an array", *c.ArrayComparison.Op)
		}

		switch *c.ArrayComparison.Op {
		case "in":
			return inArray(lhs, array), nil
		case "notin":
			return notInArray(lhs, array), nil
		default:
			return nil, lexer.Errorf(c.Pos, "unsupported array operation %q", *c.ArrayComparison.Op)
		}

	case c.ScalarComparison != nil:
		if c.ScalarComparison.Next == nil {
			return nil, lexer.Errorf(c.Pos, "missing rhs of %q", *c.ScalarComparison.Op)
		}
		rhs, err := c.ScalarComparison.Next.Evaluate(instance)
		if err != nil {
			return nil, err
		}
		return c.compare(lhs, rhs, *c.ScalarComparison.Op)

	default:
		return lhs, nil
	}
}

func (c *Comparison) compare(lhs, rhs interface{}, op string) (interface{}, error) {
	switch lhs := lhs.(type) {
	case uint64:
		switch rhs := rhs.(type) {
		case uint64:
			return uintCompare(op, lhs, rhs, c.Pos)
		case int64:
			return uintCompare(op, lhs, uint64(rhs), c.Pos)
		default:
			return nil, lexer.Errorf(c.Pos, "rhs of %q must be an integer", op)
		}
	case int64:
		switch rhs := rhs.(type) {
		case int64:
			return intCompare(op, lhs, rhs, c.Pos)
		case uint64:
			return intCompare(op, lhs, int64(rhs), c.Pos)
		default:
			return nil, lexer.Errorf(c.Pos, "rhs of %q must be an integer", op)
		}
	case string:
		rhs, ok := rhs.(string)
		if !ok {
			return nil, lexer.Errorf(c.Pos, "rhs of %q must be a string", op)
		}
		return stringCompare(op, lhs, rhs, c.Pos)
	default:
		return nil, lexer.Errorf(c.Pos, "lhs of %q must be an integer or string", op)
	}
}

// Evaluate implements Evaluatable interface
func (t *Term) Evaluate(instance Instance) (interface{}, error) {
	lhs, err := t.Unary.Evaluate(instance)
	if err != nil {
		return nil, err
	}

	if t.Op == nil {
		return lhs, nil
	}

	if t.Next == nil {
		return nil, lexer.Errorf(t.Pos, "expected rhs in binary bit operation")
	}

	rhs, err := t.Next.Evaluate(instance)
	if err != nil {
		return nil, err
	}

	op := *t.Op

	switch lhs := lhs.(type) {
	case uint64:
		switch rhs := rhs.(type) {
		case uint64:
			return uintBinaryOp(op, lhs, rhs, t.Pos)
		case int64:
			return uintBinaryOp(op, lhs, uint64(rhs), t.Pos)
		default:
			return nil, lexer.Errorf(t.Pos, `rhs of %q must be an integer`, op)
		}
	case int64:
		switch rhs := rhs.(type) {
		case int64:
			return intBinaryOp(op, lhs, rhs, t.Pos)
		case uint64:
			return intBinaryOp(op, lhs, int64(rhs), t.Pos)
		default:
			return nil, lexer.Errorf(t.Pos, `rhs of %q must be an integer`, op)
		}
	case string:
		switch rhs := rhs.(type) {
		case string:
			return stringBinaryOp(op, lhs, rhs, t.Pos)
		default:
			return nil, lexer.Errorf(t.Pos, "rhs of %q must be a string", op)
		}
	default:
		return nil, lexer.Errorf(t.Pos, "binary bit operation not supported for this type")
	}
}

// Evaluate implements Evaluatable interface
func (u *Unary) Evaluate(instance Instance) (interface{}, error) {
	if u.Value != nil {
		return u.Value.Evaluate(instance)
	}

	if u.Unary == nil || u.Op == nil {
		return nil, lexer.Errorf(u.Pos, "invalid unary operation")
	}

	rhs, err := u.Unary.Evaluate(instance)
	if err != nil {
		return nil, err
	}

	switch *u.Op {
	case "!":
		rhs, ok := rhs.(bool)
		if !ok {
			return nil, lexer.Errorf(u.Pos, "rhs of %q must be a boolean", *u.Op)
		}
		return !rhs, nil
	case "-":
		switch rhs := rhs.(type) {
		case int64:
			return -rhs, nil
		case uint64:
			return -int64(rhs), nil
		default:
			return nil, lexer.Errorf(u.Pos, "rhs of %q must be an integer", *u.Op)
		}
	case "^":
		switch rhs := rhs.(type) {
		case int64:
			return ^rhs, nil
		case uint64:
			return ^rhs, nil
		default:
			return nil, lexer.Errorf(u.Pos, "rhs of %q must be an integer", *u.Op)
		}
	default:
		return nil, lexer.Errorf(u.Pos, "unsupported unary operator %q", *u.Op)
	}
}

// Evaluate implements Evaluatable interface
func (v *Value) Evaluate(instance Instance) (interface{}, error) {
	switch {
	case v.Hex != nil:
		return strconv.ParseUint(*v.Hex, 0, 64)
	case v.Octal != nil:
		return strconv.ParseUint(*v.Octal, 8, 64)
	case v.Decimal != nil:
		return *v.Decimal, nil
	case v.String != nil:
		return *v.String, nil
	case v.Variable != nil:
		value, ok := instance.Var(*v.Variable)
		if !ok {
			value, ok = builtInVars[*v.Variable]
		}
		if !ok {
			return nil, lexer.Errorf(v.Pos, `unknown variable %q`, *v.Variable)
		}
		return coerceIntegers(value), nil
	case v.Subexpression != nil:
		return v.Subexpression.Evaluate(instance)
	case v.Call != nil:
		return v.Call.Evaluate(instance)
	}

	return nil, lexer.Errorf(v.Pos, `unsupported value type %q`, repr.String(v))
}

// Evaluate implements Evaluatable interface
func (a *Array) Evaluate(instance Instance) (interface{}, error) {
	if a.Ident != nil {
		value, ok := instance.Var(*a.Ident)
		if !ok {
			return nil, lexer.Errorf(a.Pos, `unknown variable %q used as array`, *a.Ident)
		}
		return coerceArrays(value), nil
	}
	var result []interface{}
	for _, value := range a.Values {
		v, err := value.Evaluate(instance)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, nil
}

// Evaluate implements Evaluatable interface
func (c *Call) Evaluate(instance Instance) (interface{}, error) {
	fn, ok := instance.Function(c.Name)
	if !ok {
		return nil, lexer.Errorf(c.Pos, `unknown function "%s()"`, c.Name)
	}
	args := []interface{}{}
	for _, arg := range c.Args {
		value, err := arg.Evaluate(instance)
		if err != nil {
			return nil, err
		}
		args = append(args, value)
	}

	value, err := fn(instance, args...)
	if err != nil {
		return nil, lexer.Errorf(c.Pos, `call to "%s()" failed: %v`, c.Name, err)
	}

	return coerceValues(value), nil
}
