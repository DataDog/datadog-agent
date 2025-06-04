// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

// pseudoSet is a generic data structure that maintains a unique
// set of elements. It is a "pseudo" set because it does not
// implement all methods associated with a set right now.
type pseudoSet[T comparable] struct {
	content map[T]struct{}
}

// Add inserts an element into the set
func (s *pseudoSet[T]) Add(e T) {
	s.content[e] = struct{}{}
}

// Slice returns a slice of the elements in the set
func (s *pseudoSet[T]) Slice() []T {
	var buf []T

	for k := range s.content {
		buf = append(buf, k)
	}

	return buf
}

func newPseudoSet[T comparable]() pseudoSet[T] {
	return pseudoSet[T]{
		content: make(map[T]struct{}),
	}
}
