// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"
	"reflect"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/common/types/traits"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// SECL's maxRegisterIteration cap is deliberately not applied here.
//
// It lives inside `if len(state.registers) > 0` in compiler/eval/rule.go, so it
// bounds only a rule written with an explicit iterator variable; an implicit
// comparison against an array field goes through a plain array contains and is
// unbounded. The translation turns both forms into the same exists(), so the list
// cannot tell them apart, and capping would break the common form to fix the rare
// one. Phase 2 measures how far apart the two engines actually are on a deep
// ancestry.

// iteratedList presents an iterated SECL field as a CEL list.
//
// Nothing about the elements is known until they are asked for, including how
// many there are: a fold opens a cursor and stops when its predicate is
// satisfied. That is what a list of values cannot do, and it is why an iterated
// field is not materialised into one.
type iteratedList struct {
	ctx  *eval.Context
	node *celNode
	// typ is list(element); elem is the element type, which the members are
	// selected against.
	typ  *types.Type
	elem *types.Type
}

// newIteratedList returns the list standing for an iterated field. typ is the
// list type the field tree describes for it.
func newIteratedList(ctx *eval.Context, node *celNode, typ *types.Type) ref.Val {
	elem, ok := objectListElem(typ)
	if !ok {
		return types.NewErr("iterated field of type '%s' is not a list of objects", typ)
	}
	return &iteratedList{ctx: ctx, node: node, typ: typ, elem: elem}
}

// element returns the position of one element of the list.
func (l *iteratedList) element(elem any) ref.Val {
	return &seclObject{ctx: l.ctx, typ: l.elem, node: l.node, elem: elem}
}

// Type implements ref.Val.
func (l *iteratedList) Type() ref.Type { return l.typ }

// Value implements ref.Val.
func (l *iteratedList) Value() any { return l }

// ConvertToNative implements ref.Val.
func (l *iteratedList) ConvertToNative(reflect.Type) (any, error) {
	return nil, fmt.Errorf("%w: %s cannot be converted to a native value", errUnsupportedValue, l.typ)
}

// ConvertToType implements ref.Val.
func (l *iteratedList) ConvertToType(t ref.Type) ref.Val {
	if t == types.TypeType {
		return l.typ
	}
	return types.NewErr("type conversion error from '%s' to '%s'", l.typ, t)
}

// Equal implements ref.Val. Two lists are equal when they range over the same
// field of the same event; SECL has no notion of comparing iterated fields.
func (l *iteratedList) Equal(other ref.Val) ref.Val {
	rhs, ok := other.(*iteratedList)
	if !ok {
		return types.MaybeNoSuchOverloadErr(other)
	}
	return types.Bool(l.node == rhs.node && l.ctx == rhs.ctx)
}

// Add implements traits.Adder. Concatenating an event's iterated field with
// something else is not a SECL operation, and allowing it would mean
// materialising the elements.
func (l *iteratedList) Add(other ref.Val) ref.Val {
	return types.NewErr("%s cannot be concatenated with '%s'", l.typ, other.Type())
}

// Size implements traits.Sizer. It is the one operation that has to walk the
// whole field, which is why `x.length` is translated to size() rather than
// being answered on the way past.
func (l *iteratedList) Size() ref.Val {
	var size int
	cursor := l.node.cursor(l.ctx)
	for element := cursor.next(); element != nil; element = cursor.next() {
		size++
	}
	return types.Int(size)
}

// Get implements traits.Indexer.
//
// An index past the last element is an error, as it is for any other CEL list.
// SECL answers a subscript on an array field with the field's default instead,
// which is a difference the translation would have to decide — uniformly across
// list kinds, since a plain array field is an ordinary CEL list here.
func (l *iteratedList) Get(index ref.Val) ref.Val {
	i, err := types.IndexOrError(index)
	if err != nil {
		return types.ValOrErr(index, "%v", err)
	}

	cursor := l.node.cursor(l.ctx)
	for pos := 0; i >= 0; pos++ {
		element := cursor.next()
		if element == nil {
			break
		}
		if pos == i {
			return l.element(element)
		}
	}
	return types.NewErr("index '%d' out of range in %s", i, l.typ)
}

// Contains implements traits.Container.
func (l *iteratedList) Contains(value ref.Val) ref.Val {
	cursor := l.node.cursor(l.ctx)
	for element := cursor.next(); element != nil; element = cursor.next() {
		if l.element(element).Equal(value) == types.True {
			return types.True
		}
	}
	return types.False
}

// Iterator implements traits.Iterable.
func (l *iteratedList) Iterator() traits.Iterator {
	return &iteratedListIterator{list: l, cursor: l.node.cursor(l.ctx)}
}

// iteratedListIterator walks an iterated field one element at a time.
//
// It reads one element ahead, because a CEL iterator has to answer HasNext
// before Next is called while a cursor only answers by yielding.
type iteratedListIterator struct {
	list   *iteratedList
	cursor celCursor

	peeked any
	done   bool
}

// HasNext implements traits.Iterator.
func (i *iteratedListIterator) HasNext() ref.Val {
	if i.done {
		return types.False
	}
	if i.peeked == nil {
		if i.peeked = i.cursor.next(); i.peeked == nil {
			i.done = true
			return types.False
		}
	}
	return types.True
}

// Next implements traits.Iterator.
func (i *iteratedListIterator) Next() ref.Val {
	if i.HasNext() != types.True {
		return types.NewErr("no more elements in %s", i.list.typ)
	}

	element := i.peeked
	i.peeked = nil
	return i.list.element(element)
}

// An iterator is not a value anyone can compare or convert; the methods exist
// because the interpreter passes it around as one.

// Type implements ref.Val.
func (i *iteratedListIterator) Type() ref.Type { return types.IteratorType }

// Value implements ref.Val.
func (i *iteratedListIterator) Value() any { return nil }

// ConvertToNative implements ref.Val.
func (i *iteratedListIterator) ConvertToNative(reflect.Type) (any, error) {
	return nil, fmt.Errorf("%w: an iterator cannot be converted to a native value", errUnsupportedValue)
}

// ConvertToType implements ref.Val.
func (i *iteratedListIterator) ConvertToType(t ref.Type) ref.Val {
	return types.NewErr("type conversion error from '%s' to '%s'", types.IteratorType, t)
}

// Equal implements ref.Val.
func (i *iteratedListIterator) Equal(other ref.Val) ref.Val {
	return types.MaybeNoSuchOverloadErr(other)
}
