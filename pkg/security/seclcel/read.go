// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"
	"sort"

	"github.com/google/cel-go/cel"
	"github.com/google/cel-go/common/functions"
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
	"github.com/google/cel-go/ext"
	"github.com/google/cel-go/interpreter"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
)

// A field is read by index: `evt.process.file.path` is planned as
// `secl.readString(evt, 4711)`, where 4711 is the field's place in the generated
// layout. Which field a name denotes is settled once, when the rule is optimized —
// see optimize.go — and reading it is then a slice index and one call.
//
// The functions differ only in the type they are declared to return, because that
// is what keeps a rule type-checked after the rewrite: a comparison against an
// integer field must still be a comparison against an integer. They share their
// implementations, since the readers already return the right CEL value.
const (
	// ReadStringFunc and its siblings read a leaf field.
	ReadStringFunc  = "secl.readString"
	ReadStringsFunc = "secl.readStrings"
	ReadIntFunc     = "secl.readInt"
	ReadIntsFunc    = "secl.readInts"
	ReadBoolFunc    = "secl.readBool"
	ReadBoolsFunc   = "secl.readBools"
	ReadCIDRFunc    = "secl.readCIDR"
	ReadCIDRsFunc   = "secl.readCIDRs"
)

// leafReads and elementLists are what each read function returns: the first is
// fixed, since the shapes a leaf can hold are, and the second follows the model,
// since an element read returns a list of its own element type.
//
// They declare the functions and they are what readOptimization recognises a read
// by, so the two cannot drift.
var (
	leafReads = map[string]*cel.Type{
		ReadStringFunc:  cel.StringType,
		ReadStringsFunc: cel.ListType(cel.StringType),
		ReadIntFunc:     cel.IntType,
		ReadIntsFunc:    cel.ListType(cel.IntType),
		ReadBoolFunc:    cel.BoolType,
		ReadBoolsFunc:   cel.ListType(cel.BoolType),
		ReadCIDRFunc:    ext.CIDRType,
		ReadCIDRsFunc:   cel.ListType(ext.CIDRType),
	}

	elementLists = elementListTypes()
)

func elementListTypes() map[string]*cel.Type {
	lists := make(map[string]*cel.Type, len(celElementReads))
	for typeName, read := range celElementReads {
		lists[read] = cel.ListType(types.NewObjectType(typeName))
	}
	return lists
}

// readBindings declares the read functions, with the position typed dynamically
// so that one declaration serves the event root and every element type.
//
// The element reads are declared from the generated table rather than fixed here,
// because each returns a list of its own element type: that type is what a select
// on an iteration variable is checked against, and what tells the optimization pass
// which iterated field the variable stands for.
func readBindings() []cel.EnvOption {
	opts := make([]cel.EnvOption, 0, len(leafReads)+len(elementLists))

	for _, name := range sortedReadFuncs(leafReads) {
		opts = append(opts, readFunction(name, leafReads[name], readLeaf))
	}

	for _, name := range sortedReadFuncs(elementLists) {
		// The list type is bound here rather than looked up per read, which is the
		// same trick as the index: everything that depends on the name and not on
		// the event is done once.
		opts = append(opts, readFunction(name, elementLists[name], readElementsAs(elementLists[name])))
	}

	return opts
}

func readFunction(name string, result *cel.Type, binding functions.BinaryOp) cel.EnvOption {
	return cel.Function(name,
		cel.Overload(name, []*cel.Type{cel.DynType, cel.IntType}, result,
			cel.BinaryBinding(binding)))
}

// readLeaf reads the leaf field at the given index from the given position.
func readLeaf(position, index ref.Val) ref.Val {
	at, err := resolveRead(position, index, len(celReaders))
	if err != nil {
		return types.WrapErr(err)
	}
	return celReaders[at.index](at.ctx, at.elem)
}

// readElementsAs presents an iterated field as a list of the given element type,
// without materialising it.
func readElementsAs(elements *cel.Type) functions.BinaryOp {
	return func(position, index ref.Val) ref.Val {
		at, err := resolveRead(position, index, len(celIterators))
		if err != nil {
			return types.WrapErr(err)
		}
		return newIteratedList(at.ctx, celIterators[at.index], elements)
	}
}

// readOptimization binds a read to its reader and to the name of the position it
// reads from, when the rule is planned.
//
// It is the same asymmetry planningOptions is built on, applied to the index: a
// literal in the rule is worth resolving once. What it removes is all of cel-go's
// call machinery — evaluating the index argument, testing both arguments for
// unknowns, dispatching the overload, and resolving the position through an
// attribute — which measured at around 100 ns per read against 10 ns for the reader
// itself. What is left is a name resolution and a call.
//
// A read whose position is not a plain variable — the element a subscript yielded —
// is left as cel-go planned it, as is anything else that does not match, so this can
// only make a read cheaper, never different.
func readOptimization() cel.ProgramOption {
	return cel.CustomDecoratorV2(readDecorator())
}

// readDecorator is readOptimization as the interpreter takes it, for a caller that
// plans through interpreter.NewInterpretable rather than cel.Env.Program.
func readDecorator() interpreter.InterpretableDecoratorV2 {
	return func(i interpreter.InterpretableV2) (interpreter.InterpretableV2, error) {
		call, ok := i.(interpreter.InterpretableCall)
		if !ok {
			return i, nil
		}

		read, ok := boundRead(call)
		if !ok {
			return i, nil
		}

		variable, ok := positionVariable(call)
		if !ok {
			return i, nil
		}

		return &readByIndex{id: call.ID(), variable: variable, read: read, generic: i}, nil
	}
}

// boundRead resolves what a read call reads, if it is one of ours and its index is
// the literal the optimization pass emitted.
func boundRead(call interpreter.InterpretableCall) (func(*seclPosition) ref.Val, bool) {
	args := call.Args()
	if len(args) != 2 {
		return nil, false
	}
	index, ok := args[1].(interpreter.InterpretableConst)
	if !ok {
		return nil, false
	}
	at, ok := index.Value().(types.Int)
	if !ok {
		return nil, false
	}

	if elements, ok := elementLists[call.Function()]; ok {
		if at < 0 || int(at) >= len(celIterators) {
			return nil, false
		}
		cursor := celIterators[at]
		return func(from *seclPosition) ref.Val {
			return newIteratedList(from.ctx, cursor, elements)
		}, true
	}

	if _, ok := leafReads[call.Function()]; !ok {
		return nil, false
	}
	if at < 0 || int(at) >= len(celReaders) {
		return nil, false
	}
	reader := celReaders[at]
	return func(from *seclPosition) ref.Val {
		return reader(from.ctx, from.elem)
	}, true
}

// positionVariable names the variable a read reads from, which is what lets the
// position be resolved without going through cel-go's attribute machinery.
func positionVariable(call interpreter.InterpretableCall) (string, bool) {
	attr, ok := call.Args()[0].(interpreter.InterpretableAttribute)
	if !ok {
		return "", false
	}
	namespaced, ok := attr.Attr().(interpreter.NamespacedAttribute)
	if !ok {
		return "", false
	}
	// A qualifier means the position is computed rather than named, and more than
	// one candidate name means the expression was never checked.
	names := namespaced.CandidateVariableNames()
	if len(namespaced.Qualifiers()) != 0 || len(names) != 1 {
		return "", false
	}
	return names[0], true
}

// readByIndex is a read of one field, from the position one variable holds.
type readByIndex struct {
	id       int64
	variable string
	read     func(*seclPosition) ref.Val

	// generic is the call as cel-go planned it, for an activation that hands out
	// something other than a SECL position. Nothing here does, but a caller could.
	generic interpreter.InterpretableV2
}

// ID implements interpreter.Interpretable.
func (r *readByIndex) ID() int64 { return r.id }

// Eval implements interpreter.Interpretable.
func (r *readByIndex) Eval(activation interpreter.Activation) ref.Val {
	return r.Exec(interpreter.AsFrame(activation))
}

// Exec implements interpreter.InterpretableV2.
func (r *readByIndex) Exec(frame *interpreter.ExecutionFrame) ref.Val {
	value, found := frame.ResolveName(r.variable)
	position, ok := value.(*seclPosition)
	if !found || !ok {
		return r.generic.Exec(frame)
	}
	return r.read(position)
}

// fieldRead is what both reads need: where to read from, and what to read.
type fieldRead struct {
	ctx   *eval.Context
	elem  any
	index int
}

func resolveRead(position, index ref.Val, layout int) (fieldRead, error) {
	from, ok := position.(*seclPosition)
	if !ok {
		return fieldRead{}, fmt.Errorf("%w: %T is not a SECL event position", errUnsupportedValue, position)
	}
	at, ok := index.(types.Int)
	if !ok {
		return fieldRead{}, fmt.Errorf("%w: %s is not a field index", errUnsupportedValue, index.Type())
	}
	// The index comes from the layout the rule was optimized against, so it can be
	// out of range only if the two have drifted apart.
	if at < 0 || int(at) >= layout {
		return fieldRead{}, fmt.Errorf("%w: field index %d is outside the generated layout", errUnsupportedValue, at)
	}
	return fieldRead{ctx: from.ctx, elem: from.elem, index: int(at)}, nil
}

func sortedReadFuncs(reads map[string]*cel.Type) []string {
	names := make([]string, 0, len(reads))
	for name := range reads {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
