//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/operators -output eval_operators.go

package eval

import (
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/alecthomas/participle/lexer"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/policy"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type EventType = string
type Field = string

type Model interface {
	GetEvaluator(key Field) (interface{}, error)
	ValidateField(key Field, value FieldValue) error
	SetEvent(event interface{})
	GetEvent() Event
}

type Context struct {
	Debug     bool
	evalDepth int
}

func (c *Context) Logf(format string, v ...interface{}) {
	log.Tracef(strings.Repeat("\t", c.evalDepth-1)+format, v...)
}

var (
	EmptyContext = &Context{}
)

type RuleEvaluator struct {
	Eval        func(ctx *Context) bool
	EventTypes  []EventType
	Tags        []string
	FieldValues map[Field][]FieldValue

	partialEvals map[Field]func(ctx *Context) bool
}

type IdentEvaluator struct {
	Eval func(ctx *Context) bool
}

type state struct {
	model       Model
	field       Field
	events      map[EventType]bool
	tags        map[string]bool
	fieldValues map[Field][]FieldValue
	macros      map[policy.MacroID]*MacroEvaluator
}

type FieldValueType int

const (
	ScalarValueType  FieldValueType = 1
	PatternValueType FieldValueType = 2
	BitmaskValueType FieldValueType = 4
)

type FieldValue struct {
	Value interface{}
	Type  FieldValueType
}

type Opts struct {
	Debug     bool
	Constants map[string]interface{}
	Macros    map[policy.MacroID]*MacroEvaluator
}

// NewOptsWithParams - Initializes a new Opts instance with Debug and Constants parameters
func NewOptsWithParams(debug bool, constants map[string]interface{}) Opts {
	return Opts{
		Debug:     debug,
		Constants: constants,
		Macros:    make(map[policy.MacroID]*MacroEvaluator),
	}
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

func (s *state) UpdateTags(tags []string) {
	for _, tag := range tags {
		s.tags[tag] = true
	}
}

func (s *state) UpdateFields(field Field) {
	if _, ok := s.fieldValues[field]; !ok {
		s.fieldValues[field] = []FieldValue{}
	}
}

func (s *state) UpdateFieldValues(field Field, value FieldValue) error {
	values, ok := s.fieldValues[field]
	if !ok {
		values = []FieldValue{}
	}
	values = append(values, value)
	s.fieldValues[field] = values
	return s.model.ValidateField(field, value)
}

func (s *state) Tags() []string {
	var tags []string

	for tag := range s.tags {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	return tags
}

func (s *state) Events() []EventType {
	var events []EventType

	for event := range s.events {
		events = append(events, event)
	}
	sort.Strings(events)

	return events
}

func newState(model Model, field Field, macros map[policy.MacroID]*MacroEvaluator) *state {
	if macros == nil {
		macros = make(map[policy.MacroID]*MacroEvaluator)
	}
	return &state{
		field:       field,
		macros:      macros,
		model:       model,
		events:      make(map[EventType]bool),
		tags:        make(map[string]bool),
		fieldValues: make(map[Field][]FieldValue),
	}
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
				boolEvaluator, err := Or(cmpBool, nextBool, opts, state)
				if err != nil {
					return nil, nil, obj.Pos, err
				}
				return boolEvaluator, nil, obj.Pos, nil
			case "&&":
				boolEvaluator, err := And(cmpBool, nextBool, opts, state)
				if err != nil {
					return nil, nil, obj.Pos, err
				}
				return boolEvaluator, nil, obj.Pos, nil
			}
			return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
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
				intEvaluator, err := IntAnd(bitInt, nextInt, opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return intEvaluator, nil, obj.Pos, nil
			case "|":
				IntEvaluator, err := IntOr(bitInt, nextInt, opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return IntEvaluator, nil, obj.Pos, nil
			case "^":
				IntEvaluator, err := IntXor(bitInt, nextInt, opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return IntEvaluator, nil, obj.Pos, nil
			}
			return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
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

				boolEvaluator, err := StringArrayContains(unary, nextStringArray, *obj.ArrayComparison.Op == "notin", opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return boolEvaluator, nil, obj.Pos, nil
			case *IntEvaluator:
				nextIntArray, ok := next.(*IntArray)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Array)
				}

				intEvaluator, err := IntArrayContains(unary, nextIntArray, *obj.ArrayComparison.Op == "notin", opts, state)
				if err != nil {
					return nil, nil, pos, err
				}
				return intEvaluator, nil, obj.Pos, nil
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
					boolEvaluator, err := BoolNotEquals(unary, nextBool, opts, state)
					if err != nil {
						return nil, nil, pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case "==":
					boolEvaluator, err := BoolEquals(unary, nextBool, opts, state)
					if err != nil {
						return nil, nil, pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				}
				return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			case *StringEvaluator:
				nextString, ok := next.(*StringEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.String)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					stringEvaluator, err := StringNotEquals(unary, nextString, opts, state)
					if err != nil {
						return nil, nil, pos, err
					}
					return stringEvaluator, nil, pos, nil
				case "==":
					stringEvaluator, err := StringEquals(unary, nextString, opts, state)
					if err != nil {
						return nil, nil, pos, err
					}
					return stringEvaluator, nil, pos, nil
				case "=~", "!~":
					eval, err := StringMatches(unary, nextString, *obj.ScalarComparison.Op == "!~", opts, state)
					if err != nil {
						return nil, nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op, err)
					}
					return eval, nil, obj.Pos, nil
				}
				return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
			case *IntEvaluator:
				nextInt, ok := next.(*IntEvaluator)
				if !ok {
					return nil, nil, pos, NewTypeError(pos, reflect.Int)
				}

				switch *obj.ScalarComparison.Op {
				case "<":
					boolEvaluator, err := LesserThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case "<=":
					boolEvaluator, err := LesserOrEqualThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case ">":
					boolEvaluator, err := GreaterThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case ">=":
					boolEvaluator, err := GreaterOrEqualThan(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case "!=":
					boolEvaluator, err := IntNotEquals(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				case "==":
					boolEvaluator, err := IntEquals(unary, nextInt, opts, state)
					if err != nil {
						return nil, nil, obj.Pos, err
					}
					return boolEvaluator, nil, obj.Pos, nil
				}
				return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.ScalarComparison.Op)
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
			return nil, nil, pos, NewOpUnknownError(obj.Pos, *obj.Op)
		}

		return nodeToEvaluator(obj.Primary, opts, state)
	case *ast.Primary:
		switch {
		case obj.Ident != nil:
			if accessor, ok := opts.Constants[*obj.Ident]; ok {
				return accessor, nil, obj.Pos, nil
			}

			if state.macros != nil {
				if macro, ok := state.macros[*obj.Ident]; ok {
					return macro.Value, nil, obj.Pos, nil
				}
			}

			accessor, err := state.model.GetEvaluator(*obj.Ident)
			if err != nil {
				return nil, nil, obj.Pos, err
			}

			tags, err := state.model.GetEvent().GetFieldTags(*obj.Ident)
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
			strs := obj.Strings
			sort.Strings(strs)
			return &StringArray{Values: strs}, nil, obj.Pos, nil
		} else if obj.Ident != nil {
			if state.macros != nil {
				if macro, ok := state.macros[*obj.Ident]; ok {
					return macro.Value, nil, obj.Pos, nil
				}
			}
		}
	}

	return nil, nil, lexer.Position{}, NewError(lexer.Position{}, fmt.Sprintf("unknown entity '%s'", reflect.TypeOf(obj)))
}

func (r *RuleEvaluator) PartialEval(ctx *Context, field Field) (bool, error) {
	eval, ok := r.partialEvals[field]
	if !ok {
		return false, errors.New("field not found")
	}

	return eval(ctx), nil
}

func (r *RuleEvaluator) SetPartial(field string, partialEval func(ctx *Context) bool) {
	if r.partialEvals == nil {
		r.partialEvals = make(map[string]func(ctx *Context) bool)
	}
	r.partialEvals[field] = partialEval
}

func (r *RuleEvaluator) GetFields() []string {
	fields := make([]string, len(r.FieldValues))
	i := 0
	for key, _ := range r.FieldValues {
		fields[i] = key
		i++
	}
	return fields
}

func eventFromFields(model Model, state *state) ([]string, error) {
	events := make(map[EventType]bool)
	for field := range state.fieldValues {
		eventType, err := model.GetEvent().GetFieldEventType(field)
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

func MacroToEvaluator(macro *ast.Macro, model Model, opts *Opts, field Field) (*MacroEvaluator, error) {
	var eval interface{}
	var err error
	state := newState(model, field, opts.Macros)

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

// RuleToEvaluator - Generate a rule evaluator for the provided ast
func RuleToEvaluator(rule *ast.Rule, model Model, opts *Opts) (*RuleEvaluator, error) {
	state := newState(model, "", opts.Macros)

	eval, _, _, err := nodeToEvaluator(rule.BooleanExpression, opts, state)
	if err != nil {
		return nil, err
	}

	evalBool, ok := eval.(*BoolEvaluator)
	if !ok {
		return nil, NewTypeError(rule.Pos, reflect.Bool)
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
			EventTypes:  events,
			Tags:        state.Tags(),
			FieldValues: state.fieldValues,
		}, nil
	}

	if opts.Debug {
		return &RuleEvaluator{
			Eval:        evalBool.DebugEvalFnc,
			EventTypes:  events,
			Tags:        state.Tags(),
			FieldValues: state.fieldValues,
		}, nil
	}

	return &RuleEvaluator{
		Eval:        evalBool.EvalFnc,
		EventTypes:  events,
		Tags:        state.Tags(),
		FieldValues: state.fieldValues,
	}, nil
}
