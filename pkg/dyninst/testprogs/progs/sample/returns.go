// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"errors"
	"fmt"
)

//go:noinline
func testReturnsInt(i int) int {
	fmt.Println(i)
	return i
}

//go:noinline
func testReturnsIntPointer(i int) *int {
	fmt.Println(i)
	return &i
}

//go:noinline
func testReturnsIntAndFloat(i int) (int, float64) {
	f := float64(i)
	fmt.Println("returnsIntAndFloat", i, f)
	return i, f
}

//go:noinline
func testReturnsError(failed bool) error {
	if failed {
		return errors.New("error")
	}
	return nil
}

//go:noinline
func testReturnsAnyAndError(v any, failed bool) (any, error) {
	if failed {
		return nil, errors.New("error")
	}
	return v, nil
}

//go:noinline
func testReturnsAny(v any) any {
	fmt.Println("returnsAny", v)
	return v
}

//go:noinline
func testReturnsInterface(v any) behavior {
	b, ok := v.(behavior)
	if !ok {
		return nil
	}
	fmt.Println("returnsInterface", b)
	return b
}

//go:noinline
func testNamedReturn(i int) (result int) {
	result = i
	fmt.Println("testNamedReturn", result)
	return
}

//go:noinline
func testMultipleNamedReturn(i int) (result int, result2 int) {
	result = i
	result2 = i
	fmt.Println("testMultipleNamedReturn", result, result2)
	return
}

//go:noinline
func testSomeNamedReturn(i int) (_ int, result2 int, _ int) {
	fmt.Println("testSomeNamedReturn", i)
	return i - 1, i, i + 1
}

func executeReturns() {
	testReturnsInt(42)
	testReturnsIntPointer(42)
	testReturnsIntAndFloat(42)
	testReturnsError(true)
	testReturnsError(false)
	testReturnsAnyAndError(42, true)
	testReturnsAnyAndError(42, false)

	v := firstBehavior{"foo"}
	testReturnsAny(nil)
	testReturnsAny(42)
	fortyTwo := 42
	testReturnsAny(&fortyTwo)

	testReturnsInterface(42)
	testReturnsInterface(v)
	testReturnsInterface(&v)

	testNamedReturn(40)
	testMultipleNamedReturn(41)
	testSomeNamedReturn(42)
}
