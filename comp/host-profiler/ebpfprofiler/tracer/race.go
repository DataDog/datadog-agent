//go:build race

// Copyright The OpenTelemetry Authors
// SPDX-License-Identifier: Apache-2.0

// Package tracer
package tracer

import (
	"unsafe"

	"go.opentelemetry.io/ebpf-profiler/support"
)

func traceFromRaw(raw []byte) *support.Trace {
	// workaround for "fatal error: checkptr: converted pointer straddles multiple allocations"
	fullSizeCopy := make([]byte, unsafe.Sizeof(support.Trace{}))
	copy(fullSizeCopy, raw)
	return (*support.Trace)(unsafe.Pointer(unsafe.SliceData(fullSizeCopy)))
}
