// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

type TypeWithGenerics[V comparable] struct {
	Value V
}

//go:noinline
func (x TypeWithGenerics[V]) Guess(value V) bool {
	return x.Value == value
}

func executeGenericFuncs() {
	x := TypeWithGenerics[string]{Value: "generics work"}
	x.Guess("generics work")

	y := TypeWithGenerics[int]{Value: 42}
	y.Guess(21)
}
