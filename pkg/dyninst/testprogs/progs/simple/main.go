// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Command simple is a basic go program to be used with dyninst tests.
package main

import "fmt"

func main() {
	fmt.Println("hello world")
	mapArg(map[string]int{"a": 1})
	bigMapArg(map[string]bigStruct{"b": {Field1: 1}})
	stringSliceArg([]string{"c"})
	intSliceArg([]int{1})
	stringArg("d")
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

//go:noinline
func stringSliceArg(s []string) {
	fmt.Println(s)
}

//go:noinline
func stringArg(s string) {
	fmt.Println(s)
}

//go:noinline
func intSliceArg(s []int) {
	fmt.Println(s)
}
