// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !amd64

package ptracer

import (
	"fmt"
	"runtime"
)

// resolveOTelStaticTLSOffset is not implemented on this architecture: see
// otel_span_context_amd64.go.
func resolveOTelStaticTLSOffset(_ string) (int64, error) {
	return 0, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
}

// readOTelSpanContextStaticTLS is not implemented on this architecture: the
// thread pointer (tpidr_el0 on arm64) needs PTRACE_GETREGSET(NT_ARM_TLS),
// which syscall.PtraceGetRegs does not expose. See otel_span_context_amd64.go.
func (t *Tracer) readOTelSpanContextStaticTLS(_ int, _ int64) (traceIDHi, traceIDLo, spanID uint64, ok bool, err error) {
	return 0, 0, 0, false, fmt.Errorf("unsupported architecture: %s", runtime.GOARCH)
}
