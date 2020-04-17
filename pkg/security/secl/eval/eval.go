//go:generate go run github.com/DataDog/datadog-agent/pkg/security/generators/operators -output eval_operators.go

package eval

import (
	"fmt"
	"log"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"syscall"

	"github.com/alecthomas/participle/lexer"
	"github.com/pkg/errors"

	"github.com/DataDog/datadog-agent/pkg/security/model"
	"github.com/DataDog/datadog-agent/pkg/security/secl/ast"
)

type Context struct {
	Debug     bool
	Event     *model.Event
	evalDepth int
}

func (c *Context) Logf(format string, v ...interface{}) {
	log.Printf(strings.Repeat("\t", c.evalDepth-1)+format, v...)
}

var (
	EmptyContext = &Context{}
)

type Evaluator func(ctx *Context) bool

type BoolEvaluator struct {
	Eval      func(ctx *Context) bool
	DebugEval func(ctx *Context) bool
	Value     bool
}

type IntEvaluator struct {
	Eval      func(ctx *Context) int
	DebugEval func(ctx *Context) int
	Value     int
}

type StringEvaluator struct {
	Eval      func(ctx *Context) string
	DebugEval func(ctx *Context) string
	Value     string
}

type StringArrayEvaluator struct {
	Values []string
}

type IntArrayEvaluator struct {
	Values []int
}

type ArrayEvaluator struct {
	Value *ast.Array
}

type AstToEvalError struct {
	Pos  lexer.Position
	Text string
}

var (
	constants = map[string]interface{}{
		// boolean
		"true":  &BoolEvaluator{Value: true},
		"false": &BoolEvaluator{Value: false},

		// open flags
		"O_RDONLY": &IntEvaluator{Value: syscall.O_RDONLY},
		"O_WRONLY": &IntEvaluator{Value: syscall.O_WRONLY},
		"O_RDWR":   &IntEvaluator{Value: syscall.O_RDWR},
		"O_APPEND": &IntEvaluator{Value: syscall.O_APPEND},
		"O_CREAT":  &IntEvaluator{Value: syscall.O_CREAT},
		"O_EXCL":   &IntEvaluator{Value: syscall.O_EXCL},
		"O_SYNC":   &IntEvaluator{Value: syscall.O_SYNC},
		"O_TRUNC":  &IntEvaluator{Value: syscall.O_TRUNC},

		// permissions
		"S_IEXEC":  &IntEvaluator{Value: syscall.S_IEXEC},
		"S_IFBLK":  &IntEvaluator{Value: syscall.S_IFBLK},
		"S_IFCHR":  &IntEvaluator{Value: syscall.S_IFCHR},
		"S_IFDIR":  &IntEvaluator{Value: syscall.S_IFDIR},
		"S_IFIFO":  &IntEvaluator{Value: syscall.S_IFIFO},
		"S_IFLNK":  &IntEvaluator{Value: syscall.S_IFLNK},
		"S_IFMT":   &IntEvaluator{Value: syscall.S_IFMT},
		"S_IFREG":  &IntEvaluator{Value: syscall.S_IFREG},
		"S_IFSOCK": &IntEvaluator{Value: syscall.S_IFSOCK},
		"S_IREAD":  &IntEvaluator{Value: syscall.S_IREAD},
		"S_IRGRP":  &IntEvaluator{Value: syscall.S_IRGRP},
		"S_IROTH":  &IntEvaluator{Value: syscall.S_IROTH},
		"S_IRUSR":  &IntEvaluator{Value: syscall.S_IRUSR},
		"S_IRWXG":  &IntEvaluator{Value: syscall.S_IRWXG},
		"S_IRWXO":  &IntEvaluator{Value: syscall.S_IRWXO},
		"S_IRWXU":  &IntEvaluator{Value: syscall.S_IRWXU},
		"S_ISGID":  &IntEvaluator{Value: syscall.S_ISGID},
		"S_ISUID":  &IntEvaluator{Value: syscall.S_ISUID},
		"S_ISVTX":  &IntEvaluator{Value: syscall.S_ISVTX},
		"S_IWGRP":  &IntEvaluator{Value: syscall.S_IWGRP},
		"S_IWOTH":  &IntEvaluator{Value: syscall.S_IWOTH},
		"S_IWRITE": &IntEvaluator{Value: syscall.S_IWRITE},
		"S_IWUSR":  &IntEvaluator{Value: syscall.S_IWUSR},
		"S_IXGRP":  &IntEvaluator{Value: syscall.S_IXGRP},
		"S_IXOTH":  &IntEvaluator{Value: syscall.S_IXOTH},
		"S_IXUSR":  &IntEvaluator{Value: syscall.S_IXUSR},
	}
)

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

func IntNot(a *IntEvaluator) *IntEvaluator {
	if a.Eval != nil {
		ea := a.Eval
		return &IntEvaluator{
			Eval: func(ctx *Context) int {
				return ^ea(ctx)
			},
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op := ea(ctx)
				result := ^ea(ctx)
				ctx.Logf("Evaluation ^%d => %d", op, result)
				ctx.evalDepth--
				return result
			},
		}
	}

	ea := ^a.Value
	return &IntEvaluator{
		Eval:      func(ctx *Context) int { return ea },
		DebugEval: func(ctx *Context) int { return ea },
	}
}

func StringMatches(a *StringEvaluator, b *StringEvaluator, not bool) (*BoolEvaluator, error) {
	if b.Eval != nil {
		return nil, errors.New("regex has to be a scalar string")
	}

	p := strings.ReplaceAll(b.Value, "*", ".*")
	re, err := regexp.Compile(p)
	if err != nil {
		return nil, err
	}

	if a.Eval != nil {
		ea := a.Eval
		return &BoolEvaluator{
			Eval: func(ctx *Context) bool {
				result := re.MatchString(ea(ctx))
				if not {
					return !result
				}
				return result
			},
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op := ea(ctx)
				result := re.MatchString(op)
				if not {
					return !result
				}
				ctx.Logf("Evaluating %s ~= %s => %v", op, p, result)
				ctx.evalDepth--
				return result
			},
		}, nil
	}

	ea := re.MatchString(a.Value)
	if not {
		return &BoolEvaluator{
			Value: !ea,
		}, nil
	}
	return &BoolEvaluator{
		Value: ea,
	}, nil
}

func Not(a *BoolEvaluator) *BoolEvaluator {
	if a.Eval != nil {
		ea := a.Eval
		return &BoolEvaluator{
			Eval: func(ctx *Context) bool { return !ea(ctx) },
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				op := ea(ctx)
				result := !op
				ctx.Logf("Evaluating ! %v => %v", op, result)
				ctx.evalDepth--
				return result
			},
		}
	}

	ea := !a.Value
	return &BoolEvaluator{
		Eval:      func(ctx *Context) bool { return ea },
		DebugEval: func(ctx *Context) bool { return ea },
	}
}

func Minus(a *IntEvaluator) *IntEvaluator {
	if a.Eval != nil {
		ea := a.Eval
		return &IntEvaluator{
			Eval: func(ctx *Context) int {
				return -ea(ctx)
			},
			DebugEval: func(ctx *Context) int {
				ctx.evalDepth++
				op := ea(ctx)
				result := -op
				ctx.Logf("Evaluating -%d => %d", op, result)
				ctx.evalDepth--
				return result
			},
		}
	}

	ea := -a.Value
	return &IntEvaluator{
		Eval:      func(ctx *Context) int { return ea },
		DebugEval: func(ctx *Context) int { return ea },
	}
}

func StringArrayContains(a *StringEvaluator, b *StringArrayEvaluator, not bool) *BoolEvaluator {
	if a.Eval != nil {
		ea := a.Eval
		return &BoolEvaluator{
			Eval: func(ctx *Context) bool {
				s := ea(ctx)
				i := sort.SearchStrings(b.Values, s)
				result := i < len(b.Values) && b.Values[i] == s
				if not {
					result = !result
				}
				return result
			},
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				s := ea(ctx)
				i := sort.SearchStrings(b.Values, s)
				result := i < len(b.Values) && b.Values[i] == s
				ctx.Logf("Evaluating %s in %+v => %v", s, b.Values, result)
				if not {
					result = !result
				}
				ctx.evalDepth--
				return result
			},
		}
	}

	ea := a.Value
	return &BoolEvaluator{
		Eval: func(ctx *Context) bool {
			i := sort.SearchStrings(b.Values, ea)
			result := i < len(b.Values) && b.Values[i] == ea
			if not {
				result = !result
			}
			return result
		},
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			i := sort.SearchStrings(b.Values, ea)
			result := i < len(b.Values) && b.Values[i] == ea
			if not {
				result = !result
			}
			ctx.Logf("Evaluating %s in %+v => %v", ea, b.Values, result)
			ctx.evalDepth--
			return result
		},
	}
}

func IntArrayContains(a *IntEvaluator, b *IntArrayEvaluator, not bool) *BoolEvaluator {
	if a.Eval != nil {
		ea := a.Eval
		return &BoolEvaluator{
			Eval: func(ctx *Context) bool {
				ctx.evalDepth++
				n := ea(ctx)
				i := sort.SearchInts(b.Values, n)
				result := i < len(b.Values) && b.Values[i] == n
				if not {
					result = !result
				}
				ctx.evalDepth--
				return result
			},
			DebugEval: func(ctx *Context) bool {
				ctx.evalDepth++
				n := ea(ctx)
				i := sort.SearchInts(b.Values, n)
				result := i < len(b.Values) && b.Values[i] == n
				if not {
					result = !result
				}
				ctx.Logf("Evaluating %d in %+v => %v", n, b.Values, result)
				ctx.evalDepth--
				return result
			},
		}
	}

	ea := a.Value
	return &BoolEvaluator{
		Eval: func(ctx *Context) bool {
			i := sort.SearchInts(b.Values, ea)
			result := i < len(b.Values) && b.Values[i] == ea
			if not {
				result = !result
			}
			return result
		},
		DebugEval: func(ctx *Context) bool {
			ctx.evalDepth++
			i := sort.SearchInts(b.Values, ea)
			result := i < len(b.Values) && b.Values[i] == ea
			if not {
				result = !result
			}
			ctx.Logf("Evaluating %d in %+v => %v", ea, b.Values, result)
			ctx.evalDepth--
			return result
		},
	}
}

func nodeToEvaluator(obj interface{}) (interface{}, lexer.Position, error) {
	switch obj := obj.(type) {
	case *ast.BooleanExpression:
		return nodeToEvaluator(obj.Expression)
	case *ast.Expression:
		cmp, pos, err := nodeToEvaluator(obj.Comparison)
		if err != nil {
			return nil, pos, err
		}

		if obj.Op != nil {
			cmpBool, ok := cmp.(*BoolEvaluator)
			if !ok {
				return nil, obj.Pos, NewTypeError(obj.Pos, reflect.Bool)
			}

			next, pos, err := nodeToEvaluator(obj.Next)
			if err != nil {
				return nil, pos, err
			}

			nextBool, ok := next.(*BoolEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Bool)
			}

			switch *obj.Op {
			case "||":
				return Or(cmpBool, nextBool), obj.Pos, nil
			case "&&":
				return And(cmpBool, nextBool), obj.Pos, nil
			}
			return nil, pos, NewOpError(obj.Pos, *obj.Op)
		}
		return cmp, obj.Pos, nil
	case *ast.BitOperation:
		unary, pos, err := nodeToEvaluator(obj.Unary)
		if err != nil {
			return nil, pos, err
		}

		if obj.Op != nil {
			bitInt, ok := unary.(*IntEvaluator)
			if !ok {
				return nil, obj.Pos, NewTypeError(obj.Pos, reflect.Int)
			}

			next, pos, err := nodeToEvaluator(obj.Next)
			if err != nil {
				return nil, pos, err
			}

			nextInt, ok := next.(*IntEvaluator)
			if !ok {
				return nil, pos, NewTypeError(pos, reflect.Int)
			}

			switch *obj.Op {
			case "&":
				return IntAnd(bitInt, nextInt), obj.Pos, nil
			case "|":
				return IntOr(bitInt, nextInt), obj.Pos, nil
			case "^":
				return IntXor(bitInt, nextInt), obj.Pos, nil
			}
			return nil, pos, NewOpError(obj.Pos, *obj.Op)
		}
		return unary, obj.Pos, nil

	case *ast.Comparison:
		unary, pos, err := nodeToEvaluator(obj.BitOperation)
		if err != nil {
			return nil, pos, err
		}

		if obj.ArrayComparison != nil {
			next, pos, err := nodeToEvaluator(obj.ArrayComparison)
			if err != nil {
				return nil, pos, err
			}

			switch unary := unary.(type) {
			case *StringEvaluator:
				nextStringArray, ok := next.(*StringArrayEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Array)
				}

				return StringArrayContains(unary, nextStringArray, *obj.ArrayComparison.Op == "notin"), obj.Pos, nil
			case *IntEvaluator:
				nextIntArray, ok := next.(*IntArrayEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Array)
				}

				return IntArrayContains(unary, nextIntArray, *obj.ArrayComparison.Op == "notin"), obj.Pos, nil
			default:
				return nil, pos, NewTypeError(pos, reflect.Array)
			}
		} else if obj.ScalarComparison != nil {
			next, pos, err := nodeToEvaluator(obj.ScalarComparison)
			if err != nil {
				return nil, pos, err
			}

			switch unary := unary.(type) {
			case *BoolEvaluator:
				nextBool, ok := next.(*BoolEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Bool)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					return BoolNotEquals(unary, nextBool), obj.Pos, nil
				case "==":
					return BoolEquals(unary, nextBool), obj.Pos, nil
				}
				return nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op)
			case *StringEvaluator:
				nextString, ok := next.(*StringEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.String)
				}

				switch *obj.ScalarComparison.Op {
				case "!=":
					return StringNotEquals(unary, nextString), pos, nil
				case "==":
					return StringEquals(unary, nextString), pos, nil
				case "=~", "!~":
					eval, err := StringMatches(unary, nextString, *obj.ScalarComparison.Op == "!~")
					if err != nil {
						return nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op)
					}
					return eval, obj.Pos, nil
				}
				return nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op)
			case *IntEvaluator:
				nextInt, ok := next.(*IntEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Int)
				}

				switch *obj.ScalarComparison.Op {
				case "<":
					return LesserThan(unary, nextInt), obj.Pos, nil
				case "<=":
					return LesserOrEqualThan(unary, nextInt), obj.Pos, nil
				case ">":
					return GreaterThan(unary, nextInt), obj.Pos, nil
				case ">=":
					return GreaterOrEqualThan(unary, nextInt), obj.Pos, nil
				case "!=":
					return IntNotEquals(unary, nextInt), obj.Pos, nil
				case "==":
					return IntEquals(unary, nextInt), obj.Pos, nil
				}
				return nil, pos, NewOpError(obj.Pos, *obj.ScalarComparison.Op)
			}

		} else {
			return unary, pos, nil
		}

	case *ast.ArrayComparison:
		if len(obj.Array.Numbers) != 0 {
			ints := obj.Array.Numbers
			sort.Ints(ints)
			return &IntArrayEvaluator{Values: ints}, obj.Pos, nil
		} else if len(obj.Array.Strings) != 0 {
			strings := obj.Array.Strings
			sort.Strings(strings)
			return &StringArrayEvaluator{Values: strings}, obj.Pos, nil
		}

	case *ast.ScalarComparison:
		return nodeToEvaluator(obj.Next)

	case *ast.Unary:
		if obj.Op != nil {
			unary, pos, err := nodeToEvaluator(obj.Unary)
			if err != nil {
				return nil, pos, err
			}

			switch *obj.Op {
			case "!":
				unaryBool, ok := unary.(*BoolEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Bool)
				}

				return Not(unaryBool), obj.Pos, nil
			case "-":
				unaryInt, ok := unary.(*IntEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Int)
				}

				return Minus(unaryInt), pos, nil
			case "^":
				unaryInt, ok := unary.(*IntEvaluator)
				if !ok {
					return nil, pos, NewTypeError(pos, reflect.Int)
				}

				return IntNot(unaryInt), pos, nil
			}
			return nil, pos, NewOpError(obj.Pos, *obj.Op)
		}

		return nodeToEvaluator(obj.Primary)
	case *ast.Primary:
		switch {
		case obj.Ident != nil:
			if accessor, ok := constants[*obj.Ident]; ok {
				return accessor, obj.Pos, nil
			}

			accessor, err := GetAccessor(*obj.Ident)
			if err != nil {
				return nil, obj.Pos, err
			}
			return accessor, obj.Pos, nil
		case obj.Number != nil:
			return &IntEvaluator{
				Value: *obj.Number,
			}, obj.Pos, nil
		case obj.String != nil:
			return &StringEvaluator{
				Value: *obj.String,
			}, obj.Pos, nil
		case obj.SubExpression != nil:
			return nodeToEvaluator(obj.SubExpression)
		default:
			return nil, obj.Pos, NewError(obj.Pos, fmt.Sprintf("unknown primary '%s'", reflect.TypeOf(obj)))
		}
	}

	return nil, lexer.Position{}, NewError(lexer.Position{}, fmt.Sprintf("unknown entity '%s'", reflect.TypeOf(obj)))
}

func RuleToEvaluator(rule *ast.Rule, debug bool) (Evaluator, error) {
	eval, _, err := nodeToEvaluator(rule.BooleanExpression)
	if err != nil {
		return nil, err
	}

	evalBool, ok := eval.(*BoolEvaluator)
	if !ok {
		return nil, NewTypeError(rule.Pos, reflect.Bool)
	}

	if evalBool.Eval == nil {
		return Evaluator(func(ctx *Context) bool {
			return evalBool.Value
		}), nil
	}

	if debug {
		return Evaluator(evalBool.DebugEval), nil
	}
	return Evaluator(evalBool.Eval), nil
}
