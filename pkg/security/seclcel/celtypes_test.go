// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build unix

package seclcel

import (
	"reflect"
	"testing"

	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/ext"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// TestGeneratedTypesAgreeWithEvaluators is the invariant that makes the generated
// types trustworthy: for every SECL field, the type they describe has to agree
// with the evaluator the accessors return.
//
// It is also the regression test for the trap that motivated generating them.
// Event.GetFieldMetadata reports isArray from the Go struct shape alone, so it
// says false for every field whose array-ness comes from an iterator — most of
// them. The generated types must not inherit that.
func TestGeneratedTypesAgreeWithEvaluators(t *testing.T) {
	var m model.Model
	event := model.NewFakeEvent()

	var checked, lists int
	for _, field := range event.GetFields() {
		evaluator, err := m.GetEvaluator(field, "", 0)
		if err != nil {
			continue
		}

		fieldType, listPrefix, ok := walkModel(field)
		if !ok {
			// The types deliberately omit the length and root_domain pseudo
			// fields, which are translated to size() and a helper call.
			assert.True(t, ModelFieldTypes{}.IsPseudoField(field),
				"field %q is neither typed nor a pseudo field", field)
			continue
		}
		checked++

		wantType, wantList := typeOfEvaluator(evaluator)

		// A field is a list either in its own right or through an enclosing
		// iterator; descending into the latter leaves the element type.
		gotList := listPrefix != "" || fieldType.Kind() == types.ListKind
		assert.Equal(t, wantList, gotList, "list-ness of %q (evaluator %T)", field, evaluator)
		if gotList {
			lists++
		}

		// Compare the value shape, ignoring the list wrapper the two express
		// differently.
		gotElem := fieldType
		if gotElem.Kind() == types.ListKind {
			gotElem = gotElem.Parameters()[0]
		}
		if gotElem.Kind() != types.StructKind {
			assert.Equal(t, wantType.TypeName(), gotElem.TypeName(), "type of %q", field)
		}
	}

	require.Greater(t, checked, 2000, "expected the whole field set to be covered")
	require.Greater(t, lists, 500, "expected the iterated fields to be recognised as lists")
}

// TestListnessBeatsGetFieldMetadata pins the specific divergence: an
// iterator-derived field is a list, even though GetFieldMetadata says otherwise.
func TestListnessBeatsGetFieldMetadata(t *testing.T) {
	event := model.NewFakeEvent()

	const field = "process.ancestors.file.name"

	_, _, _, metadataSaysArray, err := event.GetFieldMetadata(field)
	require.NoError(t, err)
	assert.False(t, metadataSaysArray, "guarding the premise: GetFieldMetadata under-reports this")

	assert.Equal(t, "process.ancestors", ModelFieldTypes{}.ListPrefix(field),
		"the generated types must see the iterator")

	// Within one ancestor the field holds a single string; the list-ness belongs
	// to the `ancestors` member, which is what makes the element type shareable.
	fieldType, _, ok := walkModel(field)
	require.True(t, ok)
	assert.Equal(t, types.StringType, fieldType)
}

func TestGeneratedTypesShape(t *testing.T) {
	process, ok := modelRoots["process"]
	require.True(t, ok)
	require.Equal(t, types.StructKind, process.Kind())

	// An iterated member is a list of objects, a single parent is not a list.
	ancestors, ok := modelShapes[process.TypeName()]["ancestors"]
	require.True(t, ok)
	elem, isObjectList := objectListElem(ancestors)
	require.True(t, isObjectList)

	parent, ok := modelShapes[process.TypeName()]["parent"]
	require.True(t, ok)
	assert.Equal(t, types.StructKind, parent.Kind())
	assert.Equal(t, elem.TypeName(), parent.TypeName(),
		"a parent and an ancestor expose the same members")

	// Every referenced type must exist, and no type may be unreachable.
	reachable := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		if reachable[name] {
			return
		}
		reachable[name] = true

		members, ok := modelShapes[name]
		require.True(t, ok, "type %q is referenced but not defined", name)
		for _, member := range members {
			m := member
			if m.Kind() == types.ListKind {
				m = m.Parameters()[0]
			}
			if m.Kind() == types.StructKind {
				walk(m.TypeName())
			}
		}
	}
	for _, root := range modelRoots {
		r := root
		if r.Kind() == types.ListKind {
			r = r.Parameters()[0]
		}
		if r.Kind() == types.StructKind {
			walk(r.TypeName())
		}
	}
	for name := range modelShapes {
		assert.True(t, reachable[name], "type %q is not reachable from any root", name)
	}

	// The pseudo fields are absent, which is what keeps the types a tree.
	_, _, ok = walkModel("process.ancestors.file.name.length")
	assert.False(t, ok)
}

// typeOfEvaluator maps an evaluator back to the value type and list-ness the
// generated types should describe for it.
func typeOfEvaluator(evaluator eval.Evaluator) (*types.Type, bool) {
	switch evaluator.(type) {
	case *eval.StringEvaluator:
		return types.StringType, false
	case *eval.StringArrayEvaluator:
		return types.StringType, true
	case *eval.IntEvaluator:
		return types.IntType, false
	case *eval.IntArrayEvaluator:
		return types.IntType, true
	case *eval.BoolEvaluator:
		return types.BoolType, false
	case *eval.BoolArrayEvaluator:
		return types.BoolType, true
	case *eval.CIDREvaluator:
		return ext.CIDRType, false
	case *eval.CIDRArrayEvaluator:
		return ext.CIDRType, true
	}
	panic("unexpected evaluator type " + reflect.TypeOf(evaluator).String())
}
