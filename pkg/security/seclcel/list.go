// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"github.com/google/cel-go/common/types"
	"github.com/google/cel-go/common/types/ref"
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

// newIteratedList presents an iterated SECL field as a CEL list.
//
// The elements are positions rather than values, so building the list reads only
// the element count: a field of an element is resolved when the expression selects
// it, which is what keeps a comprehension from touching fields its predicate does
// not mention.
func newIteratedList(object *seclObject) ref.Val {
	length := object.resolve(object.path+".length", types.IntType)
	if types.IsError(length) {
		return length
	}
	count, ok := length.(types.Int)
	if !ok {
		return types.NewErr("length of %q is not an integer", object.path)
	}

	// The register is named after the field, which is unique and stable: SECL
	// allows one iterator variable per rule and the model has no nested iterators.
	elements := make([]ref.Val, 0, count)
	for i := 0; i < int(count); i++ {
		elements = append(elements, object.bind(object.path, i))
	}

	return types.NewRefValList(types.DefaultTypeAdapter, elements)
}
