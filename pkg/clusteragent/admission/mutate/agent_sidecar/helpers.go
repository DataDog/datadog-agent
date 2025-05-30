// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

type PseudoSet[T comparable] struct {
	content map[T]struct{}
}

func (s *PseudoSet[T]) Add(e T) {
	s.content[e] = struct{}{}
}

func (s *PseudoSet[T]) Slice() []T {
	var buf []T

	for k := range s.content {
		buf = append(buf, k)
	}

	return buf
}

func NewPseudoSet[T comparable]() PseudoSet[T] {
	return PseudoSet[T]{
		content: make(map[T]struct{}),
	}
}
