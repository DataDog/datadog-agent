// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"github.com/google/cel-go/common/types/ref"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// celReader reads one SECL leaf field and returns it as a CEL value.
//
// elem is the element the field is read from when the field belongs to an
// iterated one, and nil when it is read from the event itself.
type celReader func(ctx *eval.Context, elem any) ref.Val

// celIterator opens a cursor over an iterated SECL field.
type celIterator func(ctx *eval.Context) celCursor

// celCursor yields the elements of an iterated field one at a time.
//
// One at a time is the point: a quantifier over process.ancestors then walks the
// chain exactly as far as its predicate needs, where asking for the elements as
// a slice would walk all of it before the predicate saw the first one.
type celCursor interface {
	// next returns the next element, or nil once they are exhausted.
	next() any
}

// modelCursor walks a model iterator through its Front/Next pair.
type modelCursor[T comparable] struct {
	iterator model.Iterator[T]
	ctx      *eval.Context

	started bool
	done    bool
}

// next implements celCursor.
func (c *modelCursor[T]) next() any {
	if c.done {
		return nil
	}

	var element T
	if c.started {
		element = c.iterator.Next(c.ctx)
	} else {
		c.started = true
		element = c.iterator.Front(c.ctx)
	}

	var zero T
	if element == zero {
		// Latching matters: a model iterator asked for another element after the
		// last one dereferences the element it stopped on.
		c.done = true
		return nil
	}
	return element
}
