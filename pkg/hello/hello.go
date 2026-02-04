// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

// Package hello provides a C++ function that prints hello world.
package hello

/*
#cgo CXXFLAGS: -std=c++11
#cgo LDFLAGS: -lstdc++

#include "hello.h"
*/
import "C"

// PrintHelloWorld calls the C++ function to print "Hello World"
func PrintHelloWorld() {
	C.PrintHelloWorld()
}
