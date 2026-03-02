// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !rust_patterns && cgo

// Package rtokenizer provides a CGO overhead microbenchmark helper.
//
// This file compiles only when rust_patterns is NOT set (Rust lib not available).
// It exposes a no-op C function for measuring raw CGO boundary cost.
// CGO cannot be used in test files, so the C call is wrapped here.
package rtokenizer

/*
#include <stdlib.h>
void cgo_noop(void) {}
*/
import "C"

// CgoNoop calls a minimal no-op C function. Used to measure raw CGO overhead.
func CgoNoop() {
	C.cgo_noop()
}
