// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package gosymname

// Interpretation is a single valid structural reading of a Go symbol name.
// A symbol may have multiple interpretations when the parse is ambiguous (e.g.
// value-receiver method vs function with inlined callee).
type Interpretation struct {
	// OuterReceiver is the receiver type name (without '*' or parentheses).
	OuterReceiver string
	// OuterReceiverKind is ReceiverPointer, ReceiverValue, or ReceiverNone.
	OuterReceiverKind ReceiverKind
	// OuterReceiverGenerics holds the generic type parameters of the receiver,
	// if any.
	OuterReceiverGenerics *GenericParams
	// OuterFunction is the function or method name.
	OuterFunction string
	// OuterFuncGenerics holds the generic type parameters of the function, if
	// any.
	OuterFuncGenerics *GenericParams
	// InlinedCalls is the chain of inlined function/method calls, in order.
	InlinedCalls []InlinedCall
	// ClosureSuffix is the closure/nesting/range/wrapper chain, e.g.
	// "func1.2", "gowrap1", "func1.func4.deferwrap".
	ClosureSuffix string
	// ClosureDepth is the nesting depth. 1 for func1, 3 for func1.2.3.
	ClosureDepth int
	// Wrapper is the wrapper kind (go, defer, method expression), if any.
	Wrapper WrapperKind
	// ABISuffix is "abi0", "abiinternal", or "" (from nm output).
	ABISuffix string
}

// IsMethod returns true if the outer function has a receiver.
func (i *Interpretation) IsMethod() bool {
	return i.OuterReceiverKind != ReceiverNone
}

// IsGeneric returns true if any part of the interpretation has generic type
// parameters.
func (i *Interpretation) IsGeneric() bool {
	if i.OuterReceiverGenerics != nil || i.OuterFuncGenerics != nil {
		return true
	}
	for idx := range i.InlinedCalls {
		if i.InlinedCalls[idx].ReceiverGenerics != nil || i.InlinedCalls[idx].FuncGenerics != nil {
			return true
		}
	}
	return false
}

// HasInlinedCalls returns true if there are inlined function calls.
func (i *Interpretation) HasInlinedCalls() bool {
	return len(i.InlinedCalls) > 0
}

// BaseName returns just the outer function name.
func (i *Interpretation) BaseName() string {
	return i.OuterFunction
}

// QualifiedName reconstructs the qualified name without the package prefix,
// e.g. "(*T).Method" or "Function".
func (i *Interpretation) QualifiedName() string {
	var b []byte
	switch i.OuterReceiverKind {
	case ReceiverPointer:
		b = append(b, "(*"...)
		b = append(b, i.OuterReceiver...)
		if i.OuterReceiverGenerics != nil {
			b = append(b, '[')
			b = append(b, i.OuterReceiverGenerics.Raw...)
			b = append(b, ']')
		}
		b = append(b, ")."...)
	case ReceiverValue:
		b = append(b, i.OuterReceiver...)
		if i.OuterReceiverGenerics != nil {
			b = append(b, '[')
			b = append(b, i.OuterReceiverGenerics.Raw...)
			b = append(b, ']')
		}
		b = append(b, '.')
	}
	b = append(b, i.OuterFunction...)
	if i.OuterFuncGenerics != nil {
		b = append(b, '[')
		b = append(b, i.OuterFuncGenerics.Raw...)
		b = append(b, ']')
	}
	return string(b)
}

// InlinedCall represents a single inlined function or method call within a
// symbol's call chain. The receiver, if present, is always a pointer receiver
// — the symbol format does not distinguish value receivers from function names
// at inlined positions.
type InlinedCall struct {
	// Receiver is the pointer-receiver type name, if this is a method call.
	Receiver string
	// HasReceiver is true if this is a pointer-receiver method call.
	HasReceiver bool
	// ReceiverGenerics holds the generic type parameters of the receiver.
	ReceiverGenerics *GenericParams
	// Function is the function or method name.
	Function string
	// FuncGenerics holds the generic type parameters of the function.
	FuncGenerics *GenericParams
	// Raw is the original text of this segment, e.g.
	// "(*Builder).AddASN1ObjectIdentifier".
	Raw string
}

// IsMethod returns true if this inlined call has a receiver.
func (ic *InlinedCall) IsMethod() bool {
	return ic.HasReceiver
}

// QualifiedFunction returns the qualified function name, e.g.
// "(*T).Method" or just "Function" for simple cases.
func (ic *InlinedCall) QualifiedFunction() string {
	if !ic.HasReceiver {
		if ic.FuncGenerics != nil {
			return ic.Function + "[" + ic.FuncGenerics.Raw + "]"
		}
		return ic.Function
	}
	var b []byte
	b = append(b, "(*"...)
	b = append(b, ic.Receiver...)
	if ic.ReceiverGenerics != nil {
		b = append(b, '[')
		b = append(b, ic.ReceiverGenerics.Raw...)
		b = append(b, ']')
	}
	b = append(b, ")."...)
	b = append(b, ic.Function...)
	if ic.FuncGenerics != nil {
		b = append(b, '[')
		b = append(b, ic.FuncGenerics.Raw...)
		b = append(b, ']')
	}
	return string(b)
}
