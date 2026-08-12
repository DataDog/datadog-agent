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

// seclPosition is where a field is read from: the event root, or one element of
// an iterated field.
//
// It is not navigated. A read carries the index of the field it wants — see
// optimize.go — so a position holds only what a reader needs, and there is one
// per evaluation for the root and one per element for an iteration.
type seclPosition struct {
	ctx *eval.Context
	typ *types.Type

	// elem is the element this position belongs to, held as the model value the
	// cursor yielded rather than as an index into it. Two fields of one element
	// therefore read from one pointer, which is what makes correlating them free
	// instead of quadratic.
	elem any
}

// Type is the type the checker gave the expression the position came from, which
// makes an error message name the shape a read was attempted on.
func (o *seclPosition) Type() ref.Type { return o.typ }

// Value implements ref.Val.
func (o *seclPosition) Value() any { return o }

// ConvertToNative implements ref.Val.
func (o *seclPosition) ConvertToNative(reflect.Type) (any, error) {
	return nil, fmt.Errorf("%w: %s cannot be converted to a native value", errUnsupportedValue, o.typ)
}

// ConvertToType implements ref.Val.
func (o *seclPosition) ConvertToType(t ref.Type) ref.Val {
	if t == types.TypeType {
		return o.typ
	}
	return types.NewErr("type conversion error from '%s' to '%s'", o.typ, t)
}

// Equal implements ref.Val. Two positions are equal when they name the same
// thing; SECL has no notion of comparing event objects.
func (o *seclPosition) Equal(other ref.Val) ref.Val {
	rhs, ok := other.(*seclPosition)
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
// There is almost nothing to resolve: the fields hang under one name, and what a
// planned rule does with it is hand it to a read that already knows which field it
// wants.
//
// An activation may be kept for as long as its context, across the events the
// context is reused for. The root it hands out holds the context rather than the
// event, and every read goes through ctx.Event, so it does not go stale when the
// context is reset onto the next event.
type activation struct {
	ctx  *eval.Context
	root ref.Val
}

// NewActivation returns the CEL activation for a SECL evaluation context, so that
// a program built by Program can be evaluated against the event it holds.
func NewActivation(ctx *eval.Context) interpreter.Activation {
	return &activation{ctx: ctx, root: &seclPosition{ctx: ctx, typ: modelRootType}}
}

// Parent implements interpreter.Activation; SECL evaluations have no enclosing
// scope.
func (a *activation) Parent() interpreter.Activation { return nil }

// ResolveName implements interpreter.Activation.
func (a *activation) ResolveName(name string) (any, bool) {
	switch name {
	case EventRoot:
		return a.root, true
	case NowVar:
		return a.ctx.Now().UnixNano(), true
	}

	// vars is declared for SECL's ${…} variables but not populated yet, so an
	// expression using one fails rather than silently reading nothing.
	return nil, false
}

// Program translates a SECL expression and returns an evaluable CEL program.
//
// Pair it with an environment from NewModelEnv and evaluate it against
// NewActivation, whose context holds the event.
func Program(env *cel.Env, expr string, fieldTypes FieldTypes) (cel.Program, error) {
	program, _, err := ProgramFields(env, expr, fieldTypes)
	return program, err
}

// ProgramFields is Program, and also returns the SECL fields the expression reads
// — which the optimization pass knows because resolving them is what it does.
//
// It is the counterpart of SECL's RuleEvaluator.GetFields: what a rule reads is
// what decides which bucket it belongs to and which approvers it can support. An
// iterated node counts as a field of its own, since reading it is a read.
func ProgramFields(env *cel.Env, expr string, fieldTypes FieldTypes) (cel.Program, []string, error) {
	checked, err := CompileWithTypes(env, expr, fieldTypes)
	if err != nil {
		return nil, nil, err
	}

	optimized, fields, err := Optimize(env, checked)
	if err != nil {
		return nil, nil, fmt.Errorf("optimizing %q: %w", expr, err)
	}

	program, err := env.Program(optimized, planningOptions()...)
	if err != nil {
		return nil, nil, fmt.Errorf("planning %q: %w", expr, err)
	}
	return program, fields, nil
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
		// Bind each field read to its reader, which is the same trick for the index
		// the optimization pass emitted.
		readOptimization(),
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
