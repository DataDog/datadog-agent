// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux

package main

import (
	"fmt"
)

func hello() {
	fmt.Println("Hello World")
}

// burnCPU is a function that burns CPU to ensure the process gets profiled.
// This is only for testing purposes. Prevent inlining of this function.
//
//go:noinline
func burnCPU() {
	for {

	}
}

func main() {
	hello()
	burnCPU()
}
