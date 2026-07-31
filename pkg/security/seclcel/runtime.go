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
	"sync"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
	"github.com/google/cel-go/interpreter"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
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
// element of an iterated field. Selecting a member of it extends the path, and
// reading a leaf resolves the joined path through the generated accessors.
//
// Nothing is read until a leaf is reached, so an expression only resolves the
// fields it actually mentions.
type seclObject struct {
	ctx  *eval.Context
	typ  *types.Type
	path string

	// register names the iterator variable a bound element reads through, empty
	// for an event root. SECL resolves an element by index through the register
	// rather than by holding the element itself.
	register string
	index    int
}

// celObjectType is what the checker sees; the runtime value carries the shape it
// was reached through so that member lookups resolve against the right type.
func (o *seclObject) Type() ref.Type { return o.typ }

// Value returns the object itself, because the field qualifier unwraps a ref.Val
// with Value() before handing it to the provider's getter.
func (o *seclObject) Value() any { return o }

// ConvertToNative implements ref.Val.
func (o *seclObject) ConvertToNative(reflect.Type) (any, error) {
	return nil, fmt.Errorf("%w: %s cannot be converted to a native value", errUnsupportedValue, o.path)
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
	return types.Bool(o.path == rhs.path && o.register == rhs.register && o.index == rhs.index)
}

// selectMember returns the position of a member of this object.
func (o *seclObject) selectMember(name string, memberType *types.Type) *seclObject {
	return &seclObject{
		ctx:      o.ctx,
		typ:      memberType,
		path:     o.path + "." + name,
		register: o.register,
		index:    o.index,
	}
}

// bind returns this object as the element of an iterated field at an index,
// reached through a register.
func (o *seclObject) bind(register string, index int) *seclObject {
	bound := *o
	bound.register = register
	bound.index = index
	return &bound
}

// resolve reads a leaf field through the generated accessors and converts the
// result for CEL. expected is the type the field tree describes, which is what
// tells a single element apart from a list of them.
func (o *seclObject) resolve(field string, expected *types.Type) ref.Val {
	evaluator, err := fieldEvaluator(field, o.register)
	if err != nil {
		return types.WrapErr(err)
	}

	if o.register != "" {
		// The accessors read the element at ctx.Registers[register]. The cache
		// entry carries the position it holds and the iterator checks it, so it
		// does not need clearing here — leaving it lets two fields read at the
		// same index share one walk of the chain.
		o.ctx.Registers[o.register] = o.index
	}

	value := evaluator.Eval(o.ctx)

	// Read through a register, a scalar field arrives as a one element slice
	// holding the element at that index, while a field that is a list within one
	// element arrives as its own list. The expected type is what distinguishes
	// them.
	if o.register != "" && expected != nil && expected.Kind() != types.ListKind {
		value = single(value)
	}

	return nativeToVal(value)
}

// single unwraps the one element slice a scalar field yields when read through a
// register. An empty slice means the element was not resolved — a `check:` guard
// on the iterated field can refuse it — and the zero value then compares as no
// match, which is what SECL reports for an empty array.
func single(value any) any {
	switch v := value.(type) {
	case []string:
		if len(v) == 0 {
			return ""
		}
		return v[0]
	case []int:
		if len(v) == 0 {
			return 0
		}
		return v[0]
	case []bool:
		if len(v) == 0 {
			return false
		}
		return v[0]
	case []net.IPNet:
		if len(v) == 0 {
			return net.IPNet{}
		}
		return v[0]
	}
	return value
}

// evaluatorCache memoises Model.GetEvaluator, whose result is a closure over the
// field rather than over any event, so it is reusable across evaluations.
var evaluatorCache sync.Map // string -> eval.Evaluator

func fieldEvaluator(field, register string) (eval.Evaluator, error) {
	key := field + "\x00" + register
	if cached, ok := evaluatorCache.Load(key); ok {
		return cached.(eval.Evaluator), nil
	}

	var m model.Model
	evaluator, err := m.GetEvaluator(field, register, 0)
	if err != nil {
		return nil, fmt.Errorf("resolving %q: %w", field, err)
	}

	evaluatorCache.Store(key, evaluator)
	return evaluator, nil
}

// nativeToVal converts what a SECL evaluator returns into a CEL value. The IP and
// CIDR cases need care: the network extension's adapter only understands the
// netip types, not net.IPNet.
func nativeToVal(value any) ref.Val {
	switch v := value.(type) {
	case net.IPNet:
		return ipNetToVal(v)
	case []net.IPNet:
		vals := make([]ref.Val, 0, len(v))
		for _, ipnet := range v {
			vals = append(vals, ipNetToVal(ipnet))
		}
		return types.NewRefValList(types.DefaultTypeAdapter, vals)
	}
	return types.DefaultTypeAdapter.NativeToValue(value)
}

func ipNetToVal(ipnet net.IPNet) ref.Val {
	addr, ok := netip.AddrFromSlice(ipnet.IP)
	if !ok {
		return types.NewErr("invalid IP address %q", ipnet.IP)
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
type activation struct {
	ctx *eval.Context
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

	root, ok := modelRoots[name]
	if !ok {
		// vars is declared for SECL's ${…} variables but not populated yet, so an
		// expression using one fails rather than silently reading nothing.
		return nil, false
	}

	object := &seclObject{ctx: a.ctx, typ: root, path: name}
	if _, isObjectList := objectListElem(root); isObjectList {
		return newIteratedList(object), true
	}
	return object, true
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

	program, err := env.Program(checked)
	if err != nil {
		return nil, fmt.Errorf("planning %q: %w", expr, err)
	}
	return program, nil
}
