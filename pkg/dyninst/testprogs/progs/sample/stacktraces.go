// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package main

import "fmt"

//nolint:all
//go:noinline
func stackA() {
	stackB()
}

//nolint:all
//go:noinline
func stackB() {
	stackC()
}

//nolint:all
//go:noinline
func stackC() string {
	return fmt.Sprintf("hello %d!", 1)
}

//nolint:all
//go:noinline
func callInlinedFuncChain() {
	inlineMe1()
}

//nolint:all
func inlineMe1() {
	inlineMe2()
}

//nolint:all
func inlineMe2() {
	inlineMe3()
}

//nolint:all
func inlineMe3() {
	notInlined()
}

//nolint:all
//go:noinline
func notInlined() string {
	return fmt.Sprintf("hello %d!", 42)
}

//nolint:all
func executeStackAndInlining() {
	stackA()
	callInlinedFuncChain()
}
