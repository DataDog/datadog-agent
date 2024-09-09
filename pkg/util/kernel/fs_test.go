// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package kernel

import (
	"runtime"
	"testing"
)

func BenchmarkHostProc(b *testing.B) {
	_ = ProcFSRoot()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		res := hostProcInternal("/proc", "a", "b", "c", "d")
		runtime.KeepAlive(res)
	}
}
