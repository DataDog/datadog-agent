// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

type typeWithGenerics[V comparable] struct {
	Value V
}

//nolint:all
//go:noinline
func (x typeWithGenerics[V]) Guess(value V) bool {
	return x.Value == value
}

//nolint:all
func executeGenericFuncs() {
	x := typeWithGenerics[string]{Value: "generics work"}
	x.Guess("generics work")

	y := typeWithGenerics[int]{Value: 42}
	y.Guess(21)
}
