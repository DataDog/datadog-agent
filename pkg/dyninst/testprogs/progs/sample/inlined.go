// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import (
	"fmt"
	"math/rand"
)

func testInlinedPrint(x int) {
	fmt.Println(x)
}

func testInlinedA(x, y int) {
	testInlinedPrint(x + y)
}

func testInlinedB(x, y int) {
	testInlinedBA(x)
	testInlinedBB(x, y)
	testInlinedBC()
}

//go:noinline
func forceOutOfLine(f func(int)) {
	f(1)
}

func testInlinedBA(x int) {
	z := 3
	testInlinedPrint(x / z)
}

func testInlinedBB(x, y int) {
	z := x * y
	testInlinedPrint(z)
	testInlinedBBA(z)
	testInlinedBBB()
}

func testInlinedBBA(x int) {
	testInlinedPrint(x)
}

func testInlinedBBB() {
	fmt.Println("inlinedBBC")
}

func testInlinedBC() {
	testInlinedBCA()
	s := 0
	for i := 0; i < 100; i++ {
		s += rand.Intn(100)
	}
	fmt.Println(s)
	testInlinedBCB()
}

func testInlinedBCA() {
	fmt.Println("inlinedBCA")
}

func testInlinedBCB() {
	fmt.Println("inlinedBCB")
}

func testInlinedSumArray(a [5]int) int {
	return a[0] + a[1] + a[2] + a[3] + a[4]
}

func testInlinedSq(x int) int {
	return x * x
}

//go:noinline
func testFrameless(x int) int {
	return testInlinedSq(x)
}

// Despite best efforts, the compiler doesn't make the following function
// frameless. Seems impossible currently to have a function inlined into
// a frameless function if it accesses stack (even though the data is put
// on the stack by the caller). Best we can do is the function above, which
// doesn't test cfa resolution, but at least we test stack unwinding. We
// keep this test in case, in the future, the compiler decides to make this
// function frameless.
//
//go:noinline
func testFramelessArray(a [5]int) int {
	return testInlinedSumArray(a)
}

//nolint:all
func executeInlined() {
	a := [5]int{1, 2, 3, 4, 5}
	x := 10
	y := testInlinedSumArray(a)
	testInlinedA(x, y)
	testInlinedB(x, y)
	forceOutOfLine(testInlinedPrint)
	fmt.Println(testFrameless(x))
	fmt.Println(testFramelessArray(a))
}
