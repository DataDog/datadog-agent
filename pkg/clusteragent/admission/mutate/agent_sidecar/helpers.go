// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build kubeapiserver

package agentsidecar

type set[T comparable] map[T]struct{}

func (s set[T]) add(e T) {
	s[e] = struct{}{}
}

func (s set[T]) toSlice() []T {
	var buf []T

	for key := range s {
		buf = append(buf, key)
	}

	return buf
}

func newSet[T comparable]() set[T] {
	return make(map[T]struct{})
}
