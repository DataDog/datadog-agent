// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package slices are utilities to deal with slices
package slices

// Map returns a new slice with the result of applying fn to each element.
func Map[S ~[]E, E any, RE any](s S, fn func(E) RE) []RE {
	x := make([]RE, 0, len(s))
	for _, v := range s {
		x = append(x, fn(v))
	}
	return x
}
