// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

package profiledefinition

// Cloneable is a generic type for objects that can duplicate themselves.
// It is exclusively used in the form [T Cloneable[T]], i.e. a type that
// has a .Clone() that returns a new instance of itself.
type Cloneable[T any] interface {
	Clone() T
}

// CloneSlice clones all the objects in a slice into a new slice.
func CloneSlice[T Cloneable[T]](t []T) []T {
	if t == nil {
		return nil
	}
	result := make([]T, 0, len(t))
	for _, v := range t {
		result = append(result, v.Clone())
	}
	return result
}

// CloneMap clones a map[K]T for any cloneable type T.
// The map keys are shallow-copied; values are cloned.
func CloneMap[K comparable, T Cloneable[T]](m map[K]T) map[K]T {
	if m == nil {
		return nil
	}
	result := make(map[K]T, len(m))
	for k, v := range m {
		result[k] = v.Clone()
	}
	return result
}
