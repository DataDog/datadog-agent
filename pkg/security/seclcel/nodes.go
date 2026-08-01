// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package seclcel

import (
	"fmt"
	"strings"

	"github.com/google/cel-go/common/types/ref"

	"github.com/DataDog/datadog-agent/pkg/security/secl/compiler/eval"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

// The node graph is the runtime half of the generated field namespace, and is
// deliberately not the type tree.
//
// The types share one shape between every path exposing the same members, which
// is what keeps 2316 fields down to 84 declared types and gives the checker good
// error messages. Sharing is exactly wrong at run time: a shared shape cannot
// know which field a member stands for, so the value has to carry the path it
// was reached through, and every read then joins strings to name a field and
// looks the accessor up by that name.
//
// A node graph has one node per path instead. Selecting a member is a map
// lookup that lands on the node holding the reader for that field, so nothing is
// named, joined or looked up by name while an event is being evaluated.

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

// celNode is one position in the SECL field namespace: an object, a leaf, or an
// iterated field.
//
// Exactly one of the three is populated beyond members, which is what tells a
// member select whether to descend, to read, or to open a cursor.
type celNode struct {
	// members are the members of an object, or of one element of an iterated
	// field. It is nil for a leaf.
	members map[string]*celNode
	// read reads the field, for a leaf.
	read celReader
	// cursor opens a cursor over the elements, for an iterated field.
	cursor celIterator
}

// celRoots holds the top level segments of the SECL field namespace, which are
// the names a CEL environment declares as variables.
//
// The graph is linked once here rather than generated as a nested literal, so
// that the generated file stays a flat map keyed by SECL field name — the same
// shape as the accessors it is generated from, and greppable by the name a rule
// would use.
var celRoots = buildCELNodes(celReaders, celIterators)

func buildCELNodes(readers map[string]celReader, iterators map[string]celIterator) map[string]*celNode {
	root := &celNode{members: map[string]*celNode{}}

	for field, reader := range readers {
		nodeAt(root, field).read = reader
	}
	for field, iterator := range iterators {
		nodeAt(root, field).cursor = iterator
	}

	if err := pruneCELNodes(root, ""); err != nil {
		// The generator rejects a field set that cannot be expressed as a tree, so
		// reaching this means the two disagree.
		panic(fmt.Sprintf("the generated CEL readers are not a tree: %s", err))
	}
	return root.members
}

// nodeAt returns the node a dotted SECL name denotes, creating the nodes on the
// way to it.
func nodeAt(root *celNode, field string) *celNode {
	node := root
	for _, segment := range strings.Split(field, ".") {
		child, ok := node.members[segment]
		if !ok {
			child = &celNode{members: map[string]*celNode{}}
			node.members[segment] = child
		}
		node = child
	}
	return node
}

// pruneCELNodes drops the empty member maps the leaves were built with, and
// reports a node that is more than one of an object, a leaf and an iterated
// field.
func pruneCELNodes(node *celNode, path string) error {
	if node.read != nil {
		if node.cursor != nil {
			return fmt.Errorf("%q is both a value and iterated", path)
		}
		if len(node.members) != 0 {
			return fmt.Errorf("%q is a value but is also the prefix of another field", path)
		}
	}
	if len(node.members) == 0 {
		if node.read == nil {
			return fmt.Errorf("%q holds nothing", path)
		}
		node.members = nil
		return nil
	}

	for segment, child := range node.members {
		if err := pruneCELNodes(child, join(path, segment)); err != nil {
			return err
		}
	}
	return nil
}

func join(prefix, segment string) string {
	if prefix == "" {
		return segment
	}
	return prefix + "." + segment
}
