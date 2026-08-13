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

// Activation binds a SECL evaluation context to CEL: it resolves the names a
// translated expression refers to against the event the context holds, and
// carries the execution frame a Rule is evaluated on.
//
// There is almost nothing to resolve: the fields hang under one name, and what a
// planned rule does with it is hand it to a read that already knows which field it
// wants.
//
// An activation may be kept for as long as its context, across the events the
// context is reused for. The root it hands out holds the context rather than the
// event, and every read goes through ctx.Event, so it does not go stale when the
// context is reset onto the next event.
type Activation struct {
	ctx  *eval.Context
	root ref.Val

	// frame is what Rule.Eval hands the planned interpretable, held for as long
	// as this activation rather than taken from cel-go's pool on every
	// evaluation, which is most of what evaluating a rule this way saves.
	//
	// ExecutionFrame's own documentation says a frame must not be stored, because
	// its lifecycle belongs to the evaluation. This one holds nothing that belongs
	// to an evaluation: parent and the shared evalContext both stay nil, since
	// only SetContext — which we never call, having no interrupt or cost tracking
	// to drive — sets them, and the activation it wraps is ours rather than one of
	// the pooled kinds Close returns. So Close would do nothing here but hand the
	// frame back, and not calling it costs a frame per context and nothing per
	// event. A comprehension still pushes and pops its child frames through the
	// pool as usual.
	//
	// A frame belongs to one context, so it is used by one goroutine at a time for
	// the same reason a context is.
	frame *interpreter.ExecutionFrame
}

// NewActivation returns the CEL activation for a SECL evaluation context, so that
// a rule built by NewRule can be evaluated against the event it holds.
func NewActivation(ctx *eval.Context) *Activation {
	a := &Activation{ctx: ctx, root: &seclPosition{ctx: ctx, typ: modelRootType}}

	// NewExecutionFrame only rejects an input that is neither an Activation nor a
	// map[string]any, and a is an Activation.
	a.frame, _ = interpreter.NewExecutionFrame(a)
	return a
}

// Parent implements interpreter.Activation; SECL evaluations have no enclosing
// scope.
func (a *Activation) Parent() interpreter.Activation { return nil }

// ResolveName implements interpreter.Activation.
func (a *Activation) ResolveName(name string) (any, bool) {
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

// globOptimization compiles the pattern of a glob against a literal, which is the form
// every translated `=~` and `~"…"` takes, and pathGlobOptimization does the same for the
// fields whose patterns are globs — see celGlobFields.
//
// eval.PatternMatches splits its pattern into segments on each call; the matcher SECL
// compiles a rule into holds the split form instead. This is the same trick cel-go
// applies to matches(), through the hook it exposes for it.
var (
	globOptimization = matcherOptimization(GlobFunc, func(pattern string) (stringMatcher, error) {
		var matcher eval.PatternStringMatcher
		return &matcher, matcher.Compile(pattern, false)
	})

	pathGlobOptimization = matcherOptimization(PathGlobFunc, func(pattern string) (stringMatcher, error) {
		var matcher eval.GlobStringMatcher
		// The case insensitive and separator normalising forms SECL uses for some
		// fields and platforms are not reproduced yet — see the package doc.
		return &matcher, matcher.Compile(pattern, false, false)
	})
)

// stringMatcher is what both of SECL's compiled pattern forms offer.
type stringMatcher interface {
	Matches(value string) bool
}

func matcherOptimization(function string, compile func(pattern string) (stringMatcher, error)) *interpreter.RegexOptimization {
	return &interpreter.RegexOptimization{
		Function:   function,
		RegexIndex: globPatternArg,
		Factory: func(call interpreter.InterpretableCall, pattern string) (interpreter.InterpretableCall, error) {
			matcher, err := compile(pattern)
			if err != nil {
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
}
