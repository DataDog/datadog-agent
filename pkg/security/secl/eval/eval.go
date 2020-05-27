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

var (
	ErrEvaluatorNotFound     = errors.New("evaluator not found")
	ErrTagsNotFound          = errors.New("tags not found")
	ErrEventTypeNotFound     = errors.New("event type not found")
	ErrSetEventValueNotFound = errors.New("set event value error field not found")
)

type Model interface {
	GetEvaluator(key string) (interface{}, error)
	GetTags(key string) ([]string, error)
	GetEventType(key string) (string, error)
	SetEvent(event interface{})
	SetEventValue(key string, value interface{}) error
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
	Eval        func(ctx *Context) bool
	EventTypes  []string
	Tags        []string
	FieldValues map[string][]FieldValue

	partialEvals map[string]func(ctx *Context) bool
}

type IdentEvaluator struct {
	Eval func(ctx *Context) bool
}

type state struct {
	model       Model
	field       string
	events      map[string]bool
	tags        map[string]bool
	fieldValues map[string][]FieldValue
	macros      map[string]*MacroEvaluator
}

type FieldValueType int

const (
	ScalarValueType  FieldValueType = 1
	PatternValueType FieldValueType = 2
)

type FieldValue struct {
	Value interface{}
	Type  FieldValueType
}

type Opts struct {
	Debug     bool
	Macros    map[string]*ast.Macro
	Constants map[string]interface{}
}

type MacroEvaluator struct {
	Value interface{}
}

type Evaluator interface {
	StringValue(ctx *Context) string
	Eval(ctx *Context) interface{}
}

type BoolEvaluator struct {
	EvalFnc      func(ctx *Context) bool
	DebugEvalFnc func(ctx *Context) bool
	Field        string
	Value        bool

	isPartial bool
}

func (b *BoolEvaluator) StringValue(ctx *Context) string {
	return fmt.Sprintf("%t", b.EvalFnc(nil))
}

func (b *BoolEvaluator) Eval(ctx *Context) interface{} {
	return b.EvalFnc(nil)
}

type IntEvaluator struct {
	EvalFnc      func(ctx *Context) int
	DebugEvalFnc func(ctx *Context) int
	Field        string
	Value        int

	isPartial bool
}

func (i *IntEvaluator) StringValue(ctx *Context) string {
	return fmt.Sprintf("%d", i.EvalFnc(nil))
}

func (i *IntEvaluator) Eval(ctx *Context) interface{} {
	return i.EvalFnc(nil)
}

type StringEvaluator struct {
	EvalFnc      func(ctx *Context) string
	DebugEvalFnc func(ctx *Context) string
	Field        string
	Value        string

	isPartial bool
}

func (s *StringEvaluator) StringValue(ctx *Context) string {
	return s.EvalFnc(ctx)
}

func (s *StringEvaluator) Eval(ctx *Context) interface{} {
	return s.EvalFnc(ctx)
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

func (s *state) UpdateTags(tags []string) {
	for _, tag := range tags {
		s.tags[tag] = true
	}
}

func (s *state) UpdateFields(field string) {
	values, ok := s.fieldValues[field]
	if !ok {
		values = []FieldValue{}
	}
	s.fieldValues[field] = values
}

func (s *state) UpdateFieldValues(field string, value FieldValue) {
	values, ok := s.fieldValues[field]
	if !ok {
		values = []FieldValue{}
	}
	values = append(values, value)
	s.fieldValues[field] = values
}

func (s *state) Tags() []string {
	var tags []string

	for tag := range s.tags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	return tags
}

func (s *state) Events() []string {
	var events []string

	for event := range s.events {
		events = append(events, event)
	}
	sort.Strings(events)

	return events
}

func (s *state) MacroToEvaluator(macro *ast.Macro, model Model, opts *Opts) (*MacroEvaluator, error) {
	var eval interface{}
	var err error

	switch {
	case macro.Expression != nil:
		eval, _, _, err = nodeToEvaluator(macro.Expression, opts, s)
	case macro.Array != nil:
		eval, _, _, err = nodeToEvaluator(macro.Array, opts, s)
	case macro.Primary != nil:
		eval, _, _, err = nodeToEvaluator(macro.Primary, opts, s)
	}

	if err != nil {
		return nil, err
	}

	return &MacroEvaluator{
		Value: eval,
	}, nil
}

func (s *state) GenMacroEvaluators(macros map[string]*ast.Macro, model Model, opts *Opts) error {
	s.macros = make(map[string]*MacroEvaluator)
	for name, macro := range macros {
		eval, err := s.MacroToEvaluator(macro, model, opts)
		if err != nil {
			return err
		}
		s.macros[name] = eval
	}

	return nil
}

func newState(model Model, field string) *state {
	return &state{
		field:       field,
		model:       model,
		events:      make(map[string]bool),
		tags:        make(map[string]bool),
		fieldValues: make(map[string][]FieldValue),
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

func nodeToEvaluator(obj interface{}, opts *Opts, state *state) (interface{}, interface{}, lexer.Position, error) {
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
			if accessor, ok := opts.Constants[*obj.Ident]; ok {
				return accessor, nil, obj.Pos, nil
			}

			if opts.Macros != nil {
				if macro, ok := state.macros[*obj.Ident]; ok {
					return macro.Value, nil, obj.Pos, nil
				}
			}

			accessor, err := state.model.GetEvaluator(*obj.Ident)
			if err != nil {
				return nil, nil, obj.Pos, err
			}

			tags, err := state.model.GetTags(*obj.Ident)
			if err == nil {
				state.UpdateTags(tags)
			}

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
				if macro, ok := state.macros[*obj.Ident]; ok {
					return macro.Value, nil, obj.Pos, nil
				}
			}
		}
	}

	return nil, nil, lexer.Position{}, NewError(lexer.Position{}, fmt.Sprintf("unknown entity '%s'", reflect.TypeOf(obj)))
}

func (r *RuleEvaluator) PartialEval(ctx *Context, field string) (bool, error) {
	eval, ok := r.partialEvals[field]
	if !ok {
		return false, errors.New("field not found")
	}

	return eval(ctx), nil
}

func (r *RuleEvaluator) GetFields() []string {
	fields := make([]string, len(r.partialEvals))
	i := 0
	for key, _ := range r.partialEvals {
		fields[i] = key
		i++
	}
	return fields
}

func eventFromFields(model Model, state *state) ([]string, error) {
	events := make(map[string]bool)
	for field := range state.fieldValues {
		eventType, err := model.GetEventType(field)
		if err != nil {
			return nil, err
		}

		if eventType != "*" {
			events[eventType] = true
		}
	}

	var uniq []string
	for event := range events {
		uniq = append(uniq, event)
	}
	return uniq, nil
}

func RuleToEvaluator(rule *ast.Rule, model Model, opts Opts) (*RuleEvaluator, error) {
	state := newState(model, "")
	if err := state.GenMacroEvaluators(opts.Macros, model, &opts); err != nil {
		return nil, err
	}

	eval, _, _, err := nodeToEvaluator(rule.BooleanExpression, &opts, state)
	if err != nil {
		return nil, err
	}

	evalBool, ok := eval.(*BoolEvaluator)
	if !ok {
		return nil, NewTypeError(rule.Pos, reflect.Bool)
	}

	// transform the whole rule to a partial boolean evaluation function depending only on the given
	// parameters. The goal is to see wheter the rule depends on the given field.
	partialEvals := make(map[string]func(ctx *Context) bool)
	for field := range state.fieldValues {
		state = newState(model, field)

		if err := state.GenMacroEvaluators(opts.Macros, model, &opts); err != nil {
			return nil, err
		}

		pEval, _, _, err := nodeToEvaluator(rule.BooleanExpression, &opts, state)
		if err != nil {
			return nil, err
		}

		pEvalBool, ok := pEval.(*BoolEvaluator)
		if !ok {
			return nil, NewTypeError(rule.Pos, reflect.Bool)
		}

		if pEvalBool.EvalFnc == nil {
			pEvalBool.EvalFnc = func(ctx *Context) bool {
				return pEvalBool.Value
			}
		}

		partialEvals[field] = pEvalBool.EvalFnc
	}

	events, err := eventFromFields(model, state)
	if err != nil {
		return nil, err
	}

	if evalBool.EvalFnc == nil {
		return &RuleEvaluator{
			Eval: func(ctx *Context) bool {
				return evalBool.Value
			},
			EventTypes:   events,
			Tags:         state.Tags(),
			FieldValues:  state.fieldValues,
			partialEvals: partialEvals,
		}, nil
	}

	if opts.Debug {
		return &RuleEvaluator{
			Eval:         evalBool.DebugEvalFnc,
			EventTypes:   events,
			Tags:         state.Tags(),
			FieldValues:  state.fieldValues,
			partialEvals: partialEvals,
		}, nil
	}

	return &RuleEvaluator{
		Eval:         evalBool.EvalFnc,
		EventTypes:   events,
		Tags:         state.Tags(),
		FieldValues:  state.fieldValues,
		partialEvals: partialEvals,
	}, nil
}
