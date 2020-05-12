//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/operators -output eval_operators.go

package eval

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var ErrFieldNotFound = errors.New("field not found")

type Model interface {
	GetEvaluator(key string) (interface{}, error)
	GetTags(key string) ([]string, error)
	SetEvent(event interface{})
}

type Context struct {
	Debug     bool
	evalDepth int
}

func (c *Context) Logf(format string, v ...interface{}) {
	log.Debugf(strings.Repeat("\t", c.evalDepth-1)+format, v...)
}

var (
	EmptyContext = &Context{}
)

type RuleEvaluator struct {
	Eval func(ctx *Context) bool
	Tags []string

	partialEval map[string]func(ctx *Context) bool
}

type IdentEvaluator struct {
	Eval func(ctx *Context) bool
}

type State struct {
	tags   map[string]bool
	fields map[string]bool
}

type Opts struct {
	Model  Model
	Field  string
	Macros map[string]*MacroEvaluator
}

type MacroEvaluator struct {
	Value interface{}
}

type Evaluator interface {
	StringValue() string
}

type BoolEvaluator struct {
	Eval      func(ctx *Context) bool
	DebugEval func(ctx *Context) bool
	Value     bool

	Field     string
	IsPartial bool
}

func (b *BoolEvaluator) StringValue() string {
	return fmt.Sprintf("%t", b.Eval(nil))
}

type IntEvaluator struct {
	Eval      func(ctx *Context) int
	DebugEval func(ctx *Context) int
	Value     int

	Field     string
	IsPartial bool
}

func (i *IntEvaluator) StringValue() string {
	return fmt.Sprintf("%d", i.Eval(nil))
}

type StringEvaluator struct {
	Eval      func(ctx *Context) string
	DebugEval func(ctx *Context) string
	Value     string

	Field     string
	IsPartial bool
}

func (s *StringEvaluator) StringValue() string {
	return s.Eval(nil)
}

type StringArray struct {
	Values []string
}

type IntArray struct {
	Values []int
}

type AstToEvalError struct {
	Pos  lexer.Position
	Text string
}

func (s *State) UpdateTags(tags []string) {
	for _, tag := range tags {
		s.tags[tag] = true
	}
}

func (s *State) UpdateFields(field string) {
	s.fields[field] = true
}

func (s *State) Tags() []string {
	var tags []string

	for tag := range s.tags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	return tags
}

func NewState() *State {
	return &State{
		tags:   make(map[string]bool),
		fields: make(map[string]bool),
	}
}

func (r *AstToEvalError) Error() string {
	return fmt.Sprintf("%s: %s", r.Text, r.Pos)
}

func NewError(pos lexer.Position, text string) *AstToEvalError {
	return &AstToEvalError{Pos: pos, Text: text}
}

func NewTypeError(pos lexer.Position, kind reflect.Kind) *AstToEvalError {
	return NewError(pos, fmt.Sprintf("%s expected", kind))
}

func NewOpError(pos lexer.Position, op string) *AstToEvalError {
	return NewError(pos, fmt.Sprintf("unknown operator %s", op))
}

func nodeToEvaluator(obj interface{}, opts *Opts, state *State) (interface{}, interface{}, lexer.Position, error) {
	switch obj := obj.(type) {
	case *ast.BooleanExpression:
		return nodeToEvaluator(obj.Expression, opts, state)
	case *ast.Expression:
		cmp, _, pos, err := nodeToEvaluator(obj.Comparison, opts, state)
		if err != nil {
			return nil, nil, pos, err
		}

		if obj.Op != nil {
			cmpBool, ok := cmp.(*BoolEvaluator)
			if !ok {
				return nil, nil, obj.Pos, NewTypeError(obj.Pos, reflect.Bool)
			}

			next, _, pos, err := nodeToEvaluator(obj.Next, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			nextBool, ok := next.(*BoolEvaluator)
			if !ok {
				return nil, nil, pos, NewTypeError(pos, reflect.Bool)
			}

			switch *obj.Op {
			case "||":
				return Or(cmpBool, nextBool, opts, state), nil, obj.Pos, nil
			case "&&":
				return And(cmpBool, nextBool, opts, state), nil, obj.Pos, nil
			}
			return nil, nil, pos, NewOpError(obj.Pos, *obj.Op)
		}
		return cmp, nil, obj.Pos, nil
	case *ast.BitOperation:
		unary, _, pos, err := nodeToEvaluator(obj.Unary, opts, state)
		if err != nil {
			return nil, nil, pos, err
		}

		if obj.Op != nil {
			bitInt, ok := unary.(*IntEvaluator)
			if !ok {
				return nil, nil, obj.Pos, NewTypeError(obj.Pos, reflect.Int)
			}

			next, _, pos, err := nodeToEvaluator(obj.Next, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			nextInt, ok := next.(*IntEvaluator)
			if !ok {
				return nil, nil, pos, NewTypeError(pos, reflect.Int)
			}

			switch *obj.Op {
			case "&":
				return IntAnd(bitInt, nextInt, opts, state), nil, obj.Pos, nil
			case "|":
				return IntOr(bitInt, nextInt, opts, state), nil, obj.Pos, nil
			case "^":
				return IntXor(bitInt, nextInt, opts, state), nil, obj.Pos, nil
			}
			return nil, nil, pos, NewOpError(obj.Pos, *obj.Op)
		}
		return unary, nil, obj.Pos, nil

	case *ast.Comparison:
		unary, _, pos, err := nodeToEvaluator(obj.BitOperation, opts, state)
		if err != nil {
			return nil, nil, pos, err
		}

		if obj.ArrayComparison != nil {
			next, _, pos, err := nodeToEvaluator(obj.ArrayComparison, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			switch unary := unary.(type) {
			case *StringEvaluator:
				nextStringArray, ok := next.(*StringArray)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Array)
				}

				return StringArrayContains(unary, nextStringArray, *obj.ArrayComparison.Op == "notin", opts, state), nil, obj.Pos, nil
			case *IntEvaluator:
				nextIntArray, ok := next.(*IntArray)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Array)
				}

				return IntArrayContains(unary, nextIntArray, *obj.ArrayComparison.Op == "notin", opts, state), nil, obj.Pos, nil
			default:
				return nil, nil, pos, NewTypeError(pos, reflect.Array)
			}
		} else if obj.ScalarComparison != nil {
			next, _, pos, err := nodeToEvaluator(obj.ScalarComparison, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			switch unary := unary.(type) {
			case *BoolEvaluator:
				nextBool, ok := next.(*BoolEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Bool)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					return BoolNotEquals(unary, nextBool, opts, state), nil, obj.Pos, nil
				case "==":
					return BoolEquals(unary, nextBool, opts, state), nil, obj.Pos, nil
				}
				return nil, nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op)
			case *StringEvaluator:
				nextString, ok := next.(*StringEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.String)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					return StringNotEquals(unary, nextString, opts, state), nil, pos, nil
				case "==":
					return StringEquals(unary, nextString, opts, state), nil, pos, nil
				case "=~", "!~":
					eval, err := StringMatches(unary, nextString, *obj.ScalarComparison.Op == "!~", opts, state)
					if err != nil {
						return nil, nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op)
					}
					return eval, nil, obj.Pos, nil
				}
				return nil, nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op)
			case *IntEvaluator:
				nextInt, ok := next.(*IntEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Int)
				}

				switch *obj.ScalarComparison.Op {
				case "<":
					return LesserThan(unary, nextInt, opts, state), nil, obj.Pos, nil
				case "<=":
					return LesserOrEqualThan(unary, nextInt, opts, state), nil, obj.Pos, nil
				case ">":
					return GreaterThan(unary, nextInt, opts, state), nil, obj.Pos, nil
				case ">=":
					return GreaterOrEqualThan(unary, nextInt, opts, state), nil, obj.Pos, nil
				case "!=":
					return IntNotEquals(unary, nextInt, opts, state), nil, obj.Pos, nil
				case "==":
					return IntEquals(unary, nextInt, opts, state), nil, obj.Pos, nil
				}
				return nil, nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op)
			}
		} else {
			return unary, nil, pos, nil
		}

	case *ast.ArrayComparison:
		return nodeToEvaluator(obj.Array, opts, state)

	case *ast.ScalarComparison:
		return nodeToEvaluator(obj.Next, opts, state)

	case *ast.Unary:
		if obj.Op != nil {
			unary, _, pos, err := nodeToEvaluator(obj.Unary, opts, state)
			if err != nil {
				return nil, nil, pos, err
			}

			switch *obj.Op {
			case "!":
				unaryBool, ok := unary.(*BoolEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Bool)
				}

				return Not(unaryBool, opts, state), nil, obj.Pos, nil
			case "-":
				unaryInt, ok := unary.(*IntEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Int)
				}

				return Minus(unaryInt, opts, state), nil, pos, nil
			case "^":
				unaryInt, ok := unary.(*IntEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Int)
				}

				return IntNot(unaryInt, opts, state), nil, pos, nil
			}
			return nil, nil, pos, NewOpError(obj.Pos, *obj.Op)
		}

		return nodeToEvaluator(obj.Primary, opts, state)
	case *ast.Primary:
		switch {
		case obj.Ident != nil:
			if accessor, ok := constants[*obj.Ident]; ok {
				return accessor, nil, obj.Pos, nil
			}

			if opts.Macros != nil {
				if macro, ok := opts.Macros[*obj.Ident]; ok {
					return macro.Value, nil, obj.Pos, nil
				}
			}

			accessor, err := opts.Model.GetEvaluator(*obj.Ident)
			if err != nil {
				return nil, nil, obj.Pos, err
			}

			tags, err := opts.Model.GetTags(*obj.Ident)
			if err != nil {
				return nil, nil, obj.Pos, err
			}

			state.UpdateTags(tags)
			state.UpdateFields(*obj.Ident)

			return accessor, nil, obj.Pos, nil
		case obj.Number != nil:
			return &IntEvaluator{
				Value: *obj.Number,
			}, nil, obj.Pos, nil
		case obj.String != nil:
			return &StringEvaluator{
				Value: *obj.String,
			}, nil, obj.Pos, nil
		case obj.SubExpression != nil:
			return nodeToEvaluator(obj.SubExpression, opts, state)
		default:
			return nil, nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("unknown primary '%s'", reflect.TypeOf(obj)))
		}
	case *ast.Array:
		if len(obj.Numbers) != 0 {
			ints := obj.Numbers
			sort.Ints(ints)
			return &IntArray{Values: ints}, nil, obj.Pos, nil
		} else if len(obj.Strings) != 0 {
			strings := obj.Strings
			sort.Strings(strings)
			return &StringArray{Values: strings}, nil, obj.Pos, nil
		} else if obj.Ident != nil {
			if opts.Macros != nil {
				if macro, ok := opts.Macros[*obj.Ident]; ok {
					return macro.Value, nil, obj.Pos, nil
				}
			}
		}
	}

	return nil, nil, lexer.Position{}, NewError(lexer.Position{}, fmt.Sprintf("unknown entity '%s'", reflect.TypeOf(obj)))
}

func (r *RuleEvaluator) IsDiscrimator(ctx *Context, field string) (bool, error) {
	eval, ok := r.partialEval[field]
	if !ok {
		return false, errors.New("field not found")
	}

	return !eval(ctx), nil
}

func (r *RuleEvaluator) GetFields() []string {
	fields := make([]string, len(r.partialEval))
	i := 0
	for key, _ := range r.partialEval {
		fields[i] = key
		i++
	}
	return fields
}

func MacroToEvaluator(macro *ast.Macro, opts *Opts, state *State) (*MacroEvaluator, error) {
	var eval interface{}
	var err error

	switch {
	case macro.Expression != nil:
		eval, _, _, err = nodeToEvaluator(macro.Expression, opts, state)
	case macro.Array != nil:
		eval, _, _, err = nodeToEvaluator(macro.Array, opts, state)
	case macro.Primary != nil:
		eval, _, _, err = nodeToEvaluator(macro.Primary, opts, state)
	}

	if err != nil {
		return nil, err
	}

	return &MacroEvaluator{
		Value: eval,
	}, nil
}

func macroEvaluators(macros map[string]*ast.Macro, field string, state *State, model Model) (map[string]*MacroEvaluator, error) {
	m := make(map[string]*MacroEvaluator)
	for name, macro := range macros {
		eval, err := MacroToEvaluator(macro, &Opts{Field: field, Model: model}, state)
		if err != nil {
			return nil, err
		}
		m[name] = eval
	}

	return m, nil
}

func RuleToEvaluator(rule *ast.Rule, macros map[string]*ast.Macro, model Model, debug bool) (*RuleEvaluator, error) {
	state := NewState()

	m, err := macroEvaluators(macros, "", state, model)
	if err != nil {
		return nil, err
	}

	eval, _, _, err := nodeToEvaluator(rule.BooleanExpression, &Opts{Macros: m, Model: model}, state)
	if err != nil {
		return nil, err
	}

	evalBool, ok := eval.(*BoolEvaluator)
	if !ok {
		return nil, NewTypeError(rule.Pos, reflect.Bool)
	}

	// Generates an evaluator which allows to guarranty that a value for a given parameter will cause the evaluation of an expression to always
	// return false regardless of the other parameters.
	// The evaluator is a function that accepts only one argument : the value of the selected parameter.
	// If the return value of this function is "false", the argument is called a discriminator :
	// if we see this discriminator for the selected parameter, we can skip the evaluation of the whole rule.
	partialEval := make(map[string]func(ctx *Context) bool)
	for field := range state.fields {
		state = NewState()

		m, err := macroEvaluators(macros, field, state, model)
		if err != nil {
			return nil, err
		}

		pEval, _, _, err := nodeToEvaluator(rule.BooleanExpression, &Opts{Field: field, Macros: m, Model: model}, state)
		if err != nil {
			return nil, err
		}

		pEvalBool, ok := pEval.(*BoolEvaluator)
		if !ok {
			return nil, NewTypeError(rule.Pos, reflect.Bool)
		}

		if pEvalBool.Eval == nil {
			pEvalBool.Eval = func(ctx *Context) bool {
				return pEvalBool.Value
			}
		}

		partialEval[field] = pEvalBool.Eval
	}

	if evalBool.Eval == nil {
		return &RuleEvaluator{
			Eval: func(ctx *Context) bool {
				return evalBool.Value
			},
			Tags:        state.Tags(),
			partialEval: partialEval,
		}, nil
	}

	if debug {
		return &RuleEvaluator{
			Eval:        evalBool.DebugEval,
			Tags:        state.Tags(),
			partialEval: partialEval,
		}, nil
	}

	return &RuleEvaluator{
		Eval:        evalBool.Eval,
		Tags:        state.Tags(),
		partialEval: partialEval,
	}, nil
}
