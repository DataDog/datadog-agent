// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Benchmark for the run_check empty-result fast path.
// Run with: go run ./pkg/collector/python/bench_run_check_result.go

//go:build ignore

package main

/*
#include <stdlib.h>
*/
import "C"

import (
	"fmt"
	"testing"
	"unsafe"
)

func main() {
	// Simulate the empty C string that run() returns on a successful check run.
	empty := C.CString("")
	defer C.free(unsafe.Pointer(empty))

	// Also benchmark the non-empty (error) path to confirm no regression.
	errStr := C.CString("check execution failed: connection refused")
	defer C.free(unsafe.Pointer(errStr))

	oldEmpty := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			// old: always calls C.GoString, which scans for NUL and builds a string
			s := C.GoString(empty)
			_ = s
		}
	})

	newEmpty := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			// new: single byte read short-circuits on the common success path
			if *(*byte)(unsafe.Pointer(empty)) != 0 {
				_ = C.GoString(empty)
			}
		}
	})

	oldErr := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			s := C.GoString(errStr)
			_ = s
		}
	})

	newErr := testing.Benchmark(func(b *testing.B) {
		b.ReportAllocs()
		for b.Loop() {
			if *(*byte)(unsafe.Pointer(errStr)) != 0 {
				_ = C.GoString(errStr)
			}
		}
	})

	fmt.Println("empty result (success path — the common case):")
	fmt.Printf("  old (C.GoString):  %d ns/op  %d allocs/op\n", oldEmpty.NsPerOp(), oldEmpty.AllocsPerOp())
	fmt.Printf("  new (byte check):  %d ns/op  %d allocs/op\n", newEmpty.NsPerOp(), newEmpty.AllocsPerOp())
	fmt.Println("non-empty result (error path — both behave identically):")
	fmt.Printf("  old (C.GoString):  %d ns/op  %d allocs/op\n", oldErr.NsPerOp(), oldErr.AllocsPerOp())
	fmt.Printf("  new (byte check):  %d ns/op  %d allocs/op\n", newErr.NsPerOp(), newErr.AllocsPerOp())
}
