// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"
	"net"
	"net/netip"
	"reflect"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
	"github.com/google/cel-go/interpreter"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// NowVar is the CEL variable holding the instant an evaluation started, in
// nanoseconds.
//
// SECL reads `field < 10m` as "less than ten minutes ago", and caches the instant
// on the context so that every comparison in one evaluation sees the same one. A
// CEL function cannot reach that context, so the translation names the instant
// instead and the activation supplies it.
const NowVar = "secl_now"

// seclObject is a position in the SECL field namespace: an event root, or one
// element of an iterated field.
//
// It carries no path. Its type names one, and there is one type per path, so
// which field a member select denotes was settled when the rule was planned —
// see bindMember. Nothing is read until a leaf is reached, so an expression only
// resolves the fields it actually mentions.
type seclObject struct {
	ctx *eval.Context
	typ *types.Type

	// elem is the element this position belongs to, held as the model value the
	// cursor yielded rather than as an index into it. Two fields of one element
	// therefore read from one pointer, which is what makes correlating them free
	// instead of quadratic.
	elem any
}

// celObjectType is what the checker sees; the runtime value carries the shape it
// was reached through so that member lookups resolve against the right type.
func (o *seclObject) Type() ref.Type { return o.typ }

// Value returns the object itself, because the field qualifier unwraps a ref.Val
// with Value() before handing it to the provider's getter.
func (o *seclObject) Value() any { return o }

// ConvertToNative implements ref.Val.
func (o *seclObject) ConvertToNative(reflect.Type) (any, error) {
	return nil, fmt.Errorf("%w: %s cannot be converted to a native value", errUnsupportedValue, o.typ)
}

// ConvertToType implements ref.Val.
func (o *seclObject) ConvertToType(t ref.Type) ref.Val {
	if t == types.TypeType {
		return o.typ
	}
	return types.NewErr("type conversion error from '%s' to '%s'", o.typ, t)
}

// Equal implements ref.Val. Two positions are equal when they name the same
// thing; SECL has no notion of comparing event objects.
func (o *seclObject) Equal(other ref.Val) ref.Val {
	rhs, ok := other.(*seclObject)
	if !ok {
		return types.MaybeNoSuchOverloadErr(other)
	}
	return types.Bool(o.typ == rhs.typ && o.elem == rhs.elem)
}

// stringsToVal and its siblings convert what a reader read into a CEL value.
//
// The generated readers call the one their field needs, so the shape is decided
// when the readers are generated rather than by a type switch on every read.
func stringsToVal(values []string) ref.Val {
	return types.NewStringList(types.DefaultTypeAdapter, values)
}

func intsToVal(values []int) ref.Val {
	return types.NewDynamicList(types.DefaultTypeAdapter, values)
}

func boolsToVal(values []bool) ref.Val {
	return types.NewDynamicList(types.DefaultTypeAdapter, values)
}

func cidrsToVal(values []net.IPNet) ref.Val {
	vals := make([]ref.Val, 0, len(values))
	for _, ipnet := range values {
		vals = append(vals, cidrToVal(ipnet))
	}
	return types.NewRefValList(types.DefaultTypeAdapter, vals)
}

// cidrToVal converts an IP or CIDR field. It needs care: the network extension's
// adapter only understands the netip types, not net.IPNet.
func cidrToVal(ipnet net.IPNet) ref.Val {
	addr, ok := netip.AddrFromSlice(ipnet.IP)
	if !ok {
		// An unset field, which SECL compares as matching nothing. The zero prefix
		// contains no address, so it reports the same.
		return ext.CIDR{}
	}
	ones, _ := ipnet.Mask.Size()

	prefix := netip.PrefixFrom(addr.Unmap(), ones)
	if !prefix.IsValid() {
		// An IPv4 address held in 16 bytes carries a 128 bit mask, which does not
		// fit the unmapped address.
		prefix = netip.PrefixFrom(addr.Unmap(), addr.Unmap().BitLen())
	}
	return ext.CIDR{Prefix: prefix}
}

// activation resolves the names a translated expression refers to against an
// event, which the SECL context holds.
//
// An activation may be kept for as long as its context, across the events the
// context is reused for. The values it hands out hold the context rather than
// the event, and every read goes through ctx.Event, so none of them go stale
// when the context is reset onto the next event.
type activation struct {
	ctx *eval.Context

	// roots caches the value standing for each top level name. Each name is
	// resolved once per evaluation, so the cache earns its keep across
	// evaluations rather than within one: a rule evaluated for every event
	// allocates its root object once rather than every time. The names are few
	// enough that a scan beats a map, and holding them inline means the cache
	// itself never allocates.
	roots  [cachedRoots]cachedRoot
	cached int
}

// cachedRoots is how many top level names an activation remembers. Expressions
// mention one or two; any beyond that simply resolve as they did before.
const cachedRoots = 4

type cachedRoot struct {
	name  string
	value ref.Val
}

// NewActivation returns the CEL activation for a SECL evaluation context, so that
// a program built by Program can be evaluated against the event it holds.
func NewActivation(ctx *eval.Context) interpreter.Activation {
	return &activation{ctx: ctx}
}

// Parent implements interpreter.Activation; SECL evaluations have no enclosing
// scope.
func (a *activation) Parent() interpreter.Activation { return nil }

// ResolveName implements interpreter.Activation.
func (a *activation) ResolveName(name string) (any, bool) {
	if name == NowVar {
		return a.ctx.Now().UnixNano(), true
	}

	for i := range a.roots[:a.cached] {
		if a.roots[i].name == name {
			return a.roots[i].value, true
		}
	}

	rootType, ok := modelRoots[name]
	if !ok {
		// vars is declared for SECL's ${…} variables but not populated yet, so an
		// expression using one fails rather than silently reading nothing.
		return nil, false
	}

	value := bindMember(name, rootType)(a.ctx, nil)
	if a.cached < len(a.roots) {
		a.roots[a.cached] = cachedRoot{name: name, value: value}
		a.cached++
	}
	return value, true
}

// Program translates a SECL expression and returns an evaluable CEL program.
//
// Pair it with an environment from NewModelEnv and evaluate it against
// NewActivation, whose context holds the event.
func Program(env *cel.Env, expr string, fieldTypes FieldTypes) (cel.Program, error) {
	checked, err := CompileWithTypes(env, expr, fieldTypes)
	if err != nil {
		return nil, err
	}

	program, err := env.Program(checked, planningOptions()...)
	if err != nil {
		return nil, fmt.Errorf("planning %q: %w", expr, err)
	}
	return program, nil
}

// planningOptions are what a rule is planned with, and are all about doing at
// planning time what would otherwise be repeated for every event.
//
// A rule is planned once and evaluated for the lifetime of the agent, so the
// asymmetry is extreme: anything derived from a literal in the rule text is
// worth deriving once. SECL does the same when it compiles a rule, which is why
// its pattern and regexp comparisons cost nothing at match time.
func planningOptions() []cel.ProgramOption {
	return []cel.ProgramOption{
		// Fold constant subexpressions, and compile the regexp of a matches()
		// against a literal pattern. Without it a `r"…"` rule recompiles its
		// regexp on every event.
		cel.EvalOptions(cel.OptOptimize),
		cel.OptimizeRegex(globOptimization),
	}
}

// globOptimization compiles the pattern of a glob against a literal, which is
// the form every translated `=~` and `~"…"` takes.
//
// eval.PatternMatches splits its pattern into segments on each call; the
// matcher SECL compiles a rule into holds the split form instead. This is the
// same trick cel-go applies to matches(), through the hook it exposes for it.
var globOptimization = &interpreter.RegexOptimization{
	Function:   GlobFunc,
	RegexIndex: globPatternArg,
	Factory: func(call interpreter.InterpretableCall, pattern string) (interpreter.InterpretableCall, error) {
		var matcher eval.PatternStringMatcher
		if err := matcher.Compile(pattern, false); err != nil {
			return nil, err
		}

		return interpreter.NewCall(call.ID(), call.Function(), call.OverloadID(), call.Args(),
			func(values ...ref.Val) ref.Val {
				if len(values) != 2 {
					return types.NoSuchOverloadErr()
				}
				subject, ok := values[0].(types.String)
				if !ok {
					return types.MaybeNoSuchOverloadErr(values[0])
				}
				return types.Bool(matcher.Matches(string(subject)))
			}), nil
	},
}
