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
	stringArg("d")
	intSliceArg([]int{1, 2, 3})
	intArrayArg([3]int{1, 2, 3})
	stringSliceArg([]string{"a", "b", "c"})
	stringArrayArg([3]string{"a", "b", "c"})
	stringArrayArgFrameless([3]string{"foo", "bar", "baz"})
	inlined(1)
	// Passing inlined function as an argument forces out-of-line instantation.
	funcArg(inlined)
	mapArg(map[string]int{"a": 1})
	bigMapArg(map[string]bigStruct{"b": {Field1: 1}})
	val := 17
	ptr1 := &val
	ptr2 := &ptr1
	ptr3 := &ptr2
	ptr4 := &ptr3
	ptr5 := &ptr4
	PointerChainArg(ptr5)
	PointerSmallChainArg(ptr2)
}

//go:noinline
func intArg(x int) {
	fmt.Println(x)
}

//go:noinline
func stringArg(s string) {
	fmt.Println(s)
}

//go:noinline
func intSliceArg(s []int) {
	fmt.Println(s)
}

//go:noinline
func intArrayArg(s [3]int) {
	fmt.Println(s)
}

//go:noinline
func stringSliceArg(s []string) {
	fmt.Println(s)
}

//go:noinline
func stringArrayArg(s [3]string) {
	fmt.Println(s)
}

//go:noinline
func stringArrayArgFrameless(s [3]string) {
}

func inlined(x int) {
	fmt.Println(x)
}

//go:noinline
func funcArg(f func(int)) {
	f(2)
}

//go:noinline
func mapArg(m map[string]int) {
	fmt.Println(m)
}

type bigStruct struct {
	Field1 int
	Field2 int
	Field3 int
	Field4 int
	Field5 int
	Field6 int
	Field7 int

	data [128]byte
}

//go:noinline
func bigMapArg(m map[string]bigStruct) {
	v, ok := m["b"]
	if ok {
		v.data[0] = 1 // use data
	}
	fmt.Println(m)
}

func PointerChainArg(ptr *****int) {
	fmt.Println(ptr)
}

func PointerSmallChainArg(ptr **int) {
	fmt.Println(ptr)
}
