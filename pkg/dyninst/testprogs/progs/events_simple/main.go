// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command simple is a basic go program to be used with dyninst tests.
package main

import (
	"fmt"
	"log"
)

func main() {
	_, err := fmt.Scanln()
	if err != nil {
		log.Fatalf("error: %v", err)
	}
	intArg(0x0123456789abcdef)
	stringArg("Hello, world!")
	a := 1
	b := 2
	c := 3
	sliceArg([]*int{&a, &b, &c})
	arrayArg([3]*int{&a, &b, &c})
	inlined(1)
	// Passing inlined function via pointer forces it to have additional out-of-line instantiation.
	outer(inlined)
}

//go:noinline
func intArg(i int) {
	fmt.Println(i)
}

//go:noinline
func stringArg(s string) {
	fmt.Println(s)
}

//go:noinline
func sliceArg(s []*int) {
	fmt.Println(s)
}

//go:noinline
func arrayArg(a [3]*int) {
	fmt.Println(a)
}

func inlined(i int) {
	fmt.Println(i)
}

//go:noinline
func outer(f func(int)) {
	f(2)
}
